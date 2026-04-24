package server

import (
	"io"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

func (s *HTTPServer) registerAgentProfileRoutes() {
	s.mux.HandleFunc("/api/v1/agent-skills", s.handleAgentSkills)
	s.mux.HandleFunc("/api/v1/agent-skills/", s.handleAgentSkills)
	s.mux.HandleFunc("/api/v1/agent-mcps", s.handleAgentMcps)
	s.mux.HandleFunc("/api/v1/agent-mcps/", s.handleAgentMcps)
	s.mux.HandleFunc("/api/v1/agent-profiles", s.handleAgentProfiles)
	s.mux.HandleFunc("/api/v1/agent-profiles/export", s.handleAgentProfiles)
	s.mux.HandleFunc("/api/v1/agent-profiles/import", s.handleAgentProfiles)
	s.mux.HandleFunc("/api/v1/agent-profile", s.handleAgentProfile)
	s.mux.HandleFunc("/api/v1/agent-profile/reset", s.handleAgentProfile)
	s.mux.HandleFunc("/api/v1/agent-profile/preview", s.handleAgentProfile)
}

func (s *HTTPServer) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ui.AgentProfileService().ListSkillCatalog(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPut:
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-skills/"), "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		var req appui.SkillCatalogItem
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			req.ID = id
		}
		payload, err := s.ui.AgentProfileService().SaveSkillCatalogItem(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodDelete:
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-skills/"), "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		payload, err := s.ui.AgentProfileService().DeleteSkillCatalogItem(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleAgentMcps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.ui.AgentProfileService().ListMcpCatalog(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPut:
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-mcps/"), "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		var req appui.McpCatalogItem
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			req.ID = id
		}
		payload, err := s.ui.AgentProfileService().SaveMcpCatalogItem(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodDelete:
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-mcps/"), "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		payload, err := s.ui.AgentProfileService().DeleteMcpCatalogItem(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleAgentProfiles(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-profiles":
		payload, err := s.ui.AgentProfileService().ListAgentProfiles(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-profiles/export":
		payload, err := s.ui.AgentProfileService().ExportAgentProfiles(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-profiles/import":
		var req appui.AgentProfilesImportPayload
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		payload, err := s.ui.AgentProfileService().ImportAgentProfiles(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	default:
		http.NotFound(w, r)
	}
}

func (s *HTTPServer) handleAgentProfile(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-profile":
		profile, err := s.ui.AgentProfileService().GetAgentProfile(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent-profile/preview":
		profileID := strings.TrimSpace(r.URL.Query().Get("profileId"))
		preview, err := s.ui.AgentProfileService().PreviewAgentProfile(r.Context(), profileID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, preview)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/agent-profile":
		var req store.AgentProfileRecord
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		profile, err := s.ui.AgentProfileService().SaveAgentProfile(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent-profile/reset":
		var req struct {
			ProfileID string `json:"profileId"`
		}
		if r.Body != nil {
			if err := decodeJSONBody(r, &req); err != nil && err != io.EOF {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
		}
		profile, err := s.ui.AgentProfileService().ResetAgentProfile(r.Context(), req.ProfileID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	default:
		http.NotFound(w, r)
	}
}
