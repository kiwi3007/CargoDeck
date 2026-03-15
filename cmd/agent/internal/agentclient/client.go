// Package agentclient implements the Playerr agent: SSE-driven install job runner.
package agentclient

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kiwi3007/cargodeck/internal/agent"
	"github.com/kiwi3007/cargodeck/internal/launcher"
)

var cryptoRandRead = rand.Read

// Config holds agent startup configuration.
type Config struct {
	ServerURL string
	Token     string
	Name      string
	Version   string
}

// Client is the agent runtime.
type Client struct {
	cfg           Config
	agentID       string
	sessionToken  string // obtained via CHAP on registration; used for all subsequent requests
	sessionMu     sync.Mutex
	stopped       atomic.Bool
	stopCh        chan struct{}
	http          *http.Client
	saveWatcher   *SaveWatcher
	scriptPaths   map[string]string // title -> run script path, updated by scan
	scriptPathsMu sync.Mutex
}

// authToken returns the active session token if available, otherwise the raw
// config token (used only during the initial CHAP exchange).
func (c *Client) authToken() string {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.sessionToken != "" {
		return c.sessionToken
	}
	return c.cfg.Token
}

// New creates a new agent client, loading or generating a stable agent ID.
func New(cfg Config) (*Client, error) {
	id, err := loadOrCreateID()
	if err != nil {
		return nil, fmt.Errorf("agent ID: %w", err)
	}
	c := &Client{
		cfg:         cfg,
		agentID:     id,
		stopCh:      make(chan struct{}),
		http:        &http.Client{Timeout: 30 * time.Second},
		scriptPaths: make(map[string]string),
	}
	c.saveWatcher = newSaveWatcher(c)
	return c, nil
}

// TestConnection attempts a single register call and returns any error.
func (c *Client) TestConnection() error {
	return c.register()
}

// Stop signals the agent to stop reconnecting.
func (c *Client) Stop() {
	c.stopped.Store(true)
	close(c.stopCh)
}

// Run registers with the server and maintains the SSE connection with exponential backoff.
// This is the only long-running goroutine in idle state.
func (c *Client) Run() {
	// Start save watcher event loop
	if c.saveWatcher != nil {
		go c.saveWatcher.Run()
		defer c.saveWatcher.Close()
	}

	// Watch shortcuts.vdf so Steam Cloud syncs from other OSes are corrected immediately.
	go c.watchShortcuts()

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

		// Check for agent update on each (re)connect; exits+restarts if a new version is available.
		c.checkAndUpdate()

		// Report current installed games on (re)connect
		go c.scanInstalledGames()

		if err := c.listenSSE(); err != nil && !c.stopped.Load() {
			log.Printf("[Agent] SSE disconnected: %v — reconnecting in %s", err, backoff)
		}

		if !c.stopped.Load() {
			c.sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// register performs CHAP-SHA256 with the server and stores the returned session token.
// 1. GET /api/v3/auth/agent-challenge → nonce
// 2. HMAC-SHA256(token, nonce) → chapResponse
// 3. POST /api/v3/agent/register with {nonce, chapResponse, ...} → sessionToken
func (c *Client) register() error {
	// Step 1: fetch challenge nonce
	challengeResp, err := c.http.Get(c.cfg.ServerURL + "/api/v3/auth/agent-challenge")
	if err != nil {
		return fmt.Errorf("challenge: %w", err)
	}
	defer challengeResp.Body.Close()
	if challengeResp.StatusCode != 200 {
		b, _ := io.ReadAll(challengeResp.Body)
		return fmt.Errorf("challenge HTTP %d: %s", challengeResp.StatusCode, strings.TrimSpace(string(b)))
	}
	var challenge struct {
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(challengeResp.Body).Decode(&challenge); err != nil {
		return fmt.Errorf("challenge decode: %w", err)
	}

	// Step 2: compute HMAC-SHA256(token, nonce)
	mac := hmac.New(sha256.New, []byte(c.cfg.Token))
	mac.Write([]byte(challenge.Nonce))
	chapResponse := hex.EncodeToString(mac.Sum(nil))

	// Step 3: register with CHAP
	body, _ := json.Marshal(map[string]any{
		"id":           c.agentID,
		"name":         c.cfg.Name,
		"platform":     runtime.GOOS + "/" + runtime.GOARCH,
		"steamPath":    launcher.FindSteamRoot(),
		"version":      c.cfg.Version,
		"installPaths": discoverInstallPaths(),
		"nonce":        challenge.Nonce,
		"chapResponse": chapResponse,
	})

	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var result struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("register decode: %w", err)
	}
	if result.SessionToken != "" {
		c.sessionMu.Lock()
		c.sessionToken = result.SessionToken
		c.sessionMu.Unlock()
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
	req.Header.Set("Authorization", "Bearer "+c.authToken())
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
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024) // 16MB max — handles large INSTALL_JOB payloads
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
			switch eventType {
			case "INSTALL_JOB":
				if dataLine != "" {
					var job agent.InstallJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad INSTALL_JOB JSON: %v", err)
					} else {
						go c.executeJob(job)
					}
				}
			case "SCAN_GAMES":
				go c.scanInstalledGames()
			case "REFRESH_SHORTCUTS":
				go c.refreshKnownShortcuts()
			case "REGEN_SCRIPTS":
				steamIDs := map[string]int{}
				if dataLine != "" && dataLine != "{}" {
					_ = json.Unmarshal([]byte(dataLine), &steamIDs)
				}
				go c.regenerateScripts(steamIDs)
			case "RESTART_STEAM":
				go restartSteam()
			case "CHECK_UPDATE":
				go c.checkAndUpdate()
			case "UNINSTALL_AGENT":
				log.Printf("[Agent] Received UNINSTALL_AGENT — uninstalling and exiting")
				go c.uninstallSelf()
			case "DELETE_GAME":
				if dataLine != "" {
					var job agent.DeleteGameJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad DELETE_GAME JSON: %v", err)
					} else {
						go c.deleteGame(job)
					}
				}
			case "READ_LOG":
				if dataLine != "" {
					var job agent.ReadLogJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad READ_LOG JSON: %v", err)
					} else {
						go c.sendLog(job)
					}
				}
			case "READ_SCRIPT":
				if dataLine != "" {
					var job agent.ReadScriptJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad READ_SCRIPT JSON: %v", err)
					} else {
						go c.sendScript(job)
					}
				}
			case "RESTORE_SAVE":
				if dataLine != "" {
					var req struct {
						GameID int    `json:"gameId"`
						Title  string `json:"title"`
					}
					if err := json.Unmarshal([]byte(dataLine), &req); err != nil {
						log.Printf("[Agent] Bad RESTORE_SAVE JSON: %v", err)
					} else {
						go c.restoreLatestSave(req.GameID, req.Title)
					}
				}
			case "UPLOAD_SAVE":
				if dataLine != "" && c.saveWatcher != nil {
					var req struct {
						Title string `json:"title"`
					}
					if err := json.Unmarshal([]byte(dataLine), &req); err != nil {
						log.Printf("[Agent] Bad UPLOAD_SAVE JSON: %v", err)
					} else {
						go c.saveWatcher.TriggerUpload(req.Title)
					}
				}
			case "SET_LAUNCH_ARGS":
				if dataLine != "" {
					var req struct {
						Title      string `json:"title"`
						LaunchArgs string `json:"launchArgs"`
						EnvVars    string `json:"envVars"`
						ProtonPath string `json:"protonPath"`
						UseSLS     *bool  `json:"useSLS"` // pointer: absent = true (default)
					}
					if err := json.Unmarshal([]byte(dataLine), &req); err != nil {
						log.Printf("[Agent] Bad SET_LAUNCH_ARGS JSON: %v", err)
					} else {
						useSLS := true
						if req.UseSLS != nil {
							useSLS = *req.UseSLS
						}
						go c.setRunScriptSettings(req.Title, req.LaunchArgs, req.EnvVars, req.ProtonPath, useSLS)
					}
				}
			case "LIST_PROTON":
				if dataLine != "" {
					var job agent.ListProtonJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad LIST_PROTON JSON: %v", err)
					} else {
						go c.listProton(job)
					}
				}
			case "CHANGE_EXE":
				if dataLine != "" {
					var req struct {
						Title   string `json:"title"`
						ExePath string `json:"exePath"`
					}
					if err := json.Unmarshal([]byte(dataLine), &req); err != nil {
						log.Printf("[Agent] Bad CHANGE_EXE JSON: %v", err)
					} else {
						go c.changeGameExe(req.Title, req.ExePath)
					}
				}
			case "LIST_DIR":
				if dataLine != "" {
					var job agent.BrowseDirJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad LIST_DIR JSON: %v", err)
					} else {
						go c.browseDir(job)
					}
				}
			case "RENAME_PREFIX":
				if dataLine != "" {
					var job agent.RenamePrefixJob
					if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
						log.Printf("[Agent] Bad RENAME_PREFIX JSON: %v", err)
					} else {
						go c.renamePrefix(job)
					}
				}
			case "SETUP_ACCELA":
				go c.setupAccela()
			case "SETUP_SLSSTEAM":
				go c.setupSLSSteam()
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
	wineprefix := filepath.Join(home, ".local", "share", "playerr", "prefix_"+safeName(job.GameTitle))
	// Known install dir inside Wine prefix (we force /DIR= to this path)
	gameInstallDir := filepath.Join(wineprefix, "pfx", "drive_c", "Games", safeName(job.GameTitle))

	// ---- Find and run installer ----
	c.reportProgress(job, agent.JobInstalling, "Looking for installer...", 85)

	var gameExe string

	if job.SelectedExe != "" {
		base := filepath.Base(job.SelectedExe)
		lower := strings.ToLower(base)
		isInstaller := strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install")
		selectedPath := findFileByBasename(downloadDir, base)

		if selectedPath == "" {
			log.Printf("[Agent] Selected exe %q not found after extraction, falling back to auto-detect", job.SelectedExe)
			goto autoDetect
		}
		if isInstaller {
			c.reportProgress(job, agent.JobInstalling, "Running installer: "+base, 87)
			logLine := func(line string) { c.reportProgress(job, agent.JobInstalling, line, 88) }
			if err := runInstallerSilent(selectedPath, wineprefix, job.GameTitle, logLine); err != nil {
				log.Printf("[Agent] Installer error (non-fatal): %v", err)
				c.reportProgress(job, agent.JobInstalling, "Installer exited with error: "+err.Error(), 89)
			} else {
				c.reportProgress(job, agent.JobInstalling, "Installer finished", 89)
			}
			gameExe = findMainExe(gameInstallDir)
			if gameExe == "" {
				gameExe = findGameExeInPrefix(wineprefix)
			}
			if gameExe != "" {
				applyCrack(downloadDir, filepath.Dir(gameExe))
			}
		} else {
			gameExe = selectedPath
		}
		goto shortcut
	}

