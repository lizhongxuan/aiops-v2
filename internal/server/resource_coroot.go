package server

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

type corootProxyConfig struct {
	BaseURL   string
	Token     string
	IframeURL string
	Timeout   time.Duration
}

func (cfg corootProxyConfig) configured() bool {
	return strings.TrimSpace(cfg.BaseURL) != ""
}

// Coroot Proxy - read-only reverse proxy to Coroot for human UI access.
// Model evidence collection must use internal/integrations/coroot tools instead.
func (rs *ResourceServer) handleCorootProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed: coroot proxy is read-only"})
		return
	}

	if isCorootConfigPath(r.URL.Path) {
		rs.handleCorootConfig(w)
		return
	}

	if !rs.coroot.configured() {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coroot not configured"})
		return
	}

	upstreamPath := corootUpstreamPath(r.URL.Path)
	if !isAllowedCorootPath(upstreamPath) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "coroot path not allowed"})
		return
	}

	target, err := url.Parse(strings.TrimSpace(rs.coroot.BaseURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid coroot base url"})
		return
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = joinCorootProxyPath(target.Path, upstreamPath)
			req.URL.RawQuery = joinCorootProxyQuery(target.RawQuery, r.URL.RawQuery)
			req.Host = target.Host
			req.Header.Set("Accept", r.Header.Get("Accept"))
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			req.Header.Del("X-Runner-Token")
			if token := strings.TrimSpace(rs.coroot.Token); token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
			log.Printf("coroot proxy error: %v", proxyErr)
			writeResourceJSON(rw, http.StatusBadGateway, map[string]string{"error": "coroot upstream error"})
		},
	}
	if rs.coroot.Timeout > 0 {
		proxy.Transport = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: rs.coroot.Timeout,
		}
	}

	proxy.ServeHTTP(w, r)
}

func (rs *ResourceServer) handleCorootConfig(w http.ResponseWriter) {
	if !rs.coroot.configured() {
		writeResourceJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"iframeMode": false,
		})
		return
	}

	iframeURL := strings.TrimSpace(rs.coroot.IframeURL)
	if iframeURL == "" {
		iframeURL = "/api/v1/coroot/"
	}

	writeResourceJSON(w, http.StatusOK, map[string]any{
		"configured": true,
		"baseUrl":    iframeURL,
		"iframeMode": true,
	})
}

func isCorootConfigPath(requestPath string) bool {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	return cleaned == "/api/v1/coroot/config"
}

func corootUpstreamPath(requestPath string) string {
	trimmed := strings.TrimPrefix(requestPath, "/api/v1/coroot")
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

func isAllowedCorootPath(upstreamPath string) bool {
	cleaned := path.Clean("/" + strings.TrimSpace(upstreamPath))
	if cleaned == "/" {
		return true
	}

	allowedReadAPIs := []string{
		"/api/project",
		"/api/v1/services",
		"/api/v1/topology",
		"/api/v1/incidents",
		"/api/v1/metrics",
		"/api/v1/status",
		"/api/v1/hosts",
		"/api/v1/slo",
		"/api/v1/slos",
	}
	for _, prefix := range allowedReadAPIs {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}

	allowedIframeAssets := []string{
		"/assets",
		"/static",
		"/p",
		"/favicon.ico",
		"/manifest.json",
	}
	for _, prefix := range allowedIframeAssets {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}
	return false
}

func joinCorootProxyPath(basePath, upstreamPath string) string {
	base := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if base == "" {
		if strings.HasPrefix(upstreamPath, "/") {
			return upstreamPath
		}
		return "/" + upstreamPath
	}
	if upstreamPath == "/" || upstreamPath == "" {
		return base + "/"
	}
	return base + "/" + strings.TrimLeft(upstreamPath, "/")
}

func joinCorootProxyQuery(baseQuery, requestQuery string) string {
	if baseQuery == "" {
		return requestQuery
	}
	if requestQuery == "" {
		return baseQuery
	}
	return baseQuery + "&" + requestQuery
}
