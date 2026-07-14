package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"

	"aiops-v2/internal/store"
)

const (
	maxCorootProbeResponseBytes = 10 << 20

	defaultCorootProductBasePath = "/coroot/"
	defaultCorootGatewayBasePath = "/_coroot/"
	defaultCorootEmbedMode       = "readonly"
	defaultCorootAuthMode        = "anonymous_readonly"
	defaultCorootReturnFallback  = "/"
)

var (
	errCorootConfigNotFound = errors.New("coroot config not found")
	errCorootConfigNil      = errors.New("coroot config is nil")
)

type corootProxyConfig struct {
	BaseURL          string
	Token            string
	Username         string
	Password         string
	Project          string
	IframeURL        string
	Timeout          time.Duration
	UiGatewayEnabled bool
	GatewayBasePath  string
	ProductBasePath  string
	EmbedMode        string
	AuthMode         string
	EmbedTrustSecret string
	AllowedViews     []string
	ReturnFallback   string
}

func (cfg corootProxyConfig) configured() bool {
	return strings.TrimSpace(cfg.BaseURL) != ""
}

func (cfg corootProxyConfig) resolvedProject() string {
	if project := strings.TrimSpace(cfg.Project); project != "" {
		return project
	}
	return "default"
}

type corootConfigRequest struct {
	BaseURL          string   `json:"baseUrl"`
	Token            string   `json:"token"`
	ClearToken       bool     `json:"clearToken"`
	Username         string   `json:"username"`
	Password         string   `json:"password"`
	ClearPassword    bool     `json:"clearPassword"`
	Project          string   `json:"project"`
	IframeURL        string   `json:"iframeUrl"`
	Timeout          string   `json:"timeout"`
	UiGatewayEnabled bool     `json:"uiGatewayEnabled"`
	GatewayBasePath  string   `json:"gatewayBasePath"`
	ProductBasePath  string   `json:"productBasePath"`
	EmbedMode        string   `json:"embedMode"`
	AuthMode         string   `json:"authMode"`
	EmbedTrustSecret string   `json:"embedTrustSecret"`
	AllowedViews     []string `json:"allowedViews"`
	ReturnFallback   string   `json:"returnFallback"`
}

// Coroot Proxy - read-only reverse proxy to Coroot for human UI access.
// Model evidence collection must use internal/integrations/coroot tools instead.
func (rs *ResourceServer) handleCorootProxy(w http.ResponseWriter, r *http.Request) {
	if isCorootConfigPath(r.URL.Path) {
		switch r.Method {
		case http.MethodGet:
			rs.handleCorootConfig(w)
		case http.MethodPost, http.MethodPut:
			rs.handleCorootConfigUpdate(w, r)
		default:
			writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	}

	if isCorootTestConnectionPath(r.URL.Path) {
		if r.Method != http.MethodPost {
			writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed: coroot connection test requires POST"})
			return
		}
		rs.handleCorootTestConnection(w, r)
		return
	}

	if r.Method != http.MethodGet {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed: coroot proxy is read-only"})
		return
	}

	cfg := rs.currentCorootProxyConfig()
	if !cfg.configured() {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coroot not configured"})
		return
	}

	upstreamPath := corootUpstreamPath(r.URL.Path)
	if !isAllowedCorootPath(upstreamPath) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "coroot path not allowed"})
		return
	}

	target, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid coroot base url"})
		return
	}
	upstreamBasePath := corootConfiguredUpstreamBasePath(target.Path, cfg.ProductBasePath)

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = joinCorootProxyPath(upstreamBasePath, upstreamPath)
			req.URL.RawQuery = joinCorootProxyQuery(target.RawQuery, r.URL.RawQuery)
			req.Host = target.Host
			req.Header.Set("Accept", r.Header.Get("Accept"))
			req.Header.Del("Cookie")
			req.Header.Del("Authorization")
			req.Header.Del("X-Runner-Token")
			setCorootAuthHeaders(req.Header, cfg.Token)
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
			log.Printf("coroot proxy error: %v", proxyErr)
			writeResourceJSON(rw, http.StatusBadGateway, map[string]string{"error": "coroot upstream error"})
		},
	}
	if cfg.Timeout > 0 {
		proxy.Transport = newCorootHTTPTransport(cfg.Timeout)
	}

	proxy.ServeHTTP(w, r)
}