autoDetect:
	{
		installer := findInstaller(downloadDir)
		if installer != "" {
			c.reportProgress(job, agent.JobInstalling, "Running installer: "+filepath.Base(installer), 87)
			logLine := func(line string) { c.reportProgress(job, agent.JobInstalling, line, 88) }
			if err := runInstallerSilent(installer, wineprefix, job.GameTitle, logLine); err != nil {
				log.Printf("[Agent] Installer error (non-fatal): %v", err)
				c.reportProgress(job, agent.JobInstalling, "Installer exited with error: "+err.Error(), 89)
			} else {
				c.reportProgress(job, agent.JobInstalling, "Installer finished", 89)
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
	}

shortcut:

	// ---- Create local Steam shortcut via wrapper script ----
	// The run.sh path is always ~/Games/{title}/run.sh — identical on every Linux device,
	// so the Steam AppID is the same regardless of which agent installs the game.
	// If a shortcut already exists (e.g. created by another device), skip adding a duplicate.
	c.reportProgress(job, agent.JobCreatingShortcut, "Creating Steam shortcut...", 95)
	if gameExe != "" {
		scriptPath := createRunScript(job.GameTitle, gameExe, wineprefix, job.LaunchArgs, job.EnvVars, job.ProtonPath, job.SteamID, job.UseSLS)
		entry := launcher.ShortcutEntry{
			// Title-based AppID is identical on all platforms (Linux, Windows, macOS)
			// so dual-boot / shared Steam installs produce one shortcut updated in-place.
			AppID:    launcher.TitleAppID(job.GameTitle),
			AppName:  job.GameTitle,
			StartDir: filepath.Dir(gameExe),
		}
		if scriptPath != "" {
			entry.Exe = currentOSExe()
			entry.LaunchOptions = shortcutLaunchOptions(job.GameTitle, scriptPath)
			entry.StartDir = filepath.Dir(scriptPath)
		} else {
			entry.Exe = gameExe
		}
		if _, err := launcher.AddSteamShortcut(entry); err != nil {
			log.Printf("[Agent] Steam shortcut error: %v", err)
		} else {
			log.Printf("[Agent] Steam shortcut written for %q (appID=%d)", job.GameTitle, entry.AppID)
			go c.fetchArtwork(job.GameTitle, entry.AppID)
			runner := launcher.FindRunner()
			if runner != nil {
				toolName := launcher.ProtonToolName(runner)
				cfgDir := launcher.FindSteamUserConfigDir()
				if toolName != "" && cfgDir != "" {
					if err := launcher.SetCompatTool(cfgDir, entry.AppID, toolName); err != nil {
						log.Printf("[Agent] localconfig.vdf update failed: %v", err)
					} else {
						log.Printf("[Agent] Set compat tool %q for appID %d", toolName, entry.AppID)
					}
				}
			}
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

	// Re-scan so server immediately reflects the new installation
	go c.scanInstalledGames()
}

// runInstallerSilent runs a Windows installer silently.
// logLine is called for each output line that passes the noise filter (may be nil).
// On Windows: runs the installer natively.
// On Linux/macOS: uses the best available runner (UMU > Proton > Wine).
func runInstallerSilent(installerPath, wineprefix, gameTitle string, logLine func(string)) error {
	silentFlags := []string{
		"/VERYSILENT",
		"/SUPPRESSMSGBOXES",
		"/NORESTART",
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		winDir := filepath.Join(os.Getenv("USERPROFILE"), "Games", safeName(gameTitle))
		cmd = exec.Command(installerPath, append(silentFlags, "/DIR="+winDir)...)
		cmd.Dir = filepath.Dir(installerPath)
	} else {
		runner := launcher.FindRunner()
		if runner == nil {
			log.Printf("[Agent] No runner found — skipping installer %s", installerPath)
			return nil
		}
		args := append(silentFlags, `/DIR=C:\Games\`+safeName(gameTitle))
		cmd = runner.RunWith(installerPath, wineprefix, args...)
		// Wine needs a display driver even for silent installs (explorer.exe creates windows).
		// If DISPLAY is not set, auto-detect from /tmp/.X11-unix/ sockets (Steam Deck runs
		// a gamescope X server that isn't inherited by systemd user services).
		if os.Getenv("DISPLAY") == "" {
			if disp := findDisplay(); disp != "" {
				log.Printf("[Agent] Auto-detected DISPLAY: %s", disp)
				if cmd.Env == nil {
					cmd.Env = os.Environ()
				}
				cmd.Env = append(cmd.Env, "DISPLAY="+disp)
			}
		}
		// If XAUTHORITY is not set, auto-detect it from /run/user/<uid>/ (Steam Deck / Linux sessions).
		if os.Getenv("XAUTHORITY") == "" {
			if xauth := findXauthority(); xauth != "" {
				log.Printf("[Agent] Auto-detected XAUTHORITY: %s", xauth)
				if cmd.Env == nil {
					cmd.Env = os.Environ()
				}
				cmd.Env = append(cmd.Env, "XAUTHORITY="+xauth)
			}
		}
	}

	pr, pw := io.Pipe()
	cmd.Stdout = io.MultiWriter(os.Stdout, pw)
	cmd.Stderr = io.MultiWriter(os.Stderr, pw)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			if logLine != nil {
				if line := installerLineFilter(sc.Text()); line != "" {
					logLine(line)
				}
			}
		}
	}()

	err := cmd.Run()
	pw.Close()
	<-done
	return err
}

// installerLineFilter strips Wine/Proton noise and returns the line if worth surfacing,
// or "" to discard it. Lines are also truncated to 120 chars to fit the UI.
func installerLineFilter(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)
	for _, prefix := range []string{"fixme:", "trace:", "wine: created stub", "0009:", "0014:", "0024:"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	if len(line) > 120 {
		line = line[:117] + "..."
	}
	return line
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
	crackNames := []string{"Crack", "crack", "SKIDROW", "CODEX", "CPY", "EMPRESS", "Crk", "RUNE", "DODI", "FLT", "PLAZA", "RELOADED", "RAZOR", "UNLEASHED", "GOG"}
	_ = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		for _, name := range crackNames {
			if strings.EqualFold(info.Name(), name) {
				log.Printf("[Agent] Applying crack: %s → %s", path, gameInstallDir)
				if err := copyDir(path, gameInstallDir); err != nil {
					log.Printf("[Agent] Crack copy error: %v", err)
				}
				return filepath.SkipDir
			}
		}
		return nil
	})
}

// findAllGameExes returns all game-like exe files in dir (same filter as isGameExe).
func findAllGameExes(dir string) []string {
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if isGameExe(strings.ToLower(info.Name())) {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// findAllGameExesInPrefix searches an entire Wine drive_c for game-like exes,
// skipping standard Windows system directories (windows, users, programdata).
func findAllGameExesInPrefix(driveC string) []string {
	skipDirs := map[string]bool{
		"windows": true, "users": true, "programdata": true,
	}
	var found []string
	_ = filepath.Walk(driveC, func(path string, info os.FileInfo, err error) error {
		if err != nil {
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
		if isGameExe(strings.ToLower(info.Name())) {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// findFileByBasename finds a file with exact base name (case-insensitive) anywhere under dir.
func findFileByBasename(dir, basename string) string {
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil
		}
		if strings.EqualFold(info.Name(), basename) {
			found = path
		}
		return nil
	})
	return found
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
	excludePrefixes := []string{"setup", "install", "unins", "redist", "dxsetup", "vcredist", "directx", "config", "crashreport", "crashpad", "crashhandler", "unitycrashandler", "bugsplat", "quicksfv"}
	for _, p := range excludePrefixes {
		if strings.HasPrefix(lower, p) {
			return false
		}
	}
	// Exclude well-known Wine/Windows system executables that are never the game.
	systemExes := map[string]bool{
		"iexplore.exe": true, "explorer.exe": true, "wmplayer.exe": true,
		"wordpad.exe": true, "write.exe": true, "charmap.exe": true,
		"notepad.exe": true, "msiexec.exe": true, "rundll32.exe": true,
		"regsvr32.exe": true, "cmd.exe": true, "powershell.exe": true,
		"werfault.exe": true, "wineboot.exe": true, "wineconsole.exe": true,
		"taskmgr.exe": true, "regedit.exe": true, "control.exe": true,
		"winhlp32.exe": true, "winemine.exe": true, "winedbg.exe": true,
	}
	return !systemExes[lower]
}

// createRunScript writes a launcher script for the game.
// On Windows: writes run.bat. On Linux: writes run.sh using the found runner.
// launchArgs are extra arguments appended after the exe and before "$@".
// envVars is a newline-separated list of KEY=VALUE pairs exported before the runner.
// protonPath overrides the auto-selected Proton binary when non-empty.
// useSLS controls whether the SLSsteam LD_AUDIT block is injected.
// Returns the script path, or "" if no runner is available and not on Windows.
func createRunScript(gameTitle, gameExe, wineprefix, launchArgs, envVars, protonPath string, steamID int, useSLS bool) string {
	scriptDir := filepath.Join(homeDir(), "Games", safeName(gameTitle))
	_ = os.MkdirAll(scriptDir, 0755)

	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(scriptDir, "run.bat")
		args := ""
		if launchArgs != "" {
			args = " " + launchArgs
		}
		content := fmt.Sprintf("@echo off\r\nstart \"\" %q%s\r\n", gameExe, args)
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

	// If a specific Proton binary is requested, use it directly via RunnerProton.
	if protonPath != "" && (runner.Type == launcher.RunnerProton || runner.Type == launcher.RunnerUMU) {
		runner = &launcher.Runner{Type: launcher.RunnerProton, BinPath: protonPath}
	}

	scriptPath := filepath.Join(scriptDir, "run.sh")
	var content string

	logFile := filepath.Join(scriptDir, "run.log")

	// exitMarker is written after the game closes so the save watcher uploads immediately.
	exitMarker := filepath.Join(scriptDir, exitMarkerName)

	// argsSuffix is appended between the game exe and "$@" in the runner invocation.
	argsSuffix := ""
	if launchArgs != "" {
		argsSuffix = " " + launchArgs
	}

	// Embed launch args as a comment so parseRunScript can recover them on regen.
	launchArgsComment := ""
	if launchArgs != "" {
		launchArgsComment = "# LAUNCH_ARGS: " + launchArgs + "\n"
	}

	// Embed env vars as a JSON-encoded comment so parseRunScript can recover them.
	envVarsComment := ""
	envVarsBlock := ""
	if envVars != "" {
		encoded, _ := json.Marshal(envVars)
		envVarsComment = "# ENV_VARS: " + string(encoded) + "\n"
		envVarsBlock = buildEnvVarsBlock(envVars)
	}

	// Embed proton path override as a comment so parseRunScript can recover it on regen.
	protonPathComment := ""
	if protonPath != "" {
		protonPathComment = "# PROTON_PATH: " + protonPath + "\n"
	}

	// Embed Steam ID as a comment so parseRunScript can recover it on regen.
	steamIDComment := ""
	if steamID != 0 {
		steamIDComment = fmt.Sprintf("# STEAM_ID: %d\n", steamID)
	}

	// Embed useSLS as a comment (only when false) so parseRunScript can recover it on regen.
	useSlsComment := ""
	if !useSLS {
		useSlsComment = "# USE_SLS: false\n"
	}

	// slsBlock activates SLSsteam (LD_AUDIT injection) at runtime if the .so is installed.
	// Using a runtime check means the script works with/without SLSsteam installed.
	slsBlock := ""
	if useSLS {
		slsBlock = "_SLS_LIB=\"$HOME/.local/share/SLSsteam/SLSsteam.so\"\n" +
			"[ -f \"$_SLS_LIB\" ] && export LD_AUDIT=\"$_SLS_LIB\"\n"
	}

	switch runner.Type {
	case launcher.RunnerUMU:
		content = fmt.Sprintf(
			"#!/bin/bash\n"+
				launchArgsComment+
				envVarsComment+
				protonPathComment+
				steamIDComment+
				useSlsComment+
				"LOG=%q\n"+
				"mkdir -p %q\n"+
				"echo \"=== Launch $(date) ===\" >> \"$LOG\"\n"+
				"export WINEPREFIX=%q\n"+
				"export PROTONPATH=%q\n"+
				"export GAMEID=0\n"+
				envVarsBlock+
				slsBlock+
				"%q %q"+argsSuffix+" \"$@\" >> \"$LOG\" 2>&1\n"+
				"touch %q\n",
			logFile, filepath.Join(wineprefix, "pfx"),
			wineprefix, runner.ProtonPath, runner.BinPath, gameExe, exitMarker)
	case launcher.RunnerProton:
		steamRoot := launcher.FindSteamRoot()
		if steamRoot == "" {
			steamRoot = filepath.Dir(filepath.Dir(runner.BinPath))
		}
		content = fmt.Sprintf(
			"#!/bin/bash\n"+
				launchArgsComment+
				envVarsComment+
				protonPathComment+
				steamIDComment+
				useSlsComment+
				"LOG=%q\n"+
				"mkdir -p %q\n"+
				"echo \"=== Launch $(date) ===\" >> \"$LOG\"\n"+
				"export STEAM_COMPAT_DATA_PATH=%q\n"+
				"export STEAM_COMPAT_CLIENT_INSTALL_PATH=%q\n"+
				"export STEAM_COMPAT_APP_ID=0\n"+
				"export PROTON_LOG=1\n"+
				envVarsBlock+
				slsBlock+
				"%q run %q"+argsSuffix+" \"$@\" >> \"$LOG\" 2>&1\n"+
				"touch %q\n",
			logFile, filepath.Join(wineprefix, "pfx"),
			wineprefix, steamRoot, runner.BinPath, gameExe, exitMarker)
	case launcher.RunnerWine:
		content = fmt.Sprintf(
			"#!/bin/bash\n"+
				launchArgsComment+
				envVarsComment+
				protonPathComment+
				useSlsComment+
				"LOG=%q\n"+
				"mkdir -p %q\n"+
				"echo \"=== Launch $(date) ===\" >> \"$LOG\"\n"+
				"export WINEPREFIX=%q\n"+
				envVarsBlock+
				slsBlock+
				"%q %q"+argsSuffix+" \"$@\" >> \"$LOG\" 2>&1\n"+
				"touch %q\n",
			logFile, wineprefix,
			wineprefix, runner.BinPath, gameExe, exitMarker)
	default:
		return ""
	}

	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		log.Printf("[Agent] Failed to write run.sh: %v", err)
		return ""
	}


	return scriptPath
}

// buildEnvVarsBlock converts newline-separated KEY=VALUE pairs into export statements.
// Each non-empty, non-comment line becomes: export KEY="VALUE"\n
func buildEnvVarsBlock(envVars string) string {
	var sb strings.Builder
	for _, line := range strings.Split(envVars, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip surrounding quotes if the user typed them
		val = strings.Trim(val, `"'`)
		fmt.Fprintf(&sb, "export %s=%q\n", key, val)
	}
	return sb.String()
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
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
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
	if pr.onProgress != nil && time.Since(pr.lastReport) >= 500*time.Millisecond {
		pr.onProgress(pr.read, pr.total)
		pr.lastReport = time.Now()
	}
	return n, err
}

// downloadFile downloads a URL to destPath.
// onProgress is called periodically with (bytesRead, totalBytes); totalBytes may be -1.
func (c *Client) downloadFile(rawURL, destPath string, onProgress func(read, total int64)) error {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken())

	// Use a no-timeout client — game files can be many GB and transfer time is unbounded.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	var reader io.Reader = resp.Body
	if onProgress != nil {
		reader = &progressReader{
			r:          resp.Body,
			total:      resp.ContentLength,
			onProgress: onProgress,
		}
	}

	if _, err := io.Copy(f, reader); err != nil {
		f.Close()
		os.Remove(destPath) // delete partial file so callers don't see corrupt data
		return err
	}
	return f.Close()
}

// scanInstalledGames walks all known install paths, collects InstalledGame records,
// and POSTs the list back to the server.
func (c *Client) scanInstalledGames() {
	log.Println("[Agent] Scanning installed games...")

	paths := discoverInstallPaths()
	shortcuts := loadShortcutEntries()

	var games []agent.InstalledGame

	for _, ip := range paths {
		entries, err := os.ReadDir(ip.Path)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			gameDir := filepath.Join(ip.Path, e.Name())
			title := e.Name()
			exePath := ""
			var exeCandidates []string

			// Detect launcher script (run.sh / run.bat)
			scriptPath := ""
			for _, name := range []string{"run.sh", "run.bat"} {
				candidate := filepath.Join(gameDir, name)
				if _, err := os.Stat(candidate); err == nil {
					scriptPath = candidate
					break
				}
			}

			// Resolve the active exe and all candidates.
			// For Wine/Proton installs the real exe is inside the prefix, not gameDir.
			if scriptPath != "" {
				wineprefix, parsedExe, _, _, _, _, _ := parseRunScript(scriptPath)
				if parsedExe != "" {
					exePath = parsedExe
				}
				if wineprefix != "" {
					// Search the entire drive_c (excluding system dirs) so we find the
					// game exe even if the installer ignored our /DIR= flag and put it
					// somewhere like Program Files instead of C:\Games\.
					driveC := filepath.Join(wineprefix, "pfx", "drive_c")
					exeCandidates = findAllGameExesInPrefix(driveC)
				} else if parsedExe != "" {
					exeCandidates = findAllGameExes(filepath.Dir(parsedExe))
				}
			}
			if exePath == "" {
				exePath = findMainExe(gameDir)
				exeCandidates = findAllGameExes(gameDir)
			}

			size := dirSize(gameDir)

			// Ensure shortcut points to the current OS launcher.
			// Steam Cloud syncs shortcuts.vdf across dual-boot installs, so the
			// shortcut written by the other OS may be present but unusable here.
			// Refreshing on every scan fixes it automatically on boot.
			// After refreshShortcut the entry is guaranteed to exist, so we set
			// hasShortcut = true directly rather than re-reading the stale
			// shortcuts list that was loaded before this loop started.
			hasShortcut := false
			if scriptPath != "" {
				refreshShortcut(title, scriptPath)
				hasShortcut = true
			} else {
				for _, s := range shortcuts {
					if strings.EqualFold(s.AppName, title) {
						hasShortcut = true
						break
					}
				}
			}

			// Simple version detection: look for a version file in the launcher dir
			// and in the actual game install dir (e.g. inside the Wine prefix).
			version := ""
			versionSearchDirs := []string{gameDir}
			if exePath != "" {
				versionSearchDirs = append(versionSearchDirs, filepath.Dir(exePath))
			}
			outer:
			for _, searchDir := range versionSearchDirs {
				for _, vf := range []string{"version.txt", "VERSION", ".version"} {
					data, err := os.ReadFile(filepath.Join(searchDir, vf))
					if err == nil {
						if v := strings.TrimSpace(string(data)); v != "" {
							version = v
							break outer
						}
					}
				}
			}
			// Engine-specific metadata files (GOG, Ren'Py, RPG Maker, Electron/ASAR).
			if version == "" && exePath != "" {
				version = readEngineVersion(filepath.Dir(exePath))
			}
			// Fallback: read ProductVersion from the exe's PE resource block.
			if version == "" && exePath != "" {
				version = readExeVersion(exePath)
			}

			games = append(games, agent.InstalledGame{
				Title:         title,
				InstallPath:   gameDir,
				ExePath:       exePath,
				ExeCandidates: exeCandidates,
				ScriptPath:    scriptPath,
				SizeBytes:     size,
				HasShortcut:   hasShortcut,
				Version:       version,
			})
		}
	}

	body, _ := json.Marshal(games)
	postURL := fmt.Sprintf("%s/api/v3/agent/%s/games", c.cfg.ServerURL, c.agentID)
	req, err := http.NewRequest("POST", postURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("[Agent] Scan report request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[Agent] Scan report POST error: %v", err)
		return
	}
	resp.Body.Close()
	log.Printf("[Agent] Reported %d installed games", len(games))

	// Keep scriptPaths up to date for the shortcut watcher.
	c.scriptPathsMu.Lock()
	c.scriptPaths = make(map[string]string, len(games))
	for _, g := range games {
		if g.ScriptPath != "" {
			c.scriptPaths[g.Title] = g.ScriptPath
		}
	}
	c.scriptPathsMu.Unlock()

	// Update save watcher with new game list
	if c.saveWatcher != nil {
		titles := make([]string, 0, len(games))
		scriptDirMap := make(map[string]string)
		for _, g := range games {
			titles = append(titles, g.Title)
			if g.ScriptPath != "" {
				scriptDirMap[safeName(g.Title)] = filepath.Dir(g.ScriptPath)
			} else {
				// Default script dir: ~/Games/{safeName}/
				scriptDirMap[safeName(g.Title)] = filepath.Join(homeDir(), "Games", safeName(g.Title))
			}
		}
		go c.saveWatcher.UpdateGames(titles, scriptDirMap)
	}
}

// uninstallSelf removes the agent binary, config dir, and systemd/launchd service,
// then exits. The server will mark the agent offline via its normal heartbeat timeout.
//
// IMPORTANT: do NOT call "systemctl stop" on ourselves — that sends SIGTERM to this
// very process before cleanup can finish. Instead we disable + remove the service
// files first, then call os.Exit(0). On Linux the binary inode stays alive until
// exit even after os.RemoveAll removes the directory entry.
func (c *Client) uninstallSelf() {
	log.Printf("[Agent] Uninstalling agent...")

	// Disable and remove systemd user service (Linux)
	if runtime.GOOS == "linux" {
		exec.Command("systemctl", "--user", "disable", "cargodeck-agent").Run()
		svcFile := filepath.Join(homeDir(), ".config", "systemd", "user", "cargodeck-agent.service")
		os.Remove(svcFile)
		exec.Command("systemctl", "--user", "daemon-reload").Run()
		log.Printf("[Agent] Removed systemd service")
	}

	// Unload launchd agent (macOS)
	if runtime.GOOS == "darwin" {
		plist := filepath.Join(homeDir(), "Library", "LaunchAgents", "com.cargodeck.agent.plist")
		exec.Command("launchctl", "unload", plist).Run()
		os.Remove(plist)
		log.Printf("[Agent] Removed launchd plist")
	}

	// Remove the entire config dir (~/.config/cargodeck-agent/) which contains
	// the binary, agent ID, and any cached state. On Linux, the running binary
	// inode is reference-counted by the kernel and stays in memory until exit.
	configDir := filepath.Dir(agentIDPath())
	log.Printf("[Agent] Removing config dir: %s", configDir)
	os.RemoveAll(configDir)

	log.Printf("[Agent] Uninstall complete — exiting")
	os.Exit(0)
}

// deleteGame removes a game directory and optionally its Steam shortcut,
// then triggers a re-scan so the server state stays current.
func (c *Client) deleteGame(job agent.DeleteGameJob) {
	log.Printf("[Agent] Deleting %q from %s", job.Title, job.InstallPath)

	// Remove game directory
	if err := os.RemoveAll(job.InstallPath); err != nil {
		log.Printf("[Agent] Delete failed: %v", err)
		c.reportDeleteProgress(job.JobID, "failed", "Delete failed: "+err.Error())
		return
	}
	log.Printf("[Agent] Deleted game directory: %s", job.InstallPath)

	// Remove launcher script directory (~/Games/{title}/) if it exists separately
	scriptDir := filepath.Join(homeDir(), "Games", safeName(job.Title))
	if scriptDir != job.InstallPath {
		_ = os.RemoveAll(scriptDir)
	}

	// Remove wineprefix if present
	wineprefix := filepath.Join(homeDir(), ".local", "share", "playerr", fmt.Sprintf("prefix_%s", safeName(job.Title)))
	_ = os.RemoveAll(wineprefix)

	// Optionally remove Steam shortcut
	if job.RemoveShortcut {
		removeShortcut(job.Title)
	}

	c.reportDeleteProgress(job.JobID, "done", "Deleted successfully")

	// Re-scan so server reflects the removal
	c.scanInstalledGames()
}

// currentOSExe returns the system executable used to launch game scripts on this OS.
func currentOSExe() string {
	if runtime.GOOS == "windows" {
		if c := os.Getenv("COMSPEC"); c != "" {
			return c
		}
		return filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	return "/bin/bash"
}

// shortcutLaunchOptions returns the LaunchOptions for the current OS.
// On Windows we use %USERPROFILE% so the shortcut is portable across all
// Windows accounts — cmd.exe expands it at launch time. On Linux we use
// the absolute path (bash does not expand $HOME in positional arguments).
func shortcutLaunchOptions(title, scriptPath string) string {
	if runtime.GOOS == "windows" {
		return `%USERPROFILE%\Games\` + safeName(title) + `\run.bat`
	}
	return scriptPath
}

// refreshShortcut ensures the Steam shortcut for a game points to the current
// OS launcher with the correct path. Skips the write if both Exe and
// LaunchOptions already match, which terminates the watch→write→watch loop.
func refreshShortcut(title, scriptPath string) {
	exe := currentOSExe()
	opts := shortcutLaunchOptions(title, scriptPath)
	if launcher.ShortcutEntryMatches(title, exe, opts) {
		return
	}
	entry := launcher.ShortcutEntry{
		AppID:         launcher.TitleAppID(title),
		AppName:       title,
		StartDir:      filepath.Dir(scriptPath),
		Exe:           exe,
		LaunchOptions: opts,
	}
	if _, err := launcher.AddSteamShortcut(entry); err != nil {
		log.Printf("[Agent] Shortcut refresh failed for %q: %v", title, err)
	} else {
		log.Printf("[Agent] Shortcut corrected for %q → %s", title, exe)
	}
}

// refreshKnownShortcuts corrects Steam shortcuts for all known installed games,
// fetches missing SteamGridDB artwork, then re-scans so the server reflects
// the updated hasShortcut state immediately.
func (c *Client) refreshKnownShortcuts() {
	c.scriptPathsMu.Lock()
	paths := make(map[string]string, len(c.scriptPaths))
	for k, v := range c.scriptPaths {
		paths[k] = v
	}
	c.scriptPathsMu.Unlock()
	for title, scriptPath := range paths {
		refreshShortcut(title, scriptPath)
		go c.fetchArtwork(title, launcher.TitleAppID(title))
	}
	log.Printf("[Agent] Refreshed shortcuts for %d games", len(paths))
	c.scanInstalledGames()
}

// watchShortcuts monitors shortcuts.vdf for external changes (e.g. Steam Cloud
// syncing the other OS's shortcuts) and immediately corrects any entries that
// have the wrong Exe for the current OS.
func (c *Client) watchShortcuts() {
	cfgDir := launcher.FindSteamUserConfigDir()
	if cfgDir == "" {
		return
	}
	vdfPath := filepath.Join(cfgDir, "shortcuts.vdf")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Agent] Cannot create shortcuts watcher: %v", err)
		return
	}
	defer w.Close()

	// Watch the file if it exists, otherwise watch the directory so we catch creation.
	if _, err := os.Stat(vdfPath); err == nil {
		_ = w.Add(vdfPath)
	} else {
		_ = w.Add(cfgDir)
	}

	debounce := time.NewTimer(0)
	<-debounce.C // drain initial tick

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if filepath.Base(event.Name) == "shortcuts.vdf" {
				debounce.Reset(500 * time.Millisecond)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("[Agent] shortcuts.vdf watcher error: %v", err)
		case <-debounce.C:
			c.refreshKnownShortcuts()
		case <-c.stopCh:
			return
		}
	}
}

// fetchArtwork resolves SteamGridDB artwork URLs via the server (which holds
// the API key) and downloads each image to the local Steam grid directory.
// Images that already exist are skipped. Non-fatal: errors are only logged.
func (c *Client) fetchArtwork(title string, appID uint32) {
	gridDir := launcher.FindSteamGridDir()
	if gridDir == "" {
		return
	}

	reqURL := fmt.Sprintf("%s/api/v3/agent/artwork?game=%s", c.cfg.ServerURL, url.QueryEscape(title))
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken())

	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[Agent] Artwork lookup failed for %q: %v", title, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 503 {
		log.Printf("[Agent] SteamGridDB not configured on server — skipping artwork for %q", title)
		return
	}
	if resp.StatusCode != 200 {
		log.Printf("[Agent] Artwork lookup for %q: HTTP %d", title, resp.StatusCode)
		return
	}

	var urls struct {
		Portrait  string `json:"portrait"`
		Landscape string `json:"landscape"`
		Hero      string `json:"hero"`
		Logo      string `json:"logo"`
		Icon      string `json:"icon"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&urls); err != nil {
		return
	}

	if err := os.MkdirAll(gridDir, 0755); err != nil {
		log.Printf("[Agent] Cannot create grid dir %s: %v", gridDir, err)
		return
	}

	idStr := fmt.Sprintf("%d", appID)
	saved := 0
	save := func(imgURL, suffix string) string {
		if imgURL == "" {
			return ""
		}
		e := artworkExt(imgURL)
		dest := filepath.Join(gridDir, idStr+suffix+e)
		if _, err := os.Stat(dest); err == nil {
			return dest // already present
		}
		dlResp, err := http.Get(imgURL) //nolint:noctx
		if err != nil {
			log.Printf("[Agent] Artwork download failed (%s%s): %v", idStr, suffix, err)
			return ""
		}
		defer dlResp.Body.Close()
		f, err := os.Create(dest)
		if err != nil {
			log.Printf("[Agent] Artwork create failed (%s): %v", dest, err)
			return ""
		}
		if _, err := io.Copy(f, dlResp.Body); err != nil {
			log.Printf("[Agent] Artwork write failed (%s): %v", dest, err)
			f.Close()
			return ""
		}
		f.Close()
		saved++
		return dest
	}

	save(urls.Portrait, "p")
	save(urls.Landscape, "")
	save(urls.Hero, "_hero")
	save(urls.Logo, "_logo")
	iconPath := save(urls.Icon, "_icon")

	// Update the shortcut's icon field if we got an icon and it isn't already set
	if iconPath != "" {
		updateShortcutIcon(title, iconPath)
	}

	log.Printf("[Agent] Artwork for %q: %d image(s) saved to %s", title, saved, gridDir)
}

// updateShortcutIcon sets the icon field on an existing shortcut entry if it is blank.
func updateShortcutIcon(title, iconPath string) {
	cfgDir := launcher.FindSteamUserConfigDir()
	if cfgDir == "" {
		return
	}
	data, err := os.ReadFile(cfgDir + "/shortcuts.vdf")
	if err != nil {
		return
	}
	for _, e := range launcher.ParseShortcutsVDF(data) {
		if strings.EqualFold(e.AppName, title) && e.Icon == "" {
			e.Icon = iconPath
			_, _ = launcher.AddSteamShortcut(e)
			log.Printf("[Agent] Set icon for %q → %s", title, iconPath)
			return
		}
	}
}

func artworkExt(imageURL string) string {
	u := strings.Split(imageURL, "?")[0]
	if e := filepath.Ext(u); e != "" {
		return e
	}
	return ".png"
}

// regenerateScripts rewrites run.sh for all installed games using the current
// runner and log-redirect format, without touching the game files themselves.
// Called on REGEN_SCRIPTS SSE event or implicitly when needed.
func (c *Client) regenerateScripts(steamIDs map[string]int) {
	gamesDir := filepath.Join(homeDir(), "Games")
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		log.Printf("[Agent] regenerateScripts: cannot read %s: %v", gamesDir, err)
		return
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		gameDir := filepath.Join(gamesDir, e.Name())
		scriptPath := filepath.Join(gameDir, "run.sh")
		if _, err := os.Stat(scriptPath); err != nil {
			continue // no run.sh, skip
		}
		// Parse wineprefix, exe, launch args, env vars, proton path, and SLS flag from existing script
		wineprefix, gameExe, launchArgs, envVars, protonPath, steamID, useSLS := parseRunScript(scriptPath)
		// Fill in steamID from server-provided map when the existing script has none.
		// This fixes games installed before their Steam ID was set in the library.
		if steamID == 0 {
			if id, ok := steamIDs[e.Name()]; ok {
				steamID = id
				log.Printf("[Agent] regenerateScripts: resolved Steam ID %d for %q from server map", id, e.Name())
			}
		}
		// Validate the parsed exe — it may be stale/wrong (e.g. UnityCrashHandler64.exe)
		if gameExe == "" || !isGameExe(strings.ToLower(filepath.Base(gameExe))) {
			if gameExe != "" {
				log.Printf("[Agent] regenerateScripts: rejecting %q for %q, re-scanning", filepath.Base(gameExe), e.Name())
			}
			gameExe = findMainExe(gameDir)
		}
		if gameExe == "" {
			log.Printf("[Agent] regenerateScripts: cannot find exe for %s, skipping", e.Name())
			continue
		}
		// If wineprefix is still empty (old broken script), generate a title-based one
		if wineprefix == "" {
			wineprefix = filepath.Join(homeDir(), ".local", "share", "playerr", "prefix_"+safeName(e.Name()))
			log.Printf("[Agent] regenerateScripts: no wineprefix found for %q, using %s", e.Name(), wineprefix)
		}
		newPath := createRunScript(e.Name(), gameExe, wineprefix, launchArgs, envVars, protonPath, steamID, useSLS)
		if newPath != "" {
			n++
			log.Printf("[Agent] Regenerated run.sh for %q", e.Name())
		}
	}
	log.Printf("[Agent] Regenerated %d run scripts", n)
	c.scanInstalledGames()
}

// changeGameExe rewrites the run.sh launcher script to use a different exe
// and updates the Steam shortcut to point at it.
func (c *Client) changeGameExe(title, exePath string) {
	scriptDir := filepath.Join(homeDir(), "Games", safeName(title))
	scriptPath := filepath.Join(scriptDir, "run.sh")
	wineprefix, _, launchArgs, envVars, protonPath, steamID, useSLS := parseRunScript(scriptPath)

	newScript := createRunScript(title, exePath, wineprefix, launchArgs, envVars, protonPath, steamID, useSLS)
	if newScript == "" {
		log.Printf("[Agent] changeGameExe: no runner available for %q", title)
		return
	}

	entry := launcher.ShortcutEntry{
		AppID:         launcher.TitleAppID(title),
		AppName:       title,
		Exe:           currentOSExe(),
		LaunchOptions: shortcutLaunchOptions(title, newScript),
		StartDir:      filepath.Dir(newScript),
	}
	if _, err := launcher.AddSteamShortcut(entry); err != nil {
		log.Printf("[Agent] changeGameExe: shortcut error: %v", err)
	} else {
		log.Printf("[Agent] changeGameExe: updated %q → %q", title, exePath)
	}

	go c.scanInstalledGames()
}

// setRunScriptSettings rewrites run.sh to update per-device launch arguments, env vars, proton override, and SLS flag
// without requiring a full reinstall.
func (c *Client) setRunScriptSettings(title, launchArgs, envVars, protonPath string, useSLS bool) {
	scriptPath := filepath.Join(homeDir(), "Games", safeName(title), "run.sh")
	wineprefix, gameExe, _, _, _, steamID, _ := parseRunScript(scriptPath)
	if gameExe == "" {
		log.Printf("[Agent] setRunScriptSettings: cannot parse exe from run.sh for %q — reinstall to apply settings", title)
		return
	}
	createRunScript(title, gameExe, wineprefix, launchArgs, envVars, protonPath, steamID, useSLS)
	log.Printf("[Agent] Updated run settings for %q: args=%q envVars=%q protonPath=%q useSLS=%v", title, launchArgs, envVars, protonPath, useSLS)
}

// parseRunScript extracts the WINEPREFIX path, game exe path, launch args, env vars, proton path, Steam ID, and useSLS from a run.sh.
// Returns ("", "", "", "", "", 0, true) if the script cannot be parsed.
func parseRunScript(scriptPath string) (wineprefix, gameExe, launchArgs, envVars, protonPath string, steamID int, useSLS bool) {
	useSLS = true // default: SLS on
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Embedded launch args comment written by createRunScript
		if strings.HasPrefix(line, "# LAUNCH_ARGS:") {
			launchArgs = strings.TrimSpace(strings.TrimPrefix(line, "# LAUNCH_ARGS:"))
		}
		// Embedded env vars comment written by createRunScript (JSON-encoded)
		if strings.HasPrefix(line, "# ENV_VARS:") {
			raw := strings.TrimSpace(strings.TrimPrefix(line, "# ENV_VARS:"))
			_ = json.Unmarshal([]byte(raw), &envVars)
		}
		// Embedded proton path override written by createRunScript
		if strings.HasPrefix(line, "# PROTON_PATH:") {
			protonPath = strings.TrimSpace(strings.TrimPrefix(line, "# PROTON_PATH:"))
		}
		// Embedded Steam App ID written by createRunScript
		if strings.HasPrefix(line, "# STEAM_ID:") {
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "# STEAM_ID:")), "%d", &steamID)
		}
		// Embedded SLS flag (only written when false; absence means true)
		if line == "# USE_SLS: false" {
			useSLS = false
		}
		// WINEPREFIX is used by UMU and Wine runners
		if strings.HasPrefix(line, "export WINEPREFIX=") {
			wineprefix = strings.Trim(strings.TrimPrefix(line, "export WINEPREFIX="), `"`)
		}
		// STEAM_COMPAT_DATA_PATH is used by the Proton runner
		if strings.HasPrefix(line, "export STEAM_COMPAT_DATA_PATH=") {
			v := strings.Trim(strings.TrimPrefix(line, "export STEAM_COMPAT_DATA_PATH="), `"`)
			if v != "" {
				wineprefix = v
			}
		}
		// Runner invocation lines end with: "exe" "$@" >> ... or "exe" "$@"
		// The exe is the last double-quoted token before "$@"
		if strings.Contains(line, `"$@"`) && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "export") {
			parts := strings.Split(line, `"`)
			for i := len(parts) - 1; i >= 1; i-- {
				if parts[i] == "$@" {
					continue
				}
				candidate := parts[i]
				if strings.HasSuffix(candidate, ".exe") || strings.HasSuffix(candidate, ".EXE") {
					gameExe = candidate
					break
				}
			}
		}
	}
	return
}

