package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/workfloweditor"
	"aiops-v2/internal/workflowgen"
	"runner/workflow"
	"runner/workflow/visual"
)

func TestRunnerStudioWorkflowAIHandlerUsesWorkflowEditorService(t *testing.T) {
	store := workfloweditor.NewMemoryWorkflowStore()
	record := store.PutWorkflow(workfloweditor.WorkflowRecord{ID: "redis-memory", Graph: runnerStudioWorkflowAITestGraph()})
	service := workfloweditor.NewService(store)
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), WithWorkflowEditorService(service))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/snapshot", map[string]any{"workflowId": "redis-memory"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status = %d, want 200", resp.StatusCode)
	}
	var snapshot workfloweditor.WorkflowSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	resp.Body.Close()
	if snapshot.Revision != record.Revision || snapshot.Describe.NodeCount == 0 {
		t.Fatalf("snapshot = %#v, want revision and describe", snapshot)
	}

	resp = postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/sessions", map[string]any{
		"drawerSessionId": "drawer",
		"workflowId":      "redis-memory",
		"baseRevision":    record.Revision,
		"sessionIntent":   "edit",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/apply", map[string]any{
		"workflowId":         "redis-memory",
		"baseRevision":       record.Revision,
		"patchId":            "patch",
		"userConfirmationId": "confirm",
		"drawerSessionId":    "drawer",
		"reason":             "rename",
		"patch": map[string]any{
			"id":           "patch",
			"baseRevision": record.Revision,
			"operations": []map[string]any{{
				"op":     "update_node",
				"nodeId": "collect",
				"fields": map[string]any{"label": "Collect Redis memory"},
			}},
		},
	})
	if resp.StatusCode != http.StatusOK {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("apply status = %d body=%+v, want 200", resp.StatusCode, body)
	}
	var applied workfloweditor.WorkflowPatchResult
	if err := json.NewDecoder(resp.Body).Decode(&applied); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	resp.Body.Close()
	if applied.RevisionAfter == record.Revision || applied.UndoCheckpoint.ID == "" {
		t.Fatalf("apply = %#v, want new revision and undo checkpoint", applied)
	}
}

func TestRunnerStudioWorkflowAIApplyRequiresConfirmation(t *testing.T) {
	store := workfloweditor.NewMemoryWorkflowStore()
	record := store.PutWorkflow(workfloweditor.WorkflowRecord{ID: "redis-memory", Graph: runnerStudioWorkflowAITestGraph()})
	service := workfloweditor.NewService(store)
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), WithWorkflowEditorService(service))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp := postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/apply", map[string]any{
		"workflowId":      "redis-memory",
		"baseRevision":    record.Revision,
		"patchId":         "patch",
		"drawerSessionId": "drawer",
		"reason":          "rename",
		"patch": map[string]any{
			"id":           "patch",
			"baseRevision": record.Revision,
			"operations": []map[string]any{{
				"op":     "update_node",
				"nodeId": "collect",
				"fields": map[string]any{"label": "Collect Redis memory"},
			}},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if !strings.Contains(body["error"], "user_confirmation") {
		t.Fatalf("body = %+v, want confirmation error", body)
	}
}

func TestRunnerStudioWorkflowAICreateDraftRequiresConfirmationAndDoesNotExecute(t *testing.T) {
	service := workfloweditor.NewService(workfloweditor.NewMemoryWorkflowStore())
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), WithWorkflowEditorService(service))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	plan := workflowgen.WorkflowGenerationPlan{
		Version: 1,
		Title:   "Redis Memory Draft",
		Trigger: workflowgen.WorkflowTrigger{Type: workflowgen.TriggerTypeManual},
		Nodes: []workflowgen.WorkflowPlanNode{{
			ID:     "collect",
			Kind:   workflowgen.NodeKindSearch,
			Title:  "Collect Redis memory",
			Action: "script.python",
		}},
		Outputs:            []workflowgen.WorkflowOutput{{ID: "summary", Target: workflowgen.OutputTargetReturn}},
		ValidationStrategy: workflowgen.ValidationStrategy{Enabled: false, Provider: workflowgen.ValidationProviderNone},
	}
	resp := postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/create-draft", map[string]any{"plan": plan})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want confirmation failure", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postRunnerStudioWorkflowAI(t, ts.URL+"/api/runner-studio/workflow-ai/create-draft", map[string]any{
		"drawerSessionId":    "drawer-create",
		"userConfirmationId": "confirm-create",
		"plan":               plan,
	})
	if resp.StatusCode != http.StatusOK {
		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		t.Fatalf("status = %d body=%+v, want 200", resp.StatusCode, body)
	}
	var result workfloweditor.WorkflowDraftFromPlanResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	resp.Body.Close()
	if result.WorkflowID == "" || result.Revision == "" || result.Published || result.Executed {
		t.Fatalf("result = %#v, want saved draft without publish/execute", result)
	}
}

func postRunnerStudioWorkflowAI(t *testing.T, url string, payload map[string]any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s error = %v", url, err)
	}
	return resp
}

func runnerStudioWorkflowAITestGraph() visual.Graph {
	step := workflow.Step{ID: "collect", Name: "collect", Targets: []string{"local"}, Action: "script.python", Args: map[string]any{"script": "print('ok')"}}
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "redis-memory",
			Description: "Redis memory workflow",
			Steps:       []workflow.Step{step},
		},
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
