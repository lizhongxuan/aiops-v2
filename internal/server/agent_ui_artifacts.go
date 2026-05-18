package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

func (rs *ResourceServer) handleAgentUIArtifacts(w http.ResponseWriter, r *http.Request) {
	service := rs.agentArtifacts
	if service == nil {
		service = appui.NewAgentUIArtifactService(nil)
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/agent-ui-artifacts"), "/")
	parts := splitResourcePath(path)

	switch r.Method {
	case http.MethodGet:
		if len(parts) == 0 {
			result, err := service.List(appui.AgentUIArtifactListRequest{
				Source: strings.TrimSpace(r.URL.Query().Get("source")),
				Type:   strings.TrimSpace(r.URL.Query().Get("type")),
				CaseID: strings.TrimSpace(r.URL.Query().Get("caseId")),
				Limit:  limitFromQuery(r),
				Cursor: strings.TrimSpace(r.URL.Query().Get("cursor")),
			})
			writeResourceResult(w, result, err)
			return
		}
		if len(parts) == 1 {
			result, err := service.Get(parts[0])
			writeResourceResult(w, result, err)
			return
		}
		writeResourceError(w, http.StatusNotFound, "agent ui artifact endpoint not found")
	case http.MethodPost:
		if len(parts) != 1 || parts[0] != "validate" {
			writeResourceError(w, http.StatusNotFound, "agent ui artifact endpoint not found")
			return
		}
		var req appui.AgentUIArtifactValidationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeResourceError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		result, err := service.Validate(req)
		writeResourceResult(w, result, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
