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
