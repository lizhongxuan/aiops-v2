package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/appui"
)

const corootGatewayPrefix = "/_coroot"

type corootEmbedIdentity struct {
	User   string
	Roles  []string
	Tenant string
}

func newCorootUIGateway(repo appui.CorootConfigRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := corootProxyConfigFromRepository(repo)
		if !cfg.configured() {
			writeCorootGatewayError(w, http.StatusServiceUnavailable, "coroot not configured", nil)
			return
		}
		if corootGatewayWriteMethod(r.Method) && !corootGatewayAuthEndpoint(r.URL.Path) && (cfg.AuthMode != "embed_trust" || cfg.EmbedMode != "full") {
			writeCorootGatewayError(w, http.StatusForbidden, "coroot write operations are disabled", nil)
			return
		}
		if corootGatewayWriteMethod(r.Method) && !corootGatewayAuthEndpoint(r.URL.Path) && strings.TrimSpace(cfg.EmbedTrustSecret) == "" {
			writeCorootGatewayError(w, http.StatusForbidden, "coroot embed trust secret is not configured", nil)
			return
		}

		target, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
		if err != nil || target.Scheme == "" || target.Host == "" {
			writeCorootGatewayError(w, http.StatusInternalServerError, "invalid coroot base url", nil)
			return
		}

		gatewayBasePath := normalizeBasePath(cfg.GatewayBasePath, defaultCorootGatewayBasePath)
		upstreamBasePath := corootConfiguredUpstreamBasePath(target.Path, cfg.ProductBasePath)
		parentOrigin := corootGatewayParentOrigin(r)
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.URL.Path = corootGatewayUpstreamPath(upstreamBasePath, r.URL.Path)
				req.URL.RawQuery = joinCorootProxyQuery(target.RawQuery, r.URL.RawQuery)
				req.Host = target.Host
				sanitizeCorootGatewayRequestHeaders(req.Header)
				if cfg.AuthMode == "session_passthrough" {
					forwardCorootSessionCookie(req.Header, r)
				}
				if cfg.AuthMode == "embed_trust" && strings.TrimSpace(cfg.EmbedTrustSecret) != "" {
					applyCorootEmbedTrustHeaders(req.Header, cfg, r, time.Now())
				} else if cfg.AuthMode != "session_passthrough" {
					setCorootAuthHeaders(req.Header, cfg.Token)
				}
				req.Header.Set("X-Aiops-Coroot-Embed", "true")
				req.Header.Set("X-Forwarded-Host", r.Host)
				if r.TLS != nil {
					req.Header.Set("X-Forwarded-Proto", "https")
				} else {
					req.Header.Set("X-Forwarded-Proto", "http")
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				rewriteCorootGatewayResponseHeaders(resp, upstreamBasePath, gatewayBasePath)
				if corootGatewayWriteMethod(r.Method) {
					log.Printf("coroot gateway write audit user=%q project=%q method=%s path=%q status=%d request_id=%q", corootEmbedIdentityFromRequest(r).User, cfg.resolvedProject(), r.Method, r.URL.Path, resp.StatusCode, requestID)
				}
				if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
					rewritten := rewriteCorootIndexHTML(body, upstreamBasePath, gatewayBasePath, parentOrigin)
					resp.Body = io.NopCloser(bytes.NewReader(rewritten))
					resp.ContentLength = int64(len(rewritten))
					resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
				}
				return nil
			},
			ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, proxyErr error) {
				log.Printf("coroot ui gateway error: %v", proxyErr)
				writeCorootGatewayError(rw, http.StatusBadGateway, "coroot upstream error", map[string]any{"detail": proxyErr.Error()})
			},
		}
		if cfg.Timeout > 0 {
			proxy.Transport = newCorootHTTPTransport(cfg.Timeout)
		}
		proxy.ServeHTTP(w, r)
	})
}

func corootProxyConfigFromRepository(repo appui.CorootConfigRepository) corootProxyConfig {
	if repo == nil {
		return corootProxyConfigFromStore(nil)
	}
	cfg, err := repo.GetCorootConfig()
	if err != nil {
		return corootProxyConfigFromStore(nil)
	}
	return corootProxyConfigFromStore(cfg)
}

func corootGatewayUpstreamPath(basePath string, requestPath string) string {
	trimmed := strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(requestPath)), corootGatewayPrefix)
	if trimmed == "" || trimmed == "." {
		trimmed = "/"
	}
	return joinCorootProxyPath(basePath, trimmed)
}

func sanitizeCorootGatewayRequestHeaders(header http.Header) {
	header.Del("Authorization")
	header.Del("Cookie")
	header.Del("X-Runner-Token")
	header.Del("X-Aiops-Session")
	header.Del("X-Aiops-Embed-User")
	header.Del("X-Aiops-Embed-Roles")
	header.Del("X-Aiops-Embed-Tenant")
	header.Del("X-Aiops-Embed-Signature")
	header.Del("X-Aiops-Embed-Timestamp")
}

func forwardCorootSessionCookie(header http.Header, r *http.Request) {
	if r == nil {
		return
	}
	if cookie, err := r.Cookie("coroot_session"); err == nil && strings.TrimSpace(cookie.Value) != "" {
		header.Set("Cookie", cookie.String())
	}
}

