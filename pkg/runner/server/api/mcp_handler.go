package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"runner/server/service"
	"runner/server/store/mcpstore"
)

type mcpHandler struct {
	svc *service.McpService
}

func NewMcpHandler(svc *service.McpService) MCPHandler {
	return &mcpHandler{svc: svc}
}

func (h *mcpHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *mcpHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *mcpHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req mcpstore.ServerRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Create(r.Context(), &req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "mcp.create", req.ID, map[string]any{
		"id":       req.ID,
		"name":     req.Name,
		"type":     req.Type,
		"command":  req.Command,
		"url":      req.URL,
		"env_keys": mapKeys(req.EnvVars),
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": req.ID,
	})
}

func (h *mcpHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req mcpstore.ServerRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Update(r.Context(), id, &req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "mcp.update", id, map[string]any{
		"id":       id,
		"name":     req.Name,
		"type":     req.Type,
		"command":  req.Command,
		"url":      req.URL,
		"env_keys": mapKeys(req.EnvVars),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id,
	})
}

func (h *mcpHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "mcp.delete", id, map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id,
	})
}

func (h *mcpHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := h.svc.Toggle(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "mcp.toggle", id, map[string]any{
		"id":     id,
		"status": item.Status,
		"type":   item.Type,
	})
	writeJSON(w, http.StatusOK, item)
}

func (h *mcpHandler) Tools(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	items, err := h.svc.ListTools(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func mapKeys(input map[string]string) []string {
	if len(input) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
