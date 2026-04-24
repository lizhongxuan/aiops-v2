package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type ReadinessChecker interface {
	Ready(r *http.Request) error
}

type HealthHandler struct {
	Checker ReadinessChecker
}

func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
	})
}

func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h != nil && h.Checker != nil {
		if err := h.Checker.Ready(r); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":    "not_ready",
				"error":     err.Error(),
				"timestamp": time.Now().UTC(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	})
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
