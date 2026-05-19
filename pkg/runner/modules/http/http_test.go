package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"runner/modules"
	"runner/workflow"
)

func TestHTTPRequestStructuredResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "yes" {
			t.Fatalf("missing header")
		}
		w.Header().Set("X-Token", "secret-value")
		_, _ = w.Write([]byte(`{"data":{"name":"alice","secret":"secret-value"}}`))
	}))
	defer server.Close()

	mod := New()
	res, err := mod.Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"method":                 "GET",
			"url":                    server.URL,
			"headers":                map[string]any{"X-Test": "yes"},
			"expected_status":        200,
			"json_path":              "$.data.name",
			"redaction":              []any{"secret-value"},
			"allow_private_networks": true,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Fatalf("http request should not report changed")
	}
	if res.Output["ok"] != true {
		t.Fatalf("ok = %#v", res.Output["ok"])
	}
	if res.Output["json_path_value"] != "alice" {
		t.Fatalf("json_path_value = %#v", res.Output["json_path_value"])
	}
	if strings.Contains(res.Output["body"].(string), "secret-value") {
		t.Fatalf("body was not redacted: %s", res.Output["body"])
	}
	if res.Output["schema_version"] != modules.RunnerResultSchemaVersion {
		t.Fatalf("schema_version = %#v", res.Output["schema_version"])
	}
	data, ok := res.Output["data"].(map[string]any)
	if !ok || data["status_code"] != 200 || data["json_path_value"] != "alice" {
		t.Fatalf("envelope data = %#v", res.Output["data"])
	}
}

func TestHTTPRequestResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":                    server.URL,
			"response_limit":         3,
			"allow_private_networks": true,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["body"] != "abc" || res.Output["truncated"] != true {
		t.Fatalf("unexpected limited output: %#v", res.Output)
	}
}

func TestHTTPRequestCatalogAliases(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("missing json content type")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"method":                 "POST",
			"url":                    server.URL,
			"headers":                map[string]any{"Content-Type": "application/json"},
			"body_json":              map[string]any{"name": "runner"},
			"expected_status":        []any{201},
			"retries":                1,
			"max_response_bytes":     1024,
			"allow_private_networks": true,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["status_code"] != 201 || res.Output["ok"] != true {
		t.Fatalf("unexpected output: %#v", res.Output)
	}
	if !strings.Contains(gotBody, `"name":"runner"`) {
		t.Fatalf("json body was not sent: %q", gotBody)
	}
}

func TestHTTPRequestUnexpectedStatusFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("queued"))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":                    server.URL,
			"expected_status":        []any{200},
			"allow_private_networks": true,
		}},
	})
	if err == nil {
		t.Fatalf("expected unexpected status error")
	}
	if res.Output["ok"] != false || res.Output["status_code"] != http.StatusAccepted {
		t.Fatalf("unexpected status output: %#v", res.Output)
	}
	if !strings.Contains(err.Error(), "unexpected status 202") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPRequestTimeoutAndStructuredRetryPolicy(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		time.Sleep(25 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":                    server.URL,
			"timeout_ms":             5,
			"retry":                  map[string]any{"max_attempts": 2, "backoff_ms": 1},
			"allow_private_networks": true,
		}},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if res.Output["attempt"] != 2 {
		t.Fatalf("attempt = %#v", res.Output["attempt"])
	}
	if !strings.Contains(res.Output["error"].(string), "Client.Timeout") && !strings.Contains(res.Output["error"].(string), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error: %#v", res.Output["error"])
	}
}

func TestHTTPRequestBlocksPrivateNetworkByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url": server.URL,
		}},
	})
	if err == nil {
		t.Fatalf("expected private network block")
	}
	if res.Output["ok"] != false {
		t.Fatalf("ok = %#v", res.Output["ok"])
	}
	if !strings.Contains(res.Output["error"].(string), "blocked private address") {
		t.Fatalf("unexpected error: %#v", res.Output["error"])
	}
}

func TestHTTPRequestRetry(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":                    server.URL,
			"retry":                  1,
			"allow_private_networks": true,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if res.Output["attempt"] != 2 {
		t.Fatalf("attempt = %#v", res.Output["attempt"])
	}
}

func TestHTTPRequestSSRFPolicyAllowHostAndCIDRs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Run("allow host pattern permits loopback", func(t *testing.T) {
		res, err := New().Apply(context.Background(), modules.Request{
			Step: workflow.Step{Args: map[string]any{
				"url":                 server.URL,
				"allow_host_patterns": []any{"127.0.0.1"},
			}},
		})
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if res.Output["ok"] != true {
			t.Fatalf("ok = %#v", res.Output["ok"])
		}
	})

	t.Run("allowed cidr permits loopback", func(t *testing.T) {
		res, err := New().Apply(context.Background(), modules.Request{
			Step: workflow.Step{Args: map[string]any{
				"url":           server.URL,
				"allowed_cidrs": []any{"127.0.0.0/8"},
			}},
		})
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if res.Output["ok"] != true {
			t.Fatalf("ok = %#v", res.Output["ok"])
		}
	})

	t.Run("blocked cidr wins", func(t *testing.T) {
		res, err := New().Apply(context.Background(), modules.Request{
			Step: workflow.Step{Args: map[string]any{
				"url":                    server.URL,
				"allow_private_networks": true,
				"blocked_cidrs":          []any{"127.0.0.0/8"},
			}},
		})
		if err == nil {
			t.Fatalf("expected blocked cidr error")
		}
		if !strings.Contains(res.Output["error"].(string), "blocked_cidrs") {
			t.Fatalf("unexpected error: %#v", res.Output["error"])
		}
	})
}

