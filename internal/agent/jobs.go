package agent

import (
	"sync"
	"time"
)

// JobStatus tracks the current phase of an install job.
type JobStatus string

const (
	JobQueued           JobStatus = "queued"
	JobDownloading      JobStatus = "downloading"
	JobExtracting       JobStatus = "extracting"
	JobInstalling       JobStatus = "installing"
	JobCreatingShortcut JobStatus = "creating_shortcut"
	JobDone             JobStatus = "done"
	JobFailed           JobStatus = "failed"
)

// InstallPath describes a storage location available on an agent.
type InstallPath struct {
	Path      string `json:"path"`
	Label     string `json:"label"`     // e.g. "Internal Storage", "SD Card"
	FreeBytes int64  `json:"freeBytes"` // -1 if unknown
}

// ActiveJob is the live state of an in-progress or recently completed install.
type ActiveJob struct {
	JobID     string    `json:"jobId"`
	GameTitle string    `json:"gameTitle"`
	Status    JobStatus `json:"status"`
	Message   string    `json:"message"`
	Percent   int       `json:"percent"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// InstallJob is the payload pushed to an agent's SSE stream.
type InstallJob struct {
	JobID      string   `json:"jobId"`
	AgentID    string   `json:"agentId"`
	GameID     int      `json:"gameId"`
	GameTitle  string   `json:"gameTitle"`
	Files      []string `json:"files"` // relative paths within game.Path
	ServerURL  string   `json:"serverUrl"`
	InstallDir string   `json:"installDir,omitempty"` // override default ~/Games/
}

// JobProgress is reported back by the agent via POST.
type JobProgress struct {
	JobID   string    `json:"jobId"`
	Status  JobStatus `json:"status"`
	Message string    `json:"message"`
	Percent int       `json:"percent"` // 0-100
}

// JobQueue holds a buffered channel per agent for pending install jobs.
type JobQueue struct {
	mu     sync.Mutex
	queues map[string]chan InstallJob
}

func NewJobQueue() *JobQueue {
	return &JobQueue{queues: make(map[string]chan InstallJob)}
}

// GetOrCreate returns the job channel for an agent (creates it on first access).
func (q *JobQueue) GetOrCreate(agentID string) chan InstallJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ch, ok := q.queues[agentID]; ok {
		return ch
	}
	ch := make(chan InstallJob, 32)
	q.queues[agentID] = ch
	return ch
}

// Enqueue adds a job to the agent's queue. Returns false if queue is full.
func (q *JobQueue) Enqueue(job InstallJob) bool {
	ch := q.GetOrCreate(job.AgentID)
	select {
	case ch <- job:
		return true
	default:
		return false
	}
}
