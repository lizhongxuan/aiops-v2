package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

type chatArchiveHTTPServices interface {
	ChatArchiveService() appui.ChatArchiveService
}

func (s *HTTPServer) handleChatOpsRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider, ok := s.ui.(chatArchiveHTTPServices)
	if !ok || provider.ChatArchiveService() == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "chat archive service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/chat/ops-runs/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ops run archive endpoint not found"})
		return
	}
	req := appui.ChatArchiveRequest{OpsRunID: strings.TrimSpace(parts[0])}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if strings.TrimSpace(req.OpsRunID) == "" {
		req.OpsRunID = strings.TrimSpace(parts[0])
	}
	service := provider.ChatArchiveService()
	switch parts[1] {
	case "archive-case":
		result, err := service.ArchiveCase(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "run-record":
		result, err := service.CreateRunRecord(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	case "experience-candidates":
		result, err := service.CreateExperienceCandidates(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ops run archive endpoint not found"})
	}
}
