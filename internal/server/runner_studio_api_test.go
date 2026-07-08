package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
)

func TestRunnerStudioAPIRoutesToEmbeddedRunnerAPI(t *testing.T) {
	type seenRequest struct {
		Method      string
		EscapedPath string
		Query       string
	}
	seen := make(chan seenRequest, 8)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- seenRequest{Method: r.Method, EscapedPath: r.URL.EscapedPath(), Query: r.URL.RawQuery}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embedded": true,
			"path":     r.URL.Path,
		})
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name       string
		method     string
		path       string
		wantMethod string
		wantPath   string
		wantQuery  string
	}{
		{
			name:       "action catalog",
			method:     http.MethodGet,
			path:       "/api/runner-studio/actions?category=script",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/actions/catalog",
			wantQuery:  "category=script",
		},
		{
			name:       "legacy action catalog alias",
			method:     http.MethodGet,
			path:       "/api/runner-studio/actions/catalog?category=script",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/actions/catalog",
			wantQuery:  "category=script",
		},
		{
			name:       "workflow list",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows",
		},
		{
			name:       "workflow delete",
			method:     http.MethodDelete,
			path:       "/api/runner-studio/workflows/pg-restore",
			wantMethod: http.MethodDelete,
			wantPath:   "/api/v1/workflows/pg-restore",
		},
		{
			name:       "workflow graph",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows/pg-restore/graph",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows/pg-restore/graph",
		},
		{
			name:       "workflow versions",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows/pg-restore/versions",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows/pg-restore/versions",
		},
		{
			name:       "workflow version detail",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows/pg-restore/versions/v%2F1",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows/pg-restore/versions/v%2F1",
		},
		{
			name:       "workflow version rollback",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/pg-restore/versions/v%2F1/rollback",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/pg-restore/versions/v%2F1/rollback",
		},
		{
			name:       "workflow bundle export",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows/pg-restore/bundle",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows/pg-restore/bundle",
		},
		{
			name:       "workflow bundle import",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/bundles/import",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/bundles/import",
		},
		{
			name:       "workflow graph escaped slash",
			method:     http.MethodGet,
			path:       "/api/runner-studio/workflows/team%2Fpg-restore/graph",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/workflows/team%2Fpg-restore/graph",
		},
		{
			name:       "graph validate",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/graph/validate",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/graph/validate",
		},
		{
			name:       "workflow validate",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/pg-restore/validate",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/pg-restore/validate",
		},
		{
			name:       "workflow publish",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/pg-restore/publish",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/pg-restore/publish",
		},
		{
			name:       "graph compile",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/graph/compile",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/graph/compile",
		},
		{
			name:       "graph parse",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/graph/parse",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/graph/parse",
		},
		{
			name:       "graph variable resolve",
			method:     http.MethodPost,
			path:       "/api/runner-studio/workflows/graph/variables/resolve",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/workflows/graph/variables/resolve",
		},
		{
			name:       "run graph",
			method:     http.MethodGet,
			path:       "/api/runner-studio/runs/run-1/graph",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/runs/run-1/graph",
		},
		{
			name:       "run event history",
			method:     http.MethodGet,
			path:       "/api/runner-studio/runs/run-1/events/history",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/runs/run-1/events/history",
		},
		{
			name:       "run live events",
			method:     http.MethodGet,
			path:       "/api/runner-studio/runs/run-1/events",
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/runs/run-1/events",
		},
		{
			name:       "approve graph node",
			method:     http.MethodPost,
			path:       "/api/runner-studio/runs/run-1/nodes/approve-1/approve",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/runs/run-1/nodes/approve-1/approve",
		},
		{
			name:       "reject graph node",
			method:     http.MethodPost,
			path:       "/api/runner-studio/runs/run-1/nodes/approve-1/reject",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/runs/run-1/nodes/approve-1/reject",
		},
		{
			name:       "cancel run",
			method:     http.MethodPost,
			path:       "/api/runner-studio/runs/run-1/cancel",
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/runs/run-1/cancel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(`{"graph":{"nodes":[]}}`))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if tc.method == http.MethodPost || tc.method == http.MethodPut {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("%s %s error = %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			got := <-seen
			if got.Method != tc.wantMethod || got.EscapedPath != tc.wantPath || got.Query != tc.wantQuery {
				t.Fatalf("embedded request = %+v, want method=%s path=%s query=%s", got, tc.wantMethod, tc.wantPath, tc.wantQuery)
			}
		})
	}
}

func TestRunnerStudioAPIHeaderPolicy(t *testing.T) {
	received := make(chan http.Header, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Connection", "upgrade")
		w.Header().Set("Set-Cookie", "runner_session=leak")
		w.Header().Set("X-Request-Id", "req-1")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/runner-studio/actions/catalog", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer main-app")
	req.Header.Set("Cookie", "main_session=secret")
	req.Header.Set("Connection", "upgrade")
	req.Header.Set("X-Request-Id", "req-1")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("GET action catalog error = %v", err)
	}
	defer resp.Body.Close()

	headers := <-received
	if headers.Get("Accept") != "application/json" {
		t.Fatalf("embedded Accept = %q, want application/json", headers.Get("Accept"))
	}
	for _, blocked := range []string{"Authorization", "Cookie", "Connection"} {
		if got := headers.Get(blocked); got != "" {
			t.Fatalf("embedded %s = %q, want empty", blocked, got)
		}
	}
	if got := resp.Header.Get("X-Request-Id"); got != "req-1" {
		t.Fatalf("response X-Request-Id = %q, want req-1", got)
	}
}

func TestRunnerStudioAPIUsesEmbeddedHandlerWithoutUpstream(t *testing.T) {
	type embeddedSeenRequest struct {
		Method      string
		EscapedPath string
		Query       string
	}
	seen := make(chan embeddedSeenRequest, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- embeddedSeenRequest{Method: r.Method, EscapedPath: r.URL.EscapedPath(), Query: r.URL.RawQuery}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedded": true})
	})
	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runner-studio/actions?category=command")
	if err != nil {
		t.Fatalf("GET action catalog error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := <-seen
	if got.Method != http.MethodGet || got.EscapedPath != "/api/v1/actions/catalog" || got.Query != "category=command" {
		t.Fatalf("embedded request = %+v", got)
	}
}

func TestRunnerStudioPublishProxiesReviewPayload(t *testing.T) {
	received := make(chan map[string]any, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/api/v1/workflows/pg-restore/publish" {
			t.Fatalf("embedded request = %s %s, want POST /api/v1/workflows/pg-restore/publish", r.Method, r.URL.EscapedPath())
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode publish body: %v", err)
		}
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "pg-restore", "status": "published"})
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/runner-studio/workflows/pg-restore/publish",
		"application/json",
		strings.NewReader(`{"save_note":"approved","validated_graph_hash":"hash-1","diff":{"semantic_changes":[{"title":"restore"}]}}`),
	)
	if err != nil {
		t.Fatalf("publish request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := <-received
	if body["validated_graph_hash"] != "hash-1" || body["diff"] == nil {
		t.Fatalf("publish body = %+v, want review graph hash and diff", body)
	}
}

func TestRunnerStudioAPIRequiresEmbeddedRunner(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runner-studio/actions/catalog")
	if err != nil {
		t.Fatalf("GET action catalog error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}
