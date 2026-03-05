package download

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
)

// FloodClient uses Flood's REST API.
type FloodClient struct {
	cfg    domain.DownloadClientConfig
	apiURL string
	mu     sync.Mutex
	cookie string
	http   *http.Client
}

func NewFloodClient(cfg domain.DownloadClientConfig) *FloodClient {
	base := buildBaseURL(cfg.Host, cfg.Port, cfg.UrlBase)
	return &FloodClient{
		cfg:    cfg,
		apiURL: base + "/api",
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *FloodClient) login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cookie != "" {
		return nil
	}

	payload, _ := json.Marshal(map[string]string{
		"username": c.cfg.Username,
		"password": c.cfg.Password,
	})
	req, _ := http.NewRequest("POST", c.apiURL+"/auth/authenticate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, ck := range resp.Cookies() {
		if strings.Contains(strings.ToLower(ck.Name), "session") || ck.Name == "jwt" {
			c.cookie = ck.Name + "=" + ck.Value
			return nil
		}
	}
	// Try any cookie
	for _, ck := range resp.Cookies() {
		c.cookie = ck.Name + "=" + ck.Value
		return nil
	}
	return nil
}

func (c *FloodClient) do(method, path string, body io.Reader) (*http.Response, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.apiURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	return c.http.Do(req)
}

func (c *FloodClient) TestConnection() (bool, string, error) {
	resp, err := c.do("GET", "/client/connection-test", nil)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, "", nil
}

func (c *FloodClient) GetVersion() (string, error) {
	return "Flood", nil
}

func (c *FloodClient) AddTorrent(torrentURL, _ string) error {
	payload, _ := json.Marshal(map[string]any{
		"urls": []string{torrentURL},
	})
	resp, err := c.do("POST", "/torrents/add-urls", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("flood add torrent returned %d", resp.StatusCode)
	}
	return nil
}

func (c *FloodClient) AddNzb(_, _ string) error {
	return fmt.Errorf("Flood does not support NZB")
}

func (c *FloodClient) RemoveDownload(id string) error {
	payload, _ := json.Marshal(map[string]any{
		"hashes":      []string{id},
		"deleteData":  true,
	})
	resp, err := c.do("DELETE", "/torrents", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *FloodClient) PauseDownload(id string) error {
	payload, _ := json.Marshal(map[string]any{"hashes": []string{id}})
	resp, err := c.do("POST", "/torrents/stop", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *FloodClient) ResumeDownload(id string) error {
	payload, _ := json.Marshal(map[string]any{"hashes": []string{id}})
	resp, err := c.do("POST", "/torrents/start", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *FloodClient) GetDownloads() ([]domain.DownloadStatus, error) {
	resp, err := c.do("GET", "/torrents", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Torrents map[string]struct {
			Name        string  `json:"name"`
			SizeBytes   int64   `json:"sizeBytes"`
			PercentComplete float64 `json:"percentComplete"`
			Status      []string `json:"status"`
			Tags        []string `json:"tags"`
			Directory   string  `json:"directory"`
		} `json:"torrents"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	var result []domain.DownloadStatus
	for hash, t := range data.Torrents {
		cat := ""
		if len(t.Tags) > 0 {
			cat = t.Tags[0]
		}
		result = append(result, domain.DownloadStatus{
			ID:           hash,
			Name:         t.Name,
			Size:         t.SizeBytes,
			Progress:     t.PercentComplete,
			State:        mapFloodState(t.Status),
			Category:     cat,
			DownloadPath: t.Directory,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapFloodState(status []string) domain.DownloadState {
	for _, s := range status {
		switch strings.ToLower(s) {
		case "downloading":
			return domain.DownloadStateDownloading
		case "stopped":
			return domain.DownloadStatePaused
		case "seeding", "complete":
			return domain.DownloadStateCompleted
		case "error":
			return domain.DownloadStateError
		case "checking":
			return domain.DownloadStateChecking
		}
	}
	return domain.DownloadStateUnknown
}