func (rs *ResourceServer) handleCorootConfig(w http.ResponseWriter) {
	writeResourceJSON(w, http.StatusOK, rs.corootConfigResponse(rs.currentCorootStoreConfig()))
}

func (rs *ResourceServer) handleCorootConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var payload corootConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "decode coroot config failed"})
		return
	}
	next, err := rs.corootConfigFromRequest(payload)
	if err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := rs.corootConfig.SaveCorootConfig(next); err != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "save coroot config failed"})
		return
	}
	writeResourceJSON(w, http.StatusOK, rs.corootConfigResponse(next))
}

func (rs *ResourceServer) currentCorootStoreConfig() *store.CorootConfig {
	if rs == nil || rs.corootConfig == nil {
		return nil
	}
	cfg, err := rs.corootConfig.GetCorootConfig()
	if err != nil {
		return nil
	}
	return cfg
}

func (rs *ResourceServer) currentCorootProxyConfig() corootProxyConfig {
	return corootProxyConfigFromStore(rs.currentCorootStoreConfig())
}

func (rs *ResourceServer) corootConfigFromRequest(payload corootConfigRequest) (*store.CorootConfig, error) {
	productBasePath := normalizeBasePath(payload.ProductBasePath, defaultCorootProductBasePath)
	baseURL, err := normalizeCorootConfigBaseURL(payload.BaseURL, productBasePath)
	if strings.TrimSpace(payload.BaseURL) == "" {
		return nil, fmt.Errorf("baseUrl is required")
	}
	if err != nil {
		return nil, fmt.Errorf("invalid baseUrl")
	}
	timeout := strings.TrimSpace(payload.Timeout)
	if timeout != "" {
		if parsedTimeout, err := time.ParseDuration(timeout); err != nil || parsedTimeout <= 0 {
			return nil, fmt.Errorf("invalid timeout")
		}
	}

	existing := rs.currentCorootStoreConfig()
	token := strings.TrimSpace(payload.Token)
	if token == "" && existing != nil && !payload.ClearToken {
		token = strings.TrimSpace(existing.Token)
	}
	username := strings.TrimSpace(payload.Username)
	if username == "" && existing != nil {
		username = strings.TrimSpace(existing.Username)
	}
	password := strings.TrimSpace(payload.Password)
	if password == "" && existing != nil && !payload.ClearPassword {
		password = strings.TrimSpace(existing.Password)
	}
	embedTrustSecret := strings.TrimSpace(payload.EmbedTrustSecret)
	if embedTrustSecret == "" && existing != nil {
		embedTrustSecret = strings.TrimSpace(existing.EmbedTrustSecret)
	}
	lastSuccessAt := ""
	var createdAt time.Time
	if existing != nil {
		lastSuccessAt = strings.TrimSpace(existing.LastSuccessAt)
		createdAt = existing.CreatedAt
	}
	return &store.CorootConfig{
		BaseURL:          baseURL,
		Token:            token,
		Username:         username,
		Password:         password,
		Project:          strings.TrimSpace(payload.Project),
		IframeURL:        strings.TrimSpace(payload.IframeURL),
		Timeout:          timeout,
		LastSuccessAt:    lastSuccessAt,
		UiGatewayEnabled: payload.UiGatewayEnabled,
		GatewayBasePath:  normalizeBasePath(payload.GatewayBasePath, defaultCorootGatewayBasePath),
		ProductBasePath:  productBasePath,
		EmbedMode:        normalizeCorootEmbedMode(payload.EmbedMode),
		AuthMode:         normalizeCorootAuthMode(payload.AuthMode),
		EmbedTrustSecret: embedTrustSecret,
		AllowedViews:     normalizeCorootAllowedViews(payload.AllowedViews),
		ReturnFallback:   normalizeCorootReturnFallback(payload.ReturnFallback),
		CreatedAt:        createdAt,
	}, nil
}

