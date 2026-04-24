package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handleChoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/answer") {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Answers []any `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	requestID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/choices/"), "/answer")
	requestID = strings.Trim(requestID, "/")
	result, err := s.ui.ChoiceService().Answer(r.Context(), appui.ChoiceAnswer{
		RequestID: requestID,
		Answers:   req.Answers,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
