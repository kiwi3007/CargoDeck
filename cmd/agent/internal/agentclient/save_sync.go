package agentclient

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// restoreLatestSave downloads the latest save snapshot from the server and
// extracts it to the game's local save directories.
func (c *Client) restoreLatestSave(gameID int, title string) {
	if c.saveWatcher == nil {
		log.Printf("[Saves] Restore: save watcher not running")
		return
	}

	c.saveWatcher.mu.Lock()
	game := c.saveWatcher.games[safeName(title)]
	c.saveWatcher.mu.Unlock()

	if game == nil || len(game.saveDirs) == 0 {
		log.Printf("[Saves] Restore: no watched save dirs for %q — skipping", title)
		return
	}

	log.Printf("[Saves] Restore: fetching latest snapshot for %q (game %d)...", title, gameID)

	fetchURL := fmt.Sprintf("%s/api/v3/save/%d/latest", c.cfg.ServerURL, gameID)
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		log.Printf("[Saves] Restore request error: %v", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	dlClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := dlClient.Do(req)
	if err != nil {
		log.Printf("[Saves] Restore download failed for %q: %v", title, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		log.Printf("[Saves] Restore: no snapshots found for %q", title)
		return
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		log.Printf("[Saves] Restore HTTP %d for %q: %s", resp.StatusCode, title, strings.TrimSpace(string(b)))
		return
	}

	// Extract tar.gz into a temp directory
	tmpDir, err := os.MkdirTemp("", "playerr-restore-*")
	if err != nil {
		log.Printf("[Saves] Restore: cannot create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGzRestoreDir(resp.Body, tmpDir); err != nil {
		log.Printf("[Saves] Restore: extract failed for %q: %v", title, err)
		return
	}

	// Snapshot top-level entries (subdirs and/or files at root).
	topEntries, _ := os.ReadDir(tmpDir)
	var topDirs []string
	hasRootFiles := false
	for _, e := range topEntries {
		if e.IsDir() {
			topDirs = append(topDirs, e.Name())
		} else {
			hasRootFiles = true
		}
	}

	restored := 0
	for _, saveDir := range game.saveDirs {
		if err := os.MkdirAll(saveDir, 0755); err != nil {
			log.Printf("[Saves] Restore: cannot create %s: %v", saveDir, err)
			continue
		}

		base := filepath.Base(saveDir)

		// 1. Exact dir name match (normal case — same path structure on both devices)
		if srcDir := filepath.Join(tmpDir, base); dirExists(srcDir) {
			copyDir(srcDir, saveDir)
			restored++
			log.Printf("[Saves] Restored %q → %s (exact match)", base, saveDir)
			continue
		}

		// 2. Partial match — snapshot has a different top-level dir name.
		//    Use the first available subdir in the snapshot. Handles cross-prefix restores
		//    where the install is on a different user account or machine.
		if len(topDirs) > 0 {
			srcDir := filepath.Join(tmpDir, topDirs[0])
			copyDir(srcDir, saveDir)
			restored++
			log.Printf("[Saves] Restored %q → %s (partial match from %q)", topDirs[0], saveDir, topDirs[0])
			continue
		}

		// 3. Files directly at snapshot root (no wrapping dir) — copy straight in.
		if hasRootFiles {
			copyDir(tmpDir, saveDir)
			restored++
			log.Printf("[Saves] Restored snapshot root → %s", saveDir)
		}
	}

	if restored > 0 {
		log.Printf("[Saves] Restore complete for %q (%d save dir(s))", title, restored)
	} else {
		log.Printf("[Saves] Restore: snapshot was empty or no save dirs configured for %q", title)
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// extractTarGzRestoreDir extracts a tar.gz reader into destDir.
// Sanitizes paths to prevent directory traversal.
func extractTarGzRestoreDir(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			continue
		}

		destPath := filepath.Join(destDir, cleanName)
		switch hdr.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(destPath, 0755)
		case tar.TypeReg, 0: // TypeReg or legacy
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

// saveSyncDebounce is how long to wait after the last file change before uploading.
const saveSyncDebounce = 30 * time.Second

// exitMarkerName is written by run.sh after the game process exits,
// triggering an immediate save upload.
const exitMarkerName = ".playerr-game-exited"

// SaveWatcher monitors game save directories using inotify (on Linux) and
// uploads snapshots when saves change or when the game exits.
type SaveWatcher struct {
	client  *Client
	watcher *fsnotify.Watcher

	mu        sync.Mutex
	games     map[string]*watchedSaveGame // safeName → game
	timers    map[string]*time.Timer      // safeName → debounce timer
	uploading map[string]bool             // safeName → in-flight upload
}

type watchedSaveGame struct {
	title     string
	gameID    int
	scriptDir string   // ~/Games/{safeName}/ — watched for exit marker
	saveDirs  []string // resolved from Ludusavi manifest
}

// newSaveWatcher creates a SaveWatcher backed by an inotify/kqueue/FSEvents watcher.
// Returns nil if the platform doesn't support it.
func newSaveWatcher(c *Client) *SaveWatcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Saves] fsnotify unavailable: %v", err)
		return nil
	}
	return &SaveWatcher{
		client:    c,
		watcher:   w,
		games:     make(map[string]*watchedSaveGame),
		timers:    make(map[string]*time.Timer),
		uploading: make(map[string]bool),
	}
}

// Run processes fsnotify events. Call as a goroutine.
func (sw *SaveWatcher) Run() {
	for {
		select {
		case event, ok := <-sw.watcher.Events:
			if !ok {
				return
			}
			sw.handleEvent(event)
		case err, ok := <-sw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[Saves] watcher error: %v", err)
		}
	}
}

// Close shuts down the underlying watcher.
func (sw *SaveWatcher) Close() {
	sw.watcher.Close()
}

// UpdateGames refreshes the watched game list.
// For each game title, it fetches save paths from the server, then adds inotify watches.
// scriptDirs maps safeName → ~/Games/{safeName}/ (for exit-marker detection).
func (sw *SaveWatcher) UpdateGames(titles []string, scriptDirs map[string]string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Remove watches for games no longer installed
	stillPresent := make(map[string]bool)
	for _, t := range titles {
		stillPresent[safeName(t)] = true
	}
	for sn, g := range sw.games {
		if !stillPresent[sn] {
			sw.unwatchLocked(g)
			delete(sw.games, sn)
		}
	}

	// Refresh watches for all installed games.
	// Always re-fetch save paths so that custom paths added via the UI are picked up
	// without requiring a full agent restart.
	for _, title := range titles {
		sn := safeName(title)
		scriptDir := scriptDirs[sn]

		// Fetch latest save paths from server (releases the lock during HTTP call)
		sw.mu.Unlock()
		saveDirs, gameID := sw.client.fetchSavePaths(title, scriptDir)
		sw.mu.Lock()

		existing := sw.games[sn]
		game := &watchedSaveGame{title: title, gameID: gameID, scriptDir: scriptDir, saveDirs: saveDirs}
		sw.games[sn] = game

		// Remove watches for directories that are no longer in the list
		if existing != nil {
			for _, old := range existing.saveDirs {
				if !containsStr(saveDirs, old) {
					_ = sw.watcher.Remove(old)
				}
			}
		}

		// Watch script dir for the exit marker (idempotent)
		if scriptDir != "" {
			_ = os.MkdirAll(scriptDir, 0755)
			if err := sw.watcher.Add(scriptDir); err != nil {
				log.Printf("[Saves] Cannot watch script dir %s: %v", scriptDir, err)
			}
		}

		// Watch each save directory recursively. Unlock during the walk: filepath.Walk
		// can be slow for large trees (e.g. Wine prefixes) and we don't access sw.*
		// fields inside addWatchRecursive — only watcher.Add which is thread-safe.
		sw.mu.Unlock()
		for _, dir := range saveDirs {
			_ = os.MkdirAll(dir, 0755)
			sw.addWatchRecursive(dir)
		}
		sw.mu.Lock()

		if existing == nil && len(saveDirs) > 0 {
			log.Printf("[Saves] Watching %q: %v", title, saveDirs)
			// Newly discovered game: restore latest save from server if local dirs are empty.
			// This ensures saves are present before the first launch.
			if gameID > 0 {
				go sw.client.tryPreRestoreSave(gameID, title)
			}
		} else if existing != nil && !equalStringSlices(existing.saveDirs, saveDirs) {
			log.Printf("[Saves] Updated watches for %q: %v", title, saveDirs)
		}
	}
}

func (sw *SaveWatcher) unwatchLocked(g *watchedSaveGame) {
	if g.scriptDir != "" {
		_ = sw.watcher.Remove(g.scriptDir)
	}
	for _, dir := range g.saveDirs {
		_ = sw.watcher.Remove(dir)
	}
}

// addWatchRecursive adds a watch for dir and all existing subdirectories.
// fsnotify does not recurse automatically, so saves nested in subdirectories
// would be missed without this.
func (sw *SaveWatcher) addWatchRecursive(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if err := sw.watcher.Add(path); err != nil {
			log.Printf("[Saves] Cannot watch %s: %v", path, err)
		}
		return nil
	})
}