func (rs *ResourceServer) corootConfigResponse(cfg *store.CorootConfig) map[string]any {
	proxy := corootProxyConfigFromStore(cfg)
	if !proxy.configured() {
		return map[string]any{
			"configured":      false,
			"iframeMode":      false,
			"tokenConfigured": false,
		}
	}
	iframeURL := strings.TrimSpace(proxy.IframeURL)
	productBasePath := normalizeBasePath(proxy.ProductBasePath, defaultCorootProductBasePath)
	gatewayBasePath := normalizeBasePath(proxy.GatewayBasePath, defaultCorootGatewayBasePath)
	baseURL, err := normalizeCorootConfigBaseURL(proxy.BaseURL, productBasePath)
	if err != nil {
		baseURL = strings.TrimRight(strings.TrimSpace(proxy.BaseURL), "/")
	}
	project := proxy.resolvedProject()
	entryPath := path.Join(productBasePath, "p", project, "applications")
	if !strings.HasPrefix(entryPath, "/") {
		entryPath = "/" + entryPath
	}
	iframeEntryPath := path.Join(gatewayBasePath, "p", project, "applications")
	if !strings.HasPrefix(iframeEntryPath, "/") {
		iframeEntryPath = "/" + iframeEntryPath
	}
	iframeEntryPath += "?embed=1"
	if iframeURL == "" {
		iframeURL = iframeEntryPath
	}
	out := map[string]any{
		"configured":         true,
		"baseUrl":            baseURL,
		"proxyBaseUrl":       "/api/v1/coroot/",
		"iframeUrl":          iframeURL,
		"iframeMode":         true,
		"project":            project,
		"timeout":            proxy.Timeout.String(),
		"tokenConfigured":    strings.TrimSpace(proxy.Token) != "",
		"username":           strings.TrimSpace(proxy.Username),
		"passwordConfigured": strings.TrimSpace(proxy.Password) != "",
		"productBasePath":    productBasePath,
		"gatewayBasePath":    gatewayBasePath,
		"entryPath":          entryPath,
		"iframeEntryPath":    iframeEntryPath,
		"authMode":           normalizeCorootAuthMode(proxy.AuthMode),
		"embedMode":          normalizeCorootEmbedMode(proxy.EmbedMode),
		"uiGatewayEnabled":   proxy.UiGatewayEnabled,
		"allowedViews":       append([]string(nil), proxy.AllowedViews...),
		"returnFallback":     normalizeCorootReturnFallback(proxy.ReturnFallback),
	}
	if cfg != nil && strings.TrimSpace(cfg.LastSuccessAt) != "" {
		out["lastSuccessAt"] = strings.TrimSpace(cfg.LastSuccessAt)
	}
	return out
}

func corootProxyConfigFromStore(cfg *store.CorootConfig) corootProxyConfig {
	if cfg == nil {
		return corootProxyConfig{
			Timeout:         30 * time.Second,
			GatewayBasePath: defaultCorootGatewayBasePath,
			ProductBasePath: defaultCorootProductBasePath,
			EmbedMode:       defaultCorootEmbedMode,
			AuthMode:        defaultCorootAuthMode,
			ReturnFallback:  defaultCorootReturnFallback,
		}
	}
	timeout := 30 * time.Second
	if raw := strings.TrimSpace(cfg.Timeout); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	return corootProxyConfig{
		BaseURL:          strings.TrimSpace(cfg.BaseURL),
		Token:            strings.TrimSpace(cfg.Token),
		Username:         strings.TrimSpace(cfg.Username),
		Password:         strings.TrimSpace(cfg.Password),
		Project:          strings.TrimSpace(cfg.Project),
		IframeURL:        strings.TrimSpace(cfg.IframeURL),
		Timeout:          timeout,
		UiGatewayEnabled: cfg.UiGatewayEnabled,
		GatewayBasePath:  normalizeBasePath(cfg.GatewayBasePath, defaultCorootGatewayBasePath),
		ProductBasePath:  normalizeBasePath(cfg.ProductBasePath, defaultCorootProductBasePath),
		EmbedMode:        normalizeCorootEmbedMode(cfg.EmbedMode),
		AuthMode:         normalizeCorootAuthMode(cfg.AuthMode),
		EmbedTrustSecret: strings.TrimSpace(cfg.EmbedTrustSecret),
		AllowedViews:     append([]string(nil), cfg.AllowedViews...),
		ReturnFallback:   normalizeCorootReturnFallback(cfg.ReturnFallback),
	}
}

