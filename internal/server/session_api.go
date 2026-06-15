package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *HTTPServer) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sessions":
		s.handleListSessions(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sessions":
		s.handleCreateSession(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/activate"):
		s.handleActivateSession(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *HTTPServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	result, err := s.ui.SessionService().ListSessions(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   string `json:"kind"`
		HostID string `json:"hostId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	result, err := s.ui.SessionService().CreateSession(r.Context(), strings.TrimSpace(req.Kind), strings.TrimSpace(req.HostID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) handleActivateSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/"), "/activate")
	sessionID = strings.Trim(sessionID, "/")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
		return
	}
	result, err := s.ui.SessionService().ActivateSession(r.Context(), sessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