// restartSteam gracefully shuts down Steam. In Game Mode (Steam Deck) the
// gamescope session manager relaunches Steam automatically after shutdown.
func restartSteam() {
	log.Println("[Agent] Restarting Steam...")
	// Try the graceful shutdown signal first
	if err := exec.Command("steam", "-shutdown").Run(); err != nil {
		log.Printf("[Agent] steam -shutdown failed (%v), falling back to pkill", err)
		_ = exec.Command("pkill", "-SIGTERM", "-f", "steam").Run()
	}
	log.Println("[Agent] Steam shutdown signal sent")
}

// postToServer marshals payload as JSON and POSTs it to endpoint with Bearer auth.
func (c *Client) postToServer(endpoint string, payload map[string]string) {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// sendLog reads the run.log for a game and POSTs its content back to the server.
func (c *Client) sendLog(job agent.ReadLogJob) {
	content := readLastLines(filepath.Join(homeDir(), "Games", safeName(job.GameTitle), "run.log"), 200)
	c.postToServer("/api/v3/agent/log", map[string]string{
		"requestId": job.RequestID,
		"gameTitle": job.GameTitle,
		"content":   content,
	})
	log.Printf("[Agent] Sent log for %q (%d bytes)", job.GameTitle, len(content))
}

func (c *Client) sendScript(job agent.ReadScriptJob) {
	scriptPath := filepath.Join(homeDir(), "Games", safeName(job.GameTitle), "run.sh")
	content, err := os.ReadFile(scriptPath)
	var body string
	if err != nil {
		batPath := filepath.Join(homeDir(), "Games", safeName(job.GameTitle), "run.bat")
		if batContent, batErr := os.ReadFile(batPath); batErr == nil {
			body = string(batContent)
		} else {
			body = fmt.Sprintf("Script not found at: %s\n\nThe game may not have been installed yet.", scriptPath)
		}
	} else {
		body = string(content)
	}
	c.postToServer("/api/v3/agent/script", map[string]string{
		"requestId": job.RequestID,
		"gameTitle": job.GameTitle,
		"content":   body,
	})
	log.Printf("[Agent] Sent script for %q (%d bytes)", job.GameTitle, len(body))
}

// readLastLines returns the last n lines of a file, or an error message if unreadable.
func readLastLines(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Log not found at: %s\n\nThe game may not have been launched yet.", path)
	}
	if len(data) == 0 {
		return fmt.Sprintf("Log is empty: %s", path)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = append([]string{fmt.Sprintf("... (showing last %d of %d lines)", n, len(lines))}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

// removeShortcut removes the Steam shortcut entry for the given game title.
func removeShortcut(title string) {
	cfgDir := launcher.FindSteamUserConfigDir()
	if cfgDir == "" {
		return
	}
	vdfPath := filepath.Join(cfgDir, "shortcuts.vdf")
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return
	}
	entries := launcher.ParseShortcutsVDF(data)
	filtered := entries[:0]
	for _, e := range entries {
		if !strings.EqualFold(e.AppName, title) {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == len(entries) {
		return // nothing removed
	}
	// Re-use AddSteamShortcut's internal builder by writing back manually
	// Since buildShortcutsVDF is unexported, we call AddSteamShortcut with a dummy to
	// trigger the write — instead, we'll write the filtered list by re-adding each entry.
	// Simplest: just delete the file and re-add remaining entries.
	_ = os.Remove(vdfPath)
	for _, e := range filtered {
		_, _ = launcher.AddSteamShortcut(e)
	}
	log.Printf("[Agent] Removed shortcut for %q", title)
}

// browseDir lists the contents of a directory on the agent and posts the result back.
func (c *Client) browseDir(job agent.BrowseDirJob) {
	path := job.Path
	if path == "~" || path == "" {
		path = homeDir()
	}

	result := agent.BrowseDirResult{RequestID: job.RequestID, Path: path}
	entries, err := os.ReadDir(path)
	if err != nil {
		result.Error = err.Error()
	} else {
		for _, e := range entries {
			entryPath := filepath.Join(path, e.Name())
			isDir := e.IsDir()
			// Follow symlinks so Wine prefix symlinks (e.g. drive_c/users/name → /home/name)
			// are correctly identified as directories rather than files.
			if e.Type()&os.ModeSymlink != 0 {
				if info, err := os.Stat(entryPath); err == nil {
					isDir = info.IsDir()
				}
			}
			result.Entries = append(result.Entries, agent.DirEntry{
				Name:  e.Name(),
				Path:  entryPath,
				IsDir: isDir,
			})
		}
	}

	body, _ := json.Marshal(result)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/browse-result", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[Agent] browse-result POST failed: %v", err)
		return
	}
	resp.Body.Close()
}

// listProton scans for available Proton installations and posts the result back to the server.
func (c *Client) listProton(job agent.ListProtonJob) {
	installs := launcher.ListAllProtonVersions()
	versions := make([]agent.ProtonVersionInfo, len(installs))
	for i, p := range installs {
		versions[i] = agent.ProtonVersionInfo{Name: p.Name, BinPath: p.BinPath}
	}
	result := agent.ListProtonResult{RequestID: job.RequestID, Versions: versions}
	body, _ := json.Marshal(result)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/proton-result", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	resp, err := c.http.Do(req)
	if err != nil {
		log.Printf("[Agent] proton-result POST failed: %v", err)
		return
	}
	resp.Body.Close()
}

// renamePrefix renames a game's Wine prefix from the old numeric format
// (prefix_{gameId}) to the title-based format (prefix_{safeTitle}) and
// updates the WINEPREFIX / STEAM_COMPAT_DATA_PATH line in run.sh to match.
func (c *Client) renamePrefix(job agent.RenamePrefixJob) {
	scriptPath := filepath.Join(homeDir(), "Games", safeName(job.GameTitle), "run.sh")
	currentPrefix, _, _, _, _, _, _ := parseRunScript(scriptPath)

	newPrefix := filepath.Join(homeDir(), ".local", "share", "playerr", "prefix_"+safeName(job.GameTitle))

	// Fall back to the old numeric naming if run.sh didn't yield a prefix
	if currentPrefix == "" {
		oldNumeric := filepath.Join(homeDir(), ".local", "share", "playerr", fmt.Sprintf("prefix_%d", job.GameID))
		if _, err := os.Stat(oldNumeric); err == nil {
			currentPrefix = oldNumeric
		}
	}

	if currentPrefix == "" || currentPrefix == newPrefix {
		log.Printf("[Agent] Prefix already correctly named (or not found) for %q", job.GameTitle)
		return
	}

	if _, err := os.Stat(currentPrefix); os.IsNotExist(err) {
		log.Printf("[Agent] Prefix directory not found, cannot rename: %s", currentPrefix)
		return
	}

	if err := os.Rename(currentPrefix, newPrefix); err != nil {
		log.Printf("[Agent] Failed to rename prefix %s → %s: %v", currentPrefix, newPrefix, err)
		return
	}
	log.Printf("[Agent] Renamed prefix: %s → %s", currentPrefix, newPrefix)

	// Update every occurrence of the old path in run.sh
	data, err := os.ReadFile(scriptPath)
	if err == nil {
		updated := strings.ReplaceAll(string(data), currentPrefix, newPrefix)
		if err := os.WriteFile(scriptPath, []byte(updated), 0755); err == nil {
			log.Printf("[Agent] Updated run.sh prefix path for %q", job.GameTitle)
		}
	}
}

func (c *Client) reportDeleteProgress(jobID, status, message string) {
	prog := map[string]any{"jobId": jobID, "status": status, "message": message, "percent": 100}
	body, _ := json.Marshal(prog)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/progress", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authToken())
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// loadShortcutEntries reads shortcuts.vdf and returns all entries.
func loadShortcutEntries() []launcher.ShortcutEntry {
	cfgDir := launcher.FindSteamUserConfigDir()
	if cfgDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(cfgDir, "shortcuts.vdf"))
	if err != nil {
		return nil
	}
	return launcher.ParseShortcutsVDF(data)
}

// dirSize returns the total size of all files under dir.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
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
	req.Header.Set("Authorization", "Bearer "+c.authToken())
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

// findXauthority returns the path to the X authority file for the current user session.
// On Linux, desktop sessions store a random xauth_* or iceauth_* file in /run/user/<uid>/.
// Steam Deck (gamescope) uses iceauth_* instead of xauth_*.
// This allows Wine installers (which need a display driver) to connect to X from a
// background service that doesn't inherit the session's XAUTHORITY env var.
func findXauthority() string {
	xauthDir := fmt.Sprintf("/run/user/%d", os.Getuid())
	entries, err := os.ReadDir(xauthDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "xauth") || strings.HasPrefix(e.Name(), "iceauth") {
			return filepath.Join(xauthDir, e.Name())
		}
	}
	return ""
}

// findDisplay detects an X11 display from /tmp/.X11-unix/ sockets.
// Useful when running as a systemd service that doesn't inherit DISPLAY from the session.
// Tries :0 first (most common), then :1.
func findDisplay() string {
	for _, n := range []string{"X0", "X1", "X2"} {
		if _, err := os.Stat(filepath.Join("/tmp/.X11-unix", n)); err == nil {
			return ":" + n[1:] // "X0" → ":0"
		}
	}
	return ""
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
		return filepath.Join(os.Getenv("APPDATA"), "cargodeck-agent", "id")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "cargodeck-agent", "id")
	default:
		return filepath.Join(home, ".config", "cargodeck-agent", "id")
	}
}