// handleEvent processes an inotify event.
func (sw *SaveWatcher) handleEvent(event fsnotify.Event) {
	if event.Op == fsnotify.Remove || event.Op == fsnotify.Rename {
		return
	}

	// If a new directory appeared, add it to the watcher immediately so that
	// save files written inside it are captured (fsnotify is non-recursive).
	if event.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
			_ = sw.watcher.Add(event.Name)
		}
	}

	sw.mu.Lock()
	defer sw.mu.Unlock()

	base := filepath.Base(event.Name)

	for sn, game := range sw.games {
		// Exit marker written by run.sh → immediate upload
		if base == exitMarkerName && game.scriptDir != "" &&
			strings.HasPrefix(filepath.Clean(event.Name), filepath.Clean(game.scriptDir)) {
			log.Printf("[Saves] Game exited: %q — uploading now", game.title)
			_ = os.Remove(event.Name)
			sw.scheduleUploadLocked(sn, 0)
			return
		}

		// File changed in a save directory → debounced upload
		for _, dir := range game.saveDirs {
			if strings.HasPrefix(filepath.Clean(event.Name), filepath.Clean(dir)) {
				sw.scheduleUploadLocked(sn, saveSyncDebounce)
				return
			}
		}
	}
}

// TriggerUpload schedules an immediate save upload for the named game.
// Used when the server sends an UPLOAD_SAVE event.
// Always fetches save paths fresh from the server so a manual upload works
// even if the cached save dirs are stale or empty.
func (sw *SaveWatcher) TriggerUpload(title string) {
	sn := safeName(title)

	// Fetch fresh save paths regardless of cache state.
	// Use the cached scriptDir if available so wineprefix detection still works.
	sw.mu.Lock()
	existing := sw.games[sn]
	scriptDir := ""
	if existing != nil {
		scriptDir = existing.scriptDir
	}
	sw.mu.Unlock()

	saveDirs, gameID := sw.client.fetchSavePaths(title, scriptDir)
	log.Printf("[Saves] TriggerUpload %q: gameID=%d saveDirs=%v", title, gameID, saveDirs)

	if len(saveDirs) == 0 {
		log.Printf("[Saves] TriggerUpload: no save dirs for %q — cannot upload", title)
		return
	}

	// Update the cached entry so the file watcher is also consistent.
	sw.mu.Lock()
	if existing != nil {
		sw.games[sn] = &watchedSaveGame{
			title:     existing.title,
			gameID:    gameID,
			scriptDir: existing.scriptDir,
			saveDirs:  saveDirs,
		}
	} else {
		sw.games[sn] = &watchedSaveGame{
			title:    title,
			gameID:   gameID,
			scriptDir: scriptDir,
			saveDirs: saveDirs,
		}
	}
	sw.scheduleUploadLocked(sn, 0)
	sw.mu.Unlock()
}

