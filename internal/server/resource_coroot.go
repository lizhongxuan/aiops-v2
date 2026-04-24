package server

import (
	"net/http"
	"strings"
)

// Coroot Proxy — read-only reverse proxy to Coroot (Req 6.7)
func (rs *ResourceServer) handleCorootProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed — Coroot proxy is read-only", http.StatusMethodNotAllowed)
		return
	}

	// Extract the sub-path after /api/v1/coroot/
	subPath := strings.TrimPrefix(r.URL.Path, "/api/v1/coroot/")

	// Stub: in production, this forwards to the Coroot backend.
	// The response format is preserved for frontend compatibility.
	writeResourceJSON(w, http.StatusOK, map[string]interface{}{
		"proxy":   "coroot",
		"path":    subPath,
		"status":  "ok",
		"message": "coroot proxy stub",
	})
}
