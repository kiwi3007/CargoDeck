package download

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
)

type NzbgetClient struct {
	cfg    domain.DownloadClientConfig
	rpcURL string
	http   *http.Client
}

func NewNzbgetClient(cfg domain.DownloadClientConfig) *NzbgetClient {
	base := buildBaseURL(cfg.Host, cfg.Port, cfg.UrlBase)
	rpc := fmt.Sprintf("%s/jsonrpc", base)
	// NZBGet uses HTTP Basic auth embedded in URL
	if cfg.Username != "" {
		// Rewrite base with credentials
		_ = rpc
		rpc = fmt.Sprintf("%s/%s:%s@%s:%d%s/jsonrpc",
			scheme(cfg.Host), cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.UrlBase)
	}
	return &NzbgetClient{
		cfg:    cfg,
		rpcURL: rpc,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func scheme(host string) string {
	if len(host) > 8 && host[:8] == "https://" {
		return "https"
	}
	return "http"
}

func (c *NzbgetClient) call(method string, params []any) ([]byte, error) {
	payload, _ := json.Marshal(map[string]any{
		"version": "1.1",
		"method":  method,
		"params":  params,
	})
	resp, err := c.http.Post(c.rpcURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *NzbgetClient) TestConnection() (bool, string, error) {
	body, err := c.call("version", nil)
	if err != nil {
		return false, "", err
	}
	var resp struct{ Result string `json:"result"` }
	json.Unmarshal(body, &resp)
	return resp.Result != "", resp.Result, nil
}

func (c *NzbgetClient) GetVersion() (string, error) {
	_, v, err := c.TestConnection()
	return v, err
}

func (c *NzbgetClient) AddTorrent(_, _ string) error {
	return fmt.Errorf("NZBGet does not support torrents")
}

func (c *NzbgetClient) AddNzb(nzbURL, category string) error {
	// appendurl(nzbfilename, category, priority, addtoTop, addPaused, dupekey, dupescore, dupemode, urlContent)
	params := []any{nzbURL, category, 0, false, false, "", 0, "SCORE", ""}
	_, err := c.call("appendurl", params)
	return err
}

func (c *NzbgetClient) RemoveDownload(id string) error {
	_, err := c.call("editqueue", []any{"GroupDelete", "", []string{id}})
	return err
}

func (c *NzbgetClient) PauseDownload(id string) error {
	_, err := c.call("editqueue", []any{"GroupPause", "", []string{id}})
	return err
}

func (c *NzbgetClient) ResumeDownload(id string) error {
	_, err := c.call("editqueue", []any{"GroupResume", "", []string{id}})
	return err
}

func (c *NzbgetClient) GetDownloads() ([]domain.DownloadStatus, error) {
	body, err := c.call("listgroups", []any{0})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result []struct {
			NZBID       int     `json:"NZBID"`
			NZBName     string  `json:"NZBName"`
			FileSizeMB  float64 `json:"FileSizeMB"`
			DownloadedMB float64 `json:"DownloadedMB"`
			Status      string  `json:"Status"`
			Category    string  `json:"Category"`
			DestDir     string  `json:"DestDir"`
		} `json:"result"`
	}
	json.Unmarshal(body, &resp)

	var result []domain.DownloadStatus
	for _, g := range resp.Result {
		var progress float64
		if g.FileSizeMB > 0 {
			progress = (g.DownloadedMB / g.FileSizeMB) * 100
		}
		result = append(result, domain.DownloadStatus{
			ID:           fmt.Sprint(g.NZBID),
			Name:         g.NZBName,
			Size:         int64(g.FileSizeMB * 1024 * 1024),
			Progress:     progress,
			State:        mapNzbgetState(g.Status),
			Category:     g.Category,
			DownloadPath: g.DestDir,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapNzbgetState(s string) domain.DownloadState {
	switch s {
	case "DOWNLOADING":
		return domain.DownloadStateDownloading
	case "PAUSED":
		return domain.DownloadStatePaused
	case "SUCCESS":
		return domain.DownloadStateCompleted
	case "FAILURE":
		return domain.DownloadStateError
	case "QUEUED":
		return domain.DownloadStateQueued
	default:
		return domain.DownloadStateUnknown
	}
}
