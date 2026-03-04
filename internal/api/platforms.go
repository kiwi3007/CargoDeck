package api

import "net/http"

func (h *Handler) GetAllPlatforms(w http.ResponseWriter, r *http.Request) {
	platforms, err := h.repo.GetAllPlatforms()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonOK(w, platforms)
}

func (h *Handler) GetPlatformByID(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt(r, "id")
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	p, err := h.repo.GetPlatformByID(id)
	if err != nil || p == nil {
		jsonErr(w, 404, "platform not found")
		return
	}
	jsonOK(w, p)
}
