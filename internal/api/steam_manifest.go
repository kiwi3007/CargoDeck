package api

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ManifestEntry describes one depot's manifest data parsed from a steamtoolz ZIP.
type ManifestEntry struct {
	DepotID     int    `json:"depotId"`
	DepotKey    string `json:"depotKey"`    // hex depot encryption key
	ManifestGID string `json:"manifestGid"` // manifest version ID (string, 64-bit uint)
	AppToken    int64  `json:"appToken"`    // Steam app access token from setManifestid()
}

// SteamManifestInfo is the JSON summary stored alongside raw manifest files.
type SteamManifestInfo struct {
	AppID   int             `json:"appId"`
	Depots  []ManifestEntry `json:"depots"`
	Source  string          `json:"source"` // e.g. "https://api.owner-eab.workers.dev/..."
}

// steamManifestDir returns the directory where manifest data is stored for a game.
func (h *Handler) steamManifestDir(gameID int) string {
	return filepath.Join(h.cfg.Dir(), "steam-manifests", strconv.Itoa(gameID))
}

// loadSteamManifestInfo reads the cached manifest info for a game (if it exists).
func (h *Handler) loadSteamManifestInfo(gameID int) (*SteamManifestInfo, error) {
	data, err := os.ReadFile(filepath.Join(h.steamManifestDir(gameID), "info.json"))
	if err != nil {
		return nil, err
	}
	var info SteamManifestInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// ---- POST /api/v3/game/{id}/steam-manifest ----
// Accepts JSON { "downloadUrl": "https://..." } or multipart form with a "file" field.
// Downloads/reads the steamtoolz ZIP, parses Lua + .manifest files, stores them.
func (h *Handler) SetSteamManifest(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 400, "invalid game id")
		return
	}

	var zipData []byte

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		// File upload
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			jsonErr(w, 400, "failed to parse form: "+err.Error())
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			jsonErr(w, 400, "file field required")
			return
		}
		defer f.Close()
		zipData, err = io.ReadAll(io.LimitReader(f, 64<<20))
		if err != nil {
			jsonErr(w, 500, "read error: "+err.Error())
			return
		}
	} else {
		// JSON with downloadUrl
		var req struct {
			DownloadURL string `json:"downloadUrl"`
		}
		if err := decodeBody(r, &req); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		if req.DownloadURL == "" {
			jsonErr(w, 400, "downloadUrl required")
			return
		}
		zipData, err = downloadZIP(req.DownloadURL)
		if err != nil {
			jsonErr(w, 502, "download failed: "+err.Error())
			return
		}
	}

	info, err := parseSteamtoolzZIP(zipData)
	if err != nil {
		jsonErr(w, 422, "invalid ZIP: "+err.Error())
		return
	}

	dir := h.steamManifestDir(gameID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonErr(w, 500, "cannot create manifest dir: "+err.Error())
		return
	}

	// Store the raw ZIP for agent download
	if err := os.WriteFile(filepath.Join(dir, "manifests.zip"), zipData, 0o644); err != nil {
		jsonErr(w, 500, "write ZIP failed: "+err.Error())
		return
	}

	// Write info JSON for quick reads
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "info.json"), infoJSON, 0o644); err != nil {
		jsonErr(w, 500, "write info failed: "+err.Error())
		return
	}

	log.Printf("[SteamManifest] stored %d depots for game %d", len(info.Depots), gameID)
	jsonOK(w, info)
}

// ---- GET /api/v3/game/{id}/steam-manifest-info ----
// Returns the parsed manifest info for a game (or 404 if not set).
func (h *Handler) GetSteamManifestInfo(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 400, "invalid game id")
		return
	}

	infoPath := filepath.Join(h.steamManifestDir(gameID), "info.json")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonErr(w, 404, "no manifest stored for this game")
			return
		}
		jsonErr(w, 500, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// ---- GET /api/v3/game/{id}/steam-manifest-zip ----
// Serves the stored manifests.zip to authenticated agents.
func (h *Handler) ServeManifestZIP(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 400, "invalid game id")
		return
	}

	zipPath := filepath.Join(h.steamManifestDir(gameID), "manifests.zip")
	data, err := os.ReadFile(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonErr(w, 404, "no manifest stored for this game")
			return
		}
		jsonErr(w, 500, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"manifests_%d.zip\"", gameID))
	w.Write(data)
}

// downloadZIP fetches a ZIP file from a URL.
func downloadZIP(url string) ([]byte, error) {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// parseSteamtoolzZIP parses a ZIP downloaded from the steamtoolz worker.
// It expects:
//   - A Lua file (e.g. "2868840.lua") with addappid() and setManifestid() calls
//   - Optional binary .manifest files (e.g. "2868843_5676998966458301302.manifest")
func parseSteamtoolzZIP(data []byte) (*SteamManifestInfo, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a valid ZIP: %w", err)
	}

	var luaContent string
	var appID int

	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".lua") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			luaContent = string(b)

			// Derive appID from filename (e.g. "2868840.lua")
			base := strings.TrimSuffix(filepath.Base(f.Name), ".lua")
			if id, err := strconv.Atoi(base); err == nil {
				appID = id
			}
		}
	}

	if luaContent == "" {
		return nil, fmt.Errorf("no Lua file found in ZIP")
	}

	depots, err := parseLua(luaContent)
	if err != nil {
		return nil, fmt.Errorf("lua parse error: %w", err)
	}

	// Remove the main app entry (the first addappid is for the app itself, not a depot)
	// We keep only entries that also have a setManifestid() call (real depots).
	var filtered []ManifestEntry
	for _, d := range depots {
		if d.ManifestGID != "" {
			filtered = append(filtered, d)
		}
	}

	return &SteamManifestInfo{
		AppID:  appID,
		Depots: filtered,
	}, nil
}

// reAddApp matches: addappid(depotId, 1, "hexKey")
var reAddApp = regexp.MustCompile(`addappid\((\d+),\s*\d+,\s*"([0-9a-fA-F]+)"\)`)

// reSetManifest matches: setManifestid(depotId, "manifestGID", appToken)
var reSetManifest = regexp.MustCompile(`setManifestid\((\d+),\s*"(\d+)",\s*(\d+)\)`)

// parseLua extracts depot keys and manifest GIDs from a steamtoolz Lua file.
func parseLua(lua string) ([]ManifestEntry, error) {
	keys := make(map[int]string)       // depotId → hexKey
	manifests := make(map[int]string)  // depotId → manifestGID
	tokens := make(map[int]int64)      // depotId → appToken
	var order []int

	scanner := bufio.NewScanner(strings.NewReader(lua))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if m := reAddApp.FindStringSubmatch(line); m != nil {
			id, _ := strconv.Atoi(m[1])
			if _, seen := keys[id]; !seen {
				order = append(order, id)
			}
			keys[id] = m[2]
		}

		if m := reSetManifest.FindStringSubmatch(line); m != nil {
			id, _ := strconv.Atoi(m[1])
			manifests[id] = m[2]
			tok, _ := strconv.ParseInt(m[3], 10, 64)
			tokens[id] = tok
		}
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("no addappid() entries found")
	}

	var entries []ManifestEntry
	for _, id := range order {
		entries = append(entries, ManifestEntry{
			DepotID:     id,
			DepotKey:    keys[id],
			ManifestGID: manifests[id], // empty for the main app entry
			AppToken:    tokens[id],
		})
	}
	return entries, nil
}
