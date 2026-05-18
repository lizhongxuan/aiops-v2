package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handleHosts(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/hosts":
		items, err := s.ui.HostService().ListHosts(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/hosts":
		var req appui.HostUpsert
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := s.ui.HostService().CreateHost(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/install"):
		hostID := hostIDFromNestedHostPath(r.URL.Path, "install")
		var req appui.HostInstallRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := s.ui.HostService().InstallHost(r.Context(), hostID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "/ssh/test"):
		hostID := hostIDFromNestedHostPath(r.URL.Path, "ssh/test")
		var req appui.HostSSHTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := s.ui.HostService().TestHostSSH(r.Context(), hostID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPut:
		hostID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/hosts/"), "/")
		var req appui.HostUpsert
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := s.ui.HostService().UpdateHost(r.Context(), hostID, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodDelete:
		hostID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/hosts/"), "/")
		if err := s.ui.HostService().DeleteHost(r.Context(), hostID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.NotFound(w, r)
	}
}

func hostIDFromNestedHostPath(path, suffix string) string {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/v1/hosts/"), "/")
	return strings.Trim(strings.TrimSuffix(trimmed, "/"+suffix), "/")
}

func (s *HTTPServer) handleSelectHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		HostID string `json:"hostId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	snapshot, err := s.ui.HostService().SelectHost(r.Context(), req.HostID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": snapshot})
}