// ---- Self-update ----

// platformString returns the platform identifier used by the server's binary endpoint.
func platformString() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux-x64"
	case "linux/arm64":
		return "linux-arm64"
	case "windows/amd64":
		return "win-x64"
	case "darwin/amd64":
		return "osx-x64"
	case "darwin/arm64":
		return "osx-arm64"
	default:
		return ""
	}
}

// checkAndUpdate fetches the server's current agent version and self-updates if it differs.
// On success it replaces the binary on disk and exits with code 1 so the service
// manager (systemd Restart=on-failure / launchd KeepAlive) restarts with the new binary.
func (c *Client) checkAndUpdate() {
	platform := platformString()
	if platform == "" {
		return // unsupported platform
	}

	// Fetch server's current agent version
	resp, err := c.http.Get(c.cfg.ServerURL + "/api/v3/agent/version")
	if err != nil || resp.StatusCode != 200 {
		return
	}
	defer resp.Body.Close()
	var vr struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return
	}
	// If the server is a dev build, don't push updates.
	// If the agent is dev but the server has a real version, update it.
	if vr.Version == "" || vr.Version == "dev" {
		return // server is dev build, skip
	}
	if vr.Version == c.cfg.Version {
		return // already up to date
	}

	log.Printf("[Agent] Update available: %s → %s — downloading...", c.cfg.Version, vr.Version)

	// Download new binary
	dlResp, err := c.http.Get(c.cfg.ServerURL + "/api/v3/agent/binary?os=" + platform)
	if err != nil || dlResp.StatusCode != 200 {
		log.Printf("[Agent] Update download failed: status %d, err %v", dlResp.StatusCode, err)
		return
	}
	defer dlResp.Body.Close()

	// Write to a temp file alongside the current binary
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("[Agent] Update: cannot determine executable path: %v", err)
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	tmpPath := exePath + ".new"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		log.Printf("[Agent] Update: cannot write temp binary: %v", err)
		return
	}
	if _, err := io.Copy(f, dlResp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		log.Printf("[Agent] Update: download write failed: %v", err)
		return
	}
	f.Close()

	// Verify the new binary is functional before committing
	testCmd := exec.Command(tmpPath,
		"--server", c.cfg.ServerURL,
		"--token", c.cfg.Token,
		"--name", "update-check",
		"--test-connection",
	)
	if err := testCmd.Run(); err != nil {
		os.Remove(tmpPath)
		log.Printf("[Agent] Update: new binary failed self-test (%v) — keeping current version", err)
		return
	}

	// Atomically replace the current binary
	if err := os.Rename(tmpPath, exePath); err != nil {
		os.Remove(tmpPath)
		log.Printf("[Agent] Update: replace failed: %v", err)
		return
	}

	log.Printf("[Agent] Updated to %s — restarting...", vr.Version)
	os.Exit(1) // systemd Restart=on-failure will restart with the new binary
}

