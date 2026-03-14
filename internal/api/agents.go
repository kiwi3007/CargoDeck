package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kiwi3007/cargodeck/internal/agent"
	"github.com/kiwi3007/cargodeck/internal/steamgriddb"
)

// ---- Agent nonce store (60s TTL, single-use) ----

var agentNonces sync.Map // string → time.Time (expires)

// ---- Agent session store (30-day TTL, tied to agentId) ----

type agentSessionEntry struct {
	agentID string
	expires time.Time
}

var agentSessionsMu sync.RWMutex
var agentSessionMap = map[string]agentSessionEntry{} // sessionToken → entry

func storeAgentSession(token, agentID string) {
	agentSessionsMu.Lock()
	agentSessionMap[token] = agentSessionEntry{agentID: agentID, expires: time.Now().Add(30 * 24 * time.Hour)}
	agentSessionsMu.Unlock()
}

// isValidAgentSessionToken returns true if the token is a live agent session.
// Used by uiAuthMiddleware to let agents pass through.
func isValidAgentSessionToken(token string) bool {
	agentSessionsMu.RLock()
	entry, ok := agentSessionMap[token]
	agentSessionsMu.RUnlock()
	return ok && time.Now().Before(entry.expires)
}

// agentIDForSession returns the agentID associated with a session token.
func agentIDForSession(token string) (string, bool) {
	agentSessionsMu.RLock()
	entry, ok := agentSessionMap[token]
	agentSessionsMu.RUnlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.agentID, true
	}
	return "", false
}

// ---- GET /api/v3/auth/agent-challenge ----

// GetAgentChallenge issues a nonce for agent CHAP-SHA256 registration.
// Unauthenticated — exempt from all middlewares.
func (h *Handler) GetAgentChallenge(w http.ResponseWriter, r *http.Request) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		jsonErr(w, 500, "rng error")
		return
	}
	nonceHex := hex.EncodeToString(nonce)
	agentNonces.Store(nonceHex, time.Now().Add(60*time.Second))
	jsonOK(w, map[string]string{"nonce": nonceHex})
}

// ---- Register ----

type registerRequest struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Platform     string              `json:"platform"`
	SteamPath    string              `json:"steamPath"`
	Version      string              `json:"version,omitempty"`
	InstallPaths []agent.InstallPath `json:"installPaths,omitempty"`
	Nonce        string              `json:"nonce"`
	ChapResponse string              `json:"chapResponse"`
}

func (h *Handler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.ID == "" || req.Name == "" {
		jsonErr(w, 400, "id and name are required")
		return
	}

	// CHAP-SHA256 validation
	agentCfg := h.cfg.LoadAgent()
	if req.Nonce == "" || req.ChapResponse == "" {
		jsonErr(w, 401, "nonce and chapResponse are required")
		return
	}
	val, ok := agentNonces.LoadAndDelete(req.Nonce)
	exp, expOK := val.(time.Time)
	if !ok || !expOK || time.Now().After(exp) {
		jsonErr(w, 401, "invalid or expired nonce")
		return
	}
	mac := hmac.New(sha256.New, []byte(agentCfg.Token))
	mac.Write([]byte(req.Nonce))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(req.ChapResponse), []byte(expected)) {
		jsonErr(w, 401, "invalid agent token")
		return
	}

	// Issue 30-day session token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		jsonErr(w, 500, "failed to generate session token")
		return
	}
	sessionToken := hex.EncodeToString(tokenBytes)
	storeAgentSession(sessionToken, req.ID)

	h.agentRegistry.Register(agent.AgentInfo{
		ID:           req.ID,
		Name:         req.Name,
		Platform:     req.Platform,
		SteamPath:    req.SteamPath,
		Version:      req.Version,
		InstallPaths: req.InstallPaths,
	})

	log.Printf("[Agent] Registered (CHAP): %s (%s) platform=%s version=%s paths=%d",
		req.Name, req.ID, req.Platform, req.Version, len(req.InstallPaths))

	// If the agent's version doesn't match what the server is hosting, tell it to update.
	// We send after a short delay so the SSE stream has time to open.
	if serverVer := h.agentVersion(); serverVer != "dev" && serverVer != "" && serverVer != req.Version {
		agentID := req.ID
		go func() {
			time.Sleep(2 * time.Second)
			log.Printf("[Agent] Triggering update for %s: %q → %q", agentID, req.Version, serverVer)
			h.agentBroker.Send(agentID, "CHECK_UPDATE", "{}")
		}()
	}

	h.publishAgentList()
	jsonOK(w, map[string]any{"agentId": req.ID, "sessionToken": sessionToken})
}

