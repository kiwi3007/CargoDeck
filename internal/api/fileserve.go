package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ServeGameFile serves a game file with Range support (resumable downloads).
//
//	GET /api/v3/game/{id}/file?path=relative/to/game/root → file stream
//	GET /api/v3/game/{id}/file                             → JSON manifest
func (h *Handler) ServeGameFile(w http.ResponseWriter, r *http.Request) {
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

	// If a server-side extraction is in progress for this game, serve from the temp dir.
	gameRoot := ""
	if raw, ok := h.installTempDirs.Load(id); ok {
		gameRoot = raw.(string)
	} else if game.Path != nil {
		gameRoot = *game.Path
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		// Return JSON manifest
		type entry struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
			Mod  string `json:"mod,omitempty"`
		}
		var entries []entry
		for _, gf := range game.GameFiles {
			e := entry{Path: gf.RelativePath, Size: gf.Size}
			if gameRoot != "" {
				if fi, err := os.Stat(filepath.Join(gameRoot, gf.RelativePath)); err == nil {
					e.Mod = fi.ModTime().Format(time.RFC3339)
				}
			}
			entries = append(entries, e)
		}
		if entries == nil {
			entries = []entry{}
		}
		jsonOK(w, map[string]any{
			"gameId":   id,
			"gameRoot": gameRoot,
			"files":    entries,
		})
		return
	}

	// Sanitize the path to prevent directory traversal
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") {
		jsonErr(w, 400, "invalid path")
		return
	}

	if gameRoot == "" {
		jsonErr(w, 404, "game has no path set")
		return
	}

	fullPath := filepath.Join(gameRoot, cleanRel)

	// Verify the resolved path is still inside gameRoot
	absRoot, _ := filepath.Abs(gameRoot)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
		jsonErr(w, 400, "invalid path")
		return
	}

	f, err := os.Open(fullPath)
	if err != nil {
		jsonErr(w, 404, "file not found")
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		jsonErr(w, 404, "not a file")
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(fullPath))
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}
