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
	JobID       string   `json:"jobId"`
	AgentID     string   `json:"agentId"`
	GameID      int      `json:"gameId"`
	GameTitle   string   `json:"gameTitle"`
	Files       []string `json:"files"` // relative paths within game.Path
	ServerURL   string   `json:"serverUrl"`
	InstallDir  string   `json:"installDir,omitempty"`  // override default ~/Games/
	SelectedExe string   `json:"selectedExe,omitempty"` // basename; empty = auto-detect
}

// JobProgress is reported back by the agent via POST.
type JobProgress struct {
	JobID   string    `json:"jobId"`
	Status  JobStatus `json:"status"`
	Message string    `json:"message"`
	Percent int       `json:"percent"` // 0-100
}

// InstalledGame describes a game installed on an agent device.
type InstalledGame struct {
	Title         string   `json:"title"`
	InstallPath   string   `json:"installPath"` // full path to the game directory
	ExePath       string   `json:"exePath,omitempty"`
	ExeCandidates []string `json:"exeCandidates,omitempty"` // all selectable game exes
	ScriptPath    string   `json:"scriptPath,omitempty"`
	SizeBytes     int64    `json:"sizeBytes"`
	HasShortcut   bool     `json:"hasShortcut"`
	Version       string   `json:"version,omitempty"`
}

// ReadLogJob is pushed to the agent to fetch the run.log for a game.
type ReadLogJob struct {
	RequestID string `json:"requestId"`
	GameTitle string `json:"gameTitle"`
}

// DeleteGameJob is pushed to agent via SSE to delete a game from a device.
type DeleteGameJob struct {
	JobID          string `json:"jobId"`
	Title          string `json:"title"`
	InstallPath    string `json:"installPath"` // full path to game directory
	RemoveShortcut bool   `json:"removeShortcut"`
}

// BrowseDirJob is pushed to the agent to list a remote directory.
type BrowseDirJob struct {
	RequestID string `json:"requestId"`
	Path      string `json:"path"` // "~" means home dir
}

// DirEntry is a single item returned by the agent's directory listing.
type DirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// BrowseDirResult is posted back by the agent after listing a directory.
type BrowseDirResult struct {
	RequestID string     `json:"requestId"`
	Path      string     `json:"path"`
	Entries   []DirEntry `json:"entries"`
	Error     string     `json:"error,omitempty"`
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
