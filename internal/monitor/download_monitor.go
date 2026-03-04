package monitor

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"playerr/internal/config"
	"playerr/internal/download"
	"playerr/internal/domain"
)

// DownloadMonitor polls enabled download clients and triggers post-processing
// for newly completed downloads.
type DownloadMonitor struct {
	cfg          *config.Service
	importStatus *ImportStatus
	processor    *Processor
	processed    map[string]bool
}

func NewDownloadMonitor(cfg *config.Service, importStatus *ImportStatus, processor *Processor) *DownloadMonitor {
	return &DownloadMonitor{
		cfg:          cfg,
		importStatus: importStatus,
		processor:    processor,
		processed:    make(map[string]bool),
	}
}

// Run starts the monitoring loop and blocks until ctx is cancelled.
func (m *DownloadMonitor) Run(ctx context.Context) {
	log.Println("[Monitor] Download monitor started.")
	for {
		settings := m.cfg.LoadPostDownload()
		interval := time.Duration(settings.MonitorIntervalSecs) * time.Second
		if interval < 5*time.Second {
			interval = 30 * time.Second
		}

		select {
		case <-ctx.Done():
			log.Println("[Monitor] Download monitor stopped.")
			return
		case <-time.After(interval):
			m.checkClients(ctx)
		}
	}
}

func (m *DownloadMonitor) checkClients(ctx context.Context) {
	clients := m.cfg.LoadDownloadClients()
	for _, cfg := range clients {
		if !cfg.Enable {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		m.checkClient(cfg)
	}
}

func (m *DownloadMonitor) checkClient(cfg domain.DownloadClientConfig) {
	client, err := download.NewClient(cfg)
	if err != nil {
		log.Printf("[Monitor] Unknown client %s: %v", cfg.Implementation, err)
		return
	}

	downloads, err := client.GetDownloads()
	if err != nil {
		log.Printf("[Monitor] Error polling %s: %v", cfg.Name, err)
		return
	}

	// Apply category filter
	if cfg.Category != "" {
		var filtered []domain.DownloadStatus
		for _, d := range downloads {
			if strings.EqualFold(d.Category, cfg.Category) {
				filtered = append(filtered, d)
			}
		}
		downloads = filtered
	}

	for _, d := range downloads {
		if d.State != domain.DownloadStateCompleted {
			continue
		}
		if m.processed[d.ID] {
			continue
		}

		// Apply remote path mapping
		if cfg.RemotePathMapping != "" && cfg.LocalPathMapping != "" {
			if strings.HasPrefix(d.DownloadPath, cfg.RemotePathMapping) {
				rel := strings.TrimPrefix(d.DownloadPath, cfg.RemotePathMapping)
				rel = strings.TrimLeft(rel, "/\\")
				d.DownloadPath = filepath.Join(cfg.LocalPathMapping, rel)
				log.Printf("[Monitor] Path mapped: -> %s", d.DownloadPath)
			}
		}

		// Don't mark as processed until the path is reachable — allows retry
		// when a NAS/mount becomes available on a later poll.
		if _, err := os.Stat(d.DownloadPath); err != nil {
			log.Printf("[Monitor] Path not reachable, will retry next poll: %s", d.DownloadPath)
			continue
		}

		m.importStatus.MarkImporting(d.ID)
		log.Printf("[Monitor] Processing completed download: %s (path: %s)", d.Name, d.DownloadPath)
		m.processed[d.ID] = true
		go func(dl domain.DownloadStatus) {
			defer m.importStatus.MarkFinished(dl.ID)
			if m.processor != nil {
				m.processor.Process(dl)
			}
		}(d)
	}
}
