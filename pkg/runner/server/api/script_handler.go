package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"runner/server/service"
)

type scriptHandler struct {
	svc *service.ScriptService
}

func NewScriptHandler(svc *service.ScriptService) ScriptHandler {
	return &scriptHandler{svc: svc}
}

func (h *scriptHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	items, err := h.svc.List(r.Context(), service.ScriptFilter{
		Language: strings.TrimSpace(r.URL.Query().Get("language")),
		Tag:      strings.TrimSpace(r.URL.Query().Get("tags")),
		Limit:    limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *scriptHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	item, err := h.svc.Get(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *scriptHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Language    string   `json:"language"`
		Content     string   `json:"content"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Create(r.Context(), &service.ScriptRecord{
		Name:        req.Name,
		Language:    req.Language,
		Content:     req.Content,
		Description: req.Description,
		Tags:        req.Tags,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "script.create", req.Name, map[string]any{
		"name":        req.Name,
		"language":    req.Language,
		"description": req.Description,
		"tags":        req.Tags,
		"content":     req.Content,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name": req.Name,
	})
}

func (h *scriptHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req struct {
		Language    string   `json:"language"`
		Content     string   `json:"content"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Update(r.Context(), name, &service.ScriptRecord{
		Language:    req.Language,
		Content:     req.Content,
		Description: req.Description,
		Tags:        req.Tags,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "script.update", name, map[string]any{
		"name":        name,
		"language":    req.Language,
		"description": req.Description,
		"tags":        req.Tags,
		"content":     req.Content,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}

func (h *scriptHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if err := h.svc.Delete(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "script.delete", name, map[string]any{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}

func (h *scriptHandler) Render(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req struct {
		Vars map[string]any `json:"vars"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	rendered, err := h.svc.Render(r.Context(), name, req.Vars)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"rendered": rendered,
	})
}