// scheduleUploadLocked resets the debounce timer. delay=0 fires immediately.
// Must be called with sw.mu held.
func (sw *SaveWatcher) scheduleUploadLocked(sn string, delay time.Duration) {
	if t, ok := sw.timers[sn]; ok {
		t.Stop()
	}
	game := sw.games[sn]
	if game == nil {
		return
	}
	// If an upload is in progress, back off and retry
	if sw.uploading[sn] {
		delay = 10 * time.Second
	}
	sw.timers[sn] = time.AfterFunc(delay, func() {
		sw.mu.Lock()
		if sw.uploading[sn] {
			sw.mu.Unlock()
			return
		}
		sw.uploading[sn] = true
		g := sw.games[sn]
		sw.mu.Unlock()

		if g != nil {
			sw.client.uploadSaves(g)
		}

		sw.mu.Lock()
		sw.uploading[sn] = false
		sw.mu.Unlock()
	})
}

// ---- fetchSavePaths asks the server for save directories for a game ----

func (c *Client) fetchSavePaths(title, scriptDir string) ([]string, int) {
	targetOS := runtime.GOOS
	if targetOS == "darwin" {
		targetOS = "mac"
	}

	// Detect wine prefix from run.sh so we can also resolve Windows save paths
	wineprefix := ""
	if scriptDir != "" {
		wineprefix = detectWineprefix(filepath.Join(scriptDir, "run.sh"))
	}

	q := url.Values{}
	q.Set("title", title)
	q.Set("os", targetOS)
	q.Set("agentId", c.agentID)
	q.Set("home", homeDir()) // agent's home dir — server resolves path templates relative to this
	if wineprefix != "" {
		q.Set("wineprefix", wineprefix)
	}

	reqURL := c.cfg.ServerURL + "/api/v3/save/paths?" + q.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, 0
	}

	var result struct {
		Paths  []string `json:"paths"`
		GameID int      `json:"gameId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0
	}
	return result.Paths, result.GameID
}

// tryPreRestoreSave restores the latest server snapshot if — and only if —
// the game's local save directories are currently empty. This runs after a game is first
// discovered (e.g. just installed) so that existing saves are present before launch.
func (c *Client) tryPreRestoreSave(gameID int, title string) {
	c.saveWatcher.mu.Lock()
	game := c.saveWatcher.games[safeName(title)]
	var saveDirs []string
	if game != nil {
		saveDirs = game.saveDirs
	}
	c.saveWatcher.mu.Unlock()

	for _, dir := range saveDirs {
		if dirHasFiles(dir) {
			log.Printf("[Saves] Pre-restore skipped for %q: existing local saves found in %s", title, dir)
			return
		}
	}
	log.Printf("[Saves] Pre-restore: checking server for saves for %q (game %d)...", title, gameID)
	c.restoreLatestSave(gameID, title)
}

// dirHasFiles reports whether dir contains any files (recursively).
func dirHasFiles(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// detectWineprefix parses WINEPREFIX or STEAM_COMPAT_DATA_PATH from a run.sh.
func detectWineprefix(scriptPath string) string {
	f, err := os.Open(scriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	re := regexp.MustCompile(`(?:export\s+)?(?:WINEPREFIX|STEAM_COMPAT_DATA_PATH)=(.+)`)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := re.FindStringSubmatch(line); len(m) == 2 {
			val := strings.Trim(m[1], `"'`)
			if strings.HasPrefix(val, "~/") {
				val = filepath.Join(homeDir(), val[2:])
			}
			if val != "" {
				return val
			}
		}
	}
	return ""
}