// setupAccela installs ACCELA on the agent machine.
// All steps are user-space only — no sudo or pacman required.
//
//  1. Install .NET SDK 9 via the official installer (to ~/.dotnet)
//  2. Download the latest ACCELA AppImage from GitHub releases
//  3. Extract to ~/.local/share/ACCELA
//  4. Write the ACCELA config file
//  5. Install SLSsteam alongside ACCELA (non-fatal)
func (c *Client) setupAccela() {
	log.Printf("[Agent] SETUP_ACCELA: starting")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[Agent] SETUP_ACCELA: cannot determine home dir: %v", err)
		return
	}

	if err := accelaInstallDotnet(c, home); err != nil {
		log.Printf("[Agent] SETUP_ACCELA: .NET install failed: %v", err)
		return
	}

	if err := accelaInstallAppImage(c, home); err != nil {
		log.Printf("[Agent] SETUP_ACCELA: ACCELA install failed: %v", err)
		return
	}

	if err := accelaWriteConfig(home); err != nil {
		log.Printf("[Agent] SETUP_ACCELA: config write failed: %v", err)
		return
	}

	// Install SLSsteam alongside ACCELA. Non-fatal — ACCELA still works without it.
	c.setupSLSSteam()

	log.Printf("[Agent] SETUP_ACCELA: done — ACCELA installed at %s/.local/share/ACCELA", home)
}

