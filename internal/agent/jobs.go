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
	LaunchArgs  string   `json:"launchArgs,omitempty"`  // extra args appended to exe in run.sh
	EnvVars     string   `json:"envVars,omitempty"`     // KEY=VALUE lines exported before the exe
	ProtonPath  string   `json:"protonPath,omitempty"`  // explicit proton binary override; empty = auto-select
	SteamID     int      `json:"steamId,omitempty"`     // Steam App ID for protonfixes; 0 = unknown
}

// ProtonVersionInfo describes a single Proton installation on an agent.
type ProtonVersionInfo struct {
	Name    string `json:"name"`
	BinPath string `json:"binPath"`
}

// SetupAccelaJob is pushed to the agent to install ACCELA + SLSsteam via enter-the-wired.
type SetupAccelaJob struct {
	JobID string `json:"jobId"`
}

// ManifestEntry describes one depot included in a steamtoolz manifest ZIP.
type ManifestEntry struct {
	DepotID     int    `json:"depotId"`
	DepotKey    string `json:"depotKey"`    // hex depot encryption key
	ManifestGID string `json:"manifestGid"` // manifest version ID
	AppToken    int64  `json:"appToken"`    // Steam app access token
}

// SteamDownloadJob is pushed to the agent to download a game from Steam CDN
// via DepotDownloaderMod (with manifest data) or anonymously via DepotDownloader.
type SteamDownloadJob struct {
	JobID           string          `json:"jobId"`
	AppID           int             `json:"appId"`
	GameTitle       string          `json:"gameTitle"`            // used as the steamapps/common subfolder name
	InstallDir      string          `json:"installDir"`           // override default steamapps/common location
	OS              string          `json:"os"`                   // "linux" or "windows"; defaults to "linux"
	ManifestEntries []ManifestEntry `json:"manifestEntries,omitempty"` // non-empty = use DepotDownloaderMod
	ManifestGameID  int             `json:"manifestGameId,omitempty"`  // CargoDeck gameId for manifest-zip download
}

// ListProtonJob is pushed to the agent to enumerate available Proton versions.
type ListProtonJob struct {
	RequestID string `json:"requestId"`
}

// ListProtonResult is posted back by the agent with the enumerated versions.
type ListProtonResult struct {
	RequestID string              `json:"requestId"`
	Versions  []ProtonVersionInfo `json:"versions"`
	Error     string              `json:"error,omitempty"`
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

// ReadScriptJob is pushed to the agent to fetch the run.sh/run.bat script for a game.
type ReadScriptJob struct {
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

// RenamePrefixJob instructs the agent to rename a game's Wine prefix directory
// from the old numeric format (prefix_{gameId}) to the title-based format
// (prefix_{safeTitle}) and update the run.sh launcher script to match.
type RenamePrefixJob struct {
	GameID    int    `json:"gameId"`
	GameTitle string `json:"gameTitle"`
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
