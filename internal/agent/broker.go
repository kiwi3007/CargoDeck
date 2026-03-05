package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// AgentBroker routes SSE events to specific agents (not fan-out).
type AgentBroker struct {
	mu      sync.Mutex
	streams map[string]chan string
}

func NewAgentBroker() *AgentBroker {
	return &AgentBroker{streams: make(map[string]chan string)}
}

func (b *AgentBroker) subscribe(agentID string) chan string {
	ch := make(chan string, 32)
	b.mu.Lock()
	b.streams[agentID] = ch
	b.mu.Unlock()
	return ch
}

func (b *AgentBroker) unsubscribe(agentID string) {
	b.mu.Lock()
	if ch, ok := b.streams[agentID]; ok {
		delete(b.streams, agentID)
		close(ch)
	}
	b.mu.Unlock()
}

// IsConnected returns true if the agent has an active SSE stream.
func (b *AgentBroker) IsConnected(agentID string) bool {
	b.mu.Lock()
	_, ok := b.streams[agentID]
	b.mu.Unlock()
	return ok
}

// Send pushes a named event to a specific agent's stream.
// Returns false if the agent is not connected or its buffer is full.
func (b *AgentBroker) Send(agentID, event, data string) bool {
	b.mu.Lock()
	ch, ok := b.streams[agentID]
	b.mu.Unlock()
	if !ok {
		return false
	}
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	select {
	case ch <- msg:
		return true
	default:
		return false
	}
}

// ServeAgent streams SSE events to a connected agent and forwards queued jobs.
// onDisconnect is called when the connection closes (use to mark agent offline).
func (b *AgentBroker) ServeAgent(
	agentID string,
	w http.ResponseWriter,
	r *http.Request,
	jobCh <-chan InstallJob,
	onDisconnect func(),
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ch := b.subscribe(agentID)
	defer func() {
		b.unsubscribe(agentID)
		if onDisconnect != nil {
			onDisconnect()
		}
	}()

	// Drain any jobs that arrived before this SSE connection was established.
	drainJobs:
	for {
		select {
		case job, ok := <-jobCh:
			if !ok {
				break drainJobs
			}
			if data, err := json.Marshal(job); err == nil {
				fmt.Fprintf(w, "event: INSTALL_JOB\ndata: %s\n\n", data)
				flusher.Flush()
			}
		default:
			break drainJobs
		}
	}

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()

		case job, ok := <-jobCh:
			if !ok {
				return
			}
			if data, err := json.Marshal(job); err == nil {
				fmt.Fprintf(w, "event: INSTALL_JOB\ndata: %s\n\n", data)
				flusher.Flush()
			}

		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