// accelaInstallDotnet downloads and runs the official .NET 9 install script.
// Installs to ~/.dotnet with no sudo required.
func accelaInstallDotnet(c *Client, home string) error {
	dotnetBin := filepath.Join(home, ".dotnet", "dotnet")
	if _, err := os.Stat(dotnetBin); err == nil {
		log.Printf("[Agent] SETUP_ACCELA: .NET already installed at %s", dotnetBin)
		return nil
	}

	log.Printf("[Agent] SETUP_ACCELA: downloading .NET 9 installer...")
	resp, err := c.http.Get("https://dot.net/v1/dotnet-install.sh")
	if err != nil {
		return fmt.Errorf("download dotnet-install.sh: %w", err)
	}
	defer resp.Body.Close()
	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read dotnet-install.sh: %w", err)
	}

	tmp, err := os.CreateTemp("", "dotnet-install-*.sh")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Write(script)
	tmp.Close()
	os.Chmod(tmp.Name(), 0o755)

	log.Printf("[Agent] SETUP_ACCELA: installing .NET 9 SDK...")
	cmd := exec.Command("bash", tmp.Name(), "--channel", "9.0", "--install-dir", filepath.Join(home, ".dotnet"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dotnet-install.sh: %w", err)
	}
	log.Printf("[Agent] SETUP_ACCELA: .NET 9 installed")
	return nil
}

