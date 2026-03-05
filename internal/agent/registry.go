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

// AgentInfo describes a connected remote agent.
type AgentInfo struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Platform  string      `json:"platform"`
	SteamPath string      `json:"steamPath"`
	Status    AgentStatus `json:"status"`
	LastSeen  time.Time   `json:"lastSeen"`
}

// Registry holds all known agents (in-memory).
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentInfo
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]*AgentInfo)}
}

// Register inserts or updates an agent entry and marks it online.
func (r *Registry) Register(info AgentInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info.Status = StatusOnline
	info.LastSeen = time.Now()
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

// List returns all registered agents.
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
