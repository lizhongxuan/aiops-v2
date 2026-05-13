package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s *HTTPServer) handleRunnerStudioAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || strings.Trim(r.URL.EscapedPath(), "/") != "api/runner-studio/ai/draft" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner studio AI endpoint not found"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload, err := sanitizeRunnerStudioAIDraftPayload(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	status := runnerStudioAIDraftStatus(payload)
	if status != "draft" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "AI patch is only allowed for draft workflows"})
		return
	}

	upstreamBody, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s.runnerStudioHandler != nil {
		req := r.Clone(r.Context())
		req.Method = http.MethodPost
		req.URL.Path = "/api/v1/workflows/ai/draft"
		req.URL.RawPath = ""
		req.RequestURI = ""
		req.Body = io.NopCloser(bytes.NewReader(upstreamBody))
		req.Header = http.Header{}
		copyRunnerStudioRequestHeaders(req.Header, r.Header)
		req.Header.Set("Content-Type", "application/json")
		s.runnerStudioHandler.ServeHTTP(w, req)
		return
	}

	upstream := strings.TrimSpace(s.runnerStudioUpstreamURL)
	if upstream == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner studio upstream is not configured"})
		return
	}
	targetURL, err := joinRunnerStudioUpstreamURL(upstream, "/api/v1/workflows/ai/draft", r.URL.RawQuery)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(upstreamBody))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	copyRunnerStudioRequestHeaders(req.Header, r.Header)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	copyRunnerStudioResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func sanitizeRunnerStudioAIDraftPayload(body []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("request body is required")
	}
	for _, key := range []string{"api_key", "apikey", "base_url", "baseURL", "model", "llm", "llm_config"} {
		delete(payload, key)
	}
	return payload, nil
}

func runnerStudioAIDraftStatus(payload map[string]any) string {
	status, _ := payload["workflow_status"].(string)
	if strings.TrimSpace(status) == "" {
		if workflow, ok := payload["workflow"].(map[string]any); ok {
			status, _ = workflow["status"].(string)
		}
	}
	return strings.ToLower(strings.TrimSpace(status))
}
