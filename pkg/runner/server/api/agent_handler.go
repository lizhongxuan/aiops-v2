package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"runner/server/service"
	"runner/server/store/agentstore"
)

type agentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) AgentHandler {
	return &agentHandler{svc: svc}
}

func (h *agentHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	items, err := h.svc.List(r.Context(), service.AgentFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Tag:    strings.TrimSpace(r.URL.Query().Get("tags")),
		Limit:  limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	safe := make([]*agentstore.AgentRecord, 0, len(items))
	for _, item := range items {
		safe = append(safe, sanitizeAgent(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": safe,
	})
}

func (h *agentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sanitizeAgent(item))
}

func (h *agentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req agentstore.AgentRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Register(r.Context(), &req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "agent.create", req.ID, map[string]any{
		"id":           req.ID,
		"name":         req.Name,
		"address":      req.Address,
		"token":        req.Token,
		"tags":         req.Tags,
		"capabilities": req.Capabilities,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": req.ID,
	})
}

func (h *agentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req agentstore.AgentRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.Update(r.Context(), id, &req); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "agent.update", id, map[string]any{
		"id":           id,
		"name":         req.Name,
		"address":      req.Address,
		"token":        req.Token,
		"tags":         req.Tags,
		"capabilities": req.Capabilities,
		"status":       req.Status,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id,
	})
}

func (h *agentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "agent.delete", id, map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id,
	})
}

func (h *agentHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var req agentstore.Heartbeat
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.svc.Heartbeat(r.Context(), id, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "agent.heartbeat", id, map[string]any{
		"id":            id,
		"status":        req.Status,
		"load":          req.Load,
		"running_tasks": req.RunningTasks,
		"error":         req.Error,
	})
	writeJSON(w, http.StatusOK, sanitizeAgent(item))
}

func (h *agentHandler) Probe(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := h.svc.Probe(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "agent.probe", id, map[string]any{"id": id})
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     id,
		"status": "ok",
	})
}

func sanitizeAgent(item *agentstore.AgentRecord) *agentstore.AgentRecord {
	if item == nil {
		return nil
	}
	cp := *item
	if strings.TrimSpace(cp.Token) != "" {
		cp.Token = "***"
	}
	cp.Tags = append([]string{}, item.Tags...)
	cp.Capabilities = append([]string{}, item.Capabilities...)
	return &cp
}
