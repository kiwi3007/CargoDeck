package api

import (
	"net/http"

	"playerr/internal/config"
)

// ---- IGDB ----

func (h *Handler) GetIgdb(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadIgdb())
}

func (h *Handler) SaveIgdb(w http.ResponseWriter, r *http.Request) {
	var v config.IgdbSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveIgdb(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "IGDB settings saved"})
}

func (h *Handler) TestIgdb(w http.ResponseWriter, r *http.Request) {
	igdb := h.cfg.LoadIgdb()
	jsonOK(w, map[string]any{
		"configured": igdb.IsConfigured(),
		"message":    "Configuration loaded",
	})
}

// ---- Steam ----

func (h *Handler) GetSteam(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadSteam())
}

func (h *Handler) SaveSteam(w http.ResponseWriter, r *http.Request) {
	var v config.SteamSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveSteam(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Steam settings saved"})
}

// ---- Prowlarr ----

func (h *Handler) GetProwlarr(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadProwlarr())
}

func (h *Handler) SaveProwlarr(w http.ResponseWriter, r *http.Request) {
	var v config.ProwlarrSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveProwlarr(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Prowlarr settings saved"})
}

// ---- Jackett ----

func (h *Handler) GetJackett(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadJackett())
}

func (h *Handler) SaveJackett(w http.ResponseWriter, r *http.Request) {
	var v config.JackettSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveJackett(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Jackett settings saved"})
}

// ---- Post-Download ----

func (h *Handler) GetPostDownload(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadPostDownload())
}

func (h *Handler) SavePostDownload(w http.ResponseWriter, r *http.Request) {
	var v config.PostDownloadSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SavePostDownload(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Post-download settings saved"})
}

// ---- Server ----

func (h *Handler) GetServer(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadServer())
}

func (h *Handler) SaveServer(w http.ResponseWriter, r *http.Request) {
	var v config.ServerSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveServer(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Server settings saved"})
}

// ---- Media Settings ----

func (h *Handler) GetMediaSettings(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadMedia())
}

func (h *Handler) SaveMediaSettings(w http.ResponseWriter, r *http.Request) {
	var v config.MediaSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveMedia(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Media settings saved"})
}

// ---- Hydra ----

func (h *Handler) GetHydra(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, h.cfg.LoadHydra())
}

func (h *Handler) SaveHydra(w http.ResponseWriter, r *http.Request) {
	var v []config.HydraConfig
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveHydra(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Hydra indexers saved"})
}
