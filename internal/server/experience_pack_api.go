package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *HTTPServer) handleExperiencePacks(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/experience-packs"), "/")

	switch {
	case r.Method == http.MethodGet && path == "candidates":
		writeJSON(w, http.StatusOK, map[string]any{
			"items":      []any{},
			"total":      0,
			"nextCursor": "",
		})
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/reuse-records"):
		writeJSON(w, http.StatusOK, map[string]any{
			"items":      []any{},
			"total":      0,
			"nextCursor": "",
		})
	case r.Method == http.MethodPost && strings.HasPrefix(path, "candidates/") && strings.HasSuffix(path, "/approve"):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack candidate not found"})
	case (r.Method == http.MethodPatch || r.Method == http.MethodPut) && strings.HasSuffix(path, "/enabled"):
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
	case r.Method == http.MethodPut && strings.HasSuffix(path, "/authorization-scopes"):
		var req struct {
			Scopes []map[string]any `json:"scopes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "experience pack not found"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
