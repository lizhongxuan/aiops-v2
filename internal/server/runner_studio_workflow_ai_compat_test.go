package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"aiops-v2/internal/appui"
	runnerapp "runner/server/app"
	runnerconfig "runner/server/config"
	"runner/server/service"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowCompatibilityApplyAndManualCommandShareApplicationStoreAndReplay(t *testing.T) {
	runtime := newWorkflowCompatibilityRuntime(t)
	graph := runnerStudioWorkflowAITestGraph()
	graph.Workflow.Name = "workflow-a"
	createRunnerWorkflowGraph(t, runtime.Handler, graph)
	runtime.SetWorkflowReferenceChecker(workflowCompatibilityNoReferences{})

	server := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(runtime.Handler),
		WithWorkflowEditApplication(runtime.WorkflowEditApplication(), workflowCommandTestScopeResolver()),
	)
	compatibilityHTTP := httptest.NewServer(server.Handler())
	defer compatibilityHTTP.Close()
	initial := readRunnerWorkflowGraph(t, runtime.Handler, "workflow-a")
	baseRevision := workflowGraphResourceVersion(t, initial)
	patch := map[string]any{"id": "legacy-two-updates", "baseRevision": baseRevision, "operations": []map[string]any{
		{"op": "update_node", "nodeId": "collect", "fields": map[string]any{"label": "Collect health"}},
		{"op": "update_node", "nodeId": "end", "fields": map[string]any{"label": "Done"}},
	}}
	secretResponse := postRunnerStudioWorkflowAI(t, compatibilityHTTP.URL+runnerStudioWorkflowAIPrefix+"apply", map[string]any{
		"workflowId": "workflow-a", "baseRevision": baseRevision, "patchId": "legacy-secret-rejected",
		"userConfirmationId": "confirmed", "drawerSessionId": "canvas-a", "reason": "token=secret-sentinel",
		"patch": map[string]any{"id": "legacy-secret-rejected", "baseRevision": baseRevision, "operations": []map[string]any{{"op": "update_node", "nodeId": "collect", "fields": map[string]any{"label": "x"}}}},
	})
	if secretResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("secret request status = %d body=%s", secretResponse.StatusCode, readResponseBody(secretResponse))
	}
	secretBody := readResponseBody(secretResponse)
	secretResponse.Body.Close()
	if bytes.Contains([]byte(secretBody), []byte("secret-sentinel")) {
		t.Fatalf("secret request was echoed: %s", secretBody)
	}
	for _, request := range []struct {
		action  string
		payload map[string]any
	}{
		{"snapshot", map[string]any{"workflowId": "workflow-a"}},
		{"validate", map[string]any{"workflowId": "workflow-a", "baseRevision": baseRevision, "patch": patch}},
		{"preview", map[string]any{"workflowId": "workflow-a", "baseRevision": baseRevision, "patch": patch}},
	} {
		response := postRunnerStudioWorkflowAI(t, compatibilityHTTP.URL+runnerStudioWorkflowAIPrefix+request.action, request.payload)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("legacy %s status = %d body=%s", request.action, response.StatusCode, readResponseBody(response))
		}
		response.Body.Close()
	}
	legacyPayload := map[string]any{
		"workflowId": "workflow-a", "baseRevision": baseRevision, "patchId": "legacy-two-updates",
		"userConfirmationId": "confirmed", "drawerSessionId": "canvas-a", "reason": "rename two nodes",
		"patch": patch,
	}
	legacy := postRunnerStudioWorkflowAI(t, compatibilityHTTP.URL+runnerStudioWorkflowAIPrefix+"apply", legacyPayload)
	defer legacy.Body.Close()
	if legacy.StatusCode != http.StatusOK {
		t.Fatalf("legacy apply status = %d body=%s", legacy.StatusCode, readResponseBody(legacy))
	}
	var legacyResult struct {
		RevisionAfter  string `json:"revisionAfter"`
		UndoCheckpoint struct {
			ID string `json:"id"`
		} `json:"undoCheckpoint"`
	}
	if err := json.NewDecoder(legacy.Body).Decode(&legacyResult); err != nil || legacyResult.RevisionAfter == "" || legacyResult.UndoCheckpoint.ID == "" {
		t.Fatalf("legacy result = %+v, %v", legacyResult, err)
	}

	manual := postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": legacyResult.RevisionAfter,
		"idempotencyKey": "manual-after-legacy", "reason": "move collect after legacy apply",
		"command": map[string]any{"kind": "move", "layout": map[string]any{"nodes": []map[string]any{{"nodeId": "collect", "position": map[string]any{"x": 280, "y": 160}}}}},
	})
	if manual.Code != http.StatusOK {
		t.Fatalf("manual command status = %d body=%s", manual.Code, manual.Body.String())
	}
	afterManual := readRunnerWorkflowGraph(t, runtime.Handler, "workflow-a")
	revisionAfterManual := workflowGraphResourceVersion(t, afterManual)
	if revisionAfterManual == legacyResult.RevisionAfter || nodeByID(t, afterManual, "collect").Label != "Collect health" || nodeByID(t, afterManual, "end").Label != "Done" || nodeByID(t, afterManual, "collect").Position != (visual.Position{X: 280, Y: 160}) {
		t.Fatalf("shared committed graph = %#v", afterManual)
	}

	replay := postRunnerStudioWorkflowAI(t, compatibilityHTTP.URL+runnerStudioWorkflowAIPrefix+"apply", legacyPayload)
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("legacy replay status = %d body=%s", replay.StatusCode, readResponseBody(replay))
	}
	afterReplay := readRunnerWorkflowGraph(t, runtime.Handler, "workflow-a")
	if got := workflowGraphResourceVersion(t, afterReplay); got != revisionAfterManual {
		t.Fatalf("idempotent legacy replay advanced revision from %q to %q", revisionAfterManual, got)
	}
}

