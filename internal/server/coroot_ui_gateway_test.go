package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/store"
)

func TestCorootGatewayUpstreamPath(t *testing.T) {
	cases := []struct {
		name        string
		basePath    string
		requestPath string
		want        string
	}{
		{"root", "/coroot", "/_coroot/", "/coroot/"},
		{"project view", "/coroot", "/_coroot/p/5hxbfx6p/applications", "/coroot/p/5hxbfx6p/applications"},
		{"api", "/coroot/", "/_coroot/api/user", "/coroot/api/user"},
		{"static", "", "/_coroot/static/js/app.js", "/static/js/app.js"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := corootGatewayUpstreamPath(tc.basePath, tc.requestPath)
			if got != tc.want {
				t.Fatalf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRewriteCorootIndexHTMLInjectsEmbedMode(t *testing.T) {
	html := []byte(`<script>window.coroot = {base_path: '/coroot/', version: 'dev'};</script><link href="/coroot/static/css/app.css"><script src="/coroot/static/js/app.js"></script>`)
	got := string(rewriteCorootIndexHTML(html, "/coroot/", "/_coroot/", "http://aiops.local"))
	if !strings.Contains(got, "base_path: '/_coroot/'") {
		t.Fatalf("rewritten html missing gateway base path: %s", got)
	}
	if strings.Contains(got, `href="/coroot/static`) || strings.Contains(got, `src="/coroot/static`) {
		t.Fatalf("rewritten html leaked upstream static asset paths: %s", got)
	}
	if !strings.Contains(got, `href="/_coroot/static/css/app.css"`) || !strings.Contains(got, `src="/_coroot/static/js/app.js"`) {
		t.Fatalf("rewritten html missing gateway static asset paths: %s", got)
	}
	if !strings.Contains(got, "embed: true") || !strings.Contains(got, "embed_host: 'aiops-v2'") {
		t.Fatalf("rewritten html missing embed flags: %s", got)
	}
	if !strings.Contains(got, "parent_origin: 'http://aiops.local'") {
		t.Fatalf("rewritten html missing parent origin: %s", got)
	}
	if !strings.Contains(got, "data-aiops-coroot-embed-style") || !strings.Contains(got, ".v-navigation-drawer") {
		t.Fatalf("rewritten html missing embed menu-hiding styles: %s", got)
	}
	if !strings.Contains(got, ".v-app-bar") || !strings.Contains(got, "left:0!important") {
		t.Fatalf("rewritten html missing embed header offset reset styles: %s", got)
	}
}

func TestRewriteCorootIndexHTMLInjectsEmbedGuardForCloudUpsell(t *testing.T) {
	html := []byte(`<!doctype html><html><head></head><body><script>window.coroot = {base_path: '/coroot/'};</script></body></html>`)
	got := string(rewriteCorootIndexHTML(html, "/coroot/", "/_coroot/", "http://aiops.local"))

	if !strings.Contains(got, "data-aiops-coroot-embed-guard") {
		t.Fatalf("rewritten html missing embed runtime guard: %s", got)
	}
	if !strings.Contains(got, "Supercharge Coroot with AI") || !strings.Contains(got, "Coroot Cloud") {
		t.Fatalf("embed runtime guard must target the Coroot Cloud upsell dialog: %s", got)
	}
	if strings.Count(injectCorootEmbedGuard(got), "data-aiops-coroot-embed-guard") != 1 {
		t.Fatalf("embed runtime guard must be injected once: %s", got)
	}
}

func TestCorootGatewayDoesNotForwardAiopsCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coroot/p/5hxbfx6p/applications" {
			t.Fatalf("path = %q, want /coroot/p/5hxbfx6p/applications", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer coroot-token" {
			t.Fatalf("Authorization = %q, want Coroot API token only", got)
		}
		if got := r.Header.Get("Cookie"); strings.Contains(got, "aiops_session=") {
			t.Fatalf("aiops cookie forwarded = %q", got)
		}
		if got := r.Header.Get("X-Aiops-Session"); got != "" {
			t.Fatalf("X-Aiops-Session forwarded = %q", got)
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<script>window.coroot = {base_path: '/coroot/'};</script>`))
	}))
	defer upstream.Close()

	repo := newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL: upstream.URL,
		Token:   "coroot-token",
		Project: "5hxbfx6p",
	})
	handler := newCorootUIGateway(repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/p/5hxbfx6p/applications", nil)
	req.Header.Set("Authorization", "Bearer aiops-token")
	req.Header.Set("Cookie", "aiops_session=secret; theme=dark")
	req.Header.Set("X-Aiops-Session", "session-secret")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "embed: true") {
		t.Fatalf("body missing embed rewrite: %s", rec.Body.String())
	}
}

func TestCorootGatewaySessionPassthroughForwardsOnlyCorootSessionCookie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Cookie")
		if !strings.Contains(got, "coroot_session=coroot-token") {
			t.Fatalf("Cookie = %q, want coroot session", got)
		}
		if strings.Contains(got, "aiops_session=") || strings.Contains(got, "theme=") {
			t.Fatalf("Cookie = %q, must not forward non-Coroot cookies", got)
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("Authorization = %q, want empty for session passthrough", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	repo := newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL:  upstream.URL + "/coroot",
		Project:  "5hxbfx6p",
		AuthMode: "session_passthrough",
		Token:    "stale-token",
	})
	handler := newCorootUIGateway(repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/p/5hxbfx6p/applications", nil)
	req.Header.Set("Cookie", "aiops_session=secret; coroot_session=coroot-token; theme=dark")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCorootHTTPTransportDisablesEnvironmentProxy(t *testing.T) {
	t.Setenv("http_proxy", "http://127.0.0.1:7897")
	transport := newCorootHTTPTransport(2 * time.Second)

	if transport.Proxy != nil {
		t.Fatalf("Coroot transport must not use environment proxy")
	}
	if transport.ResponseHeaderTimeout != 2*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %s, want 2s", transport.ResponseHeaderTimeout)
	}
}

func TestCorootGatewayRewritesSetCookiePath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "coroot_session", Value: "abc", Path: "/coroot", HttpOnly: true})
		http.SetCookie(w, &http.Cookie{Name: "coroot_root_session", Value: "def", Path: "/", HttpOnly: true})
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, &store.CorootConfig{BaseURL: upstream.URL + "/coroot"}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/", nil)

	handler.ServeHTTP(rec, req)

	if got := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); strings.Count(got, "Path=/_coroot/") != 2 {
		t.Fatalf("Set-Cookie = %q, want gateway path", got)
	}
}

func TestCorootGatewayRewritesLocationHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/coroot/login")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, &store.CorootConfig{BaseURL: upstream.URL + "/coroot"}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/", nil)

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Location"); got != "/_coroot/login" {
		t.Fatalf("Location = %q, want /_coroot/login", got)
	}
}

func TestCorootGatewayReturnsStructuredErrorWhenUnconfigured(t *testing.T) {
	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["error"] != "coroot not configured" {
		t.Fatalf("response = %#v, want structured error", got)
	}
}

func TestCorootGatewayRejectsWriteWhenEmbedTrustDisabled(t *testing.T) {
	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL:   "http://127.0.0.1:1/coroot",
		AuthMode:  "anonymous_readonly",
		EmbedMode: "readonly",
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_coroot/api/project/5hxbfx6p/settings", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["error"] != "coroot write operations are disabled" {
		t.Fatalf("response = %#v, want write gate error", got)
	}
}

func TestCorootGatewayAllowsLoginWriteForSessionPassthrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/coroot/api/login" {
			t.Fatalf("path = %q, want /coroot/api/login", r.URL.Path)
		}
		http.SetCookie(w, &http.Cookie{Name: "coroot_session", Value: "abc", Path: "/coroot", HttpOnly: true})
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL:  upstream.URL + "/coroot",
		AuthMode: "session_passthrough",
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_coroot/api/login", strings.NewReader(`{"email":"admin","password":"secret"}`))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(rec.Header().Values("Set-Cookie"), "\n"); !strings.Contains(got, "Path=/_coroot/") {
		t.Fatalf("Set-Cookie = %q, want gateway path", got)
	}
}

func TestCorootGatewayAllowsWriteWithEmbedTrustFullMode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("X-Aiops-Embed-User") == "" {
			t.Fatalf("missing embed user header")
		}
		if r.Header.Get("X-Aiops-Embed-Signature") == "" {
			t.Fatalf("missing embed signature header")
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization forwarded = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler := newCorootUIGateway(newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL:          upstream.URL + "/coroot",
		Project:          "5hxbfx6p",
		AuthMode:         "embed_trust",
		EmbedMode:        "full",
		EmbedTrustSecret: "shared-secret",
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/_coroot/api/project/5hxbfx6p/settings", nil)
	req.Header.Set("Authorization", "Bearer aiops-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCorootUIGatewayRegisteredBeforeWebFallback(t *testing.T) {
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("spa fallback"))
	})
	repo := newCorootGatewayTestRepo(t, &store.CorootConfig{
		BaseURL: "http://127.0.0.1:1/coroot",
		Project: "5hxbfx6p",
	})
	services := appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithCorootConfigRepository(repo))
	srv := NewHTTPServer(services, WithWebAssets(web))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_coroot/", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusTeapot {
		t.Fatalf("/_coroot/ was handled by SPA fallback")
	}
}

func newCorootGatewayTestRepo(t *testing.T, cfg *store.CorootConfig) appui.CorootConfigRepository {
	t.Helper()
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if cfg != nil {
		if err := repo.SaveCorootConfig(cfg); err != nil {
			t.Fatalf("SaveCorootConfig() error = %v", err)
		}
	}
	return repo
}
