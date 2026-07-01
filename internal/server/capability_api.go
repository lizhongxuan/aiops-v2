package server

import (
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) registerCapabilityRoutes() {
	s.mux.HandleFunc("/api/v1/capabilities", s.handleCapabilities)
	s.mux.HandleFunc("/api/v1/capabilities/", s.handleCapabilities)
}

func (s *HTTPServer) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	service := capabilityServiceFromHTTPServices(s.ui)
	if service == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "capability service not configured"})
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/capabilities":
		payload, err := service.ListRecords(r.Context(), capabilityListRequestFromQuery(r))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/capabilities/search":
		payload, err := service.Search(r.Context(), capabilityListRequestFromQuery(r))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/capabilities/resolve/"):
		id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/capabilities/resolve/"), "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		record, err := service.Resolve(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": record})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/capabilities/resolve":
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		record, err := service.Resolve(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": record})
	default:
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	}
}

func capabilityListRequestFromQuery(r *http.Request) appui.CapabilityListRequest {
	query := r.URL.Query()
	return appui.CapabilityListRequest{
		Query:    strings.TrimSpace(query.Get("q")),
		Kind:     strings.TrimSpace(query.Get("kind")),
		Category: strings.TrimSpace(query.Get("category")),
	}
}

func capabilityServiceFromHTTPServices(ui appui.HTTPServices) appui.CapabilityService {
	provider, ok := ui.(interface {
		CapabilityService() appui.CapabilityService
	})
	if !ok {
		return nil
	}
	return provider.CapabilityService()
}