// ---- Delete ----

func (h *Handler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentRegistry.Remove(agentID)
	revokeAgentSessions(agentID)
	h.publishAgentList()
	log.Printf("[Agent] Deleted: %s", agentID)
	w.WriteHeader(http.StatusNoContent)
}

// revokeAgentSessions removes all session tokens associated with agentID.
func revokeAgentSessions(agentID string) {
	agentSessionsMu.Lock()
	for token, entry := range agentSessionMap {
		if entry.agentID == agentID {
			delete(agentSessionMap, token)
		}
	}
	agentSessionsMu.Unlock()
}

// ---- List ----

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.agentRegistry.List()
	if agents == nil {
		agents = []agent.AgentInfo{}
	}
	jsonOK(w, agents)
}

// ---- SSE stream for agent ----

func (h *Handler) AgentEvents(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agentId")
	if agentID == "" {
		jsonErr(w, 400, "agentId query param required")
		return
	}

	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not registered")
		return
	}

	h.agentRegistry.SetOnline(agentID)
	log.Printf("[Agent] SSE stream opened: %s", agentID)

	jobCh := h.agentJobs.GetOrCreate(agentID)
	h.agentBroker.ServeAgent(agentID, w, r, jobCh, func() {
		h.agentRegistry.SetOffline(agentID)
		h.publishAgentList()
		log.Printf("[Agent] SSE stream closed: %s", agentID)
	})
}

// ---- Install preview ----

type ExeCandidate struct {
	Name        string `json:"name"`
	RelPath     string `json:"relPath"`
	Type        string `json:"type"`                  // "installer" | "game"
	FromArchive string `json:"fromArchive,omitempty"` // archive basename it came from
}

type installPreviewResponse struct {
	Candidates []ExeCandidate `json:"candidates"`
}

func classifyExe(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install") {
		return "installer"
	}
	return "game"
}

// listArchiveExes lists .exe files inside an archive using "7z l".
// Returns nil if 7z is unavailable or produces no output.
func listArchiveExes(archivePath string) []string {
	for _, bin := range []string{"7z", "7za"} {
		out, err := exec.Command(bin, "l", archivePath).Output()
		if err != nil || len(out) == 0 {
			continue
		}
		var names []string
		for _, line := range strings.Split(string(out), "\n") {
			parts := strings.Fields(line)
			// Data lines have at least 6 fields: date time attr size compressed name
			// First field is a date (YYYY-MM-DD); skip header/footer lines
			if len(parts) < 6 || len(parts[0]) != 10 || parts[0][4] != '-' {
				continue
			}
			// Skip directories (attribute field starts with 'D')
			if strings.HasPrefix(parts[2], "D") {
				continue
			}
			// Name may contain spaces — join everything after the 5th field
			name := strings.Join(parts[5:], " ")
			if strings.HasSuffix(strings.ToLower(filepath.Base(name)), ".exe") {
				names = append(names, name)
			}
		}
		return names
	}
	return nil
}

