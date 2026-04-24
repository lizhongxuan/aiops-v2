package api

import (
	"net/http"

	"runner/server/service"
)

type SystemInfo struct {
	Version     string
	BuildTime   string
	DocsURL     string
	RepoURL     string
	AuthEnabled bool
}

type SystemHandler struct {
	info    SystemInfo
	metrics *service.SystemService
}

func NewSystemHandler(info SystemInfo, metrics *service.SystemService) *SystemHandler {
	return &SystemHandler{
		info:    info,
		metrics: metrics,
	}
}

func (h *SystemHandler) Info(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":      h.info.Version,
		"build_time":   h.info.BuildTime,
		"docs_url":     h.info.DocsURL,
		"repo_url":     h.info.RepoURL,
		"auth_enabled": h.info.AuthEnabled,
	})
}

func (h *SystemHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.metrics == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "system metrics unavailable")
		return
	}
	payload, err := h.metrics.Metrics(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
