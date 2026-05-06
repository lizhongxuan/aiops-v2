package server

import (
	"io"
	"net/http"

	"aiops-v2/internal/appui"
)

type corootWebhookHTTPServices interface {
	CorootWebhookService() appui.CorootWebhookService
}

func (s *HTTPServer) handleCorootWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	provider, ok := s.ui.(corootWebhookHTTPServices)
	if !ok || provider.CorootWebhookService() == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "coroot webhook service is not configured"})
		return
	}
	payload, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unable to read request body"})
		return
	}
	result, err := provider.CorootWebhookService().Handle(r.Context(), appui.CorootWebhookCommand{Payload: payload})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
