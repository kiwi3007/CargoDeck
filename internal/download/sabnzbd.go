package download

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/kiwi3007/playerr/internal/domain"
)

type SabnzbdClient struct {
	cfg    domain.DownloadClientConfig
	apiURL string
	apiKey string
	http   *http.Client
}

func NewSabnzbdClient(cfg domain.DownloadClientConfig) *SabnzbdClient {
	base := buildBaseURL(cfg.Host, cfg.Port, cfg.UrlBase)
	return &SabnzbdClient{
		cfg:    cfg,
		apiURL: base + "/api",
		apiKey: cfg.ApiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *SabnzbdClient) get(params url.Values) ([]byte, error) {
	params.Set("apikey", c.apiKey)
	params.Set("output", "json")
	resp, err := c.http.Get(c.apiURL + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var buf []byte
	buf = make([]byte, 0)
	tmp := make([]byte, 4096)
	for {
		n, e := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if e != nil {
			break
		}
	}
	return buf, nil
}

func (c *SabnzbdClient) TestConnection() (bool, string, error) {
	body, err := c.get(url.Values{"mode": {"version"}})
	if err != nil {
		return false, "", err
	}
	var r struct{ Version string `json:"version"` }
	json.Unmarshal(body, &r)
	return r.Version != "", r.Version, nil
}

func (c *SabnzbdClient) GetVersion() (string, error) {
	_, v, err := c.TestConnection()
	return v, err
}

func (c *SabnzbdClient) AddTorrent(_, _ string) error {
	return fmt.Errorf("SABnzbd does not support torrents")
}

func (c *SabnzbdClient) AddNzb(nzbURL, category string) error {
	params := url.Values{
		"mode": {"addurl"},
		"name": {nzbURL},
	}
	if category != "" {
		params.Set("cat", category)
	}
	_, err := c.get(params)
	return err
}

func (c *SabnzbdClient) RemoveDownload(id string) error {
	_, err := c.get(url.Values{"mode": {"queue"}, "name": {"delete"}, "del_files": {"1"}, "value": {id}})
	return err
}

func (c *SabnzbdClient) PauseDownload(id string) error {
	_, err := c.get(url.Values{"mode": {"queue"}, "name": {"pause"}, "value": {id}})
	return err
}

func (c *SabnzbdClient) ResumeDownload(id string) error {
	_, err := c.get(url.Values{"mode": {"queue"}, "name": {"resume"}, "value": {id}})
	return err
}

func (c *SabnzbdClient) GetDownloads() ([]domain.DownloadStatus, error) {
	body, err := c.get(url.Values{"mode": {"queue"}})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Queue struct {
			Slots []struct {
				ID       string  `json:"nzo_id"`
				Filename string  `json:"filename"`
				MB       string  `json:"mb"`
				MBLeft   string  `json:"mbleft"`
				Pct      float64 `json:"percentage"`
				Status   string  `json:"status"`
				Cat      string  `json:"cat"`
			} `json:"slots"`
		} `json:"queue"`
	}
	json.Unmarshal(body, &resp)

	var result []domain.DownloadStatus
	for _, s := range resp.Queue.Slots {
		result = append(result, domain.DownloadStatus{
			ID:       s.ID,
			Name:     s.Filename,
			Progress: s.Pct,
			State:    mapSABState(s.Status),
			Category: s.Cat,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapSABState(s string) domain.DownloadState {
	switch s {
	case "Downloading":
		return domain.DownloadStateDownloading
	case "Paused":
		return domain.DownloadStatePaused
	case "Completed", "Extracting", "Moving":
		return domain.DownloadStateCompleted
	case "Failed":
		return domain.DownloadStateError
	case "Queued":
		return domain.DownloadStateQueued
	default:
		return domain.DownloadStateUnknown
	}
}