func TestWorkflowCompatibilityRejectsUnknownIdentityFullGraphAndUIMaps(t *testing.T) {
	application := &workflowCompatibilityRecordingApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	tests := []map[string]any{
		{"workflowId": "workflow-a", "baseRevision": "r1", "patchId": "p1", "actorId": "forged", "reason": "forged", "userConfirmationId": "yes", "patch": map[string]any{"id": "p1", "operations": []map[string]any{{"op": "update_node", "nodeId": "collect", "fields": map[string]any{"label": "x"}}}}},
		{"workflowId": "workflow-a", "baseRevision": "r1", "patchId": "p2", "reason": "full graph", "userConfirmationId": "yes", "allowFullGraphReplacement": true, "patch": map[string]any{"id": "p2", "operations": []map[string]any{{"op": "replace_full_graph", "graph": map[string]any{}}}}},
		{"workflowId": "workflow-a", "baseRevision": "r1", "patchId": "p3", "reason": "ui map", "userConfirmationId": "yes", "patch": map[string]any{"id": "p3", "operations": []map[string]any{{"op": "add_node", "node": map[string]any{"id": "x", "type": "action", "position": map[string]any{"x": 1, "y": 2}, "ui": map[string]any{"saved": true}}}}}},
	}
	for index, payload := range tests {
		response := postWorkflowCompatibilityRecorder(t, server, "apply", payload)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("case %d status = %d body=%s", index, response.Code, response.Body.String())
		}
		if bytes.Contains(response.Body.Bytes(), []byte("secret-sentinel")) {
			t.Fatalf("case %d echoed protected request material: %s", index, response.Body.String())
		}
	}
	if application.applyCalls != 0 {
		t.Fatalf("rejected compatibility requests reached application %d times", application.applyCalls)
	}
}

type workflowCompatibilityNoReferences struct{}

func (workflowCompatibilityNoReferences) ReferencesForWorkflow(context.Context, string) ([]service.WorkflowReference, error) {
	return nil, nil
}

func newWorkflowCompatibilityRuntime(t *testing.T) *runnerapp.Runtime {
	t.Helper()
	base := t.TempDir()
	config := runnerconfig.Default()
	config.Auth.Enabled, config.UI.Enabled = false, false
	config.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	config.Stores.ScriptsDir = filepath.Join(base, "scripts")
	config.Stores.SkillsDir = filepath.Join(base, "skills")
	config.Stores.EnvironmentsDir = filepath.Join(base, "environments")
	config.Stores.MCPDir = filepath.Join(base, "mcp")
	config.Stores.RunStateFile = filepath.Join(base, "run-state.json")
	config.Stores.AgentStateFile = filepath.Join(base, "agents.json")
	runtime, err := runnerapp.NewRuntime(context.Background(), runnerapp.RuntimeOptions{Config: config, WorkflowTenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	return runtime
}

func createRunnerWorkflowGraph(t *testing.T, handler http.Handler, graph visual.Graph) {
	t.Helper()
	response := serveRunnerWorkflowJSON(t, handler, http.MethodPost, "/api/v1/workflows/graph", map[string]any{"graph": graph})
	if response.Code != http.StatusCreated {
		t.Fatalf("create graph status = %d body=%s", response.Code, response.Body.String())
	}
}

func readRunnerWorkflowGraph(t *testing.T, handler http.Handler, workflowID string) visual.Graph {
	t.Helper()
	response := serveRunnerWorkflowJSON(t, handler, http.MethodGet, "/api/v1/workflows/"+workflowID+"/graph", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("read graph status = %d body=%s", response.Code, response.Body.String())
	}
	var graph visual.Graph
	if err := json.NewDecoder(response.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	return graph
}

func serveRunnerWorkflowJSON(t *testing.T, handler http.Handler, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			t.Fatal(err)
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, &body))
	return response
}

func workflowGraphResourceVersion(t *testing.T, graph visual.Graph) string {
	t.Helper()
	value, ok := graph.UI["resource_version"].(string)
	if !ok || value == "" {
		t.Fatalf("graph has no resource version: %#v", graph.UI)
	}
	return value
}

func nodeByID(t *testing.T, graph visual.Graph, id string) visual.Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %q not found", id)
	return visual.Node{}
}

func postWorkflowCompatibilityRecorder(t *testing.T, server *HTTPServer, action string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, runnerStudioWorkflowAIPrefix+action, bytes.NewReader(raw)))
	return response
}

func readResponseBody(response *http.Response) string {
	raw, _ := io.ReadAll(response.Body)
	return string(raw)
}

type workflowCompatibilityRecordingApplication struct{ applyCalls int }

func (*workflowCompatibilityRecordingApplication) ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	return service.WorkflowManualCommandResult{}, nil
}
func (*workflowCompatibilityRecordingApplication) ReadLegacySnapshot(context.Context, edit.WorkflowKey) (service.WorkflowLegacySnapshotResult, error) {
	return service.WorkflowLegacySnapshotResult{}, nil
}
func (*workflowCompatibilityRecordingApplication) ValidateLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowLegacyPatchValidationResult, error) {
	return service.WorkflowLegacyPatchValidationResult{}, nil
}
func (*workflowCompatibilityRecordingApplication) PreviewLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowEditPreviewResult, error) {
	return service.WorkflowEditPreviewResult{}, nil
}
func (application *workflowCompatibilityRecordingApplication) ApplyLegacyPatch(context.Context, service.WorkflowLegacyPatchRequest) (service.WorkflowManualCommandResult, error) {
	application.applyCalls++
	return service.WorkflowManualCommandResult{}, nil
}
