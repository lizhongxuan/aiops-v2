package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"runner/server/service"
	"runner/server/store/envstore"
)

type environmentHandler struct {
	svc *service.EnvironmentService
}

func NewEnvironmentHandler(svc *service.EnvironmentService) EnvironmentHandler {
	return &environmentHandler{svc: svc}
}

func (h *environmentHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *environmentHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	item, err := h.svc.Get(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *environmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req envstore.EnvironmentRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Create(r.Context(), &req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "environment.create", req.Name, map[string]any{
		"name":        req.Name,
		"description": req.Description,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name": req.Name,
	})
}

func (h *environmentHandler) AddVar(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req envstore.EnvVar
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.AddVar(r.Context(), name, req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "environment.var.create", name, map[string]any{
		"name":        name,
		"var_name":    req.Key,
		"description": req.Description,
		"sensitive":   req.Sensitive,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name": name,
		"key":  req.Key,
	})
}

func (h *environmentHandler) UpdateVar(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	key := strings.TrimSpace(r.PathValue("key"))
	var req envstore.EnvVar
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.UpdateVar(r.Context(), name, key, req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "environment.var.update", name, map[string]any{
		"name":        name,
		"var_name":    key,
		"description": req.Description,
		"sensitive":   req.Sensitive,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
		"key":  key,
	})
}

func (h *environmentHandler) DeleteVar(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	key := strings.TrimSpace(r.PathValue("key"))
	if err := h.svc.DeleteVar(r.Context(), name, key); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "environment.var.delete", name, map[string]any{
		"name":     name,
		"var_name": key,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
		"key":  key,
	})
}
