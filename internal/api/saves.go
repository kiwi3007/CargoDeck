package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kiwi3007/playerr/internal/agent"
	"github.com/kiwi3007/playerr/internal/manifest"
)

// ---- GET /api/v3/save/paths ----
// Query: title=<game title>&os=<linux|windows|mac>&wineprefix=<optional>&agentId=<optional>
// Returns: []string of resolved save directory paths.
// Custom per-device paths are prepended to (not replacing) manifest/fallback paths.
// Used by the agent to discover where to look for saves.
func (h *Handler) GetSavePaths(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	targetOS := r.URL.Query().Get("os")
	wineprefix := r.URL.Query().Get("wineprefix")
	agentID := r.URL.Query().Get("agentId")
	// agentHome is the home directory on the agent's machine.
	// Path templates (e.g. <xdgData>/godot/...) must be resolved relative to the
	// agent's home, not the server's, so the agent sends it explicitly.
	agentHome := r.URL.Query().Get("home")
	if agentHome == "" {
		agentHome, _ = os.UserHomeDir()
	}

	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if targetOS == "" {
		targetOS = "linux"
	}

	game, _ := h.repo.GetGameByTitle(title)

	gameID := 0
	if game != nil {
		gameID = game.ID
	}

	// If the user specified a custom save path for this device, use it exclusively.
	// Don't mix with manifest/fallback — the user told us exactly where saves are.
	var customPath string
	if agentID != "" && game != nil {
		customPath, _ = h.repo.GetAgentSavePath(game.ID, agentID)
	}
	if customPath == "" && game != nil && game.SavePath != "" {
		customPath = game.SavePath
	}
	if customPath != "" {
		log.Printf("[Saves] GetSavePaths %q agent=%s: using custom path %s", title, agentID, customPath)
		jsonOK(w, map[string]any{"paths": []string{customPath}, "gameId": gameID})
		return
	}

	// No custom path — auto-detect: manifest → prefix-scan → fallback
	var basePaths []string
	if err := h.manifest.EnsureLoaded(); err != nil {
		log.Printf("[Saves] Manifest load error: %v", err)
	} else {
		if entry, found := h.manifest.FindByTitle(title); found {
			if paths := manifest.ResolvePaths(entry, targetOS, wineprefix, agentHome); len(paths) > 0 {
				basePaths = append(basePaths, paths...)
			}
		}
	}
	if len(basePaths) == 0 && wineprefix != "" {
		if found := scanPrefixForGame(wineprefix, title); len(found) > 0 {
			basePaths = append(basePaths, found...)
		}
	}
	if len(basePaths) == 0 {
		basePaths = fallbackSavePaths(title, agentHome, wineprefix)
	}
	log.Printf("[Saves] GetSavePaths %q agent=%s: auto-detected %v", title, agentID, basePaths)
	jsonOK(w, map[string]any{"paths": basePaths, "gameId": gameID})
}

