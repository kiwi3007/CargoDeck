package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/metadata/igdb"
	"github.com/kiwi3007/playerr/internal/repository"
)

func (h *Handler) GetAllGames(w http.ResponseWriter, r *http.Request) {
	games, err := h.repo.GetAllGames()
	if err != nil {
		log.Printf("[API] GetAllGames error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, games)
}

func (h *Handler) GetGameByID(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	game, err := h.repo.GetGameByID(id)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	// Compute derived fields
	isInstallable := isPathInstallable(game.Path)
	downloadPath := h.findDownloadPath(game)

	jsonOK(w, map[string]any{
		"id":                 game.ID,
		"title":              game.Title,
		"alternativeTitle":   game.AlternativeTitle,
		"year":               game.Year,
		"overview":           game.Overview,
		"storyline":          game.Storyline,
		"platformId":         game.PlatformID,
		"platform":           game.Platform,
		"added":              game.Added,
		"images":             game.Images,
		"genres":             game.Genres,
		"availablePlatforms": game.AvailablePlatforms,
		"developer":          game.Developer,
		"publisher":          game.Publisher,
		"releaseDate":        game.ReleaseDate,
		"rating":             game.Rating,
		"ratingCount":        game.RatingCount,
		"status":             game.Status,
		"monitored":          game.Monitored,
		"path":               game.Path,
		"sizeOnDisk":         game.SizeOnDisk,
		"igdbId":             game.IgdbID,
		"steamId":            game.SteamID,
		"gogId":              game.GogID,
		"isInstallable":      isInstallable,
		"isExternal":         game.IsExternal,
		"gameFiles":          game.GameFiles,
		"downloadPath":       downloadPath,
	})
}

func (h *Handler) CreateGame(w http.ResponseWriter, r *http.Request) {
	var game domain.Game
	if err := decodeBody(r, &game); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	created, err := h.repo.CreateGame(&game)
	if err != nil {
		log.Printf("[API] CreateGame error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v3/game/%d", created.ID))
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, created)
}

func (h *Handler) UpdateGame(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	existing, err := h.repo.GetGameByID(id)
	if err != nil || existing == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	var update domain.Game
	if err := decodeBody(r, &update); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	// Apply partial update — only non-zero/non-nil fields
	if update.Title != "" {
		existing.Title = update.Title
	}
	if update.IgdbID != nil {
		existing.IgdbID = update.IgdbID
		// Fetch full metadata from IGDB and apply it
		igdbCfg := h.cfg.LoadIgdb()
		if igdbCfg.IsConfigured() {
			client := igdb.NewClient(igdbCfg.ClientId, igdbCfg.ClientSecret)
			if games, err := client.GetGamesByIds([]int{*update.IgdbID}); err == nil && len(games) > 0 {
				g := games[0]
				if g.Summary != "" {
					existing.Overview = &g.Summary
				}
				if g.Cover != nil {
					cu := igdb.ImageURL(g.Cover.ImageID, igdb.SizeCoverBig)
					cl := igdb.ImageURL(g.Cover.ImageID, igdb.SizeHD)
					existing.Images.CoverUrl = &cu
					existing.Images.CoverLargeUrl = &cl
				}
				if g.FirstReleaseDate != nil {
					existing.Year = time.Unix(*g.FirstReleaseDate, 0).UTC().Year()
				}
				existing.Genres = nil
				for _, genre := range g.Genres {
					existing.Genres = append(existing.Genres, genre.Name)
				}
				var screenshots, artworks []string
				for _, s := range g.Screenshots {
					screenshots = append(screenshots, igdb.ImageURL(s.ImageID, igdb.SizeScreenshotHuge))
				}
				for _, a := range g.Artworks {
					artworks = append(artworks, igdb.ImageURL(a.ImageID, igdb.SizeHD))
				}
				existing.Images.Screenshots = screenshots
				existing.Images.Artworks = artworks
				if len(artworks) > 0 {
					existing.Images.BackgroundUrl = &artworks[0]
				} else if len(screenshots) > 0 {
					existing.Images.BackgroundUrl = &screenshots[0]
				}
			}
		}
	}
	if update.Path != nil && *update.Path != "" {
		existing.Path = update.Path
	}
	if update.Status != 0 {
		existing.Status = update.Status
	}
	if update.Overview != nil {
		existing.Overview = update.Overview
	}
	if update.Year != 0 {
		existing.Year = update.Year
	}
	if update.Rating != nil {
		existing.Rating = update.Rating
	}
	if len(update.Genres) > 0 {
		existing.Genres = update.Genres
	}
	if update.Images.CoverUrl != nil {
		existing.Images = update.Images
	}
	if update.SteamID != nil {
		existing.SteamID = update.SteamID
	}
	if update.PlatformID != 0 {
		existing.PlatformID = update.PlatformID
	}

	updated, err := h.repo.UpdateGame(id, existing)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, updated)
}

// GetAgentLaunchArgs returns all per-device run settings for a game.
// GET /api/v3/game/{id}/agent-launch-args → {agentId: {launchArgs, envVars}}
func (h *Handler) GetAgentLaunchArgs(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	result, err := h.repo.GetAllAgentRunSettings(id)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, result)
}