func (rs *ResourceServer) handleCorootTestConnection(w http.ResponseWriter, r *http.Request) {
	cfg, persistLastSuccess, err := rs.corootProbeConfigFromRequest(r)
	if err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !cfg.configured() {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "coroot not configured"})
		return
	}
	target, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || target.Scheme == "" || target.Host == "" {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid coroot base url"})
		return
	}
	baseTarget := *target
	project := cfg.resolvedProject()
	probePath := "/api/project/" + url.PathEscape(project) + "/overview/applications"
	target.Path = joinCorootProxyPath(corootConfiguredUpstreamBasePath(target.Path, cfg.ProductBasePath), probePath)
	target.RawQuery = ""

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout, Transport: newCorootHTTPTransport(timeout)}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String(), nil)
	if err != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "build coroot probe request failed"})
		return
	}
	req.Header.Set("Accept", "application/json")
	applyCorootProbeAuthHeaders(req.Header, cfg, r)
	if err := ensureCorootProbeWebSession(r.Context(), client, &baseTarget, req.Header, cfg); err != nil {
		writeCorootProbeError(w, http.StatusBadGateway, "coroot login failed", map[string]any{
			"detail":  err.Error(),
			"project": project,
			"uri":     corootSafeURI(target),
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("coroot test connection error: %v", err)
		writeCorootProbeError(w, http.StatusBadGateway, "coroot upstream error", map[string]any{
			"detail":  fmt.Sprintf("Coroot probe request failed for GET %s: %v", corootSafeURI(target), err),
			"project": project,
			"uri":     corootSafeURI(target),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCorootProbeResponseBytes))
	if err != nil {
		writeCorootProbeError(w, http.StatusBadGateway, "read coroot upstream response failed", map[string]any{
			"detail":      fmt.Sprintf("Read Coroot response failed for GET %s: %v", corootSafeURI(target), err),
			"project":     project,
			"uri":         corootSafeURI(target),
			"statusCode":  resp.StatusCode,
			"contentType": resp.Header.Get("Content-Type"),
		})
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := fmt.Sprintf("Coroot upstream returned HTTP %d for GET %s", resp.StatusCode, corootSafeURI(target))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			detail += ". Check the Coroot API Key and Project ID permissions."
		}
		writeCorootProbeError(w, http.StatusBadGateway, "coroot upstream returned non-success status", map[string]any{
			"detail":          detail,
			"statusCode":      resp.StatusCode,
			"project":         project,
			"uri":             corootSafeURI(target),
			"contentType":     resp.Header.Get("Content-Type"),
			"responsePreview": corootResponsePreview(body),
		})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.TrimSpace(contentType) == "" {
			contentType = "unknown"
		}
		writeCorootProbeError(w, http.StatusBadGateway, "decode coroot upstream response failed", map[string]any{
			"detail":          fmt.Sprintf("Coroot upstream returned a non-JSON response for GET %s (HTTP %d, Content-Type: %s). Check Base URL and Project ID; AIOps appends the Coroot product path automatically.", corootSafeURI(target), resp.StatusCode, contentType),
			"statusCode":      resp.StatusCode,
			"project":         project,
			"uri":             corootSafeURI(target),
			"contentType":     contentType,
			"responsePreview": corootResponsePreview(body),
		})
		return
	}
	data, hasData := payload["data"]
	if !hasData || data == nil {
		writeCorootProbeError(w, http.StatusBadGateway, fmt.Sprintf("coroot project %q has no application data", project), map[string]any{
			"detail":          fmt.Sprintf("Coroot response for GET %s did not contain a non-null data field.", corootSafeURI(target)),
			"statusCode":      resp.StatusCode,
			"project":         project,
			"uri":             corootSafeURI(target),
			"contentType":     resp.Header.Get("Content-Type"),
			"responsePreview": corootResponsePreview(body),
		})
		return
	}

	lastSuccessAt := time.Now().UTC().Format(time.RFC3339)
	if persistLastSuccess {
		rs.persistCorootLastSuccess(lastSuccessAt)
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"project":          project,
		"baseUrl":          strings.TrimSpace(cfg.BaseURL),
		"applicationCount": corootApplicationCount(data),
		"lastSuccessAt":    lastSuccessAt,
	})
}

