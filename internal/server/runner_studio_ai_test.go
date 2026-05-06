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

func TestRunnerStudioAIGenerateDraftProxiesStructuredPatch(t *testing.T) {
	type upstreamSeen struct {
		Method string
		Path   string
		Body   map[string]any
	}
	seen := make(chan upstreamSeen, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		seen <- upstreamSeen{Method: r.Method, Path: r.URL.EscapedPath(), Body: body}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"graph_patch": map[string]any{
				"operations": []map[string]any{{"op": "add_node", "node_id": "restore"}},
			},
			"diff_summary": map[string]any{
				"semantic_changes": []map[string]any{{"title": "add restore", "detail": "shell.run"}},
			},
		})
	}))
	defer upstream.Close()

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioUpstreamURL(upstream.URL),
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
		t.Fatalf("upstream = %+v, want POST /api/v1/workflows/ai/draft", req)
	}
	for _, forbidden := range []string{"api_key", "apikey", "base_url", "model"} {
		if _, ok := req.Body[forbidden]; ok {
			t.Fatalf("upstream body contains frontend LLM field %q: %+v", forbidden, req.Body)
		}
	}
}

func TestRunnerStudioAIPatchRequiresDraftWorkflow(t *testing.T) {
	upstreamHit := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioUpstreamURL(upstream.URL),
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
	case <-upstreamHit:
		t.Fatal("non-draft AI patch should not reach upstream")
	default:
	}
}

func TestRunnerStudioAIFailureExplanationDoesNotRequireGraphPatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_explanation": "缺少目标主机，无法生成安全 patch",
		})
	}))
	defer upstream.Close()

	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioUpstreamURL(upstream.URL),
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
