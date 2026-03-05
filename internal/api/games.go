package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
	"github.com/kiwi3007/playerr/internal/launcher"
	"github.com/kiwi3007/playerr/internal/metadata/igdb"
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
	uninstallerPath := findUninstaller(game.Path)
	downloadPath := h.findDownloadPath(game)

	isInstaller := game.Status == domain.GameStatusInstallerDetected ||
		(game.ExecutablePath != nil &&
			(strings.HasSuffix(strings.ToLower(*game.ExecutablePath), "setup.exe") ||
				strings.HasSuffix(strings.ToLower(*game.ExecutablePath), "install.exe")))

	canPlay := (game.SteamID != nil && *game.SteamID > 0) ||
		(game.ExecutablePath != nil &&
			fileExists(*game.ExecutablePath) &&
			!isInstaller)

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
		"installPath":        game.InstallPath,
		"isInstallable":      isInstallable,
		"executablePath":     game.ExecutablePath,
		"isExternal":         game.IsExternal,
		"gameFiles":          game.GameFiles,
		"uninstallerPath":    uninstallerPath,
		"downloadPath":       downloadPath,
		"canPlay":            canPlay,
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
	if update.InstallPath != nil && *update.InstallPath != "" {
		existing.InstallPath = update.InstallPath
	}
	if update.ExecutablePath != nil && *update.ExecutablePath != "" {
		existing.ExecutablePath = update.ExecutablePath
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

func (h *Handler) PlayGame(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	game, _ := h.repo.GetGameByID(id)
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	pathOverride := r.URL.Query().Get("path")

	// Steam launch
	if game.SteamID != nil && *game.SteamID > 0 {
		steamURL := fmt.Sprintf("steam://rungameid/%d", *game.SteamID)
		if err := openURL(steamURL); err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, map[string]string{"message": fmt.Sprintf("Launching %s via Steam...", game.Title)})
		return
	}

	// Native launch
	exePath := pathOverride
	if exePath == "" && game.ExecutablePath != nil {
		exePath = *game.ExecutablePath
	}
	if exePath == "" {
		jsonErr(w, 400, "no executable path set")
		return
	}

	if err := launchProcess(exePath); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": fmt.Sprintf("Launching %s...", game.Title)})
}

func (h *Handler) InstallGame(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	game, _ := h.repo.GetGameByID(id)
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	pathOverride := r.URL.Query().Get("path")
	targetPath := pathOverride
	if targetPath == "" && game.Path != nil {
		targetPath = *game.Path
	}
	if targetPath == "" {
		jsonErr(w, 400, "game path not set")
		return
	}

	// Use manual install path if set
	if game.InstallPath != nil && *game.InstallPath != "" && fileExists(*game.InstallPath) {
		if err := launchInstaller(*game.InstallPath); err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonOK(w, map[string]string{"message": "Installer launched"})
		return
	}

	installer := findInstaller(targetPath, game.Title)
	if installer == "" {
		jsonErr(w, 400, "no installer found")
		return
	}

	if err := launchInstaller(installer); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": fmt.Sprintf("Installer launched: %s", filepath.Base(installer))})
}

func (h *Handler) UninstallGame(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	game, _ := h.repo.GetGameByID(id)
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	uninstaller := findUninstaller(game.Path)
	if uninstaller == "" {
		jsonErr(w, 404, "no uninstaller found")
		return
	}

	if err := launchInstaller(uninstaller); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Uninstaller launched"})
}

func (h *Handler) AddSteamShortcut(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	game, _ := h.repo.GetGameByID(id)
	if game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	exePath := ""
	if game.ExecutablePath != nil {
		exePath = *game.ExecutablePath
	}
	if exePath == "" {
		jsonErr(w, 400, "no executable path set for this game")
		return
	}

	startDir := filepath.Dir(exePath)
	entry := shortcutEntry{
		AppName:  game.Title,
		Exe:      exePath,
		StartDir: startDir,
	}

	appID, err := addSteamShortcut(entry)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]any{
		"message": fmt.Sprintf("Steam shortcut added for %q. Restart Steam to see it.", game.Title),
		"appId":   appID,
	})
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

func findUninstaller(path *string) string {
	if path == nil || *path == "" {
		return ""
	}
	root := *path
	if !dirExists(root) {
		return ""
	}

	isUninstaller := func(lower string) bool {
		return strings.HasPrefix(lower, "unins") || strings.Contains(lower, "uninstall")
	}

	candidates := scanExes(root, isUninstaller)
	if len(candidates) == 0 {
		return ""
	}
	for _, c := range candidates {
		if strings.HasPrefix(strings.ToLower(filepath.Base(c)), "unins") {
			return c
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

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func launchInstaller(path string) error {
	return launchProcess(path)
}

func launchProcess(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command(path)
		cmd.Dir = filepath.Dir(path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		// Linux: for .exe files try Proton, then Wine
		if strings.HasSuffix(strings.ToLower(path), ".exe") {
			if protonCmd := launcher.TryProton(path, "playerr"); protonCmd != nil {
				return protonCmd.Start()
			}
			cmd = exec.Command("wine", path)
		} else {
			cmd = exec.Command(path)
		}
		cmd.Dir = filepath.Dir(path)
	}
	return cmd.Start()
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