func (rs *ResourceServer) corootProbeConfigFromRequest(r *http.Request) (corootProxyConfig, bool, error) {
	if r == nil || r.Body == nil {
		return rs.currentCorootProxyConfig(), true, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return corootProxyConfig{}, false, fmt.Errorf("read coroot connection test payload failed")
	}
	if strings.TrimSpace(string(body)) == "" {
		return rs.currentCorootProxyConfig(), true, nil
	}
	var payload corootConfigRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return corootProxyConfig{}, false, fmt.Errorf("decode coroot connection test payload failed")
	}
	cfg, err := rs.corootConfigFromRequest(payload)
	if err != nil {
		return corootProxyConfig{}, false, err
	}
	return corootProxyConfigFromStore(cfg), false, nil
}

func applyCorootProbeAuthHeaders(header http.Header, cfg corootProxyConfig, source *http.Request) {
	switch cfg.AuthMode {
	case "embed_trust":
		if strings.TrimSpace(cfg.EmbedTrustSecret) == "" {
			return
		}
		signed := signedCorootEmbedHeaders(corootEmbedIdentity{
			User:   "aiops-v2",
			Roles:  []string{"coroot-readonly"},
			Tenant: "default",
		}, cfg.EmbedTrustSecret, time.Now())
		for key, values := range signed {
			for _, value := range values {
				header.Add(key, value)
			}
		}
	case "session_passthrough":
		forwardCorootSessionCookie(header, source)
		if header.Get("Cookie") == "" {
			setCorootAuthHeaders(header, cfg.Token)
		}
	default:
		setCorootAuthHeaders(header, cfg.Token)
	}
}

func ensureCorootProbeWebSession(ctx context.Context, client *http.Client, target *url.URL, header http.Header, cfg corootProxyConfig) error {
	if client == nil || target == nil || header == nil {
		return nil
	}
	if strings.TrimSpace(header.Get("Cookie")) != "" || strings.TrimSpace(cfg.AuthMode) == "embed_trust" {
		return nil
	}
	if strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "" {
		return nil
	}
	cookie, err := corootLoginSessionCookie(ctx, client, target, cfg)
	if err != nil {
		return err
	}
	header.Set("Cookie", cookie.String())
	return nil
}

func corootLoginSessionCookie(ctx context.Context, client *http.Client, target *url.URL, cfg corootProxyConfig) (*http.Cookie, error) {
	loginURL := *target
	loginURL.Path = joinCorootProxyPath(corootConfiguredUpstreamBasePath(target.Path, cfg.ProductBasePath), "/api/login")
	loginURL.RawQuery = ""
	payload, _ := json.Marshal(map[string]string{
		"email":    strings.TrimSpace(cfg.Username),
		"password": strings.TrimSpace(cfg.Password),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build Coroot login request failed: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Coroot login request failed for POST %s: %w", corootSafeURI(&loginURL), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Coroot login returned HTTP %d for POST %s: %s", resp.StatusCode, corootSafeURI(&loginURL), corootResponsePreview(body))
	}
	for _, cookie := range resp.Cookies() {
		if cookie != nil && cookie.Name == "coroot_session" && strings.TrimSpace(cookie.Value) != "" {
			return &http.Cookie{Name: "coroot_session", Value: strings.TrimSpace(cookie.Value)}, nil
		}
	}
	return nil, fmt.Errorf("Coroot login did not return coroot_session cookie")
}

func (rs *ResourceServer) persistCorootLastSuccess(lastSuccessAt string) {
	if rs == nil || rs.corootConfig == nil {
		return
	}
	cfg := rs.currentCorootStoreConfig()
	if cfg == nil {
		return
	}
	cfg.LastSuccessAt = strings.TrimSpace(lastSuccessAt)
	if err := rs.corootConfig.SaveCorootConfig(cfg); err != nil {
		log.Printf("persist coroot last success failed: %v", err)
	}
}