// SetAgentLaunchArgs upserts run settings for one agent+game.
// PATCH /api/v3/game/{id}/agent-launch-args  body: {agentId, launchArgs, envVars}
func (h *Handler) SetAgentLaunchArgs(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	var req struct {
		AgentID    string `json:"agentId"`
		LaunchArgs string `json:"launchArgs"`
		EnvVars    string `json:"envVars"`
		ProtonPath string `json:"protonPath"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.AgentID == "" {
		jsonErr(w, 400, "agentId required")
		return
	}
	s := repository.AgentRunSettings{LaunchArgs: req.LaunchArgs, EnvVars: req.EnvVars, ProtonPath: req.ProtonPath}
	if err := h.repo.SetAgentRunSettings(id, req.AgentID, s); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	// Push SET_LAUNCH_ARGS to the agent so run.sh is updated without reinstalling.
	type payload struct {
		Title      string `json:"title"`
		LaunchArgs string `json:"launchArgs"`
		EnvVars    string `json:"envVars"`
		ProtonPath string `json:"protonPath"`
	}
	game, _ := h.repo.GetGameByID(id)
	if game != nil {
		data, _ := json.Marshal(payload{Title: game.Title, LaunchArgs: req.LaunchArgs, EnvVars: req.EnvVars, ProtonPath: req.ProtonPath})
		h.agentBroker.Send(req.AgentID, "SET_LAUNCH_ARGS", string(data))
	}
	jsonOK(w, map[string]string{"message": "ok"})
}

func (h *Handler) DeleteGame(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	game, _ := h.repo.GetGameByID(id)
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	// Handle file deletion params
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"
	targetPath := r.URL.Query().Get("targetPath")
	deleteDownload := r.URL.Query().Get("deleteDownloadFiles") == "true"
	downloadPath := r.URL.Query().Get("downloadPath")

	if deleteFiles && game.Path != nil {
		pathToDelete := *game.Path
		if targetPath != "" {
			pathToDelete = targetPath
		}
		if !isCriticalPath(pathToDelete) {
			_ = os.RemoveAll(pathToDelete)
		}
	}

	if deleteDownload && downloadPath != "" && !isCriticalPath(downloadPath) {
		_ = os.RemoveAll(downloadPath)
	}

	ok, err := h.repo.DeleteGame(id)
	if err != nil || !ok {
		jsonErr(w, 404, "game not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteAllGames(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.DeleteAllGames(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


// ---- helpers ----

func isPathInstallable(path *string) bool {
	if path == nil || *path == "" {
		return false
	}
	p := *path
	if fi, err := os.Stat(p); err == nil {
		if fi.IsDir() {
			return findInstaller(p, "") != ""
		}
		ext := strings.ToLower(filepath.Ext(p))
		return ext == ".exe" || ext == ".iso" || ext == ".zip" || ext == ".rar" || ext == ".7z"
	}
	return false
}

// scanExes reads a directory (non-recursively) and returns .exe files whose
// base name matches any of the given prefix/name checks.
// Uses os.ReadDir to avoid filepath.Glob bracket-metacharacter issues with
// directory names like "[FitGirl Repack]".
func scanExes(dir string, check func(lower string) bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if strings.HasSuffix(lower, ".exe") && check(lower) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func findInstaller(root, titleHint string) string {
	if root == "" {
		return ""
	}
	// If root is an exe directly
	if fileExists(root) && strings.ToLower(filepath.Ext(root)) == ".exe" {
		return root
	}
	if !dirExists(root) {
		return ""
	}

	isInstaller := func(lower string) bool {
		return strings.HasPrefix(lower, "setup") ||
			strings.HasPrefix(lower, "install") ||
			lower == "game.exe"
	}

	candidates := scanExes(root, isInstaller)

	// Depth-1 subdirectories
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			candidates = append(candidates, scanExes(filepath.Join(root, e.Name()), isInstaller)...)
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// Prioritize by title hint
	if titleHint != "" {
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(filepath.Base(c)), strings.ToLower(titleHint)) {
				return c
			}
		}
	}

	// Prioritize known names
	for _, prefer := range []string{"setup.exe", "install.exe", "installer.exe"} {
		for _, c := range candidates {
			if strings.EqualFold(filepath.Base(c), prefer) {
				return c
			}
		}
	}

	return candidates[0]
}


func (h *Handler) findDownloadPath(game *domain.Game) string {
	settings := h.cfg.LoadMedia()
	dlRoot := settings.DownloadPath
	if dlRoot == "" || !dirExists(dlRoot) {
		return ""
	}

	entries, err := os.ReadDir(dlRoot)
	if err != nil {
		return ""
	}

	titleLower := strings.ToLower(game.Title)
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.Contains(name, titleLower) {
			return filepath.Join(dlRoot, e.Name())
		}
	}
	return ""
}

func isCriticalPath(path string) bool {
	if path == "" {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	abs = strings.TrimRight(abs, string(filepath.Separator))

	// Block root and very short paths
	if len(abs) <= 3 {
		return true
	}

	sensitive := []string{
		"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/proc", "/root",
		"/run", "/sbin", "/sys", "/tmp", "/usr", "/var", "/Users",
		"C:\\", "C:\\Windows", "C:\\Program Files", "C:\\Users",
	}
	absLower := strings.ToLower(abs)
	for _, s := range sensitive {
		if absLower == strings.ToLower(s) {
			return true
		}
	}
	return false
}

// GetServerFiles walks game.Path and returns actual files on disk with sizes.
// Each entry includes metadata from the matching GameFile DB record if one exists.
// GET /api/v3/game/{id}/server-files
func (h *Handler) GetServerFiles(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	game, err := h.repo.GetGameByID(id)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.Path == nil || *game.Path == "" {
		jsonOK(w, []any{})
		return
	}
	root := *game.Path
	if !dirExists(root) {
		jsonOK(w, []any{})
		return
	}

	// Build a lookup of relativePath → GameFile for DB metadata
	dbFiles := map[string]domain.GameFile{}
	for _, f := range game.GameFiles {
		dbFiles[filepath.ToSlash(f.RelativePath)] = f
	}

	type serverFile struct {
		RelativePath string  `json:"relativePath"`
		Name         string  `json:"name"`
		Size         int64   `json:"size"`
		GameFileID   *int    `json:"gameFileId,omitempty"`
		Quality      *string `json:"quality,omitempty"`
		ReleaseGroup *string `json:"releaseGroup,omitempty"`
	}

	var files []serverFile
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		relSlash := filepath.ToSlash(rel)
		sf := serverFile{
			RelativePath: relSlash,
			Name:         info.Name(),
			Size:         info.Size(),
		}
		if dbf, ok := dbFiles[relSlash]; ok {
			sf.GameFileID = &dbf.ID
			sf.Quality = dbf.Quality
			sf.ReleaseGroup = dbf.ReleaseGroup
		}
		files = append(files, sf)
		return nil
	})
	if files == nil {
		files = []serverFile{}
	}
	jsonOK(w, files)
}

// DeleteServerFile deletes a physical file from game.Path and any matching DB record.
// DELETE /api/v3/game/{id}/server-file
// Body: {"relativePath": "some/file.iso"}
func (h *Handler) DeleteServerFile(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	game, err := h.repo.GetGameByID(id)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.Path == nil || *game.Path == "" {
		jsonErr(w, 400, "game has no path")
		return
	}

	var body struct {
		RelativePath string `json:"relativePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RelativePath == "" {
		jsonErr(w, 400, "relativePath required")
		return
	}

	// Security: ensure resolved path stays within game.Path
	root := filepath.Clean(*game.Path)
	target := filepath.Clean(filepath.Join(root, filepath.FromSlash(body.RelativePath)))
	if !strings.HasPrefix(target, root+string(filepath.Separator)) && target != root {
		jsonErr(w, 400, "invalid path")
		return
	}

	// Delete the physical file
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		jsonErr(w, 500, "delete failed: "+err.Error())
		return
	}

	// Remove matching GameFile DB record if one exists
	relSlash := filepath.ToSlash(body.RelativePath)
	for _, f := range game.GameFiles {
		if filepath.ToSlash(f.RelativePath) == relSlash {
			_ = h.repo.DeleteGameFile(f.ID)
			break
		}
	}

	jsonOK(w, map[string]bool{"ok": true})
}

