package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kiwi3007/playerr/internal/agent"
)

// ---- Register ----

type registerRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	SteamPath string `json:"steamPath"`
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
		ID:        req.ID,
		Name:      req.Name,
		Platform:  req.Platform,
		SteamPath: req.SteamPath,
	})

	log.Printf("[Agent] Registered: %s (%s) platform=%s", req.Name, req.ID, req.Platform)
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
		log.Printf("[Agent] SSE stream closed: %s", agentID)
	})
}

// ---- Dispatch install job ----

type dispatchRequest struct {
	GameID int `json:"gameId"`
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

	serverURL := resolveServerURL(r)
	jobID := fmt.Sprintf("%d-%d", req.GameID, time.Now().UnixMilli())

	job := agent.InstallJob{
		JobID:     jobID,
		AgentID:   agentID,
		GameID:    req.GameID,
		GameTitle: game.Title,
		Files:     files,
		ServerURL: serverURL,
	}

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

// ---- Progress report from agent ----

func (h *Handler) AgentProgress(w http.ResponseWriter, r *http.Request) {
	var prog agent.JobProgress
	if err := decodeBody(r, &prog); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	data, _ := json.Marshal(prog)
	h.broker.Publish("AGENT_PROGRESS", string(data))

	log.Printf("[Agent] Progress %s: %s %d%%", prog.JobID, prog.Status, prog.Percent)
	w.WriteHeader(http.StatusNoContent)
}

// ---- Agent settings (masked token for browser) ----

func (h *Handler) GetAgentSettings(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadAgent()
	jsonOK(w, map[string]string{"token": cfg.Token})
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
