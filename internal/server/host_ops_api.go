package server

import (
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

const hostOpsChildAgentsPrefix = "/api/v1/host-ops/child-agents/"

func (s *HTTPServer) handleHostOpsChildAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if r.URL.Path == strings.TrimSuffix(hostOpsChildAgentsPrefix, "/")+"/transcript" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "childAgentId is required"})
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/transcript") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops endpoint not found"})
		return
	}
	childAgentID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, hostOpsChildAgentsPrefix), "/transcript")
	childAgentID = strings.Trim(childAgentID, "/")
	if childAgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "childAgentId is required"})
		return
	}
	service := s.hostOpsService()
	if service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "host ops service is not available"})
		return
	}
	transcript, err := service.ChildTranscript(r.Context(), childAgentID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if transcript.ChildAgentID == "" {
		transcript.ChildAgentID = childAgentID
	}
	writeJSON(w, http.StatusOK, transcript)
}

func (s *HTTPServer) hostOpsService() appui.HostOpsService {
	if s == nil || s.ui == nil {
		return nil
	}
	provider, ok := s.ui.(interface {
		HostOpsService() appui.HostOpsService
	})
	if !ok {
		return nil
	}
	return provider.HostOpsService()
}
