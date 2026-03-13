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

type TransmissionClient struct {
	cfg       domain.DownloadClientConfig
	rpcURL    string
	mu        sync.Mutex
	sessionID string
	http      *http.Client
}

func NewTransmissionClient(cfg domain.DownloadClientConfig) *TransmissionClient {
	base := buildBaseURL(cfg.Host, cfg.Port, "")
	return &TransmissionClient{
		cfg:    cfg,
		rpcURL: base + "/transmission/rpc",
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *TransmissionClient) rpc(method string, args any) ([]byte, error) {
	payload, _ := json.Marshal(map[string]any{"method": method, "arguments": args})

	var sessionID string
	c.mu.Lock()
	sessionID = c.sessionID
	c.mu.Unlock()

	doReq := func(sid string) (*http.Response, error) {
		req, err := http.NewRequest("POST", c.rpcURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.cfg.Username != "" {
			req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
		}
		if sid != "" {
			req.Header.Set("X-Transmission-Session-Id", sid)
		}
		return c.http.Do(req)
	}

	resp, err := doReq(sessionID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 409 {
		newSID := resp.Header.Get("X-Transmission-Session-Id")
		c.mu.Lock()
		c.sessionID = newSID
		c.mu.Unlock()
		resp.Body.Close()

		resp2, err := doReq(newSID)
		if err != nil {
			return nil, err
		}
		defer resp2.Body.Close()
		return io.ReadAll(resp2.Body)
	}

	return io.ReadAll(resp.Body)
}

func (c *TransmissionClient) TestConnection() (bool, string, error) {
	body, err := c.rpc("session-get", nil)
	if err != nil {
		return false, "", err
	}
	var resp struct {
		Result    string `json:"result"`
		Arguments struct {
			Version string `json:"version"`
		} `json:"arguments"`
	}
	json.Unmarshal(body, &resp)
	if resp.Result == "success" {
		return true, resp.Arguments.Version, nil
	}
	return false, "", nil
}

func (c *TransmissionClient) GetVersion() (string, error) {
	_, v, err := c.TestConnection()
	return v, err
}

func (c *TransmissionClient) AddTorrent(torrentURL, _ string) error {
	args := map[string]any{}
	if strings.HasPrefix(strings.ToLower(torrentURL), "magnet:?") {
		args["filename"] = torrentURL
	} else {
		args["filename"] = torrentURL
	}
	body, err := c.rpc("torrent-add", args)
	if err != nil {
		return err
	}
	var resp struct{ Result string `json:"result"` }
	json.Unmarshal(body, &resp)
	if resp.Result != "success" {
		return fmt.Errorf("transmission: %s", resp.Result)
	}
	return nil
}

func (c *TransmissionClient) AddNzb(_, _ string) error {
	return fmt.Errorf("transmission does not support NZB")
}

func (c *TransmissionClient) RemoveDownload(id string) error {
	args := map[string]any{"ids": []any{id}, "delete-local-data": true}
	_, err := c.rpc("torrent-remove", args)
	return err
}

func (c *TransmissionClient) PauseDownload(id string) error {
	_, err := c.rpc("torrent-stop", map[string]any{"ids": []any{id}})
	return err
}

func (c *TransmissionClient) ResumeDownload(id string) error {
	_, err := c.rpc("torrent-start", map[string]any{"ids": []any{id}})
	return err
}

func (c *TransmissionClient) GetDownloads() ([]domain.DownloadStatus, error) {
	args := map[string]any{
		"fields": []string{"id", "name", "totalSize", "percentDone", "status", "downloadDir"},
	}
	body, err := c.rpc("torrent-get", args)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Arguments struct {
			Torrents []struct {
				ID          int     `json:"id"`
				Name        string  `json:"name"`
				TotalSize   int64   `json:"totalSize"`
				PercentDone float64 `json:"percentDone"`
				Status      int     `json:"status"`
				DownloadDir string  `json:"downloadDir"`
			} `json:"torrents"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	var result []domain.DownloadStatus
	for _, t := range resp.Arguments.Torrents {
		result = append(result, domain.DownloadStatus{
			ID:           fmt.Sprint(t.ID),
			Name:         t.Name,
			Size:         t.TotalSize,
			Progress:     t.PercentDone * 100,
			State:        mapTransmissionState(t.Status),
			DownloadPath: t.DownloadDir,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapTransmissionState(s int) domain.DownloadState {
	switch s {
	case 0:
		return domain.DownloadStatePaused
	case 1, 2:
		return domain.DownloadStateChecking
	case 3:
		return domain.DownloadStateQueued
	case 4:
		return domain.DownloadStateDownloading
	case 5:
		return domain.DownloadStateQueued
	case 6:
		return domain.DownloadStateCompleted
	default:
		return domain.DownloadStateUnknown
	}
}
