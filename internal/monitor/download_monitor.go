package monitor

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kiwi3007/playerr/internal/config"
	"github.com/kiwi3007/playerr/internal/download"
	"github.com/kiwi3007/playerr/internal/domain"
)

// DownloadMonitor polls enabled download clients and triggers post-processing
// for newly completed downloads.
type DownloadMonitor struct {
	cfg           *config.Service
	importStatus  *ImportStatus
	processor     *Processor
	processed     map[string]bool
	notFound      map[string]int // consecutive polls where path was os.IsNotExist
	processedFile string
}

// notFoundThreshold is the number of consecutive polls where the download
// path doesn't exist before the entry is silently skipped. This handles
// files already processed (and moved/deleted) across a server restart without
// spamming the log. A temporary NAS outage returns a different error and won't
// increment this counter.
const notFoundThreshold = 3

func NewDownloadMonitor(cfg *config.Service, importStatus *ImportStatus, processor *Processor) *DownloadMonitor {
	m := &DownloadMonitor{
		cfg:           cfg,
		importStatus:  importStatus,
		processor:     processor,
		processed:     make(map[string]bool),
		notFound:      make(map[string]int),
		processedFile: filepath.Join(cfg.Dir(), "download_processed.json"),
	}
	m.loadProcessed()
	return m
}

// loadProcessed restores the processed set from disk so completed downloads
// are not re-processed after a server restart.
func (m *DownloadMonitor) loadProcessed() {
	data, err := os.ReadFile(m.processedFile)
	if err != nil {
		return
	}
	var ids []string
	if json.Unmarshal(data, &ids) == nil {
		for _, id := range ids {
			m.processed[id] = true
		}
		log.Printf("[Monitor] Loaded %d processed download ID(s) from disk", len(ids))
	}
}

// saveProcessed writes the current processed set to disk.
func (m *DownloadMonitor) saveProcessed() {
	ids := make([]string, 0, len(m.processed))
	for id := range m.processed {
		ids = append(ids, id)
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return
	}
	_ = os.WriteFile(m.processedFile, data, 0644)
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
			if os.IsNotExist(err) {
				// Track consecutive "not found" polls. After the threshold the
				// files are permanently gone (already imported + moved/deleted),
				// so we stop retrying. The post-processor has its own idempotency
				// check so marking processed here won't cause a double-import.
				m.notFound[d.ID]++
				if m.notFound[d.ID] >= notFoundThreshold {
					log.Printf("[Monitor] Path gone for %d polls, marking processed: %s", m.notFound[d.ID], d.DownloadPath)
					m.processed[d.ID] = true
					delete(m.notFound, d.ID)
					m.saveProcessed()
				}
			} else {
				log.Printf("[Monitor] Path not reachable, will retry next poll: %s", d.DownloadPath)
			}
			continue
		}
		delete(m.notFound, d.ID) // reset if path comes back (e.g. NAS remounted)

		m.importStatus.MarkImporting(d.ID)
		log.Printf("[Monitor] Processing completed download: %s (path: %s)", d.Name, d.DownloadPath)
		m.processed[d.ID] = true
		m.saveProcessed()
		go func(dl domain.DownloadStatus) {
			defer m.importStatus.MarkFinished(dl.ID)
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Monitor] Panic processing %s: %v", dl.Name, r)
				}
			}()
			if m.processor != nil {
				m.processor.Process(dl)
			}
		}(d)
	}
}
