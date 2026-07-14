package coroot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientGetJSONUnwrapsCorootDataAndSetsAuth(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coroot/api/project/prod/overview/applications" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		if r.URL.RawQuery != "query=checkout" {
			http.Error(w, "unexpected query: "+r.URL.RawQuery, http.StatusInternalServerError)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "unexpected auth: "+got, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"default:Deployment:checkout"}]},"context":{"status":"ok"}}`))
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: upstream.URL + "/coroot",
		Token:   "test-token",
		Timeout: time.Second,
		Project: "prod",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var raw json.RawMessage
	ref, err := client.GetJSON(context.Background(), "/api/project/prod/overview/applications", url.Values{"query": {"checkout"}}, &raw)
	if err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
	if !json.Valid(raw) || string(raw) != `{"applications":[{"id":"default:Deployment:checkout"}]}` {
		t.Fatalf("raw = %s, want unwrapped data object", raw)
	}
	if ref.URI == "" || ref.Digest == "" || ref.Bytes == 0 {
		t.Fatalf("raw ref not populated: %#v", ref)
	}
}

func TestClientGetJSONUsesCorootSessionCookieToken(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "coroot_session=session-value" {
			http.Error(w, "unexpected cookie: "+got, http.StatusInternalServerError)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			http.Error(w, "unexpected bearer auth for session cookie: "+got, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"applications":[]}}`))
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: upstream.URL + "/coroot",
		Token:   "coroot_session=session-value",
		Timeout: time.Second,
		Project: "prod",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var raw json.RawMessage
	if _, err := client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
}

func TestClientLoginStoresCorootSessionCookieForWebAPI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/coroot/api/login":
			if r.Method != http.MethodPost {
				http.Error(w, "unexpected method", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "coroot_session", Value: "login-session", Path: "/"})
		case "/coroot/api/project/prod/overview/applications":
			if got := r.Header.Get("Cookie"); got != "coroot_session=login-session" {
				http.Error(w, "unexpected cookie: "+got, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"default:Deployment:checkout"}]}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{BaseURL: upstream.URL + "/coroot", Timeout: time.Second, Project: "prod"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.Login(context.Background(), "admin", "secret"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	var raw json.RawMessage
	if _, err := client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
}

func TestClientBaseURLUsesConfiguredCorootProductBasePath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coroot/api/project/prod/overview/applications" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"applications":[]}}`))
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{
		BaseURL:         upstream.URL,
		ProductBasePath: "/coroot/",
		Timeout:         time.Second,
		Project:         "prod",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var raw json.RawMessage
	if _, err := client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw); err != nil {
		t.Fatalf("GetJSON() error = %v", err)
	}
}

func TestClientGetJSONMapsHTMLResponseToAuthenticationError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><body>login</body></html>`))
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{
		BaseURL: upstream.URL + "/coroot",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var raw json.RawMessage
	_, err = client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw)
	if err == nil {
		t.Fatal("GetJSON() error = nil, want authentication error")
	}
	var corootErr *CorootError
	if !errors.As(err, &corootErr) {
		t.Fatalf("error type = %T, want *CorootError", err)
	}
	if corootErr.Kind != "authentication_required" {
		t.Fatalf("CorootError.Kind = %q, want authentication_required", corootErr.Kind)
	}
	if corootErr.StatusCode != http.StatusOK {
		t.Fatalf("CorootError.StatusCode = %d, want 200", corootErr.StatusCode)
	}
}

func TestClientResolveProjectTreatsDefaultAsPlaceholderWhenConfiguredProjectDiffers(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "http://coroot.example",
		Project: "5hxbfx6p",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if got := client.ResolveProject("default"); got != "5hxbfx6p" {
		t.Fatalf("ResolveProject(default) = %q, want configured project", got)
	}
	if got := client.ResolveProject(""); got != "5hxbfx6p" {
		t.Fatalf("ResolveProject(empty) = %q, want configured project", got)
	}
	if got := client.ResolveProject("prod-west"); got != "prod-west" {
		t.Fatalf("ResolveProject(prod-west) = %q, want explicit project", got)
	}
}

func TestClientBypassesProxyForPrivateCorootHosts(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "172.18.13.11", want: true},
		{host: "127.0.0.1", want: true},
		{host: "localhost", want: true},
		{host: "coroot.example.com", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			if got := corootShouldBypassProxy(tc.host); got != tc.want {
				t.Fatalf("corootShouldBypassProxy(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestClientGetJSONMapsUpstreamFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantKind   string
	}{
		{name: "client error", statusCode: http.StatusNotFound, body: `{"error":"missing"}`, wantKind: "upstream_client_error"},
		{name: "server error", statusCode: http.StatusBadGateway, body: `bad gateway`, wantKind: "upstream_server_error"},
		{name: "empty response", statusCode: http.StatusOK, body: ``, wantKind: "empty_response"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()

			client, err := NewClient(ClientConfig{BaseURL: upstream.URL, Timeout: time.Second})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			var raw json.RawMessage
			_, err = client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw)
			if err == nil {
				t.Fatal("GetJSON() error = nil, want mapped CorootError")
			}
			var corootErr *CorootError
			if !errors.As(err, &corootErr) {
				t.Fatalf("error type = %T, want *CorootError", err)
			}
			if corootErr.Kind != tc.wantKind {
				t.Fatalf("CorootError.Kind = %q, want %q", corootErr.Kind, tc.wantKind)
			}
			if corootErr.StatusCode != tc.statusCode {
				t.Fatalf("CorootError.StatusCode = %d, want %d", corootErr.StatusCode, tc.statusCode)
			}
		})
	}
}

func TestClientGetJSONMapsTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer upstream.Close()

	client, err := NewClient(ClientConfig{BaseURL: upstream.URL, Timeout: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var raw json.RawMessage
	_, err = client.GetJSON(context.Background(), "/api/project/prod/overview/applications", nil, &raw)
	if err == nil {
		t.Fatal("GetJSON() error = nil, want timeout")
	}
	var corootErr *CorootError
	if !errors.As(err, &corootErr) {
		t.Fatalf("error type = %T, want *CorootError", err)
	}
	if corootErr.Kind != "timeout" {
		t.Fatalf("CorootError.Kind = %q, want timeout", corootErr.Kind)
	}
}
