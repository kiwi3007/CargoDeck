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
	"github.com/kiwi3007/cargodeck/internal/ddm"
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

// ---- POST /api/v3/game/{id}/fetch-manifest ----
// Fetches a steamtoolz-compatible manifest ZIP from Morrenus by the game's SteamID.
func (h *Handler) FetchManifestFromMorrenus(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 400, "invalid game id")
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.SteamID == nil {
		jsonErr(w, 400, "game has no Steam ID")
		return
	}

	morrenus := h.cfg.LoadMorrenus()
	if !morrenus.IsConfigured() {
		jsonErr(w, 404, "Morrenus not configured — add an API key in Settings")
		return
	}

	manifestURL := fmt.Sprintf("https://manifest.morrenus.xyz/api/v1/manifest/%d?api_key=%s", *game.SteamID, morrenus.APIKey)
	zipData, err := downloadZIP(manifestURL)
	if err != nil {
		jsonErr(w, 502, "Morrenus fetch failed: "+err.Error())
		return
	}

	info, err := parseSteamtoolzZIP(zipData)
	if err != nil {
		jsonErr(w, 422, "invalid ZIP from Morrenus: "+err.Error())
		return
	}
	// Use the game's known SteamID if the Lua filename didn't encode it
	if info.AppID == 0 {
		info.AppID = *game.SteamID
	}
	info.Source = manifestURL

	dir := h.steamManifestDir(gameID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonErr(w, 500, "cannot create manifest dir: "+err.Error())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "manifests.zip"), zipData, 0o644); err != nil {
		jsonErr(w, 500, "write ZIP failed: "+err.Error())
		return
	}
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "info.json"), infoJSON, 0o644); err != nil {
		jsonErr(w, 500, "write info failed: "+err.Error())
		return
	}

	log.Printf("[SteamManifest] Morrenus: stored %d depots for game %d (appId %d)", len(info.Depots), gameID, info.AppID)
	jsonOK(w, info)
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

// ---- POST /api/v3/game/{id}/steam-download ----
// Triggers a server-side DepotDownloaderMod download using stored manifest data.
// Returns immediately with { "jobId": "..." }; progress is emitted via SSE
// as STEAM_DOWNLOAD_PROGRESS events.
func (h *Handler) SteamDownload(w http.ResponseWriter, r *http.Request) {
	gameID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 400, "invalid game id")
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.SteamID == nil {
		jsonErr(w, 400, "game has no Steam ID")
		return
	}

	info, err := h.loadSteamManifestInfo(gameID)
	if err != nil {
		if os.IsNotExist(err) {
			jsonErr(w, 404, "no manifest stored for this game — upload or fetch a manifest first")
			return
		}
		jsonErr(w, 500, err.Error())
		return
	}
	if len(info.Depots) == 0 {
		jsonErr(w, 400, "manifest contains no depots")
		return
	}

	// Optional body: { "depotIds": [123, 456] } to select specific depots.
	// If omitted or empty, all depots in the manifest are used.
	var req struct {
		DepotIDs []int `json:"depotIds"`
	}
	_ = decodeBody(r, &req) // ignore decode error — body is optional

	if !h.ddm.IsAvailable() {
		jsonErr(w, 503, "DepotDownloaderMod binary not found — check PLAYERR_DDM_BIN or rebuild the container")
		return
	}

	media := h.cfg.LoadMedia()

	selected := info.Depots
	if len(req.DepotIDs) > 0 {
		keep := make(map[int]bool, len(req.DepotIDs))
		for _, id := range req.DepotIDs {
			keep[id] = true
		}
		filtered := info.Depots[:0]
		for _, d := range info.Depots {
			if keep[d.DepotID] {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			jsonErr(w, 400, "none of the requested depotIds exist in this manifest")
			return
		}
		selected = filtered
	}

	depots := make([]ddm.DepotEntry, len(selected))
	for i, d := range selected {
		depots[i] = ddm.DepotEntry{
			DepotID:     d.DepotID,
			DepotKey:    d.DepotKey,
			ManifestGID: d.ManifestGID,
			AppToken:    d.AppToken,
		}
	}

	// After the download completes, set the game's Path so it appears in the library.
	onComplete := func(gameDir string) {
		if err := h.repo.UpdateGamePath(gameID, gameDir); err != nil {
			log.Printf("[DDM] UpdateGamePath game=%d dir=%s: %v", gameID, gameDir, err)
			return
		}
		log.Printf("[DDM] Set game %d path → %s", gameID, gameDir)
		// Notify the browser so the library updates without a manual refresh.
		h.broker.Publish("LIBRARY_UPDATED", fmt.Sprintf(`{"gameId":%d}`, gameID))
	}

	jobID, err := h.ddm.Download(
		gameID,
		info.AppID,
		game.Title,
		media.DownloadPath,
		h.steamManifestDir(gameID),
		depots,
		onComplete,
	)
	if err != nil {
		jsonErr(w, 409, err.Error()) // 409 = already running
		return
	}

	log.Printf("[DDM] Started server-side download for game %d (app %d), job %s", gameID, info.AppID, jobID)
	jsonOK(w, map[string]string{"jobId": jobID})
}