// accelaInstallAppImage fetches the latest ACCELA AppImage from GitHub releases
// and extracts it to ~/.local/share/ACCELA.
func accelaInstallAppImage(c *Client, home string) error {
	installDir := filepath.Join(home, ".local", "share", "ACCELA")

	// Check if already installed
	if data, err := os.ReadFile(filepath.Join(installDir, ".version")); err == nil {
		log.Printf("[Agent] SETUP_ACCELA: ACCELA already installed (version %s)", strings.TrimSpace(string(data)))
		return nil
	}

	log.Printf("[Agent] SETUP_ACCELA: fetching latest ACCELA release info...")
	resp, err := c.http.Get("https://api.github.com/repos/ciscosweater/enter-the-wired/releases/latest")
	if err != nil {
		return fmt.Errorf("github API: %w", err)
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}

	var assetURL string
	for _, a := range release.Assets {
		if strings.HasSuffix(a.Name, "-linux-appimage.tar.gz") {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		return fmt.Errorf("no AppImage asset found in release %s", release.TagName)
	}

	log.Printf("[Agent] SETUP_ACCELA: downloading ACCELA %s...", release.TagName)
	dlResp, err := c.http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download AppImage: %w", err)
	}
	defer dlResp.Body.Close()

	tmp, err := os.CreateTemp("", "accela-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, dlResp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("save AppImage archive: %w", err)
	}
	tmp.Close()

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return err
	}

	log.Printf("[Agent] SETUP_ACCELA: extracting to %s...", installDir)
	cmd := exec.Command("tar", "-xzf", tmp.Name(), "-C", installDir, "--strip-components=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Mark installed version
	os.WriteFile(filepath.Join(installDir, ".version"), []byte(release.TagName), 0o644)
	log.Printf("[Agent] SETUP_ACCELA: ACCELA %s extracted", release.TagName)
	return nil
}

