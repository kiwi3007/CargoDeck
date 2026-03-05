// Package agentclient implements the Playerr agent: SSE-driven install job runner.
package agentclient

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kiwi3007/playerr/internal/agent"
	"github.com/kiwi3007/playerr/internal/launcher"
)

var cryptoRandRead = rand.Read

// Config holds agent startup configuration.
type Config struct {
	ServerURL string
	Token     string
	Name      string
}

// Client is the agent runtime.
type Client struct {
	cfg     Config
	agentID string
	stopped atomic.Bool
	stopCh  chan struct{}
	http    *http.Client
}

// New creates a new agent client, loading or generating a stable agent ID.
func New(cfg Config) (*Client, error) {
	id, err := loadOrCreateID()
	if err != nil {
		return nil, fmt.Errorf("agent ID: %w", err)
	}
	return &Client{
		cfg:     cfg,
		agentID: id,
		stopCh:  make(chan struct{}),
		http:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Stop signals the agent to stop reconnecting.
func (c *Client) Stop() {
	c.stopped.Store(true)
	close(c.stopCh)
}

// Run registers with the server and maintains the SSE connection with exponential backoff.
// This is the only long-running goroutine in idle state.
func (c *Client) Run() {
	backoff := time.Second
	maxBackoff := 60 * time.Second

	for !c.stopped.Load() {
		if err := c.register(); err != nil {
			log.Printf("[Agent] Register failed: %v — retrying in %s", err, backoff)
			c.sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
			continue
		}

		log.Printf("[Agent] Connected to %s as %q (id=%s)", c.cfg.ServerURL, c.cfg.Name, c.agentID)
		backoff = time.Second // reset on successful connect

		if err := c.listenSSE(); err != nil && !c.stopped.Load() {
			log.Printf("[Agent] SSE disconnected: %v — reconnecting in %s", err, backoff)
		}

		if !c.stopped.Load() {
			c.sleep(backoff)
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// register calls POST /api/v3/agent/register.
func (c *Client) register() error {
	body, _ := json.Marshal(map[string]string{
		"id":        c.agentID,
		"name":      c.cfg.Name,
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
		"steamPath": launcher.FindSteamRoot(),
	})

	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// listenSSE opens the SSE stream and dispatches jobs as they arrive.
// Returns when the stream closes or an error occurs.
func (c *Client) listenSSE() error {
	url := fmt.Sprintf("%s/api/v3/agent/events?agentId=%s", c.cfg.ServerURL, c.agentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Cache-Control", "no-cache")

	// Use a client with no timeout for the SSE stream
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SSE HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	log.Println("[Agent] SSE stream established — idle (waiting for jobs)")

	scanner := bufio.NewScanner(resp.Body)
	var eventType, dataLine string

	for scanner.Scan() {
		if c.stopped.Load() {
			return nil
		}

		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			// End of event — dispatch if we have an event type and data
			if eventType == "INSTALL_JOB" && dataLine != "" {
				var job agent.InstallJob
				if err := json.Unmarshal([]byte(dataLine), &job); err != nil {
					log.Printf("[Agent] Bad job JSON: %v", err)
				} else {
					go c.executeJob(job)
				}
			}
			eventType = ""
			dataLine = ""
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

// executeJob runs an install job: download → extract if needed → run installer → shortcut.
func (c *Client) executeJob(job agent.InstallJob) {
	log.Printf("[Agent] Job %s: installing %q", job.JobID, job.GameTitle)

	installDir := filepath.Join(homeDir(), "Games", safeName(job.GameTitle))
	if err := os.MkdirAll(installDir, 0755); err != nil {
		c.reportProgress(job, agent.JobFailed, "Cannot create install dir: "+err.Error(), 0)
		return
	}

	// ---- Download files ----
	c.reportProgress(job, agent.JobDownloading, "Downloading files...", 0)
	total := len(job.Files)
	for i, relPath := range job.Files {
		pct := (i * 80) / total
		c.reportProgress(job, agent.JobDownloading, "Downloading: "+relPath, pct)

		destPath := filepath.Join(installDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			c.reportProgress(job, agent.JobFailed, "mkdir failed: "+err.Error(), pct)
			return
		}

		url := fmt.Sprintf("%s/api/v3/game/%d/file?path=%s", job.ServerURL, job.GameID, url.QueryEscape(relPath))
		if err := c.downloadFile(url, destPath); err != nil {
			c.reportProgress(job, agent.JobFailed, "Download failed: "+err.Error(), pct)
			return
		}
	}

	// ---- Find and run installer ----
	c.reportProgress(job, agent.JobInstalling, "Looking for installer...", 85)
	installer := findInstaller(installDir)
	if installer != "" {
		c.reportProgress(job, agent.JobInstalling, "Running installer: "+filepath.Base(installer), 87)
		if err := runInstaller(installer, job.GameID); err != nil {
			log.Printf("[Agent] Installer error (non-fatal): %v", err)
		}
	}

	// ---- Create Steam shortcut ----
	c.reportProgress(job, agent.JobCreatingShortcut, "Creating Steam shortcut...", 95)
	shortcutURL := fmt.Sprintf("%s/api/v3/game/%d/shortcut", job.ServerURL, job.GameID)
	req, _ := http.NewRequest("POST", shortcutURL, nil)
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}

	c.reportProgress(job, agent.JobDone, "Install complete. Files in: "+installDir, 100)
	log.Printf("[Agent] Job %s: done", job.JobID)
}

// downloadFile downloads a URL to destPath, resuming if the file already exists.
func (c *Client) downloadFile(url, destPath string) error {
	tmpPath := destPath + ".tmp"

	// Check for existing partial download
	var offset int64
	if fi, err := os.Stat(tmpPath); err == nil {
		offset = fi.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	dlClient := &http.Client{} // no timeout for file downloads
	resp, err := dlClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == 206 {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(tmpPath, flags, 0644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	f.Close()

	return os.Rename(tmpPath, destPath)
}

// reportProgress POSTs a JobProgress to the server.
func (c *Client) reportProgress(job agent.InstallJob, status agent.JobStatus, message string, pct int) {
	prog := agent.JobProgress{
		JobID:   job.JobID,
		Status:  status,
		Message: message,
		Percent: pct,
	}
	body, _ := json.Marshal(prog)
	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v3/agent/progress", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// sleep blocks until duration elapses or the agent is stopped.
func (c *Client) sleep(d time.Duration) {
	select {
	case <-time.After(d):
	case <-c.stopCh:
	}
}

// ---- Helpers ----

func findInstaller(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		if strings.HasSuffix(lower, ".exe") &&
			(strings.HasPrefix(lower, "setup") || strings.HasPrefix(lower, "install")) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func runInstaller(path string, gameID int) error {
	suffix := fmt.Sprintf("playerr_%d", gameID)
	if cmd := launcher.TryProton(path, suffix); cmd != nil {
		return cmd.Run()
	}
	if w, err := exec.LookPath("wine"); err == nil {
		cmd := exec.Command(w, path)
		cmd.Dir = filepath.Dir(path)
		return cmd.Run()
	}
	log.Printf("[Agent] No Proton or Wine found — skipping installer %s", path)
	return nil
}

func safeName(title string) string {
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

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// ---- Stable agent ID ----

func loadOrCreateID() (string, error) {
	idPath := agentIDPath()
	if data, err := os.ReadFile(idPath); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	}

	// Generate new UUID-like ID
	b := make([]byte, 16)
	if _, err := cryptoRandRead(b); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

	_ = os.MkdirAll(filepath.Dir(idPath), 0755)
	_ = os.WriteFile(idPath, []byte(id), 0600)
	return id, nil
}

func agentIDPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "playerr-agent", "id")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "playerr-agent", "id")
	default:
		return filepath.Join(home, ".config", "playerr-agent", "id")
	}
}
