package server

import (
	"encoding/json"
	"net/http"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/terminalpolicy"
)

func (s *HTTPServer) handleTerminalPolicies(w http.ResponseWriter, r *http.Request) {
	service := s.terminalPolicyService()
	if service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "terminal policy service is not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		config, err := service.GetConfig(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, config)
	case http.MethodPut:
		var config terminalpolicy.Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		updated, err := service.UpdateConfig(r.Context(), config)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, updated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) terminalPolicyService() appui.TerminalPolicyService {
	if s == nil || s.ui == nil {
		return nil
	}
	provider, ok := s.ui.(interface {
		TerminalPolicyService() appui.TerminalPolicyService
	})
	if !ok {
		return nil
	}
	return provider.TerminalPolicyService()
}
