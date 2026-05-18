package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

type hostAgentHTTPServices interface {
	HostAgentService() appui.HostAgentService
}

func (s *HTTPServer) handleHostAgents(w http.ResponseWriter, r *http.Request) {
	service, ok := hostAgentServiceFromHTTP(s.ui)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/host-agents/register":
		var req appui.HostAgentRegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := service.Register(r.Context(), req, hostAgentTokenFromRequest(r))
		if err != nil {
			writeHostAgentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/host-agents/heartbeat":
		var req appui.HostAgentHeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		resp, err := service.Heartbeat(r.Context(), req, hostAgentTokenFromRequest(r))
		if err != nil {
			writeHostAgentError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		http.NotFound(w, r)
	}
}

func hostAgentServiceFromHTTP(ui appui.HTTPServices) (appui.HostAgentService, bool) {
	provider, ok := ui.(hostAgentHTTPServices)
	if !ok {
		return nil, false
	}
	service := provider.HostAgentService()
	return service, service != nil
}

func hostAgentTokenFromRequest(r *http.Request) string {
	if token := hostAgentBearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	return strings.TrimSpace(r.Header.Get("X-Host-Agent-Token"))
}

func hostAgentBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) >= 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return header
}

func writeHostAgentError(w http.ResponseWriter, err error) {
	if errors.Is(err, appui.ErrHostAgentUnauthorized) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