func isCorootConfigPath(requestPath string) bool {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	return cleaned == "/api/v1/coroot/config"
}

func isCorootTestConnectionPath(requestPath string) bool {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	return cleaned == "/api/v1/coroot/test-connection"
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

func setCorootAuthHeaders(header http.Header, token string) {
	if token = strings.TrimSpace(token); token == "" {
		return
	}
	if cookieValue := corootSessionCookieValue(token); cookieValue != "" {
		header.Set("Cookie", (&http.Cookie{Name: "coroot_session", Value: cookieValue}).String())
		return
	}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Api-Key", token)
}

func corootSessionCookieValue(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(token), "cookie:") {
		token = strings.TrimSpace(token[len("cookie:"):])
	}
	for _, part := range strings.Split(token, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "coroot_session=") {
			return strings.TrimSpace(part[len("coroot_session="):])
		}
	}
	if strings.Contains(token, ".") && !strings.ContainsAny(token, " \t\r\n;") {
		return token
	}
	return ""
}

func newCorootHTTPTransport(responseHeaderTimeout time.Duration) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if responseHeaderTimeout > 0 {
		transport.ResponseHeaderTimeout = responseHeaderTimeout
	}
	return transport
}

func normalizeBasePath(raw, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = fallback
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	return v
}

func normalizeCorootConfigBaseURL(raw, productBasePath string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("baseUrl is required")
	}
	parsed, err := url.Parse(v)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid baseUrl")
	}
	parsed.Path = stripCorootProductPath(parsed.Path, productBasePath)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func stripCorootProductPath(rawPath string, productBasePath string) string {
	base := strings.TrimRight(strings.TrimSpace(rawPath), "/")
	if base == "" || base == "." || base == "/" {
		return ""
	}
	product := strings.TrimRight(normalizeBasePath(productBasePath, defaultCorootProductBasePath), "/")
	if product == "" || product == "/" {
		return base
	}
	if base == product {
		return ""
	}
	if strings.HasSuffix(base, product) {
		return strings.TrimRight(strings.TrimSuffix(base, product), "/")
	}
	return base
}

func corootConfiguredUpstreamBasePath(basePath string, productBasePath string) string {
	base := strings.TrimRight(strings.TrimSpace(basePath), "/")
	if base == "." || base == "/" {
		base = ""
	}
	product := strings.TrimRight(normalizeBasePath(productBasePath, defaultCorootProductBasePath), "/")
	if product == "" || product == "/" {
		if base == "" {
			return "/"
		}
		return base
	}
	if base == "" {
		return product
	}
	if base == product || strings.HasSuffix(base, product) {
		return base
	}
	return base + product
}

func normalizeCorootAuthMode(raw string) string {
	switch strings.TrimSpace(raw) {
	case "embed_trust", "session_passthrough":
		return strings.TrimSpace(raw)
	default:
		return defaultCorootAuthMode
	}
}

func normalizeCorootEmbedMode(raw string) string {
	switch strings.TrimSpace(raw) {
	case "full":
		return "full"
	case "readonly":
		return "readonly"
	default:
		return defaultCorootEmbedMode
	}
}

func normalizeCorootReturnFallback(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return defaultCorootReturnFallback
	}
	if strings.HasPrefix(v, "/") {
		return v
	}
	return "/" + v
}

func normalizeCorootAllowedViews(raw []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func corootApplicationCount(data any) int {
	obj, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	apps, ok := obj["applications"].([]any)
	if !ok {
		return 0
	}
	return len(apps)
}

func writeCorootProbeError(w http.ResponseWriter, status int, message string, fields map[string]any) {
	payload := map[string]any{"error": message}
	for key, value := range fields {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				payload[key] = typed
			}
		default:
			if value != nil {
				payload[key] = value
			}
		}
	}
	writeResourceJSON(w, status, payload)
}

func corootSafeURI(target *url.URL) string {
	if target == nil {
		return ""
	}
	safe := *target
	safe.User = nil
	return safe.String()
}

func corootResponsePreview(body []byte) string {
	preview := strings.Join(strings.Fields(string(body)), " ")
	if len(preview) > 500 {
		return preview[:500] + "..."
	}
	return preview
}