// ---- POST /api/v3/save/snapshot ----
// Agent uploads a .tar.gz of save files.
// Query: agentId=<id>&title=<game title>
// Body: application/octet-stream (.tar.gz)
// Auth: Bearer token required.
func (h *Handler) UploadSaveSnapshot(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	title := r.URL.Query().Get("title")
	gameIDStr := r.URL.Query().Get("gameId")

	if agentID == "" || title == "" {
		http.Error(w, "agentId and title are required", http.StatusBadRequest)
		return
	}

	// Try to resolve game ID from library (by title)
	gameID := 0
	if gameIDStr != "" {
		gameID, _ = strconv.Atoi(gameIDStr)
	}
	if gameID == 0 {
		game, err := h.repo.GetGameByTitle(title)
		if err == nil && game != nil {
			gameID = game.ID
		}
	}
	if gameID == 0 {
		// Store under "unknown" game ID (0) — still useful
		gameID = 0
	}

	// Determine save storage directory
	savesDir := h.saveSnapshotDir(gameID, agentID)
	if err := os.MkdirAll(savesDir, 0755); err != nil {
		http.Error(w, "cannot create save dir", http.StatusInternalServerError)
		return
	}

	// Capture the current latest snapshot metadata before writing the new one (for conflict detection)
	existingPath, _, _ := h.findLatestSnapshotDir(gameID)
	prevLatestAgentID := ""
	if existingPath != "" {
		prevLatestAgentID = filepath.Base(filepath.Dir(existingPath))
	}

	// Extract tar.gz into a timestamped snapshot directory
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	snapshotDir := filepath.Join(savesDir, timestamp)
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		http.Error(w, "cannot create snapshot dir", http.StatusInternalServerError)
		return
	}

	if err := extractTarGz(r.Body, snapshotDir); err != nil {
		log.Printf("[Saves] Extract error for %q agent=%s: %v", title, agentID, err)
		_ = os.RemoveAll(snapshotDir)
		http.Error(w, "extract failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	size := dirSizeBytes(snapshotDir)
	log.Printf("[Saves] Snapshot saved: game=%d agent=%s title=%q ts=%s size=%d", gameID, agentID, title, timestamp, size)

	// Post-upload: conflict detection + auto-sync to other agents
	if gameID != 0 {
		go h.postSnapshotSync(gameID, title, agentID, prevLatestAgentID)
	}

	jsonOK(w, map[string]any{"ok": true, "timestamp": timestamp, "sizeBytes": size})
}

// ---- GET /api/v3/save/{gameId} ----
// Lists all save snapshots for a library game.
func (h *Handler) ListSaveSnapshots(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	if gameID == 0 {
		http.Error(w, "invalid gameId", http.StatusBadRequest)
		return
	}

	type Snapshot struct {
		ID        string    `json:"id"`
		AgentID   string    `json:"agentId"`
		Timestamp time.Time `json:"timestamp"`
		SizeBytes int64     `json:"sizeBytes"`
	}

	var snapshots []Snapshot

	// Walk: config/saves/{gameId}/{agentId}/{timestamp}/
	baseDir := h.saveBaseDir(gameID)
	agentEntries, err := os.ReadDir(baseDir)
	if err != nil {
		jsonOK(w, []Snapshot{})
		return
	}

	for _, agentEntry := range agentEntries {
		if !agentEntry.IsDir() {
			continue
		}
		agentID := agentEntry.Name()
		agentDir := filepath.Join(baseDir, agentID)

		tsEntries, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}
		for _, tsEntry := range tsEntries {
			if !tsEntry.IsDir() {
				continue
			}
			ts, err := time.Parse("2006-01-02T15-04-05Z", tsEntry.Name())
			if err != nil {
				continue
			}
			snapshotDir := filepath.Join(agentDir, tsEntry.Name())
			size := dirSizeBytes(snapshotDir)
			snapshots = append(snapshots, Snapshot{
				ID:        fmt.Sprintf("%s/%s", agentID, tsEntry.Name()),
				AgentID:   agentID,
				Timestamp: ts,
				SizeBytes: size,
			})
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	jsonOK(w, snapshots)
}

// ---- GET /api/v3/save/{gameId}/latest ----
// Streams the newest snapshot tar.gz across ALL agents for a game.
// The agent uses this to restore the latest save regardless of origin device.
func (h *Handler) ServeLatestSave(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	if gameID == 0 {
		http.Error(w, "invalid gameId", http.StatusBadRequest)
		return
	}

	path, ts, err := h.findLatestSnapshotDir(gameID)
	if err != nil || path == "" {
		http.Error(w, "no snapshots found", http.StatusNotFound)
		return
	}

	// Stream the snapshot dir as a fresh tar.gz
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Snapshot-Timestamp", ts.Format(time.RFC3339))
	w.Header().Set("Content-Disposition", "attachment; filename=save.tar.gz")

	gzw := gzip.NewWriter(w)
	tw := tar.NewWriter(gzw)

	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		_, _ = io.Copy(tw, f)
		return nil
	})
	_ = tw.Close()
	_ = gzw.Close()
}

