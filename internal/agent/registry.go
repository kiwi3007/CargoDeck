package agent

import (
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
	GameTitle string
}

// Registry holds all known agents (in-memory).
type Registry struct {
	mu       sync.RWMutex
	agents   map[string]*AgentInfo
	jobIndex map[string]jobMeta // jobID → (agentID, gameTitle)
}

func NewRegistry() *Registry {
	return &Registry{
		agents:   make(map[string]*AgentInfo),
		jobIndex: make(map[string]jobMeta),
	}
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
	}
}

// TrackJob records that a job has been dispatched to an agent.
// This lets UpdateJobProgress look up which agent owns a given job ID.
func (r *Registry) TrackJob(agentID, jobID, gameTitle string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobIndex[jobID] = jobMeta{AgentID: agentID, GameTitle: gameTitle}
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
