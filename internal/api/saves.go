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
	"github.com/kiwi3007/playerr/internal/manifest"
)

// ---- GET /api/v3/save/paths ----
// Query: title=<game title>&os=<linux|windows|mac>&wineprefix=<optional>&agentId=<optional>
// Returns: []string of resolved save directory paths.
// Priority: per-device override > global game override > manifest > prefix scan > fallback.
// Used by the agent to discover where to look for saves.
func (h *Handler) GetSavePaths(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	targetOS := r.URL.Query().Get("os")
	wineprefix := r.URL.Query().Get("wineprefix")
	agentID := r.URL.Query().Get("agentId")

	if title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if targetOS == "" {
		targetOS = "linux"
	}

	game, _ := h.repo.GetGameByTitle(title)

	// 1. Per-device (per-agent) override
	if agentID != "" && game != nil {
		if path, _ := h.repo.GetAgentSavePath(game.ID, agentID); path != "" {
			jsonOK(w, []string{path})
			return
		}
	}

	// 2. Global per-game custom save path (legacy / server-local games)
	if game != nil && game.SavePath != "" {
		jsonOK(w, []string{game.SavePath})
		return
	}

	// 3. Manifest lookup
	if err := h.manifest.EnsureLoaded(); err != nil {
		log.Printf("[Saves] Manifest load error: %v", err)
	} else {
		if entry, found := h.manifest.FindByTitle(title); found {
			if paths := manifest.ResolvePaths(entry, targetOS, wineprefix); len(paths) > 0 {
				jsonOK(w, paths)
				return
			}
		}
	}

	// 4. Scan the wineprefix for existing directories matching the game title.
	//    This catches games not in the manifest that have already been launched.
	if wineprefix != "" {
		if found := scanPrefixForGame(wineprefix, title); len(found) > 0 {
			jsonOK(w, found)
			return
		}
	}

	// 5. Fallback candidate paths
	home, _ := os.UserHomeDir()
	jsonOK(w, fallbackSavePaths(title, home, wineprefix))
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

// ---- helpers ----

// scanPrefixForGame scans common Wine prefix user-data locations for directories
// whose name contains the game title. Returns existing directories only.
// This discovers saves for games not in the Ludusavi manifest.
func scanPrefixForGame(wineprefix, gameName string) []string {
	winUser := manifest.WinUserInPrefix(wineprefix)
	userDir := filepath.Join(wineprefix, "pfx", "drive_c", "users", winUser)
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
	for _, root := range scanRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(e.Name()), titleLower) {
				continue
			}
			p := filepath.Join(root, e.Name())
			// Resolve symlinks to deduplicate (e.g. My Documents → Documents)
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				resolved = p
			}
			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			inode := inoOf(info)
			if inode != 0 && seenInodes[inode] {
				continue
			}
			if inode != 0 {
				seenInodes[inode] = true
			}
			found = append(found, p) // return the original (non-resolved) path for display
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

// GetSavePathsInfo returns a JSON summary of save path resolution for a game.
// GET /api/v3/save/paths-info?gameId=<id>&os=linux&wineprefix=<optional>&agentId=<optional>
// Includes: paths, source, agentPaths (per-device overrides), steamId.
func (h *Handler) GetSavePathsInfo(w http.ResponseWriter, r *http.Request) {
	gameIDStr := r.URL.Query().Get("gameId")
	targetOS := r.URL.Query().Get("os")
	wineprefix := r.URL.Query().Get("wineprefix")
	agentID := r.URL.Query().Get("agentId")

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

	// Always include per-device overrides in the response
	agentPaths, _ := h.repo.GetAllAgentSavePaths(gameID)

	// 1. Per-device override for this specific agent
	if agentID != "" {
		if path, _ := h.repo.GetAgentSavePath(gameID, agentID); path != "" {
			jsonOK(w, map[string]any{
				"source":     "agent-custom",
				"paths":      []string{path},
				"savePath":   path,
				"agentPaths": agentPaths,
			})
			return
		}
	}

	// 2. Global per-game custom save path
	if game.SavePath != "" {
		jsonOK(w, map[string]any{
			"source":     "custom",
			"paths":      []string{game.SavePath},
			"savePath":   game.SavePath,
			"agentPaths": agentPaths,
		})
		return
	}

	// 3. Manifest lookup
	_ = h.manifest.EnsureLoaded()
	entry, found := h.manifest.FindByTitle(game.Title)
	if found {
		paths := manifest.ResolvePaths(entry, targetOS, wineprefix)
		if len(paths) > 0 {
			steamID := 0
			if entry.Steam != nil {
				steamID = entry.Steam.ID
			}
			jsonOK(w, map[string]any{
				"source":     "manifest",
				"paths":      paths,
				"steamId":    steamID,
				"agentPaths": agentPaths,
			})
			return
		}
	}

	// 4. Prefix scan
	if wineprefix != "" {
		if found := scanPrefixForGame(wineprefix, game.Title); len(found) > 0 {
			jsonOK(w, map[string]any{
				"source":     "prefix-scan",
				"paths":      found,
				"agentPaths": agentPaths,
			})
			return
		}
	}

	// 5. Fallback
	home, _ := os.UserHomeDir()
	fallback := fallbackSavePaths(game.Title, home, wineprefix)
	if len(fallback) > 0 {
		jsonOK(w, map[string]any{
			"source":     "fallback",
			"paths":      fallback,
			"agentPaths": agentPaths,
		})
		return
	}

	jsonOK(w, map[string]any{
		"source":     "none",
		"paths":      []string{},
		"agentPaths": agentPaths,
	})
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