func rewriteCorootGatewayResponseHeaders(resp *http.Response, upstreamBasePath string, gatewayBasePath string) {
	resp.Header.Del("X-Frame-Options")
	resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self'")
	if location := resp.Header.Get("Location"); location != "" {
		resp.Header.Set("Location", rewriteCorootGatewayLocation(location, upstreamBasePath, gatewayBasePath))
	}
	rewriteCorootGatewayCookies(resp, upstreamBasePath, gatewayBasePath)
}

func rewriteCorootIndexHTML(body []byte, upstreamBasePath string, gatewayBasePath string, parentOrigin string) []byte {
	upstreamBasePath = normalizeBasePath(upstreamBasePath, defaultCorootProductBasePath)
	gatewayBasePath = normalizeBasePath(gatewayBasePath, defaultCorootGatewayBasePath)
	html := string(body)
	html = strings.ReplaceAll(html, "base_path: '{{.BasePath}}'", "base_path: '"+gatewayBasePath+"'")
	html = strings.ReplaceAll(html, "base_path: '/coroot/'", "base_path: '"+gatewayBasePath+"'")
	html = strings.ReplaceAll(html, "base_path: \"/coroot/\"", "base_path: \""+gatewayBasePath+"\"")
	html = rewriteCorootIndexAssetURLs(html, upstreamBasePath, gatewayBasePath)
	html = injectCorootEmbedStyles(html)
	html = injectCorootEmbedGuard(html)
	inject := "embed: true,\n            embed_host: 'aiops-v2',"
	if parentOrigin != "" {
		inject += "\n            parent_origin: '" + escapeSingleQuotedJS(parentOrigin) + "',"
	}
	if strings.Contains(html, "window.coroot = {") && !strings.Contains(html, "embed: true") {
		html = strings.Replace(html, "window.coroot = {", "window.coroot = {\n            "+inject, 1)
	}
	return []byte(html)
}

func injectCorootEmbedStyles(html string) string {
	if strings.Contains(html, "data-aiops-coroot-embed-style") {
		return html
	}
	style := `<style data-aiops-coroot-embed-style>body .v-navigation-drawer{display:none!important;}body .v-main{padding-left:0!important;}body .v-application--wrap{padding-left:0!important;}body .v-app-bar,body header.v-toolbar{left:0!important;right:0!important;width:100%!important;}</style>`
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", style+"</head>", 1)
	}
	return style + html
}

func injectCorootEmbedGuard(html string) string {
	if strings.Contains(html, "data-aiops-coroot-embed-guard") {
		return html
	}
	script := `<script data-aiops-coroot-embed-guard>(function(){var markers=["Supercharge Coroot with AI","Coroot Cloud","AI-powered root cause analysis"];function promo(node){var text=(node&&node.textContent||"").replace(/\s+/g," ");return markers.some(function(marker){return text.indexOf(marker)!==-1;});}function hide(node){if(!node)return;node.setAttribute("data-aiops-coroot-hidden","cloud-upsell");node.style.setProperty("display","none","important");node.style.setProperty("visibility","hidden","important");node.style.setProperty("pointer-events","none","important");}function sweep(){var matched=false;document.querySelectorAll(".v-dialog__content,.v-dialog").forEach(function(node){if(promo(node)){hide(node.closest(".v-dialog__content")||node);matched=true;}});if(matched){document.querySelectorAll(".v-overlay,.v-overlay__scrim").forEach(hide);document.documentElement.style.removeProperty("overflow");document.body&&document.body.style.removeProperty("overflow");}}function schedule(){if(window.requestAnimationFrame){window.requestAnimationFrame(sweep);}else{setTimeout(sweep,0);}}if(document.readyState==="loading"){document.addEventListener("DOMContentLoaded",schedule,{once:true});}else{schedule();}new MutationObserver(schedule).observe(document.documentElement,{childList:true,subtree:true});})();</script>`
	if strings.Contains(html, "</body>") {
		return strings.Replace(html, "</body>", script+"</body>", 1)
	}
	if strings.Contains(html, "</head>") {
		return strings.Replace(html, "</head>", script+"</head>", 1)
	}
	return script + html
}

func rewriteCorootIndexAssetURLs(html string, upstreamBasePath string, gatewayBasePath string) string {
	upstreamBasePath = normalizeBasePath(upstreamBasePath, defaultCorootProductBasePath)
	gatewayBasePath = normalizeBasePath(gatewayBasePath, defaultCorootGatewayBasePath)
	replacements := []string{
		`href="` + upstreamBasePath,
		`href="` + gatewayBasePath,
		`src="` + upstreamBasePath,
		`src="` + gatewayBasePath,
		`href='` + upstreamBasePath,
		`href='` + gatewayBasePath,
		`src='` + upstreamBasePath,
		`src='` + gatewayBasePath,
	}
	return strings.NewReplacer(replacements...).Replace(html)
}

