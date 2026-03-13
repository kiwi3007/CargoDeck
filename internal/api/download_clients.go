package api

import (
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kiwi3007/cargodeck/internal/domain"
	"github.com/kiwi3007/cargodeck/internal/download"
	"github.com/kiwi3007/cargodeck/internal/monitor"
)

func (h *Handler) GetDownloadClients(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadDownloadClients())
}

func (h *Handler) GetDownloadClient(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	clients := h.cfg.LoadDownloadClients()
	for _, c := range clients {
		if c.ID == id {
			jsonOK(w, c)
			return
		}
	}
	jsonErr(w, 404, "client not found")
}

func (h *Handler) CreateDownloadClient(w http.ResponseWriter, r *http.Request) {
	var c domain.DownloadClientConfig
	if err := decodeBody(r, &c); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	clients := h.cfg.LoadDownloadClients()
	maxID := 0
	for _, existing := range clients {
		if existing.ID > maxID {
			maxID = existing.ID
		}
	}
	c.ID = maxID + 1
	clients = append(clients, c)
	h.cfg.SaveDownloadClients(clients)
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, c)
}

func (h *Handler) UpdateDownloadClient(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	clients := h.cfg.LoadDownloadClients()

	for i, c := range clients {
		if c.ID == id {
			var update domain.DownloadClientConfig
			if err := decodeBody(r, &update); err != nil {
				jsonErr(w, 400, err.Error())
				return
			}
			update.ID = id
			clients[i] = update
			h.cfg.SaveDownloadClients(clients)
			jsonOK(w, update)
			return
		}
	}
	jsonErr(w, 404, "client not found")
}

func (h *Handler) DeleteDownloadClient(w http.ResponseWriter, r *http.Request) {
	id, _ := paramInt(r, "id")
	clients := h.cfg.LoadDownloadClients()

	var updated []domain.DownloadClientConfig
	found := false
	for _, c := range clients {
		if c.ID == id {
			found = true
			continue
		}
		updated = append(updated, c)
	}
	if !found {
		jsonErr(w, 404, "client not found")
		return
	}
	h.cfg.SaveDownloadClients(updated)
	w.WriteHeader(http.StatusNoContent)
}

// collectQueue fetches the current download queue from all enabled clients.
func (h *Handler) collectQueue() []domain.DownloadStatus {
	clients := h.cfg.LoadDownloadClients()
	var all []domain.DownloadStatus
	for _, cfg := range clients {
		if !cfg.Enable {
			continue
		}
		client, err := download.NewClient(cfg)
		if err != nil {
			log.Printf("[Queue] Unknown client %s: %v", cfg.Implementation, err)
			continue
		}
		downloads, err := client.GetDownloads()
		if err != nil {
			log.Printf("[Queue] Error from %s: %v", cfg.Name, err)
			all = append(all, domain.DownloadStatus{
				ClientID:   cfg.ID,
				ClientName: cfg.Name,
				ID:         "error-" + strconv.Itoa(cfg.ID),
				Name:       "Connection Error: " + err.Error(),
				State:      domain.DownloadStateError,
			})
			continue
		}
		if cfg.Category != "" {
			var filtered []domain.DownloadStatus
			for _, d := range downloads {
				if strings.EqualFold(d.Category, cfg.Category) {
					filtered = append(filtered, d)
				}
			}
			downloads = filtered
		}
		for i := range downloads {
			downloads[i].ClientID = cfg.ID
			downloads[i].ClientName = cfg.Name
			if h.importStatus.IsImporting(downloads[i].ID) {
				downloads[i].State = domain.DownloadStateImporting
			}
		}
		all = append(all, downloads...)
	}
	if all == nil {
		all = []domain.DownloadStatus{}
	}
	return all
}

func (h *Handler) GetQueue(w http.ResponseWriter, r *http.Request) {
	all := h.collectQueue()
	if all == nil {
		all = []domain.DownloadStatus{}
	}
	jsonOK(w, all)
}

