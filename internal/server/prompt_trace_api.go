package server

import (
	"net/http"
	"strconv"

	"aiops-v2/internal/appui"
)

func (s *HTTPServer) handlePromptTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	includeControlChain := false
	if raw := r.URL.Query().Get("includeControlChain"); raw != "" {
		var err error
		includeControlChain, err = strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, "invalid includeControlChain", http.StatusBadRequest)
			return
		}
	}
	result, err := s.promptTraceService().ListModelInputTraces(r.Context(), appui.PromptTraceListRequest{
		Limit:               limit,
		Query:               r.URL.Query().Get("q"),
		CaseID:              r.URL.Query().Get("caseId"),
		Trace:               r.URL.Query().Get("trace"),
		SessionID:           r.URL.Query().Get("sessionId"),
		TurnID:              r.URL.Query().Get("turnId"),
		IncludeControlChain: includeControlChain,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) handlePromptTraceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := s.promptTraceService().GetModelInputTraceFile(r.Context(), appui.PromptTraceFileRequest{
		Path: r.URL.Query().Get("path"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *HTTPServer) promptTraceService() appui.PromptTraceService {
	if s.promptTraces != nil {
		return s.promptTraces
	}
	return appui.NewPromptTraceService("")
}
