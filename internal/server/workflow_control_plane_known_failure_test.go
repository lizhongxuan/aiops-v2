package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/workfloweditor"
)

func TestWorkflowCompatibilityCannotRestoreIndependentLegacyStore(t *testing.T) {
	graph := runnerStudioWorkflowAITestGraph()
	runner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workflows/shared/graph" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(graph)
	})
	srv := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(runner),
		WithWorkflowEditorService(workfloweditor.NewService(workfloweditor.NewMemoryWorkflowStore())),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/api/runner-studio/workflows/shared/graph")
	if err != nil {
		t.Fatalf("read Runner graph: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Runner graph status = %d, want 200", resp.StatusCode)
	}

	resp = postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/snapshot", map[string]any{
		"workflowId": "shared",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("legacy service restored an independent write/read path: snapshot status = %d, want 410", resp.StatusCode)
	}
}