func writeCorootGatewayError(w http.ResponseWriter, status int, message string, fields map[string]any) {
	payload := map[string]any{"error": message}
	for key, value := range fields {
		if value != nil {
			payload[key] = value
		}
	}
	writeResourceJSON(w, status, payload)
}

func signedCorootEmbedHeaders(identity corootEmbedIdentity, secret string, now time.Time) http.Header {
	header := http.Header{}
	header.Set("X-Aiops-Embed-User", identity.User)
	header.Set("X-Aiops-Embed-Roles", strings.Join(identity.Roles, ","))
	header.Set("X-Aiops-Embed-Tenant", identity.Tenant)
	header.Set("X-Aiops-Embed-Timestamp", now.UTC().Format(time.RFC3339))
	payload := identity.User + "\n" + strings.Join(identity.Roles, ",") + "\n" + identity.Tenant + "\n" + header.Get("X-Aiops-Embed-Timestamp")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	header.Set("X-Aiops-Embed-Signature", hex.EncodeToString(mac.Sum(nil)))
	return header
}

func applyCorootEmbedTrustHeaders(header http.Header, cfg corootProxyConfig, r *http.Request, now time.Time) {
	signed := signedCorootEmbedHeaders(corootEmbedIdentityFromRequest(r), cfg.EmbedTrustSecret, now)
	for key, values := range signed {
		header.Del(key)
		for _, value := range values {
			header.Add(key, value)
		}
	}
}

func corootEmbedIdentityFromRequest(r *http.Request) corootEmbedIdentity {
	roles := []string{"coroot-readonly"}
	if r != nil && corootGatewayWriteMethod(r.Method) {
		roles = []string{"coroot-admin"}
	}
	return corootEmbedIdentity{
		User:   "aiops-v2",
		Roles:  roles,
		Tenant: "default",
	}
}

func corootGatewayWriteMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead
}

func corootGatewayAuthEndpoint(requestPath string) bool {
	upstreamPath := corootGatewayUpstreamPath("", requestPath)
	switch path.Clean(upstreamPath) {
	case "/api/login", "/api/logout":
		return true
	default:
		return false
	}
}

func rewriteCorootGatewayCookies(resp *http.Response, upstreamBasePath string, gatewayBasePath string) {
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return
	}
	resp.Header.Del("Set-Cookie")
	for _, cookie := range cookies {
		cookie.Path = rewriteCorootGatewayCookiePath(cookie.Path, upstreamBasePath, gatewayBasePath)
		resp.Header.Add("Set-Cookie", cookie.String())
	}
}

func rewriteCorootGatewayCookiePath(cookiePath string, upstreamBasePath string, gatewayBasePath string) string {
	upstreamBasePath = normalizeBasePath(upstreamBasePath, defaultCorootProductBasePath)
	gatewayBasePath = normalizeBasePath(gatewayBasePath, defaultCorootGatewayBasePath)
	cookiePath = strings.TrimSpace(cookiePath)
	if cookiePath == "" {
		return gatewayBasePath
	}
	if cookiePath == "/" {
		return gatewayBasePath
	}
	upstreamNoSlash := strings.TrimRight(upstreamBasePath, "/")
	if cookiePath == upstreamNoSlash || cookiePath == upstreamBasePath {
		return gatewayBasePath
	}
	if strings.HasPrefix(cookiePath, upstreamBasePath) {
		return gatewayBasePath + strings.TrimPrefix(cookiePath, upstreamBasePath)
	}
	return cookiePath
}

func rewriteCorootGatewayLocation(location string, upstreamBasePath string, gatewayBasePath string) string {
	parsed, err := url.Parse(location)
	if err != nil {
		return location
	}
	if parsed.Path == "" {
		return location
	}
	rewrittenPath := rewriteCorootGatewayPath(parsed.Path, upstreamBasePath, gatewayBasePath)
	if rewrittenPath == parsed.Path {
		return location
	}
	parsed.Path = rewrittenPath
	if parsed.Scheme != "" || parsed.Host != "" {
		parsed.Scheme = ""
		parsed.Host = ""
		parsed.User = nil
	}
	return parsed.String()
}

func rewriteCorootGatewayPath(rawPath string, upstreamBasePath string, gatewayBasePath string) string {
	upstreamBasePath = normalizeBasePath(upstreamBasePath, defaultCorootProductBasePath)
	gatewayBasePath = normalizeBasePath(gatewayBasePath, defaultCorootGatewayBasePath)
	upstreamNoSlash := strings.TrimRight(upstreamBasePath, "/")
	gatewayNoSlash := strings.TrimRight(gatewayBasePath, "/")
	switch {
	case rawPath == upstreamNoSlash:
		return gatewayNoSlash
	case rawPath == upstreamBasePath:
		return gatewayBasePath
	case strings.HasPrefix(rawPath, upstreamBasePath):
		return gatewayBasePath + strings.TrimPrefix(rawPath, upstreamBasePath)
	default:
		return rawPath
	}
}

func corootGatewayParentOrigin(r *http.Request) string {
	if r == nil {
		return ""
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	if strings.TrimSpace(r.Host) == "" {
		return ""
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func escapeSingleQuotedJS(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", "", "\r", "")
	return replacer.Replace(value)
}
