package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AgentStatus represents whether an agent is currently connected.
type AgentStatus string

const (
	StatusOnline  AgentStatus = "online"
	StatusOffline AgentStatus = "offline"
)

const maxRecentJobs = 10

// AgentInfo describes a connected remote agent.
type AgentInfo struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Platform       string          `json:"platform"`
	SteamPath      string          `json:"steamPath"`
	Version        string          `json:"version,omitempty"`
	Status         AgentStatus     `json:"status"`
	LastSeen       time.Time       `json:"lastSeen"`
	InstallPaths   []InstallPath   `json:"installPaths,omitempty"`
	CurrentJob     *ActiveJob      `json:"currentJob,omitempty"`
	RecentJobs     []ActiveJob     `json:"recentJobs,omitempty"`
	InstalledGames []InstalledGame `json:"installedGames,omitempty"`
	LastScanned    *time.Time      `json:"lastScanned,omitempty"`
}

// jobMeta links a job ID to the agent and game title.
type jobMeta struct {
	AgentID   string
	GameID    int
	GameTitle string
}

// Registry holds all known agents, persisted to {dir}/agents.json.
type Registry struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInfo
	jobIndex map[string]jobMeta // jobID → (agentID, gameTitle)
	dir      string
}

// NewRegistry creates a registry. If dir is non-empty, agents are persisted to
// {dir}/agents.json and loaded on startup (all marked offline).
func NewRegistry(dir string) *Registry {
	r := &Registry{
		agents:   make(map[string]*AgentInfo),
		jobIndex: make(map[string]jobMeta),
		dir:      dir,
	}
	if dir != "" {
		r.load()
	}
	return r
}

// load reads persisted agents from disk and marks them all offline.
func (r *Registry) load() {
	data, err := os.ReadFile(filepath.Join(r.dir, "agents.json"))
	if err != nil {
		return
	}
	var list []AgentInfo
	if err := json.Unmarshal(data, &list); err != nil {
		return
	}
	for _, a := range list {
		a.Status = StatusOffline
		a.CurrentJob = nil // don't restore in-flight jobs
		cp := a
		r.agents[a.ID] = &cp
	}
}

// save writes the current agent list to disk. Must be called with mu held.
func (r *Registry) save() {
	if r.dir == "" {
		return
	}
	list := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		list = append(list, *a)
	}
	data, err := json.Marshal(list)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(r.dir, "agents.json"), data, 0o644)
}

// Remove deletes an agent from the registry permanently.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, id)
	r.save()
}

// Register inserts or updates an agent entry and marks it online.
func (r *Registry) Register(info AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info.Status = StatusOnline
	info.LastSeen = time.Now()
	// Preserve job history if agent re-registers
	if existing, ok := r.agents[info.ID]; ok {
		info.CurrentJob = existing.CurrentJob
		info.RecentJobs = existing.RecentJobs
	}
	r.agents[info.ID] = &info
	r.save()
}

// SetOnline marks an agent as online and refreshes LastSeen.
func (r *Registry) SetOnline(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[id]; ok {
		a.Status = StatusOnline
		a.LastSeen = time.Now()
	}
}

// SetOffline marks an agent as offline.
func (r *Registry) SetOffline(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[id]; ok {
		a.Status = StatusOffline
		r.save()
	}
}

// Get returns a copy of an agent by ID.
func (r *Registry) Get(id string) (*AgentInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[id]
	if !ok {
		return nil, false
	}
	cp := *a
	return &cp, true
}

// List returns all registered agents sorted by name.
func (r *Registry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentInfo, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, *a)
	}
	return out
}

// OnlineCount returns the number of online agents.
func (r *Registry) OnlineCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, a := range r.agents {
		if a.Status == StatusOnline {
			count++
		}
	}
	return count
}

// SetInstalledGames stores the scan result for an agent.
func (r *Registry) SetInstalledGames(agentID string, games []InstalledGame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.agents[agentID]; ok {
		a.InstalledGames = games
		now := time.Now()
		a.LastScanned = &now
		r.save()
	}
}

// HasActiveJobForGame returns true if the agent already has a queued or
// in-progress job for the given game, preventing duplicate dispatches.
func (r *Registry) HasActiveJobForGame(agentID string, gameID int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.jobIndex {
		if m.AgentID == agentID && m.GameID == gameID {
			return true
		}
	}
	return false
}

// GetJobGameID returns the game ID associated with a pending job.
// Must be called before UpdateJobProgress, which removes the entry on completion.
func (r *Registry) GetJobGameID(jobID string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.jobIndex[jobID]
	return m.GameID, ok
}

// TrackJob records that a job has been dispatched to an agent.
// This lets UpdateJobProgress look up which agent owns a given job ID.
func (r *Registry) TrackJob(agentID, jobID, gameTitle string, gameID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobIndex[jobID] = jobMeta{AgentID: agentID, GameID: gameID, GameTitle: gameTitle}
	if a, ok := r.agents[agentID]; ok {
		job := &ActiveJob{
			JobID:     jobID,
			GameTitle: gameTitle,
			Status:    JobQueued,
			Message:   "Queued",
			Percent:   0,
			UpdatedAt: time.Now(),
		}
		a.CurrentJob = job
	}
}

// UpdateJobProgress updates the live status of a job from an agent progress report.
// Completed/failed jobs are moved to RecentJobs.
func (r *Registry) UpdateJobProgress(prog JobProgress) {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta, ok := r.jobIndex[prog.JobID]
	if !ok {
		return
	}
	a, ok := r.agents[meta.AgentID]
	if !ok {
		return
	}

	switch prog.Status {
	case JobDone, JobFailed:
		finished := ActiveJob{
			JobID:     prog.JobID,
			GameTitle: meta.GameTitle,
			Status:    prog.Status,
			Message:   prog.Message,
			Percent:   prog.Percent,
			UpdatedAt: time.Now(),
		}
		// Prepend to recent jobs, cap at maxRecentJobs
		a.RecentJobs = append([]ActiveJob{finished}, a.RecentJobs...)
		if len(a.RecentJobs) > maxRecentJobs {
			a.RecentJobs = a.RecentJobs[:maxRecentJobs]
		}
		a.CurrentJob = nil
		delete(r.jobIndex, prog.JobID)
	default:
		// Always update currentJob to reflect what the agent is actually running.
		// Without this, dispatching a new job while an old one is still in-flight
		// would leave currentJob at the new job's 0%/queued state, causing the
		// AGENTS_UPDATED event and the direct AGENT_PROGRESS event to carry
		// different percent values and make the UI oscillate between them.
		a.CurrentJob = &ActiveJob{
			JobID:     prog.JobID,
			GameTitle: meta.GameTitle,
			Status:    prog.Status,
			Message:   prog.Message,
			Percent:   prog.Percent,
			UpdatedAt: time.Now(),
		}
	}
}
