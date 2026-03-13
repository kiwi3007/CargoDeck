package api

import "net/http"

type scanRequest struct {
	FolderPath string `json:"folderPath"`
	Platform   string `json:"platform"`
}

func (h *Handler) TriggerScan(w http.ResponseWriter, r *http.Request) {
	// IGDB check
	igdb := h.cfg.LoadIgdb()
	if !igdb.IsConfigured() {
		w.WriteHeader(400)
		jsonOK(w, map[string]any{
			"success":   false,
			"errorCode": "IGDB_NOT_CONFIGURED",
			"message":   "IGDB credentials are required for scanning. Please configure them in the Metadata section.",
		})
		return
	}

	var req scanRequest
	_ = decodeBody(r, &req)

	media := h.cfg.LoadMedia()
	folder := req.FolderPath
	if folder == "" {
		folder = media.FolderPath
	}

	go h.scanner.Scan(folder, req.Platform)

	jsonOK(w, map[string]string{"message": "Scan started in background."})
}

func (h *Handler) StopScan(w http.ResponseWriter, r *http.Request) {
	h.scanner.StopScan()
	jsonOK(w, map[string]string{"message": "Scan stopping."})
}

func (h *Handler) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{
		"isScanning":     h.scanner.IsScanning(),
		"lastGameFound":  h.scanner.LastGameFound(),
		"gamesAddedCount": h.scanner.GamesAdded(),
	})
}

func (h *Handler) CleanLibrary(w http.ResponseWriter, r *http.Request) {
	if err := h.scanner.CleanLibrary(); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Library cleaned."})
}
