package api

import (
	"net/http"

	"runner/server/service"
)

type dashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) DashboardHandler {
	return &dashboardHandler{svc: svc}
}

func (h *dashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "dashboard service unavailable")
		return
	}
	stats, err := h.svc.Stats(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
