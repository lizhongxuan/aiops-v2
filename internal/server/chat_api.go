package server

import (
	"encoding/json"
	"io"
	"net/http"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handleChatStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req appui.StopCommand
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}

	result, err := s.ui.ChatService().StopTurn(r.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "no active turn found" {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatMessageResponse{
		Accepted:        result.Error == "",
		SessionID:       result.SessionID,
		TurnID:          result.TurnID,
		ClientTurnID:    result.ClientTurnID,
		ClientMessageID: result.ClientMessageID,
		Status:          result.Status,
		Output:          result.Output,
		Error:           result.Error,
	})
}
