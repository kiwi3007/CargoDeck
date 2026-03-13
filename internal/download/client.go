package download

import (
	"fmt"
	"strings"

	"github.com/kiwi3007/playerr/internal/domain"
)

// Client is the unified download client interface.
type Client interface {
	TestConnection() (bool, string, error)
	GetVersion() (string, error)
	AddTorrent(url, category string) error
	AddNzb(url, category string) error
	RemoveDownload(id string) error
	PauseDownload(id string) error
	ResumeDownload(id string) error
	GetDownloads() ([]domain.DownloadStatus, error)
}

// NewClient creates the appropriate client based on implementation name.
func NewClient(cfg domain.DownloadClientConfig) (Client, error) {
	impl := strings.ToLower(cfg.Implementation)
	switch impl {
	case "qbittorrent":
		return NewQBittorrentClient(cfg), nil
	case "transmission":
		return NewTransmissionClient(cfg), nil
	case "sabnzbd":
		return NewSabnzbdClient(cfg), nil
	case "nzbget":
		return NewNzbgetClient(cfg), nil
	case "deluge":
		return NewDelugeClient(cfg), nil
	case "rtorrent":
		return NewRTorrentClient(cfg), nil
	case "flood":
		return NewFloodClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported download client: %s", cfg.Implementation)
	}
}

// buildBaseURL constructs a normalized http://host:port/urlbase URL.
func buildBaseURL(host string, port int, urlBase string) string {
	h := strings.TrimSpace(host)
	if !strings.HasPrefix(strings.ToLower(h), "http://") && !strings.HasPrefix(strings.ToLower(h), "https://") {
		h = "http://" + h
	}
	h = strings.TrimRight(h, "/")

	ub := ""
	if urlBase != "" {
		ub = strings.TrimRight(urlBase, "/")
		if !strings.HasPrefix(ub, "/") {
			ub = "/" + ub
		}
	}
	return fmt.Sprintf("%s:%d%s", h, port, ub)
}
