package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"runner/server/service"
	"runner/server/store/skillstore"
)

type skillHandler struct {
	svc *service.SkillService
}

func NewSkillHandler(svc *service.SkillService) SkillHandler {
	return &skillHandler{svc: svc}
}

func (h *skillHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *skillHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	item, err := h.svc.Get(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *skillHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req skillstore.SkillRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Create(r.Context(), &req); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"name": req.Name,
	})
}

func (h *skillHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req skillstore.SkillRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Update(r.Context(), name, &req); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}

func (h *skillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if err := h.svc.Delete(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}
