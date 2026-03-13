package api

import (
	"net/http"

	"github.com/kiwi3007/cargodeck/internal/config"
)

// ---- IGDB ----

func (h *Handler) GetIgdb(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadIgdb()
	masked := config.IgdbSettings{ClientId: cfg.ClientId}
	if cfg.ClientSecret != "" {
		masked.ClientSecret = maskSentinel
	}
	jsonOK(w, masked)
}

func (h *Handler) SaveIgdb(w http.ResponseWriter, r *http.Request) {
	var v config.IgdbSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v.ClientSecret == maskSentinel {
		existing := h.cfg.LoadIgdb()
		v.ClientSecret = existing.ClientSecret
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

func (h *Handler) DeleteIgdb(w http.ResponseWriter, r *http.Request) {
	if err := h.cfg.SaveIgdb(config.IgdbSettings{}); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "IGDB settings cleared"})
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

func (h *Handler) DeleteSteam(w http.ResponseWriter, r *http.Request) {
	if err := h.cfg.SaveSteam(config.SteamSettings{}); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Steam settings cleared"})
}

func (h *Handler) TestSteam(w http.ResponseWriter, r *http.Request) {
	steam := h.cfg.LoadSteam()
	jsonOK(w, map[string]any{
		"configured": steam.IsConfigured(),
		"message":    "Configuration loaded",
	})
}

func (h *Handler) SyncSteam(w http.ResponseWriter, r *http.Request) {
	steam := h.cfg.LoadSteam()
	if !steam.IsConfigured() {
		jsonErr(w, 400, "Steam not configured")
		return
	}
	jsonOK(w, map[string]string{"message": "Steam sync triggered"})
}

// ---- Prowlarr ----

func (h *Handler) GetProwlarr(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadProwlarr()
	masked := config.ProwlarrSettings{Url: cfg.Url}
	if cfg.ApiKey != "" {
		masked.ApiKey = maskSentinel
	}
	jsonOK(w, masked)
}

func (h *Handler) SaveProwlarr(w http.ResponseWriter, r *http.Request) {
	var v config.ProwlarrSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v.ApiKey == maskSentinel {
		existing := h.cfg.LoadProwlarr()
		v.ApiKey = existing.ApiKey
	}
	if err := h.cfg.SaveProwlarr(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Prowlarr settings saved"})
}

// ---- Jackett ----

func (h *Handler) GetJackett(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadJackett()
	masked := config.JackettSettings{Url: cfg.Url}
	if cfg.ApiKey != "" {
		masked.ApiKey = maskSentinel
	}
	jsonOK(w, masked)
}

func (h *Handler) SaveJackett(w http.ResponseWriter, r *http.Request) {
	var v config.JackettSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v.ApiKey == maskSentinel {
		existing := h.cfg.LoadJackett()
		v.ApiKey = existing.ApiKey
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
	cfg := h.cfg.LoadServer()
	jsonOK(w, map[string]any{
		"port":             cfg.Port,
		"useAllInterfaces": cfg.UseAllInterfaces,
		"uiPasswordSet":    cfg.UIPassword != "",
	})
}

func (h *Handler) SaveServer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port             int    `json:"port"`
		UseAllInterfaces bool   `json:"useAllInterfaces"`
		UIPassword       string `json:"uiPassword"`
	}
	if err := decodeBody(r, &body); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	existing := h.cfg.LoadServer()
	v := config.ServerSettings{
		Port:             body.Port,
		UseAllInterfaces: body.UseAllInterfaces,
		UIPassword:       body.UIPassword,
	}
	// Empty string means "clear the password"; sentinel means "keep existing"
	if body.UIPassword == maskSentinel {
		v.UIPassword = existing.UIPassword
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

// ---- SteamGridDB ----

func (h *Handler) GetSteamGridDB(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadSteamGridDB()
	// Mask the key in the response
	masked := config.SteamGridDBSettings{}
	if cfg.ApiKey != "" {
		masked.ApiKey = "••••••••"
	}
	jsonOK(w, masked)
}

func (h *Handler) SaveSteamGridDB(w http.ResponseWriter, r *http.Request) {
	var v config.SteamGridDBSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if err := h.cfg.SaveSteamGridDB(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "SteamGridDB settings saved"})
}

func (h *Handler) DeleteSteamGridDB(w http.ResponseWriter, r *http.Request) {
	if err := h.cfg.SaveSteamGridDB(config.SteamGridDBSettings{}); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "SteamGridDB settings cleared"})
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

// ---- Discord ----

func (h *Handler) GetDiscord(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.LoadDiscord()
	masked := config.DiscordSettings{CheckIntervalHours: cfg.CheckIntervalHours}
	if cfg.WebhookURL != "" {
		masked.WebhookURL = maskSentinel
	}
	jsonOK(w, masked)
}

func (h *Handler) SaveDiscord(w http.ResponseWriter, r *http.Request) {
	var v config.DiscordSettings
	if err := decodeBody(r, &v); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if v.WebhookURL == maskSentinel {
		existing := h.cfg.LoadDiscord()
		v.WebhookURL = existing.WebhookURL
	}
	if err := h.cfg.SaveDiscord(v); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, map[string]string{"message": "Discord settings saved"})
}