func (h *Handler) GetInstallPreview(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}

	gameIDStr := r.URL.Query().Get("gameId")
	if gameIDStr == "" {
		jsonErr(w, 400, "gameId query param required")
		return
	}
	var gameID int
	if _, err := fmt.Sscan(gameIDStr, &gameID); err != nil {
		jsonErr(w, 400, "invalid gameId")
		return
	}

	game, err := h.repo.GetGameByID(gameID)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}
	if game.Path == nil || *game.Path == "" {
		jsonOK(w, installPreviewResponse{Candidates: []ExeCandidate{}})
		return
	}

	root := *game.Path
	seen := map[string]bool{}
	var candidates []ExeCandidate

	archiveExts := map[string]bool{".zip": true, ".rar": true, ".iso": true, ".7z": true}

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if ext == ".exe" {
			name := info.Name()
			key := rel
			if !seen[key] {
				seen[key] = true
				candidates = append(candidates, ExeCandidate{
					Name:    name,
					RelPath: rel,
					Type:    classifyExe(name),
				})
			}
			return nil
		}

		if archiveExts[ext] {
			archiveName := filepath.Base(path)
			for _, name := range listArchiveExes(path) {
				base := filepath.Base(name)
				key := archiveName + ":" + name
				if !seen[key] {
					seen[key] = true
					candidates = append(candidates, ExeCandidate{
						Name:        base,
						RelPath:     name,
						Type:        classifyExe(base),
						FromArchive: archiveName,
					})
				}
			}
		}

		return nil
	})

	if candidates == nil {
		candidates = []ExeCandidate{}
	}
	jsonOK(w, installPreviewResponse{Candidates: candidates})
}

// ---- Dispatch install job ----

type dispatchRequest struct {
	GameID      int    `json:"gameId"`
	InstallDir  string `json:"installDir,omitempty"`  // override agent default install location
	SelectedExe string `json:"selectedExe,omitempty"` // basename chosen by user; empty = auto-detect
}

func (h *Handler) DispatchInstall(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	agentInfo, ok := h.agentRegistry.Get(agentID)
	if !ok {
		jsonErr(w, 404, "agent not found")
		return
	}

	var req dispatchRequest
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.GameID == 0 {
		jsonErr(w, 400, "gameId required")
		return
	}

	game, err := h.repo.GetGameByID(req.GameID)
	if err != nil || game == nil {
		jsonErr(w, 404, "game not found")
		return
	}

	var files []string
	for _, gf := range game.GameFiles {
		files = append(files, gf.RelativePath)
	}
	if len(files) == 0 && game.Path != nil && *game.Path != "" {
		// No GameFile records — walk the directory and enumerate files
		root := *game.Path
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
	}

	if len(files) == 0 {
		jsonErr(w, 400, "no game files available for this game — add a file path in the library first")
		return
	}

	// If the library files are archives, extract to a temp dir on the server and send
	// the extracted contents instead. The temp dir is cleaned up when the job finishes.
	if containsArchives(files) && game.Path != nil {
		tempDir, extractedFiles, err := extractToTemp(*game.Path)
		if err != nil {
			log.Printf("[Agent] Server-side extraction failed for game %d: %v — sending archives directly", req.GameID, err)
		} else if len(extractedFiles) > 0 {
			files = extractedFiles
			h.setInstallTempDir(req.GameID, tempDir)
			log.Printf("[Agent] Extracted game %d to temp %s (%d files)", req.GameID, tempDir, len(files))
		}
	}

	serverURL := resolveServerURL(r)
	jobID := fmt.Sprintf("%d-%d", req.GameID, time.Now().UnixMilli())

	runSettings, _ := h.repo.GetAgentRunSettings(req.GameID, agentID)

	job := agent.InstallJob{
		JobID:       jobID,
		AgentID:     agentID,
		GameID:      req.GameID,
		GameTitle:   game.Title,
		Files:       files,
		ServerURL:   serverURL,
		InstallDir:  req.InstallDir,
		SelectedExe: req.SelectedExe,
		LaunchArgs:  runSettings.LaunchArgs,
		EnvVars:     runSettings.EnvVars,
		ProtonPath:  runSettings.ProtonPath,
	}
	if game.SteamID != nil {
		job.SteamID = *game.SteamID
	}

	if h.agentRegistry.HasActiveJobForGame(agentID, req.GameID) {
		jsonErr(w, 409, "a job for this game is already queued or in progress on this agent")
		return
	}

	h.agentRegistry.TrackJob(agentID, jobID, game.Title, req.GameID)

	if !h.agentJobs.Enqueue(job) {
		jsonErr(w, 503, "agent job queue full")
		return
	}

	// Notify browser via fan-out broker
	progressJSON, _ := json.Marshal(agent.JobProgress{
		JobID:   jobID,
		Status:  agent.JobQueued,
		Message: fmt.Sprintf("Queued install of %q on agent %q", game.Title, agentInfo.Name),
		Percent: 0,
	})
	h.broker.Publish("AGENT_JOB_QUEUED", string(progressJSON))

	log.Printf("[Agent] Dispatched job %s → agent %s (game %d)", jobID, agentID, req.GameID)
	jsonOK(w, map[string]string{"jobId": jobID})
}

