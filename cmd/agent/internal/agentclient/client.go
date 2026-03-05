// Package agentclient implements the Playerr agent: SSE-driven install job runner.
package agentclient

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kiwi3007/playerr/internal/agent"
	"github.com/kiwi3007/playerr/internal/launcher"
)

var cryptoRandRead = rand.Read

// Config holds agent startup configuration.
type Config struct {
	ServerURL string
	Token     string
	Name      string
}

// Client is the agent runtime.
type Client struct {
	cfg     Config
	agentID string
	stopped atomic.Bool
	stopCh  chan struct{}
	http    *http.Client
}

// New creates a new agent client, loading or generating a stable agent ID.
func New(cfg Config) (*Client, error) {
	id, err := loadOrCreateID()
	if err != nil {
		return nil, fmt.Errorf("agent ID: %w", err)
	}
	return &Client{
		cfg:     cfg,
		agentID: id,
		stopCh:  make(chan struct{}),
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Stop signals the agent to stop reconnecting.
func (c *Client) Stop() {
	c.stopped.Store(true)
	close(c.stopCh)
}

// Run registers with the server and maintains the SSE connection with exponential backoff.
// This is the only long-running goroutine in idle state.
func (c *Client) Run() {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for !c.stopped.Load() {
		if err := c.register(); err != nil {
			log.Printf("[Agent] Register failed: %v — retrying in %s", err, backoff)
			c.sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		log.Printf("[Agent] Connected to %s as %q (id=%s)", c.cfg.ServerURL, c.cfg.Name, c.agentID)
		backoff = time.Second // reset on successful connect

		if err := c.listenSSE(); err != nil && !c.stopped.Load() {
			log.Printf("[Agent] SSE disconnected: %v — reconnecting in %s", err, backoff)
		}

		if !c.stopped.Load() {
			c.sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// register calls POST /api/v3/agent/register.
func (c *Client) register() error {
	body, _ := json.Marshal(map[string]any{
		"id":           c.agentID,
		"name":         c.cfg.Name,
		"platform":     runtime.GOOS + "/" + runtime.GOARCH,
		"steamPath":    launcher.FindSteamRoot(),
		"installPaths": discoverInstallPaths(),
	})

	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// listenSSE opens the SSE stream and dispatches jobs as they arrive.
// Returns when the stream closes or an error occurs.
func (c *Client) listenSSE() error {
	url := fmt.Sprintf("%s/api/v3/agent/events?agentId=%s", c.cfg.ServerURL, c.agentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client with no timeout for the SSE stream
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	log.Println("[Agent] SSE stream established — idle (waiting for jobs)")

	scanner := bufio.NewScanner(resp.Body)
	var eventType, dataLine string

	for scanner.Scan() {
		if c.stopped.Load() {
			return nil
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			// End of event — dispatch if we have an event type and data
			if eventType == "INSTALL_JOB" && dataLine != "" {
				var job agent.InstallJob
				if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
					log.Printf("[Agent] Bad job JSON: %v", err)
				} else {
					go c.executeJob(job)
				}
			}
			eventType = ""
			dataLine = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

// executeJob runs an install job: download → extract → install silently → apply crack → shortcut.
func (c *Client) executeJob(job agent.InstallJob) {
	log.Printf("[Agent] Job %s: installing %q", job.JobID, job.GameTitle)

	// Use agent-chosen install dir, or fall back to ~/Games
	baseDir := job.InstallDir
	if baseDir == "" {
		baseDir = filepath.Join(homeDir(), "Games")
	}
	downloadDir := filepath.Join(baseDir, safeName(job.GameTitle))
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		c.reportProgress(job, agent.JobFailed, "Cannot create download dir: "+err.Error(), 0)
		return
	}

	// ---- Download files ----
	c.reportProgress(job, agent.JobDownloading, "Downloading files...", 0)
	total := len(job.Files)
	for i, relPath := range job.Files {
		fileBasePct := (i * 75) / total
		fileEndPct := ((i + 1) * 75) / total
		label := fmt.Sprintf("Downloading (%d/%d): %s", i+1, total, filepath.Base(relPath))
		c.reportProgress(job, agent.JobDownloading, label, fileBasePct)

		destPath := filepath.Join(downloadDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			c.reportProgress(job, agent.JobFailed, "mkdir failed: "+err.Error(), fileBasePct)
			return
		}

		dlURL := fmt.Sprintf("%s/api/v3/game/%d/file?path=%s", job.ServerURL, job.GameID, url.QueryEscape(relPath))
		if err := c.downloadFile(dlURL, destPath, func(bytesRead, totalBytes int64) {
			var pct int
			if totalBytes > 0 {
				filePct := int(bytesRead * 100 / totalBytes)
				pct = fileBasePct + (filePct*(fileEndPct-fileBasePct))/100
			} else {
				pct = fileBasePct + (fileEndPct-fileBasePct)/2
			}
			c.reportProgress(job, agent.JobDownloading, label, pct)
		}); err != nil {
			c.reportProgress(job, agent.JobFailed, "Download failed: "+err.Error(), fileBasePct)
			return
		}
	}

	// ---- Extract any ISO / archive files ----
	// .bin is intentionally excluded — game data files commonly use .bin and are not archives
	archiveExts := map[string]bool{".iso": true, ".zip": true, ".rar": true, ".7z": true}
	_ = filepath.Walk(downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !archiveExts[ext] {
			return nil
		}
		extractDir := strings.TrimSuffix(path, filepath.Ext(path))
		c.reportProgress(job, agent.JobExtracting, "Extracting: "+filepath.Base(path), 82)
		log.Printf("[Agent] Extracting: %s → %s", path, extractDir)
		if extractHeadless(path, extractDir) {
			// Clean up archive after successful extraction
			if err := os.Remove(path); err != nil {
				log.Printf("[Agent] Could not remove archive %s: %v", path, err)
			} else {
				log.Printf("[Agent] Removed archive: %s", path)
			}
		}
		return nil
	})

	// ---- Determine wineprefix path (outside Steam directories) ----
	home := homeDir()
	wineprefix := filepath.Join(home, ".local", "share", "playerr", fmt.Sprintf("prefix_%d", job.GameID))
	// Known install dir inside Wine prefix (we force /DIR= to this path)
	gameInstallDir := filepath.Join(wineprefix, "pfx", "drive_c", "Games", safeName(job.GameTitle))

	// ---- Find and run installer ----
	c.reportProgress(job, agent.JobInstalling, "Looking for installer...", 85)
	installer := findInstaller(downloadDir)

	var gameExe string

	if installer != "" {
		c.reportProgress(job, agent.JobInstalling, "Running installer: "+filepath.Base(installer), 87)
		if err := runInstallerSilent(installer, wineprefix, job.GameTitle); err != nil {
			log.Printf("[Agent] Installer error (non-fatal): %v", err)
		}
		// Try known install dir first, then search entire prefix
		gameExe = findMainExe(gameInstallDir)
		if gameExe == "" {
			gameExe = findGameExeInPrefix(wineprefix)
		}
		// Apply crack files: copy from Crack/SKIDROW/etc dirs to wherever game was installed
		if gameExe != "" {
			applyCrack(downloadDir, filepath.Dir(gameExe))
		}
	} else {
		// No installer — portable game already in download dir
		gameExe = findMainExe(downloadDir)
	}

	// ---- Create local Steam shortcut via wrapper script ----
	c.reportProgress(job, agent.JobCreatingShortcut, "Creating Steam shortcut...", 95)
	if gameExe != "" {
		scriptPath := createRunScript(job.GameTitle, gameExe, wineprefix)
		exeForShortcut := gameExe
		if scriptPath != "" {
			exeForShortcut = scriptPath
		}
		entry := launcher.ShortcutEntry{
			AppName:  job.GameTitle,
			Exe:      exeForShortcut,
			StartDir: filepath.Dir(exeForShortcut),
		}
		if _, err := launcher.AddSteamShortcut(entry); err != nil {
			log.Printf("[Agent] Steam shortcut error: %v", err)
		} else {
			log.Printf("[Agent] Steam shortcut created for %q → %s", job.GameTitle, exeForShortcut)
		}
	} else {
		log.Printf("[Agent] No game exe found, skipping shortcut")
	}

	doneMsg := "Install complete. Files in: " + downloadDir
	if gameExe != "" {
		doneMsg += ". Restart Steam to see the shortcut."
	}
	c.reportProgress(job, agent.JobDone, doneMsg, 100)
	log.Printf("[Agent] Job %s: done", job.JobID)
}

// runInstallerSilent runs a Windows installer silently.
// On Windows: runs the installer natively.
// On Linux/macOS: uses the best available runner (UMU > Proton > Wine).
func runInstallerSilent(installerPath, wineprefix, gameTitle string) error {
	silentFlags := []string{
		"/VERYSILENT",
		"/SUPPRESSMSGBOXES",
		"/NORESTART",
	}

	if runtime.GOOS == "windows" {
		winDir := filepath.Join(os.Getenv("USERPROFILE"), "Games", safeName(gameTitle))
		cmd := exec.Command(installerPath, append(silentFlags, "/DIR="+winDir)...)
		cmd.Dir = filepath.Dir(installerPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	runner := launcher.FindRunner()
	if runner == nil {
		log.Printf("[Agent] No runner found — skipping installer %s", installerPath)
		return nil
	}
	args := append(silentFlags, `/DIR=C:\Games\`+safeName(gameTitle))
	cmd := runner.RunWith(installerPath, wineprefix, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findGameExeInPrefix searches the Proton wine prefix for a non-system game exe.
func findGameExeInPrefix(compatData string) string {
	driveC := filepath.Join(compatData, "pfx", "drive_c")
	skipDirs := map[string]bool{
		"windows": true, "users": true, "programdata": true,
	}

	var found string
	_ = filepath.Walk(driveC, func(path string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(driveC, path)
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) >= 1 && skipDirs[strings.ToLower(parts[0])] {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(info.Name())
		if isGameExe(lower) {
			found = path
		}
		return nil
	})
	return found
}

// applyCrack copies files from Crack/SKIDROW/CODEX/etc subdirs to the game install dir.
func applyCrack(srcDir, gameInstallDir string) {
	if gameInstallDir == "" {
		return
	}
	crackNames := []string{"Crack", "crack", "SKIDROW", "CODEX", "CPY", "EMPRESS", "Crk"}
	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		for _, name := range crackNames {
			if strings.EqualFold(info.Name(), name) {
				log.Printf("[Agent] Applying crack: %s → %s", path, gameInstallDir)
				copyDir(path, gameInstallDir)
				return filepath.SkipDir
			}
		}
		return nil
	})
}

// findInstaller finds setup*.exe or install*.exe recursively in dir.
func findInstaller(dir string) string {
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil
		}
		lower := strings.ToLower(info.Name())
		if strings.HasSuffix(lower, ".exe") &&
			(strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install")) {
			found = path
		}
		return nil
	})
	return found
}

// findMainExe finds the primary game exe (excludes setup/install/unins/redist).
func findMainExe(dir string) string {
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil
		}
		lower := strings.ToLower(info.Name())
		if isGameExe(lower) {
			found = path
		}
		return nil
	})
	return found
}

func isGameExe(lower string) bool {
	if !strings.HasSuffix(lower, ".exe") {
		return false
	}
	excludePrefixes := []string{"setup", "install", "unins", "redist", "dxsetup", "vcredist", "directx", "config", "crashreport", "crashpad", "bugsplat"}
	for _, p := range excludePrefixes {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	return true
}

// createRunScript writes a launcher script for the game.
// On Windows: writes run.bat. On Linux: writes run.sh using the found runner.
// Returns the script path, or "" if no runner is available and not on Windows.
func createRunScript(gameTitle, gameExe, wineprefix string) string {
	scriptDir := filepath.Join(homeDir(), "Games", safeName(gameTitle))
	_ = os.MkdirAll(scriptDir, 0755)

	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(scriptDir, "run.bat")
		content := fmt.Sprintf("@echo off\r\nstart \"\" %q\r\n", gameExe)
		if err := os.WriteFile(scriptPath, []byte(content), 0644); err != nil {
			log.Printf("[Agent] Failed to write run.bat: %v", err)
			return ""
		}
		return scriptPath
	}

	runner := launcher.FindRunner()
	if runner == nil {
		return ""
	}

	scriptPath := filepath.Join(scriptDir, "run.sh")
	var content string

	switch runner.Type {
	case launcher.RunnerUMU:
		content = fmt.Sprintf("#!/bin/bash\nexport WINEPREFIX=%q\nexport PROTONPATH=%q\nexport GAMEID=0\nexec %q %q \"$@\"\n",
			wineprefix, runner.ProtonPath, runner.BinPath, gameExe)
	case launcher.RunnerProton:
		fakeSteam := filepath.Join(homeDir(), ".config", "playerr-agent", "fake-steam-root")
		content = fmt.Sprintf("#!/bin/bash\nexport STEAM_COMPAT_DATA_PATH=%q\nexport STEAM_COMPAT_CLIENT_INSTALL_PATH=%q\nexec %q run %q \"$@\"\n",
			wineprefix, fakeSteam, runner.BinPath, gameExe)
	case launcher.RunnerWine:
		content = fmt.Sprintf("#!/bin/bash\nexport WINEPREFIX=%q\nexec %q %q \"$@\"\n",
			wineprefix, runner.BinPath, gameExe)
	default:
		return ""
	}

	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		log.Printf("[Agent] Failed to write run.sh: %v", err)
		return ""
	}
	return scriptPath
}

// extractHeadless extracts an archive non-interactively.
// Tries 7z/7za first (fast, handles most formats), falls back to unrar for
// RAR archives that 7z can't handle. Cleans up a partial extractDir on retry.
// Returns true on success.
func extractHeadless(archivePath, extractDir string) bool {
	isRar := strings.HasSuffix(strings.ToLower(archivePath), ".rar")

	// Try 7z/7za first (handles zip, 7z, iso, and most rar variants)
	for _, bin := range []string{"7z", "7za"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		// Remove partial extractDir from a previous failed attempt
		_ = os.RemoveAll(extractDir)
		cmd := exec.Command(bin, "x", archivePath, "-o"+extractDir, "-y", "-bd")
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err == nil {
			return true
		}
	}

	// Fall back to unrar for RAR archives (handles RAR5, multi-part, etc.)
	if isRar {
		if unrar, err := exec.LookPath("unrar"); err == nil {
			_ = os.RemoveAll(extractDir)
			_ = os.MkdirAll(extractDir, 0755)
			cmd := exec.Command(unrar, "x", "-y", "-idq", archivePath, extractDir+"/")
			cmd.Stdin = nil
			cmd.Stdout = nil
			cmd.Stderr = nil
			if err := cmd.Run(); err == nil {
				return true
			}
		}
	}

	log.Printf("[Agent] Extraction failed for %s", archivePath)
	return false
}

// discoverInstallPaths returns available storage locations on this device.
func discoverInstallPaths() []agent.InstallPath {
	home := homeDir()
	var paths []agent.InstallPath

	if runtime.GOOS == "windows" {
		// Default: %USERPROFILE%\Games
		defaultPath := filepath.Join(home, "Games")
		_ = os.MkdirAll(defaultPath, 0755)
		paths = append(paths, agent.InstallPath{
			Path:      defaultPath,
			Label:     "Local Disk",
			FreeBytes: diskFree(defaultPath),
		})
		// Additional drives (C-Z, skip the drive containing home)
		homeVol := strings.ToUpper(filepath.VolumeName(home))
		for _, letter := range "CDEFGHIJKLMNOPQRSTUVWXYZ" {
			vol := string(letter) + ":"
			if strings.ToUpper(vol) == homeVol {
				continue
			}
			root := vol + `\`
			if _, err := os.Stat(root); err != nil {
				continue
			}
			gamesPath := vol + `\Games`
			paths = append(paths, agent.InstallPath{
				Path:      gamesPath,
				Label:     string(letter) + " Drive",
				FreeBytes: diskFree(root),
			})
		}
		return paths
	}

	// Linux/macOS: default internal storage
	defaultPath := filepath.Join(home, "Games")
	_ = os.MkdirAll(defaultPath, 0755)
	paths = append(paths, agent.InstallPath{
		Path:      defaultPath,
		Label:     "Internal Storage",
		FreeBytes: diskFree(defaultPath),
	})

	// Removable media: /run/media/{username}/*/
	username := filepath.Base(home)
	mediaRoot := filepath.Join("/run/media", username)
	entries, err := os.ReadDir(mediaRoot)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			mountPath := filepath.Join(mediaRoot, e.Name())
			gamesPath := filepath.Join(mountPath, "Games")
			_ = os.MkdirAll(gamesPath, 0755)
			paths = append(paths, agent.InstallPath{
				Path:      gamesPath,
				Label:     e.Name(),
				FreeBytes: diskFree(mountPath),
			})
		}
	}
	return paths
}

// copyDir copies all files from src to dst recursively.
func copyDir(src, dst string) {
	_ = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		dest := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode())
		}
		return copyFileAtomic(path, dest)
	})
}

func copyFileAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// progressReader wraps a reader and calls onProgress at most every 2 seconds.
type progressReader struct {
	r          io.Reader
	read       int64
	total      int64
	lastReport time.Time
	onProgress func(read, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.read += int64(n)
	if pr.onProgress != nil && time.Since(pr.lastReport) >= 2*time.Second {
		pr.onProgress(pr.read, pr.total)
		pr.lastReport = time.Now()
	}
	return n, err
}

// downloadFile downloads a URL to destPath, resuming if the file already exists.
// onProgress is called periodically with (bytesRead, totalBytes); totalBytes may be -1.
func (c *Client) downloadFile(rawURL, destPath string, onProgress func(read, total int64)) error {
	tmpPath := destPath + ".tmp"

	// Check for existing partial download
	var offset int64
	if fi, err := os.Stat(tmpPath); err == nil {
		offset = fi.Size()
	}

	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	dlClient := &http.Client{} // no timeout for file downloads
	resp, err := dlClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == 206 {
		flags |= os.O_APPEND
		offset = 0 // already appending; don't add to read count
	}
	f, err := os.OpenFile(tmpPath, flags, 0644)
	if err != nil {
		return err
	}

	// Total bytes: Content-Length of this response + already-downloaded offset
	total := resp.ContentLength
	if total > 0 && resp.StatusCode == 200 {
		total += offset
	}

	var reader io.Reader = resp.Body
	if onProgress != nil {
		reader = &progressReader{
			r:          resp.Body,
			read:       offset,
			total:      total,
			onProgress: onProgress,
		}
	}

	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return os.Rename(tmpPath, destPath)
}

// reportProgress POSTs a JobProgress to the server.
func (c *Client) reportProgress(job agent.InstallJob, status agent.JobStatus, message string, pct int) {
	prog := agent.JobProgress{
		JobID:   job.JobID,
		Status:  status,
		Message: message,
		Percent: pct,
	}
	body, _ := json.Marshal(prog)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/progress", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// sleep blocks until duration elapses or the agent is stopped.
func (c *Client) sleep(d time.Duration) {
	select {
	case <-time.After(d):
	case <-c.stopCh:
	}
}

// ---- Helpers ----

func safeName(title string) string {
	var b strings.Builder
	for _, ch := range title {
		if ch == '/' || ch == '\\' || ch == ':' || ch == '*' || ch == '?' || ch == '"' || ch == '<' || ch == '>' || ch == '|' {
			b.WriteRune('-')
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// ---- Stable agent ID ----

func loadOrCreateID() (string, error) {
	idPath := agentIDPath()
	if data, err := os.ReadFile(idPath); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	// Generate new UUID-like ID
	b := make([]byte, 16)
	if _, err := cryptoRandRead(b); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	_ = os.MkdirAll(filepath.Dir(idPath), 0755)
	_ = os.WriteFile(idPath, []byte(id), 0600)
	return id, nil
}

func agentIDPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "playerr-agent", "id")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "playerr-agent", "id")
	default:
		return filepath.Join(home, ".config", "playerr-agent", "id")
	}
}
