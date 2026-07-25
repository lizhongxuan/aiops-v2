package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"runner/workflow"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowCompatibilityEndpointsReturnTypedMigrationRequired(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	for _, action := range []string{"sessions", "plan", "patch", "undo", "create-draft"} {
		t.Run(action, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, runnerStudioWorkflowAIPrefix+action, nil)
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusGone {
				t.Fatalf("status = %d, want 410", recorder.Code)
			}
			var response edit.DomainError
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
				t.Fatalf("decode typed error: %v", err)
			}
			if response.Kind != "error" || response.Code != edit.ErrorMissingConfiguration || response.RetryHint == "" {
				t.Fatalf("response = %+v, want typed migration error", response)
			}
		})
	}
}

func TestWorkflowCompatibleEndpointsFailClosedWithoutCanonicalApplication(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	for _, action := range []string{"snapshot", "validate", "preview", "apply"} {
		t.Run(action, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, runnerStudioWorkflowAIPrefix+action, nil)
			server.Handler().ServeHTTP(recorder, request)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", recorder.Code)
			}
			var response edit.DomainError
			if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil || response.Code != edit.ErrorMissingConfiguration {
				t.Fatalf("response = %+v, error = %v", response, err)
			}
		})
	}
}

func TestWorkflowCompatibilityUnknownEndpointRemainsNotFound(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, runnerStudioWorkflowAIPrefix+"unknown", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestWorkflowCompatibilityRejectsNonPostMethod(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, runnerStudioWorkflowAIPrefix+"apply", nil)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func postRunnerStudioWorkflowAI(t *testing.T, url string, payload map[string]any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return response
}

func runnerStudioWorkflowAITestGraph() visual.Graph {
	step := workflow.Step{ID: "collect", Name: "collect", Targets: []string{"local"}, Action: "script.python", Args: map[string]any{"script": "print('ok')"}}
	return visual.Graph{
		Version:  visual.GraphVersion,
		Workflow: workflow.Workflow{Version: "v0.1", Name: "redis-memory", Steps: []workflow.Step{step}},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart, Label: "Start"},
			{ID: "collect", Type: visual.NodeTypeAction, Label: "Collect", StepID: "collect", Step: &step},
			{ID: "end", Type: visual.NodeTypeEnd, Label: "End"},
		},
		Edges: []visual.Edge{
			{ID: "start-collect", Source: "start", Target: "collect", Kind: visual.EdgeKindNext},
			{ID: "collect-end", Source: "collect", Target: "end", Kind: visual.EdgeKindNext},
		},
	}
}