// accelaWriteConfig writes the ACCELA config file with CargoDeck-appropriate defaults.
// Existing keys are preserved; only missing keys are added.
func accelaWriteConfig(home string) error {
	cfgDir := filepath.Join(home, ".config", "Tachibana Labs")
	cfgPath := filepath.Join(cfgDir, "ACCELA.conf")

	defaults := [][2]string{
		{"auto_skip_single_choice", "true"},
		{"library_mode", "true"},
		{"max_downloads", "16"},
		{"sls_config_management", "true"},
		{"slssteam_mode", "true"},
		{"use_steamless", "true"},
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}

	existing, _ := os.ReadFile(cfgPath)
	content := string(existing)
	if !strings.Contains(content, "[General]") {
		content = "[General]\n" + content
	}
	for _, kv := range defaults {
		if !strings.Contains(content, kv[0]+"=") {
			content += kv[0] + "=" + kv[1] + "\n"
		}
	}

	return os.WriteFile(cfgPath, []byte(content), 0o644)
}

// setupSLSSteam installs SLSsteam on the agent machine.
// Downloads the latest SLSsteam-Any.7z from GitHub and extracts SLSsteam.so to
// ~/.local/share/SLSsteam/. Requires 7z or 7za to be available.
// SLSsteam is activated at game launch via LD_AUDIT in run.sh.
func (c *Client) setupSLSSteam() {
	log.Printf("[Agent] SETUP_SLSSTEAM: starting")

	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: cannot determine home dir: %v", err)
		return
	}

	installDir := filepath.Join(home, ".local", "share", "SLSsteam")
	soPath := filepath.Join(installDir, "SLSsteam.so")

	// Check installed version
	if data, err := os.ReadFile(filepath.Join(installDir, ".version")); err == nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: already installed (version %s)", strings.TrimSpace(string(data)))
		return
	}

	log.Printf("[Agent] SETUP_SLSSTEAM: fetching latest release info...")
	resp, err := c.http.Get("https://api.github.com/repos/AceSLS/SLSsteam/releases/latest")
	if err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: github API error: %v", err)
		return
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: decode release: %v", err)
		return
	}

	var assetURL string
	for _, a := range release.Assets {
		if strings.HasSuffix(a.Name, "-Any.7z") {
			assetURL = a.BrowserDownloadURL
			break
		}
	}
	if assetURL == "" {
		log.Printf("[Agent] SETUP_SLSSTEAM: no SLSsteam-Any.7z in release %s", release.TagName)
		return
	}

	log.Printf("[Agent] SETUP_SLSSTEAM: downloading SLSsteam %s...", release.TagName)
	dlResp, err := c.http.Get(assetURL)
	if err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: download error: %v", err)
		return
	}
	defer dlResp.Body.Close()

	tmp, err := os.CreateTemp("", "slssteam-*.7z")
	if err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: temp file: %v", err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, dlResp.Body); err != nil {
		tmp.Close()
		log.Printf("[Agent] SETUP_SLSSTEAM: save archive: %v", err)
		return
	}
	tmp.Close()

	// Extract to a temporary directory, then locate SLSsteam.so inside.
	tmpExtract, err := os.MkdirTemp("", "slssteam-extract-*")
	if err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: tempdir: %v", err)
		return
	}
	defer os.RemoveAll(tmpExtract)

	extracted := false
	for _, bin := range []string{"7z", "7za"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		log.Printf("[Agent] SETUP_SLSSTEAM: extracting with %s...", bin)
		cmd := exec.Command(bin, "x", tmp.Name(), "-o"+tmpExtract, "-y")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			extracted = true
			break
		}
	}
	if !extracted {
		log.Printf("[Agent] SETUP_SLSSTEAM: extraction failed — is 7z installed?")
		return
	}

	// Walk the extracted tree to find SLSsteam.so.
	var soSrc string
	_ = filepath.Walk(tmpExtract, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.EqualFold(filepath.Base(path), "SLSsteam.so") {
			soSrc = path
			return fmt.Errorf("found")
		}
		return nil
	})
	if soSrc == "" {
		log.Printf("[Agent] SETUP_SLSSTEAM: SLSsteam.so not found in archive")
		return
	}

	if err := os.MkdirAll(installDir, 0o755); err != nil {
		log.Printf("[Agent] SETUP_SLSSTEAM: mkdir: %v", err)
		return
	}

	// Move .so to install dir; fall back to copy if cross-device.
	if err := os.Rename(soSrc, soPath); err != nil {
		srcData, readErr := os.ReadFile(soSrc)
		if readErr != nil {
			log.Printf("[Agent] SETUP_SLSSTEAM: copy .so: %v", readErr)
			return
		}
		if writeErr := os.WriteFile(soPath, srcData, 0o755); writeErr != nil {
			log.Printf("[Agent] SETUP_SLSSTEAM: write .so: %v", writeErr)
			return
		}
	}

	os.WriteFile(filepath.Join(installDir, ".version"), []byte(release.TagName), 0o644)
	log.Printf("[Agent] SETUP_SLSSTEAM: installed SLSsteam %s at %s", release.TagName, soPath)
}