// ImportGame manually triggers the post-processor pipeline on a path for a game.
// POST /api/v3/game/{id}/import
// Body (optional): {"path": "/some/download/dir"}
func (h *Handler) ImportGame(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	game, err := h.repo.GetGameByID(id)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	_ = decodeBody(r, &req)

	path := req.Path
	if path == "" {
		path = h.findDownloadPath(game)
	}
	if path == "" && game.Path != nil {
		path = *game.Path
	}
	if path == "" {
		jsonErr(w, 400, "no path found to import")
		return
	}

	go h.processor.Process(domain.DownloadStatus{
		Name:         game.Title,
		DownloadPath: path,
	})

	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, map[string]string{"message": "import started"})
}

// CheckGameUpdate triggers an immediate update check for a single game.
func (h *Handler) CheckGameUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	game, err := h.repo.GetGameByID(id)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.CurrentVersion == "" {
		jsonErr(w, 400, "game has no detected version")
		return
	}
	if err := h.checker.CheckGame(*game); err != nil {
		log.Printf("[API] CheckGameUpdate %d: %v", id, err)
	}
	// Re-fetch to get updated fields
	updated, _ := h.repo.GetGameByID(id)
	if updated == nil {
		updated = game
	}
	jsonOK(w, map[string]any{
		"latestVersion":   updated.LatestVersion,
		"updateAvailable": updated.UpdateAvailable,
	})
}

// CheckAllUpdates triggers an immediate update check for all games with a known version.
func (h *Handler) CheckAllUpdates(w http.ResponseWriter, r *http.Request) {
	go h.checker.CheckAll()
	jsonOK(w, map[string]string{"message": "update check started"})
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