func (h *Handler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	clientID, _ := strconv.Atoi(chi.URLParam(r, "clientId"))
	downloadID := chi.URLParam(r, "downloadId")

	client, cfg, err := h.getClientByID(clientID)
	if err != nil {
		jsonErr(w, 404, "client not found")
		return
	}
	_ = cfg

	if err := client.RemoveDownload(downloadID); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) PauseDownload(w http.ResponseWriter, r *http.Request) {
	clientID, _ := strconv.Atoi(chi.URLParam(r, "clientId"))
	downloadID := chi.URLParam(r, "downloadId")

	client, _, err := h.getClientByID(clientID)
	if err != nil {
		jsonErr(w, 404, "client not found")
		return
	}
	if err := client.PauseDownload(downloadID); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) ResumeDownload(w http.ResponseWriter, r *http.Request) {
	clientID, _ := strconv.Atoi(chi.URLParam(r, "clientId"))
	downloadID := chi.URLParam(r, "downloadId")

	client, _, err := h.getClientByID(clientID)
	if err != nil {
		jsonErr(w, 404, "client not found")
		return
	}
	if err := client.ResumeDownload(downloadID); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

type testClientRequest struct {
	Implementation string `json:"implementation"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	UrlBase        string `json:"urlBase"`
	ApiKey         string `json:"apiKey"`
	UseSsl         bool   `json:"useSsl"`
}

func (h *Handler) TestDownloadClient(w http.ResponseWriter, r *http.Request) {
	var req testClientRequest
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	cfg := domain.DownloadClientConfig{
		Implementation: req.Implementation,
		Host:           req.Host,
		Port:           req.Port,
		Username:       req.Username,
		Password:       req.Password,
		UrlBase:        req.UrlBase,
		ApiKey:         req.ApiKey,
		UseSsl:         req.UseSsl,
	}

	client, err := download.NewClient(cfg)
	if err != nil {
		jsonOK(w, map[string]any{"connected": false, "message": err.Error()})
		return
	}

	ok, version, err := client.TestConnection()
	if err != nil {
		jsonOK(w, map[string]any{"connected": false, "message": err.Error()})
		return
	}
	if !ok {
		jsonOK(w, map[string]any{"connected": false, "message": "Connection failed"})
		return
	}
	jsonOK(w, map[string]any{
		"connected": true,
		"version":   version,
		"message":   "Connection successful",
	})
}

type addTorrentRequest struct {
	URL      string `json:"url"`
	Protocol string `json:"protocol"`
	GameID   int    `json:"gameId,omitempty"`
}

func (h *Handler) AddTorrent(w http.ResponseWriter, r *http.Request) {
	var req addTorrentRequest
	if err := decodeBody(r, &req); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	isNZB := strings.EqualFold(req.Protocol, "nzb") ||
		strings.EqualFold(req.Protocol, "usenet") ||
		strings.HasSuffix(strings.ToLower(req.URL), ".nzb")

	clients := h.cfg.LoadDownloadClients()

	var target *domain.DownloadClientConfig
	for i := range clients {
		c := &clients[i]
		if !c.Enable {
			continue
		}
		impl := strings.ToLower(c.Implementation)
		isUsenet := impl == "sabnzbd" || impl == "nzbget"
		if isNZB == isUsenet {
			if target == nil || c.Priority < target.Priority {
				target = c
			}
		}
	}

	if target == nil {
		jsonErr(w, 400, "no enabled download client found")
		return
	}

	client, err := download.NewClient(*target)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	var infoHash string // populated for link-store recording
	if isNZB {
		err = client.AddNzb(req.URL, target.Category)
	} else if strings.HasPrefix(req.URL, "magnet:") {
		infoHash = monitor.ExtractMagnetInfoHash(req.URL)
		err = client.AddTorrent(req.URL, target.Category)
	} else {
		// Proxy-download the .torrent file server-side so the download client
		// doesn't need direct network access to the indexer URL (e.g. Prowlarr
		// behind a private hostname, or TorrentLeech requiring auth cookies).
		data, filename, fetchErr := fetchTorrentFile(req.URL)
		if fetchErr != nil {
			if mr, ok := fetchErr.(*errMagnetRedirect); ok {
				// Download URL redirects to a magnet (e.g. Hydra) — send it directly.
				log.Printf("[AddTorrent] URL redirected to magnet, sending to %s: %.120s", target.Name, mr.MagnetURL)
				infoHash = monitor.ExtractMagnetInfoHash(mr.MagnetURL)
				err = client.AddTorrent(mr.MagnetURL, target.Category)
			} else {
				log.Printf("[AddTorrent] Failed to fetch torrent file from %q: %v", req.URL, fetchErr)
				jsonErr(w, 500, fmt.Sprintf("failed to fetch torrent file: %v", fetchErr))
				return
			}
		} else {
			log.Printf("[AddTorrent] Proxying torrent file %q (%d bytes) to %s", filename, len(data), target.Name)
			infoHash = monitor.ExtractTorrentInfoHash(data)

			type fileAdder interface {
				AddTorrentFile(filename string, data []byte, category string) error
			}
			if fa, ok := client.(fileAdder); ok {
				err = fa.AddTorrentFile(filename, data, target.Category)
			} else {
				// Client doesn't support file upload — fall back to URL
				err = client.AddTorrent(req.URL, target.Category)
			}
		}
	}

	if err != nil {
		log.Printf("[AddTorrent] Client error: %v", err)
		jsonErr(w, 500, err.Error())
		return
	}

	// Record the game → download link so the post-processor can match
	// the completed download back to the game the user triggered it from.
	if req.GameID != 0 && infoHash != "" && h.linkStore != nil {
		h.linkStore.Add(infoHash, req.GameID)
		log.Printf("[AddTorrent] Linked infohash %s to game %d", infoHash, req.GameID)
	}

	jsonOK(w, map[string]string{"message": "Download added successfully"})
}

func (h *Handler) getClientByID(id int) (download.Client, domain.DownloadClientConfig, error) {
	clients := h.cfg.LoadDownloadClients()
	for _, cfg := range clients {
		if cfg.ID == id {
			client, err := download.NewClient(cfg)
			return client, cfg, err
		}
	}
	return nil, domain.DownloadClientConfig{}, http.ErrNoCookie
}

// errMagnetRedirect is returned by fetchTorrentFile when the URL redirects to
// a magnet link. The caller should send MagnetURL directly to the download client.
type errMagnetRedirect struct{ MagnetURL string }

func (e *errMagnetRedirect) Error() string { return "redirected to magnet: " + e.MagnetURL }

// fetchTorrentFile downloads a .torrent file from torrentURL and returns its
// raw bytes plus a filename. The server does this so the download client doesn't
// need direct network access to the indexer URL.
// Returns *errMagnetRedirect if the URL redirects to a magnet link (e.g. Hydra).
func fetchTorrentFile(torrentURL string) (data []byte, filename string, err error) {
	var magnetURL string
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Prefer the raw Location header to avoid Go's URL parser
			// mangling magnet: URIs (e.g. re-encoding tracker parameters).
			loc := req.URL.String()
			if req.Response != nil {
				if raw := req.Response.Header.Get("Location"); raw != "" {
					loc = raw
				}
			}
			if strings.HasPrefix(loc, "magnet:") {
				magnetURL = loc
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	resp, err := client.Get(torrentURL)
	if magnetURL != "" {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, "", &errMagnetRedirect{MagnetURL: magnetURL}
	}
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("indexer returned HTTP %d", resp.StatusCode)
	}

	data, err = io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 20 MB cap
	if err != nil {
		return nil, "", err
	}

	// Derive filename from Content-Disposition, then URL path.
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, parseErr := mime.ParseMediaType(cd); parseErr == nil {
			if fn := params["filename"]; fn != "" {
				filename = fn
			}
		}
	}
	if filename == "" {
		if parsed, parseErr := url.Parse(torrentURL); parseErr == nil {
			filename = path.Base(parsed.Path)
		}
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".torrent") {
		filename = "download.torrent"
	}
	return data, filename, nil
}
