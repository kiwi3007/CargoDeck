package download

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kiwi3007/cargodeck/internal/domain"
)

type QBittorrentClient struct {
	cfg     domain.DownloadClientConfig
	baseURL string
	mu      sync.Mutex
	cookie  string
	http    *http.Client
}

func NewQBittorrentClient(cfg domain.DownloadClientConfig) *QBittorrentClient {
	return &QBittorrentClient{
		cfg:     cfg,
		baseURL: buildBaseURL(cfg.Host, cfg.Port, cfg.UrlBase) + "/api/v2",
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *QBittorrentClient) login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cookie != "" {
		return nil
	}

	form := url.Values{"username": {c.cfg.Username}, "password": {c.cfg.Password}}
	req, _ := http.NewRequest("POST", c.baseURL+"/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", strings.Replace(c.baseURL, "/api/v2", "/", 1))

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, ck := range resp.Cookies() {
		if ck.Name == "SID" {
			c.cookie = "SID=" + ck.Value
			return nil
		}
	}
	// Try Set-Cookie header directly
	for _, h := range resp.Header["Set-Cookie"] {
		if strings.HasPrefix(h, "SID=") {
			c.cookie = strings.SplitN(h, ";", 2)[0]
			return nil
		}
	}
	return nil
}

func (c *QBittorrentClient) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	if err := c.login(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	return c.http.Do(req)
}

func (c *QBittorrentClient) TestConnection() (bool, string, error) {
	resp, err := c.do("GET", "/app/version", nil, "")
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, "", nil
	}
	b, _ := io.ReadAll(resp.Body)
	return true, string(b), nil
}

func (c *QBittorrentClient) GetVersion() (string, error) {
	ok, v, err := c.TestConnection()
	if !ok || err != nil {
		return "", err
	}
	return v, nil
}

func (c *QBittorrentClient) AddTorrent(torrentURL, category string) error {
	var b strings.Builder
	mw := multipart.NewWriter(&b)
	_ = mw.WriteField("urls", torrentURL)
	if category != "" {
		_ = mw.WriteField("category", category)
	}
	mw.Close()

	resp, err := c.do("POST", "/torrents/add", strings.NewReader(b.String()), mw.FormDataContentType())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("qBittorrent add torrent returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.EqualFold(strings.TrimSpace(string(body)), "ok.") {
		return fmt.Errorf("qBittorrent rejected torrent URL: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

// AddTorrentFile uploads a raw .torrent file to qBittorrent.
// This is more reliable than AddTorrent(url) when qBittorrent can't reach the indexer URL.
func (c *QBittorrentClient) AddTorrentFile(filename string, data []byte, category string) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("torrents", filename)
	if err != nil {
		return err
	}
	if _, err = fw.Write(data); err != nil {
		return err
	}
	if category != "" {
		_ = mw.WriteField("category", category)
	}
	mw.Close()

	resp, err := c.do("POST", "/torrents/add", &buf, mw.FormDataContentType())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("qBittorrent add torrent file returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if !strings.EqualFold(strings.TrimSpace(string(body)), "ok.") {
		return fmt.Errorf("qBittorrent rejected torrent file: %s", strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *QBittorrentClient) AddNzb(_, _ string) error {
	return fmt.Errorf("qBittorrent does not support NZB")
}

func (c *QBittorrentClient) RemoveDownload(id string) error {
	form := url.Values{"hashes": {id}, "deleteFiles": {"true"}}
	resp, err := c.do("POST", "/torrents/delete", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *QBittorrentClient) PauseDownload(id string) error {
	form := url.Values{"hashes": {id}}
	resp, err := c.do("POST", "/torrents/stop", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *QBittorrentClient) ResumeDownload(id string) error {
	form := url.Values{"hashes": {id}}
	resp, err := c.do("POST", "/torrents/start", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type qbTorrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	Progress    float64 `json:"progress"`
	State       string  `json:"state"`
	Category    string  `json:"category"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
}

func (c *QBittorrentClient) GetDownloads() ([]domain.DownloadStatus, error) {
	resp, err := c.do("GET", "/torrents/info", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var torrents []qbTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, err
	}

	var result []domain.DownloadStatus
	for _, t := range torrents {
		dlPath := t.ContentPath
		if dlPath == "" {
			dlPath = filepath.Join(t.SavePath, t.Name)
		}
		result = append(result, domain.DownloadStatus{
			ID:           t.Hash,
			InfoHash:     t.Hash,
			Name:         t.Name,
			Size:         t.Size,
			Progress:     t.Progress * 100,
			State:        mapQBState(t.State),
			Category:     t.Category,
			DownloadPath: dlPath,
		})
	}
	if result == nil {
		result = []domain.DownloadStatus{}
	}
	return result, nil
}

func mapQBState(s string) domain.DownloadState {
	switch strings.ToLower(s) {
	case "downloading", "stalleddl":
		return domain.DownloadStateDownloading
	case "pauseddl", "stoppeddl":
		return domain.DownloadStatePaused
	case "queueddl":
		return domain.DownloadStateQueued
	case "checkingdl", "checkingresumedata":
		return domain.DownloadStateChecking
	case "uploading", "stalledup", "pausedup", "stoppedup", "queuedup", "checkingup", "moving":
		return domain.DownloadStateCompleted
	case "missingfiles", "error":
		return domain.DownloadStateError
	default:
		return domain.DownloadStateUnknown
	}
}