// ---- GET /api/v3/save/{gameId}/latest-info ----
// Returns metadata about the newest snapshot: timestamp, agentId, sizeBytes.
func (h *Handler) GetLatestSaveInfo(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	if gameID == 0 {
		http.Error(w, "invalid gameId", http.StatusBadRequest)
		return
	}

	path, ts, err := h.findLatestSnapshotDir(gameID)
	if err != nil || path == "" {
		http.Error(w, "no snapshots found", http.StatusNotFound)
		return
	}

	// Extract agentId from path: saves/{gameId}/{agentId}/{timestamp}
	agentID := filepath.Base(filepath.Dir(path))
	jsonOK(w, map[string]any{
		"timestamp": ts,
		"agentId":   agentID,
		"sizeBytes": dirSizeBytes(path),
	})
}

// findLatestSnapshotDir walks all agent subdirs for a game and returns the most recent
// snapshot directory path and its timestamp. Returns ("", zero, nil) if none found.
func (h *Handler) findLatestSnapshotDir(gameID int) (string, time.Time, error) {
	baseDir := h.saveBaseDir(gameID)
	agentEntries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", time.Time{}, nil // no snapshots yet
	}

	var latestPath string
	var latestTS time.Time

	for _, agentEntry := range agentEntries {
		if !agentEntry.IsDir() {
			continue
		}
		agentDir := filepath.Join(baseDir, agentEntry.Name())
		tsEntries, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}
		for _, tsEntry := range tsEntries {
			if !tsEntry.IsDir() {
				continue
			}
			ts, err := time.Parse("2006-01-02T15-04-05Z", tsEntry.Name())
			if err != nil {
				continue
			}
			if ts.After(latestTS) {
				latestTS = ts
				latestPath = filepath.Join(agentDir, tsEntry.Name())
			}
		}
	}
	return latestPath, latestTS, nil
}

