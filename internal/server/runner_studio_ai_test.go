package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
)

func TestRunnerStudioAIGenerateDraftUsesEmbeddedStructuredPatch(t *testing.T) {
	type embeddedSeen struct {
		Method string
		Path   string
		Body   map[string]any
	}
	seen := make(chan embeddedSeen, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode embedded body: %v", err)
		}
		seen <- embeddedSeen{Method: r.Method, Path: r.URL.EscapedPath(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"graph_patch": map[string]any{
				"operations": []map[string]any{{"op": "add_node", "node_id": "restore"}},
			},
			"diff_summary": map[string]any{
				"semantic_changes": []map[string]any{{"title": "add restore", "detail": "shell.run"}},
			},
		})
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/runner-studio/ai/draft",
		"application/json",
		strings.NewReader(`{"workflow_status":"draft","instruction":"生成恢复流程","graph":{"nodes":[]}}`),
	)
	if err != nil {
		t.Fatalf("POST ai draft error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["graph_patch"] == nil || got["diff_summary"] == nil {
		t.Fatalf("response = %+v, want graph_patch and diff_summary", got)
	}

	req := <-seen
	if req.Method != http.MethodPost || req.Path != "/api/v1/workflows/ai/draft" {
		t.Fatalf("embedded = %+v, want POST /api/v1/workflows/ai/draft", req)
	}
	for _, forbidden := range []string{"api_key", "apikey", "base_url", "model"} {
		if _, ok := req.Body[forbidden]; ok {
			t.Fatalf("embedded body contains frontend LLM field %q: %+v", forbidden, req.Body)
		}
	}
}

func TestRunnerStudioAIPatchRequiresDraftWorkflow(t *testing.T) {
	embeddedHit := make(chan struct{}, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embeddedHit <- struct{}{}
		w.WriteHeader(http.StatusOK)
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/runner-studio/ai/draft",
		"application/json",
		strings.NewReader(`{"workflow_status":"published","instruction":"修改生产流程","graph":{"nodes":[]}}`),
	)
	if err != nil {
		t.Fatalf("POST ai draft error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	payload, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(payload), "draft") {
		t.Fatalf("response = %s, want draft guard explanation", payload)
	}
	select {
	case <-embeddedHit:
		t.Fatal("non-draft AI patch should not reach embedded runner")
	default:
	}
}

func TestRunnerStudioAIGenerateDraftUsesEmbeddedHandler(t *testing.T) {
	type embeddedSeen struct {
		Method string
		Path   string
	}
	seen := make(chan embeddedSeen, 1)
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- embeddedSeen{Method: r.Method, Path: r.URL.EscapedPath()}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_explanation": "embedded draft response",
		})
	})
	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/runner-studio/ai/draft",
		"application/json",
		strings.NewReader(`{"workflow_status":"draft","instruction":"生成恢复流程","graph":{"nodes":[]}}`),
	)
	if err != nil {
		t.Fatalf("POST ai draft error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	req := <-seen
	if req.Method != http.MethodPost || req.Path != "/api/v1/workflows/ai/draft" {
		t.Fatalf("embedded = %+v, want POST /api/v1/workflows/ai/draft", req)
	}
}

func TestRunnerStudioAIFailureExplanationDoesNotRequireGraphPatch(t *testing.T) {
	embedded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_explanation": "缺少目标主机，无法生成安全 patch",
		})
	})

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(embedded),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(
		ts.URL+"/api/runner-studio/ai/draft",
		"application/json",
		strings.NewReader(`{"workflow_status":"draft","instruction":"生成恢复流程","graph":{"nodes":[]}}`),
	)
	if err != nil {
		t.Fatalf("POST ai draft error = %v", err)
	}
	defer resp.Body.Close()
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["error_explanation"] == "" {
		t.Fatalf("response = %+v, want error_explanation", got)
	}
	if got["graph_patch"] != nil {
		t.Fatalf("response = %+v, failure explanation should not include graph_patch", got)
	}
}
