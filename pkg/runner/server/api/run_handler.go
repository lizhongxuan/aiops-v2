package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"runner/server/service"
)

type runHandler struct {
	svc *service.RunService
}

func NewRunHandler(svc *service.RunService) RunHandler {
	return &runHandler{svc: svc}
}

func (h *runHandler) Submit(w http.ResponseWriter, r *http.Request) {
	var req service.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.svc.Submit(r.Context(), &req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "run.submit", resp.RunID, map[string]any{
		"run_id":          resp.RunID,
		"workflow_name":   req.WorkflowName,
		"idempotency_key": req.IdempotencyKey,
		"triggered_by":    req.TriggeredBy,
		"vars":            req.Vars,
	})
	writeJSON(w, http.StatusAccepted, resp)
}

func (h *runHandler) Get(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	item, err := h.svc.Get(r.Context(), runID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *runHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	items, err := h.svc.List(r.Context(), service.RunFilter{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Workflow: strings.TrimSpace(r.URL.Query().Get("workflow")),
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

func (h *runHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	if err := h.svc.Cancel(r.Context(), runID); err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "run.cancel", runID, map[string]any{
		"run_id": runID,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"status": "cancel_requested",
	})
}

func (h *runHandler) EventsHistory(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	items, err := h.svc.History(r.Context(), runID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *runHandler) Events(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("id"))
	ch, cancel, err := h.svc.Subscribe(r.Context(), runID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case evt, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