// dispatchSimpleJob validates the agent exists, sends a no-payload job, and returns a message.
func (h *Handler) dispatchSimpleJob(w http.ResponseWriter, r *http.Request, jobType, msg string) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentBroker.Send(agentID, jobType, "{}")
	log.Printf("[Agent] Dispatched %s → %s", jobType, agentID)
	jsonOK(w, map[string]string{"message": msg})
}

// ---- Dispatch scan job ----

func (h *Handler) DispatchScan(w http.ResponseWriter, r *http.Request) {
	h.dispatchSimpleJob(w, r, "SCAN_GAMES", "scan requested")
}

// ---- Dispatch shortcut refresh ----

func (h *Handler) DispatchRefreshShortcuts(w http.ResponseWriter, r *http.Request) {
	h.dispatchSimpleJob(w, r, "REFRESH_SHORTCUTS", "shortcut refresh requested")
}

// ---- Receive installed games report from agent ----

func (h *Handler) ReportInstalledGames(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	var games []agent.InstalledGame
	if err := decodeBody(r, &games); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	h.agentRegistry.SetInstalledGames(agentID, games)
	h.publishAgentList()
	log.Printf("[Agent] Received %d installed games from %s", len(games), agentID)

	// Persist detected versions to DB.
	// Only update if no version is currently stored — the release-name parse
	// at download time has higher priority and must not be overwritten by
	// less reliable agent-side detection (PE exe, version files).
	for _, ig := range games {
		if ig.Version == "" {
			continue
		}
		game, err := h.repo.GetGameByTitle(ig.Title)
		if err != nil || game == nil {
			continue
		}
		if game.CurrentVersion == "" {
			_ = h.repo.UpdateGameVersion(game.ID, ig.Version)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- Dispatch Steam restart ----

func (h *Handler) DispatchRestartSteam(w http.ResponseWriter, r *http.Request) {
	h.dispatchSimpleJob(w, r, "RESTART_STEAM", "Steam restart requested")
}

// ---- Dispatch save restore ----

func (h *Handler) DispatchRestoreSave(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	var req struct {
		GameID int    `json:"gameId"`
		Title  string `json:"title"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.GameID == 0 || req.Title == "" {
		jsonErr(w, 400, "gameId and title are required")
		return
	}
	data, _ := json.Marshal(req)
	h.agentBroker.Send(agentID, "RESTORE_SAVE", string(data))
	log.Printf("[Agent] Dispatched RESTORE_SAVE game=%d %q → %s", req.GameID, req.Title, agentID)
	jsonOK(w, map[string]string{"message": "restore requested"})
}

// ---- Dispatch upload-save job ----

// DispatchUploadSave tells an agent to immediately upload its current saves for a game.
// POST /api/v3/agent/{agentId}/upload-save
// Body: {"title": "Game Title"}
func (h *Handler) DispatchUploadSave(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.Title == "" {
		jsonErr(w, 400, "title is required")
		return
	}
	data, _ := json.Marshal(req)
	h.agentBroker.Send(agentID, "UPLOAD_SAVE", string(data))
	log.Printf("[Agent] Dispatched UPLOAD_SAVE %q → %s", req.Title, agentID)
	jsonOK(w, map[string]string{"message": "upload requested"})
}

// ---- Dispatch change-exe job ----

func (h *Handler) DispatchChangeExe(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	var req struct {
		Title   string `json:"title"`
		ExePath string `json:"exePath"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.Title == "" || req.ExePath == "" {
		jsonErr(w, 400, "title and exePath are required")
		return
	}
	data, _ := json.Marshal(req)
	h.agentBroker.Send(agentID, "CHANGE_EXE", string(data))
	log.Printf("[Agent] Dispatched CHANGE_EXE %q → %q on %s", req.ExePath, req.Title, agentID)
	jsonOK(w, map[string]string{"message": "exe change requested"})
}

// ---- Dispatch script regeneration ----

func (h *Handler) DispatchRegenScripts(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	// Build safeName(title) → steamID map for all games that have a Steam ID.
	// The agent uses directory names (= safeName(title)) to match entries.
	steamIDs := map[string]int{}
	games, err := h.repo.GetAllGames()
	if err != nil {
		log.Printf("[Agent] DispatchRegenScripts: failed to load games: %v", err)
	} else {
		for _, g := range games {
			if g.SteamID != nil && *g.SteamID != 0 {
				steamIDs[safeGameName(g.Title)] = *g.SteamID
			}
		}
	}
	data, _ := json.Marshal(steamIDs)
	h.agentBroker.Send(agentID, "REGEN_SCRIPTS", string(data))
	log.Printf("[Agent] Dispatched REGEN_SCRIPTS → %s (%d steam IDs)", agentID, len(steamIDs))
	jsonOK(w, map[string]string{"message": "script regeneration requested"})
}

// safeGameName mirrors the agent's safeName() — replaces filesystem-unsafe
// characters with hyphens so directory names match the server-side title.
func safeGameName(title string) string {
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

// dispatchReadJob dispatches a read-file job (log or script) to an agent and returns a requestId.
func (h *Handler) dispatchReadJob(w http.ResponseWriter, r *http.Request, sseEvent, idPrefix string, makeJob func(requestID, title string) any) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	var req struct {
		GameTitle string `json:"gameTitle"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.GameTitle == "" {
		jsonErr(w, 400, "gameTitle required")
		return
	}
	requestID := fmt.Sprintf("%s-%d", idPrefix, time.Now().UnixMilli())
	data, _ := json.Marshal(makeJob(requestID, req.GameTitle))
	h.agentBroker.Send(agentID, sseEvent, string(data))
	log.Printf("[Agent] Dispatched %s %q → %s (req=%s)", sseEvent, req.GameTitle, agentID, requestID)
	jsonOK(w, map[string]string{"requestId": requestID})
}

// receiveAgentContent decodes a {requestId, gameTitle, content} payload from an agent and publishes it as an SSE event.
func (h *Handler) receiveAgentContent(w http.ResponseWriter, r *http.Request, eventName string) {
	var payload struct {
		RequestID string `json:"requestId"`
		GameTitle string `json:"gameTitle"`
		Content   string `json:"content"`
	}
	if err := decodeBody(r, &payload); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	data, _ := json.Marshal(payload)
	h.broker.Publish(eventName, string(data))
	w.WriteHeader(http.StatusNoContent)
}

// ---- Dispatch read-log job ----

func (h *Handler) DispatchReadLog(w http.ResponseWriter, r *http.Request) {
	h.dispatchReadJob(w, r, "READ_LOG", "log", func(reqID, title string) any {
		return agent.ReadLogJob{RequestID: reqID, GameTitle: title}
	})
}

// ---- Receive log content from agent ----

func (h *Handler) ReceiveAgentLog(w http.ResponseWriter, r *http.Request) {
	h.receiveAgentContent(w, r, "AGENT_LOG_DATA")
}

// ---- Dispatch read script ----

func (h *Handler) DispatchReadScript(w http.ResponseWriter, r *http.Request) {
	h.dispatchReadJob(w, r, "READ_SCRIPT", "script", func(reqID, title string) any {
		return agent.ReadScriptJob{RequestID: reqID, GameTitle: title}
	})
}

// ---- Receive script content from agent ----

func (h *Handler) ReceiveAgentScript(w http.ResponseWriter, r *http.Request) {
	h.receiveAgentContent(w, r, "AGENT_SCRIPT_DATA")
}

// ---- Dispatch delete game job ----

func (h *Handler) DispatchDeleteGame(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}

	var req struct {
		Title          string `json:"title"`
		InstallPath    string `json:"installPath"`
		RemoveShortcut bool   `json:"removeShortcut"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.InstallPath == "" {
		jsonErr(w, 400, "installPath required")
		return
	}

	job := agent.DeleteGameJob{
		JobID:          fmt.Sprintf("del-%d", time.Now().UnixMilli()),
		Title:          req.Title,
		InstallPath:    req.InstallPath,
		RemoveShortcut: req.RemoveShortcut,
	}

	data, _ := json.Marshal(job)
	h.agentBroker.Send(agentID, "DELETE_GAME", string(data))
	log.Printf("[Agent] Dispatched DELETE_GAME %q → %s", req.Title, agentID)
	jsonOK(w, map[string]string{"jobId": job.JobID})
}

// ---- POST /api/v3/agent/{agentId}/uninstall ----

func (h *Handler) DispatchUninstallAgent(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentBroker.Send(agentID, "UNINSTALL_AGENT", "")
	log.Printf("[Agent] Dispatched UNINSTALL_AGENT → %s", agentID)
	jsonOK(w, map[string]bool{"ok": true})
}

// ---- Progress report from agent ----

func (h *Handler) AgentProgress(w http.ResponseWriter, r *http.Request) {
	var prog agent.JobProgress
	if err := decodeBody(r, &prog); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	// Capture game ID before UpdateJobProgress removes the job from the index.
	var cleanupGameID int
	if prog.Status == agent.JobDone || prog.Status == agent.JobFailed {
		if gameID, ok := h.agentRegistry.GetJobGameID(prog.JobID); ok {
			cleanupGameID = gameID
		}
	}

	h.agentRegistry.UpdateJobProgress(prog)
	h.publishAgentList()

	data, _ := json.Marshal(prog)
	h.broker.Publish("AGENT_PROGRESS", string(data))

	// Clean up server-side temp extraction dir after the job finishes.
	if cleanupGameID != 0 {
		if tempDir, ok := h.takeInstallTempDir(cleanupGameID); ok {
			if err := os.RemoveAll(tempDir); err != nil {
				log.Printf("[Agent] Failed to clean up temp dir %s: %v", tempDir, err)
			} else {
				log.Printf("[Agent] Cleaned up temp install dir for game %d: %s", cleanupGameID, tempDir)
			}
		}
	}

	log.Printf("[Agent] Progress %s: %s %d%%", prog.JobID, prog.Status, prog.Percent)
	w.WriteHeader(http.StatusNoContent)
}

// ---- SteamGridDB artwork URL resolver (agent-authenticated) ----

// GetArtworkURLs searches SteamGridDB and returns CDN URLs for each artwork
// type. The agent downloads the images directly from the CDN and saves them
// to its local Steam grid directory — the API key never leaves the server.
func (h *Handler) GetArtworkURLs(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("game")
	if game == "" {
		jsonErr(w, 400, "game query param required")
		return
	}
	cfg := h.cfg.LoadSteamGridDB()
	if !cfg.IsConfigured() {
		jsonErr(w, 503, "SteamGridDB not configured")
		return
	}
	client := steamgriddb.NewClient(cfg.ApiKey)
	urls, err := client.GetImageURLs(game)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	jsonOK(w, urls)
}

// ---- Agent settings (masked token for browser) ----

func (h *Handler) GetAgentSettings(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadAgent()
	jsonOK(w, map[string]string{"token": cfg.Token})
}

// ---- Queue broadcaster ----

// RunQueueBroadcaster pushes DOWNLOAD_QUEUE_UPDATED to all browser SSE clients every 3s.
// Run as a goroutine; stops when ctx is cancelled.
func (h *Handler) RunQueueBroadcaster(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queue := h.collectQueue()
			if data, err := json.Marshal(queue); err == nil {
				h.broker.Publish("DOWNLOAD_QUEUE_UPDATED", string(data))
			}
		}
	}
}

// ---- Helpers ----

// publishAgentList broadcasts the current agent list to all browser SSE clients.
func (h *Handler) publishAgentList() {
	agents := h.agentRegistry.List()
	if agents == nil {
		agents = []agent.AgentInfo{}
	}
	if data, err := json.Marshal(agents); err == nil {
		h.broker.Publish("AGENTS_UPDATED", string(data))
	}
}

// ---- Dispatch prefix rename ----

func (h *Handler) DispatchRenamePrefix(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	var req struct {
		GameID    int    `json:"gameId"`
		GameTitle string `json:"gameTitle"`
	}
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if req.GameID == 0 || req.GameTitle == "" {
		jsonErr(w, 400, "gameId and gameTitle are required")
		return
	}
	job := agent.RenamePrefixJob{GameID: req.GameID, GameTitle: req.GameTitle}
	data, _ := json.Marshal(job)
	h.agentBroker.Send(agentID, "RENAME_PREFIX", string(data))
	log.Printf("[Agent] Dispatched RENAME_PREFIX %q → %s", req.GameTitle, agentID)
	jsonOK(w, map[string]string{"message": "prefix rename requested"})
}

// ---- Agent filesystem browser ----

// BrowseAgentDir dispatches a LIST_DIR job to an online agent and waits up to
// 10 s for the directory listing. GET /api/v3/agent/{agentId}/browse?path=
func (h *Handler) BrowseAgentDir(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "~"
	}

	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found or offline")
		return
	}

	requestID := fmt.Sprintf("browse-%d", time.Now().UnixNano())
	ch := make(chan agent.BrowseDirResult, 1)
	h.browsePending.Store(requestID, ch)
	defer h.browsePending.Delete(requestID)

	job := agent.BrowseDirJob{RequestID: requestID, Path: path}
	data, _ := json.Marshal(job)
	h.agentBroker.Send(agentID, "LIST_DIR", string(data))

	select {
	case result := <-ch:
		if result.Error != "" {
			jsonErr(w, 422, result.Error)
			return
		}
		jsonOK(w, result)
	case <-time.After(10 * time.Second):
		jsonErr(w, 504, "agent did not respond in time")
	}
}

// ReceiveBrowseResult is called by the agent with the directory listing result.
// POST /api/v3/agent/browse-result (agent-authenticated)
func (h *Handler) ReceiveBrowseResult(w http.ResponseWriter, r *http.Request) {
	var result agent.BrowseDirResult
	if err := decodeBody(r, &result); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v, ok := h.browsePending.Load(result.RequestID); ok {
		ch, ok2 := v.(chan agent.BrowseDirResult)
		if !ok2 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		select {
		case ch <- result:
		default:
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListProtonVersions dispatches a LIST_PROTON job to an online agent and waits up to
// 10 s for the list of available Proton versions.
// GET /api/v3/agent/{agentId}/proton-versions
func (h *Handler) ListProtonVersions(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found or offline")
		return
	}

	requestID := fmt.Sprintf("proton-%d", time.Now().UnixNano())
	ch := make(chan agent.ListProtonResult, 1)
	h.protonPending.Store(requestID, ch)
	defer h.protonPending.Delete(requestID)

	job := agent.ListProtonJob{RequestID: requestID}
	data, _ := json.Marshal(job)
	h.agentBroker.Send(agentID, "LIST_PROTON", string(data))

	select {
	case result := <-ch:
		if result.Error != "" {
			jsonErr(w, 422, result.Error)
			return
		}
		jsonOK(w, result.Versions)
	case <-time.After(10 * time.Second):
		jsonErr(w, 504, "agent did not respond in time")
	}
}

// ReceiveProtonResult is called by the agent with the Proton version list.
// POST /api/v3/agent/proton-result (agent-authenticated)
func (h *Handler) ReceiveProtonResult(w http.ResponseWriter, r *http.Request) {
	var result agent.ListProtonResult
	if err := decodeBody(r, &result); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v, ok := h.protonPending.Load(result.RequestID); ok {
		ch, ok2 := v.(chan agent.ListProtonResult)
		if !ok2 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		select {
		case ch <- result:
		default:
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Auth middleware for agent-only endpoints ----

func (h *Handler) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !isValidAgentSessionToken(token) {
			jsonErr(w, 401, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Helpers ----

// resolveServerURL returns the base URL the agent should use to call back.
// Handles Docker (r.Host may be a container name) and reverse-proxy setups.
func resolveServerURL(r *http.Request) string {
	if host := r.Header.Get("X-Forwarded-Host"); host != "" && isValidHostHeader(host) {
		scheme := "http"
		if r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		return scheme + "://" + host
	}

	host := r.Host
	if host != "" && !isContainerName(host) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + host
	}

	// Fall back to a local network interface IP
	if ip := localNetworkIP(); ip != "" {
		port := "5002"
		if parts := strings.SplitN(r.Host, ":", 2); len(parts) == 2 {
			port = parts[1]
		}
		return "http://" + ip + ":" + port
	}

	return "http://" + r.Host
}

// isValidHostHeader validates an X-Forwarded-Host value to prevent header injection.
// Accepts hostname:port or bare IP. Rejects anything with path/query characters.
func isValidHostHeader(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	if net.ParseIP(h) != nil {
		return true
	}
	// Validate each DNS label: letters, digits, hyphens only.
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 {
			return false
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return len(h) > 0
}

// isContainerName returns true if the host looks like a Docker container name
// (no dots, not localhost or a plain IP).
func isContainerName(host string) bool {
	bare := strings.SplitN(host, ":", 2)[0]
	if bare == "localhost" || bare == "127.0.0.1" || bare == "::1" {
		return false
	}
	return !strings.Contains(bare, ".")
}

// localNetworkIP picks the first non-loopback IPv4 address on the host.
func localNetworkIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return ""
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

// ---- installTempDirs typed helpers ----

func (h *Handler) setInstallTempDir(gameID int, dir string) {
	h.installTempDirs.Store(gameID, dir)
}

func (h *Handler) takeInstallTempDir(gameID int) (string, bool) {
	raw, ok := h.installTempDirs.LoadAndDelete(gameID)
	if !ok {
		return "", false
	}
	dir, ok := raw.(string)
	return dir, ok
}

// ---- Server-side archive extraction helpers ----

var serverArchiveExts = map[string]bool{".zip": true, ".rar": true, ".7z": true}

// containsArchives reports whether any file in the list is a top-level archive.
func containsArchives(files []string) bool {
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if serverArchiveExts[ext] {
			return true
		}
	}
	return false
}

// extractToTemp extracts all archives in srcDir into a new OS temp directory.
// Returns the temp dir path and the relative file list within it.
// The caller must call os.RemoveAll(tempDir) when done.
func extractToTemp(srcDir string) (tempDir string, files []string, err error) {
	tempDir, err = os.MkdirTemp("", "playerr-install-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	tool := serverFind7z()
	if tool == "" {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("7z not found; cannot extract server-side")
	}

	extracted := false
	_ = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !serverArchiveExts[ext] {
			return nil
		}
		if serverIsMultiPartNotFirst(filepath.Base(path)) {
			return nil
		}
		c := exec.Command(tool, "x", path, "-o"+tempDir, "-y")
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		if runErr := c.Run(); runErr != nil {
			log.Printf("[Install] 7z extraction failed for %s: %v", filepath.Base(path), runErr)
			return nil
		}
		extracted = true
		return nil
	})

	if !extracted {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("no archives extracted from %s", srcDir)
	}

	// Collect the resulting file list relative to tempDir.
	_ = filepath.Walk(tempDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(tempDir, path)
		if relErr == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return tempDir, files, nil
}

var (
	sevenZPath     string
	sevenZPathOnce sync.Once
)

// serverFind7z returns the path to a 7z binary, caching the result after the first lookup.
func serverFind7z() string {
	sevenZPathOnce.Do(func() {
		for _, name := range []string{"7z", "7za", "7zz"} {
			if p, err := exec.LookPath(name); err == nil {
				sevenZPath = p
				return
			}
		}
	})
	return sevenZPath
}


func serverIsMultiPartNotFirst(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, ".part") {
		return !strings.Contains(lower, ".part01.") && !strings.Contains(lower, ".part1.")
	}
	if len(lower) >= 4 {
		ext := lower[len(lower)-4:]
		if ext[0] == '.' && ext[1] >= '0' && ext[1] <= '9' {
			return ext != ".001"
		}
	}
	return false
}
