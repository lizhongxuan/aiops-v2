package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

type incidentHTTPServices interface {
	IncidentService() appui.IncidentService
}

type incidentStartChatHTTPServices interface {
	CorootWebhookService() appui.CorootWebhookService
}

func (s *HTTPServer) handleIncidents(w http.ResponseWriter, r *http.Request) {
	provider, ok := s.ui.(incidentHTTPServices)
	if !ok || provider.IncidentService() == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "incident service is not configured"})
		return
	}
	service := provider.IncidentService()
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/incidents"), "/")
	switch {
	case r.Method == http.MethodGet && path == "":
		items, err := service.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"incidents": items})
	case r.Method == http.MethodPost && path == "":
		var req appui.IncidentCreateCommand
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		incident, err := service.Create(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/close"):
		id := strings.TrimSuffix(path, "/close")
		var req appui.IncidentCloseCommand
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		incident, err := service.Close(r.Context(), id, req)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/start-chat"):
		startProvider, ok := s.ui.(incidentStartChatHTTPServices)
		if !ok || startProvider.CorootWebhookService() == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "coroot webhook service is not configured"})
			return
		}
		id := strings.TrimSuffix(path, "/start-chat")
		var req appui.CorootWebhookStartChatCommand
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		if strings.TrimSpace(req.IncidentID) == "" {
			req.IncidentID = id
		}
		result, err := startProvider.CorootWebhookService().StartChat(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodGet && path != "":
		incident, ok := service.Get(r.Context(), path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "incident not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