func TestHTTPRequestRedirectPolicy(t *testing.T) {
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer forbidden.Close()
	forbiddenURL := strings.Replace(forbidden.URL, "127.0.0.1", "localhost", 1)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, forbiddenURL, http.StatusFound)
	}))
	defer redirector.Close()

	t.Run("redirect to forbidden host is rejected", func(t *testing.T) {
		res, err := New().Apply(context.Background(), modules.Request{
			Step: workflow.Step{Args: map[string]any{
				"url":                    redirector.URL,
				"allow_host_patterns":    []any{"127.0.0.1"},
				"allow_private_networks": true,
			}},
		})
		if err == nil {
			t.Fatalf("expected redirect host block")
		}
		if !strings.Contains(res.Output["error"].(string), "allow policy") {
			t.Fatalf("unexpected error: %#v", res.Output["error"])
		}
	})

	t.Run("follow_redirect false returns redirect response", func(t *testing.T) {
		res, err := New().Apply(context.Background(), modules.Request{
			Step: workflow.Step{Args: map[string]any{
				"url":                    redirector.URL,
				"allow_private_networks": true,
				"follow_redirect":        false,
				"expected_status":        302,
			}},
		})
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if res.Output["status_code"] != http.StatusFound {
			t.Fatalf("status_code = %#v", res.Output["status_code"])
		}
	})
}

func TestHTTPRequestSecretRefAuthIsRedacted(t *testing.T) {
	const secret = "super-secret-token"
	authTests := []struct {
		name string
		args map[string]any
		want func(*http.Request) bool
	}{
		{
			name: "bearer",
			args: map[string]any{"auth": map[string]any{"type": "bearer", "secret_ref": "api_token"}},
			want: func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer "+secret },
		},
		{
			name: "basic",
			args: map[string]any{"auth": map[string]any{"type": "basic", "username": "alice", "secret_ref": "api_token"}},
			want: func(r *http.Request) bool {
				user, pass, ok := r.BasicAuth()
				return ok && user == "alice" && pass == secret
			},
		},
		{
			name: "header",
			args: map[string]any{"auth": map[string]any{"type": "header", "name": "X-Api-Key", "secret_ref": "api_token"}},
			want: func(r *http.Request) bool { return r.Header.Get("X-Api-Key") == secret },
		},
	}

	for _, tt := range authTests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tt.want(r) {
					t.Fatalf("missing expected auth")
				}
				w.WriteHeader(http.StatusTeapot)
				w.Header().Set("X-Echo-Token", secret)
				_, _ = w.Write([]byte(`{"echo":"` + secret + `"}`))
			}))
			defer server.Close()
			tt.args["url"] = server.URL
			tt.args["allow_private_networks"] = true
			tt.args["expected_status"] = 200
			tt.args["secrets"] = map[string]any{"api_token": secret}

			res, err := New().Apply(context.Background(), modules.Request{
				Step: workflow.Step{Args: tt.args},
			})
			if err == nil {
				t.Fatalf("expected status error")
			}
			dumped := stringify(res.Output) + err.Error()
			if strings.Contains(dumped, secret) {
				t.Fatalf("secret leaked in output/error: %s", dumped)
			}
		})
	}
}

func TestHTTPRequestJSONPathsMultipleAndFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"a":{"b":["first","second"]},
			"status":{"conditions":[
				{"type":"Pending","status":"False"},
				{"type":"Ready","status":"True"}
			]}
		}`))
	}))
	defer server.Close()

	res, err := New().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":                    server.URL,
			"allow_private_networks": true,
			"output": map[string]any{"json_paths": map[string]any{
				"first": "$.a.b[0]",
				"ready": "$.status.conditions[?(@.type=='Ready')].status",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	values := res.Output["json_paths"].(map[string]any)
	if values["first"] != "first" {
		t.Fatalf("first path = %#v", values["first"])
	}
	if values["ready"] != "True" {
		t.Fatalf("ready path = %#v", values["ready"])
	}
}

func stringify(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		var b strings.Builder
		for key, item := range v {
			b.WriteString(key)
			b.WriteString(":")
			b.WriteString(stringify(item))
			b.WriteString(";")
		}
		return b.String()
	case map[string]string:
		var b strings.Builder
		for key, item := range v {
			b.WriteString(key)
			b.WriteString(":")
			b.WriteString(item)
			b.WriteString(";")
		}
		return b.String()
	case []string:
		return strings.Join(v, ",")
	case []any:
		var b strings.Builder
		for _, item := range v {
			b.WriteString(stringify(item))
			b.WriteString(";")
		}
		return b.String()
	default:
		return ""
	}
}