// ---- uploadSaves creates a tar.gz of all save directories and POSTs it ----

func (c *Client) uploadSaves(g *watchedSaveGame) {
	if len(g.saveDirs) == 0 {
		log.Printf("[Saves] Upload skipped for %q: no save dirs configured", g.title)
		return
	}

	log.Printf("[Saves] Building snapshot for %q: %v", g.title, g.saveDirs)

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	totalFiles := 0
	for _, dir := range g.saveDirs {
		n, err := tarDir(tw, dir)
		if err != nil {
			log.Printf("[Saves] tarDir %s: %v", dir, err)
		}
		totalFiles += n
	}

	_ = tw.Close()
	_ = gzw.Close()

	if totalFiles == 0 {
		log.Printf("[Saves] No save files found for %q — skipping", g.title)
		return
	}

	// POST tar.gz to server
	q := url.Values{}
	q.Set("agentId", c.agentID)
	q.Set("title", g.title)
	if g.gameID > 0 {
		// Send the library game ID directly so the server doesn't have to match
		// by title — avoids snapshots landing under gameId=0 if titles differ.
		q.Set("gameId", strconv.Itoa(g.gameID))
	}
	uploadURL := fmt.Sprintf("%s/api/v3/save/snapshot?%s", c.cfg.ServerURL, q.Encode())

	req, err := http.NewRequest("POST", uploadURL, bytes.NewReader(buf.Bytes()))
	if err != nil {
		log.Printf("[Saves] Upload request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	uploadClient := &http.Client{Timeout: 10 * time.Minute}
	resp, err := uploadClient.Do(req)
	if err != nil {
		log.Printf("[Saves] Upload failed for %q: %v", g.title, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		log.Printf("[Saves] Snapshot uploaded for %q (%d files, %d bytes)", g.title, totalFiles, buf.Len())
	} else {
		b, _ := io.ReadAll(resp.Body)
		log.Printf("[Saves] Upload HTTP %d for %q: %s", resp.StatusCode, g.title, strings.TrimSpace(string(b)))
	}
}

// containsStr reports whether s is in slice.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// equalStringSlices reports whether a and b have the same elements in the same order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// tarDir archives the full directory tree under dir into tw, including empty
// subdirectories. Paths in the archive are relative to dir's parent, prefixed
// with dir's basename. Returns the number of files (not dirs) archived.
func tarDir(tw *tar.Writer, dir string) (int, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	baseDir := filepath.Dir(absDir)
	count := 0

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		hdr.Name = filepath.ToSlash(rel)

		if info.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}

		// Skip files > 256 MB (not saves)
		if info.Size() > 256*1024*1024 {
			log.Printf("[Saves] Skipping oversized file: %s (%d MB)", path, info.Size()>>20)
			return nil
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}
