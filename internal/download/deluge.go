package download

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kiwi3007/cargodeck/internal/domain"
)

type DelugeClient struct {
	cfg    domain.DownloadClientConfig
	rpcURL string
	mu     sync.Mutex
	cookie string
	http   *http.Client
	reqID  int
}

func NewDelugeClient(cfg domain.DownloadClientConfig) *DelugeClient {
	scheme := "http"
	if cfg.UseSsl {
		scheme = "https"
	}
	rpcURL := fmt.Sprintf("%s://%s:%d/json", scheme, cfg.Host, cfg.Port)
	return &DelugeClient{
		cfg:    cfg,
		rpcURL: rpcURL,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *DelugeClient) call(method string, params []any) ([]byte, error) {
	c.mu.Lock()
	c.reqID++
	id := c.reqID
	c.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{
		"method": method,
		"params": params,
		"id":     id,
	})

	req, err := http.NewRequest("POST", c.rpcURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Save session cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "_session_id" {
			c.mu.Lock()
			c.cookie = "_session_id=" + ck.Value
			c.mu.Unlock()
		}
	}

	return io.ReadAll(resp.Body)
}

func (c *DelugeClient) login() error {
	body, err := c.call("auth.login", []any{c.cfg.Password})
	if err != nil {
		return err
	}
	var resp struct{ Result bool `json:"result"` }
	json.Unmarshal(body, &resp)
	if !resp.Result {
		return fmt.Errorf("deluge auth failed")
	}
	return nil
}

func (c *DelugeClient) TestConnection() (bool, string, error) {
	if err := c.login(); err != nil {
		return false, "", err
	}
	body, err := c.call("daemon.info", nil)
	if err != nil {
		return false, "", err
	}
	var resp struct{ Result string `json:"result"` }
	json.Unmarshal(body, &resp)
	return true, resp.Result, nil
}

func (c *DelugeClient) GetVersion() (string, error) {
	_, v, err := c.TestConnection()
	return v, err
}

func (c *DelugeClient) AddTorrent(torrentURL, _ string) error {
	_ = c.login()
	params := []any{torrentURL, map[string]any{}}
	_, err := c.call("core.add_torrent_url", params)
	return err
}

func (c *DelugeClient) AddNzb(_, _ string) error {
	return fmt.Errorf("Deluge does not support NZB")
}

func (c *DelugeClient) RemoveDownload(id string) error {
	_ = c.login()
	_, err := c.call("core.remove_torrent", []any{id, true})
	return err
}

func (c *DelugeClient) PauseDownload(id string) error {
	_ = c.login()
	_, err := c.call("core.pause_torrent", []any{[]string{id}})
	return err
}

func (c *DelugeClient) ResumeDownload(id string) error {
	_ = c.login()
	_, err := c.call("core.resume_torrent", []any{[]string{id}})
	return err
}

func (c *DelugeClient) GetDownloads() ([]domain.DownloadStatus, error) {
	_ = c.login()
	fields := []string{"name", "total_size", "progress", "state", "label", "download_location"}
	body, err := c.call("core.get_torrents_status", []any{map[string]any{}, fields})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result map[string]struct {
			Name     string  `json:"name"`
			TotalSize int64  `json:"total_size"`
			Progress float64 `json:"progress"`
			State    string  `json:"state"`
			Label    string  `json:"label"`
			DLDir    string  `json:"download_location"`
		} `json:"result"`
	}
	json.Unmarshal(body, &resp)

	var result []domain.DownloadStatus
	for id, t := range resp.Result {
		result = append(result, domain.DownloadStatus{
			ID:           id,
			Name:         t.Name,
			Size:         t.TotalSize,
			Progress:     t.Progress,
			State:        mapDelugeState(t.State),
			Category:     t.Label,
			DownloadPath: t.DLDir,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapDelugeState(s string) domain.DownloadState {
	switch s {
	case "Downloading":
		return domain.DownloadStateDownloading
	case "Paused":
		return domain.DownloadStatePaused
	case "Seeding", "Finished":
		return domain.DownloadStateCompleted
	case "Error":
		return domain.DownloadStateError
	case "Queued":
		return domain.DownloadStateQueued
	case "Checking":
		return domain.DownloadStateChecking
	default:
		return domain.DownloadStateUnknown
	}
}