// ---- DELETE /api/v3/save/{gameId}/{snapshotId...} ----
// Deletes a specific snapshot. snapshotId is "{agentId}/{timestamp}".
func (h *Handler) DeleteSaveSnapshot(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	snapshotID := chi.URLParam(r, "*")

	if gameID == 0 || snapshotID == "" {
		http.Error(w, "invalid params", http.StatusBadRequest)
		return
	}

	snapshotDir := filepath.Join(h.saveBaseDir(gameID), filepath.FromSlash(snapshotID))

	// Security: ensure path is under saves dir
	savesBase := h.saveBaseDir(gameID)
	if !strings.HasPrefix(filepath.Clean(snapshotDir), filepath.Clean(savesBase)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	if err := os.RemoveAll(snapshotDir); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// ---- POST /api/v3/save/{gameId}/promote-snapshot ----
// Promotes the specified agent's latest snapshot to be the globally newest by copying it
// with a fresh timestamp, then dispatches RESTORE_SAVE to all online agents with the game.
// Used for conflict resolution when the user wants to keep a specific device's saves.
func (h *Handler) PromoteAgentSnapshot(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	sourceAgentID := r.URL.Query().Get("sourceAgentId")
	if gameID == 0 || sourceAgentID == "" {
		http.Error(w, "gameId and sourceAgentId are required", http.StatusBadRequest)
		return
	}

	// Find the source agent's latest snapshot directory
	sourceAgentDir := h.saveSnapshotDir(gameID, sourceAgentID)
	tsEntries, err := os.ReadDir(sourceAgentDir)
	if err != nil {
		http.Error(w, "no snapshots from source agent", http.StatusNotFound)
		return
	}
	var latestTS time.Time
	var latestEntry string
	for _, e := range tsEntries {
		if !e.IsDir() {
			continue
		}
		ts, err := time.Parse("2006-01-02T15-04-05Z", e.Name())
		if err != nil {
			continue
		}
		if ts.After(latestTS) {
			latestTS = ts
			latestEntry = e.Name()
		}
	}
	if latestEntry == "" {
		http.Error(w, "no snapshots from source agent", http.StatusNotFound)
		return
	}

	srcPath := filepath.Join(sourceAgentDir, latestEntry)
	newTS := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	dstPath := filepath.Join(sourceAgentDir, newTS)

	if err := copySnapshotDir(srcPath, dstPath); err != nil {
		http.Error(w, "copy failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	// Dispatch RESTORE_SAVE to all online agents that have this game
	restorePayload, _ := json.Marshal(map[string]any{"gameId": gameID, "title": game.Title})
	agents := h.agentRegistry.List()
	for _, a := range agents {
		if a.Status != agent.StatusOnline {
			continue
		}
		for _, ig := range a.InstalledGames {
			if ig.Title == game.Title {
				h.agentBroker.Send(a.ID, "RESTORE_SAVE", string(restorePayload))
				log.Printf("[Saves] Promote: dispatched RESTORE_SAVE %q → %s", game.Title, a.ID)
				break
			}
		}
	}

	log.Printf("[Saves] Promoted snapshot from agent=%s game=%d as %s", sourceAgentID, gameID, newTS)
	jsonOK(w, map[string]any{"ok": true, "newTimestamp": newTS})
}

// postSnapshotSync runs after a new snapshot is saved.
// If the previous latest was from a DIFFERENT agent, a conflict is emitted to the browser.
// Otherwise, RESTORE_SAVE is dispatched to all other online agents with the game installed.
func (h *Handler) postSnapshotSync(gameID int, title, uploaderID, prevLatestAgentID string) {
	if prevLatestAgentID != "" && prevLatestAgentID != uploaderID {
		// Conflict: the previous latest was from a different agent.
		// The uploader may have been playing with outdated saves.
		conflictData, _ := json.Marshal(map[string]any{
			"gameId":             gameID,
			"title":              title,
			"uploadingAgentId":   uploaderID,
			"conflictingAgentId": prevLatestAgentID,
		})
		h.broker.Publish("SAVE_CONFLICT", string(conflictData))
		log.Printf("[Saves] Conflict: %q uploaded by %s, but %s had previous latest — notifying browser", title, uploaderID, prevLatestAgentID)
		return
	}

	// No conflict — auto-restore to other online agents that have this game installed
	restorePayload, _ := json.Marshal(map[string]any{"gameId": gameID, "title": title})
	agents := h.agentRegistry.List()
	for _, a := range agents {
		if a.ID == uploaderID || a.Status != agent.StatusOnline {
			continue
		}
		for _, ig := range a.InstalledGames {
			if ig.Title == title {
				h.agentBroker.Send(a.ID, "RESTORE_SAVE", string(restorePayload))
				log.Printf("[Saves] Auto-restore: dispatched RESTORE_SAVE %q → %s", title, a.ID)
				break
			}
		}
	}
}

// ---- helpers ----

// scanPrefixForGame scans common Wine prefix user-data locations for directories
// whose name contains the game title. Returns existing directories only.
// This discovers saves for games not in the Ludusavi manifest.
func scanPrefixForGame(wineprefix, gameName string) []string {
	winUser := manifest.WinUserInPrefix(wineprefix)
	// Support both Proton-style ({prefix}/pfx/drive_c) and Wine-style ({prefix}/drive_c).
	driveC := filepath.Join(wineprefix, "pfx", "drive_c")
	if _, err := os.Stat(driveC); os.IsNotExist(err) {
		driveC = filepath.Join(wineprefix, "drive_c")
	}
	userDir := filepath.Join(driveC, "users", winUser)
	titleLower := strings.ToLower(gameName)

	// Common locations where Windows games store saves.
	// We scan the root of each location, not recursively.
	scanRoots := []string{
		filepath.Join(userDir, "Documents"),
		filepath.Join(userDir, "Documents", "My Games"),
		filepath.Join(userDir, "AppData", "Roaming"),
		filepath.Join(userDir, "AppData", "Local"),
		filepath.Join(userDir, "Saved Games"),
	}

	seenInodes := map[uint64]bool{} // avoid duplicates from symlinks
	var found []string

	addIfMatch := func(p string) {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			resolved = p
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return
		}
		inode := inoOf(info)
		if inode != 0 && seenInodes[inode] {
			return
		}
		if inode != 0 {
			seenInodes[inode] = true
		}
		found = append(found, p)
	}

	for _, root := range scanRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := strings.ToLower(e.Name())
			if strings.Contains(name, titleLower) {
				// Direct match: Saved Games/Cairn/
				addIfMatch(filepath.Join(root, e.Name()))
			} else {
				// One level deeper: publisher-namespaced saves like Saved Games/TheGameBakers/Cairn_RETAIL/
				subEntries, err := os.ReadDir(filepath.Join(root, e.Name()))
				if err != nil {
					continue
				}
				for _, se := range subEntries {
					if !se.IsDir() {
						continue
					}
					if strings.Contains(strings.ToLower(se.Name()), titleLower) {
						addIfMatch(filepath.Join(root, e.Name(), se.Name()))
					}
				}
			}
		}
	}
	return found
}

// fallbackSavePaths returns candidate save directories when neither manifest nor prefix scan
// yields results. Directories are created by the agent if they don't exist.
func fallbackSavePaths(gameName, home, wineprefix string) []string {
	lower := strings.ToLower(gameName)
	paths := []string{
		filepath.Join(home, ".config", gameName),
		filepath.Join(home, ".local", "share", gameName),
		filepath.Join(home, "."+lower),
	}
	if wineprefix != "" {
		winUser := manifest.WinUserInPrefix(wineprefix)
		userDir := filepath.Join(wineprefix, "pfx", "drive_c", "users", winUser)
		paths = append(paths,
			// Most common Wine save location — Documents/{Name}
			filepath.Join(userDir, "Documents", gameName),
			// Other common locations
			filepath.Join(userDir, "AppData", "Roaming", gameName),
			filepath.Join(userDir, "Documents", "My Games", gameName),
			filepath.Join(userDir, "Saved Games", gameName),
			filepath.Join(userDir, "AppData", "Local", gameName),
		)
	}
	return paths
}

func (h *Handler) saveBaseDir(gameID int) string {
	return filepath.Join(h.cfg.Dir(), "..", "saves", strconv.Itoa(gameID))
}

func (h *Handler) saveSnapshotDir(gameID int, agentID string) string {
	return filepath.Join(h.saveBaseDir(gameID), agentID)
}

func extractTarGz(r io.Reader, destDir string) error {
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

		// Sanitize path to prevent directory traversal
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			continue
		}

		destPath := filepath.Join(destDir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
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

// inoOf returns the inode number for a file, or 0 on non-Unix systems.
func inoOf(fi os.FileInfo) uint64 {
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		return sys.Ino
	}
	return 0
}

func dirSizeBytes(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// copySnapshotDir deep-copies a snapshot directory tree to a new destination.
func copySnapshotDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// dedupePaths returns paths with empty strings removed and duplicates eliminated,
// preserving order.
func dedupePaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

// prependIfNonEmpty prepends p to paths if p is non-empty.
func prependIfNonEmpty(p string, paths []string) []string {
	if p == "" {
		return paths
	}
	result := make([]string, 0, 1+len(paths))
	result = append(result, p)
	result = append(result, paths...)
	return result
}

// GetSavePathsInfo returns a JSON summary of save path resolution for a game.
// GET /api/v3/save/paths-info?gameId=<id>&os=linux&wineprefix=<optional>&agentId=<optional>
// Response: { source, paths (merged), customPaths (user-added only), agentPaths (map), steamId }
func (h *Handler) GetSavePathsInfo(w http.ResponseWriter, r *http.Request) {
	gameIDStr := r.URL.Query().Get("gameId")
	targetOS := r.URL.Query().Get("os")
	wineprefix := r.URL.Query().Get("wineprefix")

	if gameIDStr == "" {
		http.Error(w, "gameId required", http.StatusBadRequest)
		return
	}
	if targetOS == "" {
		targetOS = "linux"
	}

	gameID, _ := strconv.Atoi(gameIDStr)
	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		http.Error(w, "game not found", http.StatusNotFound)
		return
	}

	// Per-device overrides map (agentId → path)
	agentPaths, _ := h.repo.GetAllAgentSavePaths(gameID)

	// Collect base paths from manifest / prefix-scan / fallback
	var basePaths []string
	source := "none"
	steamID := 0

	_ = h.manifest.EnsureLoaded()
	if entry, found := h.manifest.FindByTitle(game.Title); found {
		if paths := manifest.ResolvePaths(entry, targetOS, wineprefix, ""); len(paths) > 0 {
			basePaths = paths
			source = "manifest"
			if entry.Steam != nil {
				steamID = entry.Steam.ID
			}
		}
	}
	if len(basePaths) == 0 && wineprefix != "" {
		if found := scanPrefixForGame(wineprefix, game.Title); len(found) > 0 {
			basePaths = found
			source = "prefix-scan"
		}
	}
	if len(basePaths) == 0 {
		home, _ := os.UserHomeDir()
		if fb := fallbackSavePaths(game.Title, home, wineprefix); len(fb) > 0 {
			basePaths = fb
			source = "fallback"
		}
	}

	// Collect all per-agent custom paths (deduplicated, for remove-button display)
	seenCustom := map[string]bool{}
	var customPaths []string
	for _, p := range agentPaths {
		if p != "" && !seenCustom[p] {
			seenCustom[p] = true
			customPaths = append(customPaths, p)
		}
	}
	// Include legacy global path as a custom path
	if game.SavePath != "" && !seenCustom[game.SavePath] {
		customPaths = append(customPaths, game.SavePath)
	}

	// Merged list: custom paths first, then detected base paths, deduped
	merged := dedupePaths(append(customPaths, basePaths...))

	resp := map[string]any{
		"source":      source,
		"paths":       merged,
		"customPaths": customPaths,
		"agentPaths":  agentPaths,
	}
	if steamID != 0 {
		resp["steamId"] = steamID
	}
	jsonOK(w, resp)
}

// SetSavePath sets or clears the custom save path for a game.
// PATCH /api/v3/save/{gameId}/path
// Body: {"savePath": "/absolute/path", "agentId": "optional"}
// - If agentId provided: sets per-device override for that agent.
// - If agentId omitted: sets global per-game fallback path.
// Empty savePath clears the override.
func (h *Handler) SetSavePath(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	if gameID == 0 {
		http.Error(w, "invalid gameId", http.StatusBadRequest)
		return
	}

	var body struct {
		SavePath string `json:"savePath"`
		AgentID  string `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if body.AgentID != "" {
		// Per-device override
		if err := h.repo.SetAgentSavePath(gameID, body.AgentID, body.SavePath); err != nil {
			http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Trigger the agent to re-scan so it picks up the new custom path immediately
		if agentInfo, ok := h.agentRegistry.Get(body.AgentID); ok && agentInfo.Status == agent.StatusOnline {
			h.agentBroker.Send(body.AgentID, "SCAN_GAMES", "{}")
			log.Printf("[Saves] Triggered SCAN_GAMES on %s after custom path update for game %d", body.AgentID, gameID)
		}
	} else {
		// Global per-game path (legacy / server-local games)
		if err := h.repo.UpdateGameSavePath(gameID, body.SavePath); err != nil {
			http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	jsonOK(w, map[string]any{"ok": true, "savePath": body.SavePath, "agentId": body.AgentID})
}

// GetAgentSavePaths returns all per-device save path overrides for a game.
// GET /api/v3/save/{gameId}/agent-paths
func (h *Handler) GetAgentSavePaths(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameId")
	gameID, _ := strconv.Atoi(gameIDStr)
	if gameID == 0 {
		http.Error(w, "invalid gameId", http.StatusBadRequest)
		return
	}
	paths, err := h.repo.GetAllAgentSavePaths(gameID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, paths)
}
