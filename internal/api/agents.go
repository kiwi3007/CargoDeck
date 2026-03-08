package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kiwi3007/playerr/internal/agent"
	"github.com/kiwi3007/playerr/internal/steamgriddb"
)

// ---- Register ----

type registerRequest struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Platform     string              `json:"platform"`
	SteamPath    string              `json:"steamPath"`
	Version      string              `json:"version,omitempty"`
	InstallPaths []agent.InstallPath `json:"installPaths,omitempty"`
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

	h.agentRegistry.Register(agent.AgentInfo{
		ID:           req.ID,
		Name:         req.Name,
		Platform:     req.Platform,
		SteamPath:    req.SteamPath,
		Version:      req.Version,
		InstallPaths: req.InstallPaths,
	})

	log.Printf("[Agent] Registered: %s (%s) platform=%s version=%s paths=%d",
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
	jsonOK(w, map[string]string{"agentId": req.ID})
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

	serverURL := resolveServerURL(r)
	jobID := fmt.Sprintf("%d-%d", req.GameID, time.Now().UnixMilli())

	job := agent.InstallJob{
		JobID:       jobID,
		AgentID:     agentID,
		GameID:      req.GameID,
		GameTitle:   game.Title,
		Files:       files,
		ServerURL:   serverURL,
		InstallDir:  req.InstallDir,
		SelectedExe: req.SelectedExe,
	}

	h.agentRegistry.TrackJob(agentID, jobID, game.Title)

	if !h.agentJobs.Enqueue(job) {
		jsonErr(w, 503, "agent job queue full")
		return
	}

	// Push directly via SSE if agent is already connected
	if data, err := json.Marshal(job); err == nil {
		h.agentBroker.Send(agentID, "INSTALL_JOB", string(data))
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

// ---- Dispatch scan job ----

func (h *Handler) DispatchScan(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentBroker.Send(agentID, "SCAN_GAMES", "{}")
	log.Printf("[Agent] Dispatched SCAN_GAMES → %s", agentID)
	jsonOK(w, map[string]string{"message": "scan requested"})
}

// ---- Dispatch shortcut refresh ----

func (h *Handler) DispatchRefreshShortcuts(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentBroker.Send(agentID, "REFRESH_SHORTCUTS", "{}")
	log.Printf("[Agent] Dispatched REFRESH_SHORTCUTS → %s", agentID)
	jsonOK(w, map[string]string{"message": "shortcut refresh requested"})
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

	// Persist detected versions to DB
	for _, ig := range games {
		if ig.Version == "" {
			continue
		}
		game, err := h.repo.GetGameByTitle(ig.Title)
		if err != nil || game == nil {
			continue
		}
		if game.CurrentVersion != ig.Version {
			_ = h.repo.UpdateGameVersion(game.ID, ig.Version)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- Dispatch Steam restart ----

func (h *Handler) DispatchRestartSteam(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")
	if _, ok := h.agentRegistry.Get(agentID); !ok {
		jsonErr(w, 404, "agent not found")
		return
	}
	h.agentBroker.Send(agentID, "RESTART_STEAM", "{}")
	log.Printf("[Agent] Dispatched RESTART_STEAM → %s", agentID)
	jsonOK(w, map[string]string{"message": "Steam restart requested"})
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
	h.agentBroker.Send(agentID, "REGEN_SCRIPTS", "{}")
	log.Printf("[Agent] Dispatched REGEN_SCRIPTS → %s", agentID)
	jsonOK(w, map[string]string{"message": "script regeneration requested"})
}

// ---- Dispatch read-log job ----

func (h *Handler) DispatchReadLog(w http.ResponseWriter, r *http.Request) {
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
	requestID := fmt.Sprintf("log-%d", time.Now().UnixMilli())
	job := agent.ReadLogJob{RequestID: requestID, GameTitle: req.GameTitle}
	data, _ := json.Marshal(job)
	h.agentBroker.Send(agentID, "READ_LOG", string(data))
	log.Printf("[Agent] Dispatched READ_LOG %q → %s (req=%s)", req.GameTitle, agentID, requestID)
	jsonOK(w, map[string]string{"requestId": requestID})
}

// ---- Receive log content from agent ----

func (h *Handler) ReceiveAgentLog(w http.ResponseWriter, r *http.Request) {
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
	h.broker.Publish("AGENT_LOG_DATA", string(data))
	w.WriteHeader(http.StatusNoContent)
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

// ---- Progress report from agent ----

func (h *Handler) AgentProgress(w http.ResponseWriter, r *http.Request) {
	var prog agent.JobProgress
	if err := decodeBody(r, &prog); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	h.agentRegistry.UpdateJobProgress(prog)
	h.publishAgentList()

	data, _ := json.Marshal(prog)
	h.broker.Publish("AGENT_PROGRESS", string(data))

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

// ---- Auth middleware for agent-only endpoints ----

func (h *Handler) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := h.cfg.LoadAgent()
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != cfg.Token {
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
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
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
