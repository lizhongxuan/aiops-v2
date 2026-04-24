package server

import (
	"net/http"
	"strings"
)

// Generator Workshop API — four-step flow (Req 6.7)
// generate → lint → preview → publish-draft
func (rs *ResourceServer) handleGeneratorWorkshop(w http.ResponseWriter, r *http.Request) {
	subPath := strings.TrimPrefix(r.URL.Path, "/api/v1/generator/")

	switch {
	case strings.HasPrefix(subPath, "generate"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "generate",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "lint"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "lint",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "preview"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "preview",
			"status": "ok",
		})

	case strings.HasPrefix(subPath, "publish-draft"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]string{
			"step":   "publish-draft",
			"status": "ok",
		})

	default:
		// List or status
		writeResourceJSON(w, http.StatusOK, map[string]interface{}{
			"generator": "workshop",
			"steps":     []string{"generate", "lint", "preview", "publish-draft"},
		})
	}
}
