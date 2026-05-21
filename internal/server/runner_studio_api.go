package server

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const runnerStudioAPIPrefix = "/api/runner-studio"

func (s *HTTPServer) handleRunnerStudio(w http.ResponseWriter, r *http.Request) {
	targetPath, err := runnerStudioTargetPath(r.Method, r.URL.EscapedPath())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if s.runnerStudioHandler != nil {
		s.serveEmbeddedRunnerStudio(w, r, targetPath)
		return
	}
	upstream := strings.TrimSpace(s.runnerStudioUpstreamURL)
	if upstream == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runner studio upstream is not configured"})
		return
	}
	targetURL, err := joinRunnerStudioUpstreamURL(upstream, targetPath, r.URL.RawQuery)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	copyRunnerStudioRequestHeaders(req.Header, r.Header)
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

func (s *HTTPServer) serveEmbeddedRunnerStudio(w http.ResponseWriter, r *http.Request, targetPath string) {
	req := r.Clone(r.Context())
	req.URL.Path = targetPath
	req.URL.RawPath = ""
	req.RequestURI = ""
	req.Header = http.Header{}
	copyRunnerStudioRequestHeaders(req.Header, r.Header)
	s.runnerStudioHandler.ServeHTTP(w, req)
}

func runnerStudioTargetPath(method, path string) (string, error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, runnerStudioAPIPrefix), "/")
	parts := splitRunnerStudioPath(trimmed)
	switch {
	case method == http.MethodGet && (trimmed == "actions" || trimmed == "actions/catalog"):
		return "/api/v1/actions/catalog", nil
	case method == http.MethodGet && trimmed == "workflows":
		return "/api/v1/workflows", nil
	case method == http.MethodPost && trimmed == "workflows/bundles/import":
		return "/api/v1/workflows/bundles/import", nil
	case method == http.MethodPost && trimmed == "workflows/graph":
		return "/api/v1/workflows/graph", nil
	case method == http.MethodPost && trimmed == "workflows/graph/compile":
		return "/api/v1/workflows/graph/compile", nil
	case method == http.MethodPost && trimmed == "workflows/graph/parse":
		return "/api/v1/workflows/graph/parse", nil
	case method == http.MethodPost && trimmed == "workflows/graph/validate":
		return "/api/v1/workflows/graph/validate", nil
	case len(parts) == 3 && parts[0] == "workflows" && parts[2] == "validate" && method == http.MethodPost:
		return "/api/v1/workflows/" + parts[1] + "/validate", nil
	case method == http.MethodPost && trimmed == "workflows/graph/dry-run":
		return "/api/v1/workflows/graph/dry-run", nil
	case method == http.MethodPost && trimmed == "runs":
		return "/api/v1/workflows/graph/runs", nil
	case method == http.MethodGet && trimmed == "runs":
		return "/api/v1/runs", nil
	case method == http.MethodPost && trimmed == "workflows/graph/variables/resolve":
		return "/api/v1/workflows/graph/variables/resolve", nil
	case len(parts) == 3 && parts[0] == "workflows" && parts[2] == "graph" && (method == http.MethodGet || method == http.MethodPut):
		return "/api/v1/workflows/" + parts[1] + "/graph", nil
	case len(parts) == 3 && parts[0] == "workflows" && parts[2] == "bundle" && method == http.MethodGet:
		return "/api/v1/workflows/" + parts[1] + "/bundle", nil
	case len(parts) == 3 && parts[0] == "workflows" && parts[2] == "versions" && method == http.MethodGet:
		return "/api/v1/workflows/" + parts[1] + "/versions", nil
	case len(parts) == 4 && parts[0] == "workflows" && parts[2] == "versions" && method == http.MethodGet:
		return "/api/v1/workflows/" + parts[1] + "/versions/" + parts[3], nil
	case len(parts) == 5 && parts[0] == "workflows" && parts[2] == "versions" && parts[4] == "rollback" && method == http.MethodPost:
		return "/api/v1/workflows/" + parts[1] + "/versions/" + parts[3] + "/rollback", nil
	case len(parts) == 3 && parts[0] == "workflows" && parts[2] == "publish" && method == http.MethodPost:
		return "/api/v1/workflows/" + parts[1] + "/publish", nil
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "graph" && method == http.MethodGet:
		return "/api/v1/runs/" + parts[1] + "/graph", nil
	case len(parts) == 4 && parts[0] == "runs" && parts[2] == "events" && parts[3] == "history" && method == http.MethodGet:
		return "/api/v1/runs/" + parts[1] + "/events/history", nil
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "events" && method == http.MethodGet:
		return "/api/v1/runs/" + parts[1] + "/events", nil
	case len(parts) == 5 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "approve" && method == http.MethodPost:
		return "/api/v1/runs/" + parts[1] + "/nodes/" + parts[3] + "/approve", nil
	case len(parts) == 5 && parts[0] == "runs" && parts[2] == "nodes" && parts[4] == "reject" && method == http.MethodPost:
		return "/api/v1/runs/" + parts[1] + "/nodes/" + parts[3] + "/reject", nil
	case len(parts) == 3 && parts[0] == "runs" && parts[2] == "cancel" && method == http.MethodPost:
		return "/api/v1/runs/" + parts[1] + "/cancel", nil
	default:
		return "", fmt.Errorf("runner studio endpoint not found")
	}
}

func splitRunnerStudioPath(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw := strings.Split(path, "/")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func joinRunnerStudioUpstreamURL(upstream, path, rawQuery string) (string, error) {
	base, err := url.Parse(strings.TrimRight(upstream, "/"))
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid runner studio upstream url")
	}
	prefix := base.Scheme + "://"
	if base.User != nil {
		prefix += base.User.String() + "@"
	}
	prefix += base.Host + strings.TrimRight(base.EscapedPath(), "/")
	if rawQuery != "" {
		return prefix + path + "?" + rawQuery, nil
	}
	return prefix + path, nil
}

func copyRunnerStudioRequestHeaders(dst, src http.Header) {
	for key, values := range src {
		if !isAllowedRunnerStudioRequestHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyRunnerStudioResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if !isAllowedRunnerStudioResponseHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isAllowedRunnerStudioRequestHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Accept", "Content-Type", "Idempotency-Key", "User-Agent", "X-Request-Id", "X-Trace-Id":
		return true
	default:
		return false
	}
}

func isAllowedRunnerStudioResponseHeader(key string) bool {
	switch http.CanonicalHeaderKey(key) {
	case "Cache-Control", "Content-Type", "Etag", "Last-Modified", "Location", "Retry-After", "X-Request-Id", "X-Trace-Id":
		return true
	default:
		return false
	}
}
