package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"runner/server/api"
	"runner/server/config"
	"runner/server/service"
	"runner/workflow"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditFacadeCanonicalKeyIsolation(t *testing.T) {
	visualService, facade := newWorkflowEditFacadeTestServices(t)
	keyA := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "shared-workflow"}
	keyB := edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: "shared-workflow"}
	storageA, err := workflowStorageName(keyA)
	if err != nil {
		t.Fatal(err)
	}
	storageB, err := workflowStorageName(keyB)
	if err != nil {
		t.Fatal(err)
	}
	if storageA == storageB || storageA == keyA.WorkflowID || storageB == keyB.WorkflowID {
		t.Fatalf("tenant storage namespaces are not isolated: A=%q B=%q", storageA, storageB)
	}
	createWorkflowEditFacadeGraph(t, visualService, storageA, "echo tenant-a")
	createWorkflowEditFacadeGraph(t, visualService, storageB, "echo tenant-b")

	snapshotA, err := facade.GetGraph(context.Background(), keyA)
	if err != nil {
		t.Fatal(err)
	}
	snapshotB, err := facade.GetGraph(context.Background(), keyB)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotA.Key != keyA || snapshotB.Key != keyB {
		t.Fatalf("canonical keys were not preserved: A=%+v B=%+v", snapshotA.Key, snapshotB.Key)
	}
	if snapshotA.Graph.Nodes[1].Step.Args["script"] == snapshotB.Graph.Nodes[1].Step.Args["script"] {
		t.Fatal("same workflowId in different tenants resolved to the same graph")
	}
	if _, exists := reflect.TypeOf(facade).MethodByName("ListWorkflows"); exists {
		t.Fatal("narrow facade must not expose workflow enumeration")
	}
}

func TestWorkflowEditFacadeMatchesVisualWorkflowService(t *testing.T) {
	visualService, facade := newWorkflowEditFacadeTestServices(t)
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "inspect"}
	storageName, err := workflowStorageName(key)
	if err != nil {
		t.Fatal(err)
	}
	createWorkflowEditFacadeGraph(t, visualService, storageName, "uptime")

	snapshot, err := facade.GetGraph(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	directGraph, err := visualService.GetGraph(context.Background(), storageName)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.Graph, directGraph) {
		t.Fatalf("facade graph differs from Visual API graph")
	}
	directHashes, err := service.VisualWorkflowGraphHashes(directGraph)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision.SemanticHash != directHashes.SemanticHash || snapshot.Revision.LayoutHash != directHashes.LayoutHash {
		t.Fatalf("facade hashes differ: facade=%+v visual=%+v", snapshot.Revision, directHashes)
	}
	if snapshot.Revision.ResourceVersion == "" {
		t.Fatal("facade revision omitted Visual API resource version")
	}
	if got := directGraph.UI[workflowResourceVersionField]; got != snapshot.Revision.ResourceVersion {
		t.Fatalf("facade resource version = %q, Visual API = %q", snapshot.Revision.ResourceVersion, got)
	}
	if hashes, err := facade.GetHashes(context.Background(), key); err != nil || hashes != snapshot.Revision {
		t.Fatalf("GetHashes() = %+v, %v; want %+v", hashes, err, snapshot.Revision)
	}

	validation, err := facade.Validate(context.Background(), key, snapshot.Graph)
	if err != nil || !validation.Valid {
		t.Fatalf("Validate() = %+v, %v", validation, err)
	}
	compiled, err := facade.Compile(context.Background(), key, snapshot.Graph)
	if err != nil || compiled.Workflow.Name != storageName {
		t.Fatalf("Compile() = %+v, %v", compiled, err)
	}
	actions, err := facade.ListActions(context.Background(), key, service.ActionCatalogFilter{})
	if err != nil || len(actions) == 0 {
		t.Fatalf("ListActions() count=%d error=%v", len(actions), err)
	}
}

func TestWorkflowEditFacadeIsExposedByRuntime(t *testing.T) {
	base := t.TempDir()
	cfg := config.Default()
	cfg.Auth.Enabled = false
	cfg.UI.Enabled = false
	cfg.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(base, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(base, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(base, "environments")
	cfg.Stores.MCPDir = filepath.Join(base, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(base, "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(base, "agents.json")
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	if runtime.WorkflowEditFacade() == nil {
		t.Fatal("runtime did not expose its workflow edit facade")
	}
	if runtime.WorkflowEditApplication() == nil || runtime.WorkflowEditApplication() != runtime.WorkflowEditFacade().Application() {
		t.Fatal("runtime did not expose the facade-owned application singleton")
	}
}

func TestWorkflowEditFacadeApplicationMatchesRunnerHTTPAndReusesInstance(t *testing.T) {
	visualService, facade := newWorkflowEditFacadeTestServices(t)
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "http-contract"}
	storageName, err := workflowStorageName(key)
	if err != nil {
		t.Fatal(err)
	}
	createWorkflowEditFacadeGraph(t, visualService, storageName, "uptime")
	application := facade.Application()
	if application == nil || application != facade.Application() {
		t.Fatal("facade did not retain one application instance")
	}
	contextResult, err := application.GetContext(context.Background(), key, service.WorkflowContextQuery{View: service.WorkflowContextFullView, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	backendSnapshot, err := facade.ReadWorkflow(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+storageName+"/graph", nil)
	req.SetPathValue("name", storageName)
	recorder := httptest.NewRecorder()
	api.NewVisualWorkflowHandler(visualService).GetGraph(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("HTTP graph status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var httpGraph visual.Graph
	if err := json.Unmarshal(recorder.Body.Bytes(), &httpGraph); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(httpGraph, backendSnapshot.Graph) || !reflect.DeepEqual(contextResult.Graph.Nodes, httpGraph.Nodes) || !reflect.DeepEqual(contextResult.Graph.Edges, httpGraph.Edges) || contextResult.Revision != backendSnapshot.Revision {
		t.Fatalf("application/backend/HTTP snapshot drift: app=%+v backend=%+v", contextResult.Revision, backendSnapshot.Revision)
	}
	httpRevision, err := workflowGraphRevision(httpGraph)
	if err != nil || httpRevision != contextResult.Revision {
		t.Fatalf("HTTP revision=%+v,%v application=%+v", httpRevision, err, contextResult.Revision)
	}
}

func TestWorkflowEditFacadeManualCommandMigratesLegacyWorkflowBeforeApply(t *testing.T) {
	root := t.TempDir()
	workflowsDir := filepath.Join(root, "workflows")
	if err := os.MkdirAll(workflowsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "legacy-shell"}
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Name: key.WorkflowID, Version: "v0.1", Plan: workflow.Plan{Mode: "auto", Strategy: "sequential"},
			Inventory: workflow.Inventory{Groups: map[string]workflow.Group{"local": {Hosts: []string{"local"}}}, Hosts: map[string]workflow.Host{"local": {Address: "local"}}},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart, Position: visual.Position{X: 80, Y: 160}, Ports: []visual.Port{{ID: "next", Type: "output"}}},
			{ID: "action", Type: visual.NodeTypeAction, Position: visual.Position{X: 300, Y: 160}, Ports: []visual.Port{{ID: "in", Type: "input"}, {ID: "next", Type: "output"}}, Step: &workflow.Step{Name: "action", ID: "action", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo existing"}}},
			{ID: "end", Type: visual.NodeTypeEnd, Position: visual.Position{X: 720, Y: 160}, Ports: []visual.Port{{ID: "in", Type: "input"}}},
		},
		Edges: []visual.Edge{
			{ID: "start-action", Source: "start", SourcePort: "next", Target: "action", TargetPort: "in", Kind: visual.EdgeKindNext},
			{ID: "action-end", Source: "action", SourcePort: "next", Target: "end", TargetPort: "in", Kind: visual.EdgeKindNext},
		},
	}
	compiled, err := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{}).Compile(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, key.WorkflowID+".yaml"), []byte(compiled.YAML), 0o600); err != nil {
		t.Fatal(err)
	}

	workflowService := service.NewWorkflowService(workflowsDir)
	workflowService.SetWorkflowTenantID(key.TenantID)
	visualService := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{
		WorkflowService: workflowService, CommitStore: workflowService.WorkflowCommitStore(), TenantID: key.TenantID,
		AllowedActions: []string{"script.shell"},
	})
	facade, err := newWorkflowEditMutationFacade(
		visualService, []string{"script.shell"}, key.TenantID,
		filepath.Join(workflowsDir, ".transactions"), workflowService.WorkflowCommitStore(),
	)
	if err != nil {
		t.Fatal(err)
	}
	facade.SetWorkflowReferenceChecker(&workflowEditNoReferences{})
	legacy, err := facade.ReadWorkflow(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}

	nodeType := visual.NodeTypeAction
	position := visual.Position{X: 520, Y: 180}
	label, stepName := "Shell Script", "script-shell-1-test"
	stepID := "script-shell"
	ports := []visual.Port{{ID: "in", Type: "input"}, {ID: "next", Type: "output"}, {ID: "failure", Type: "output"}}
	source, target := "action", "end"
	input, output := "in", "next"
	edgeKind := visual.EdgeKindNext
	request := service.WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: key, ExpectedRevision: legacy.Revision.ResourceVersion,
		IdempotencyKey: "legacy-shell-insert", ActorID: "manual-user", CanvasSessionID: "manual-canvas",
		SessionID: "manual-session", Reason: "insert shell before end", Confirmed: true,
		Commands: []service.WorkflowManualCommand{
			{Kind: service.WorkflowManualDisconnectEdge, EdgeID: "action-end"},
			{Kind: service.WorkflowManualAddNode, Node: &edit.EditableNode{ID: stepID, NodePatch: edit.NodePatch{
				Type: &nodeType, Position: &position, Label: &label, Ports: &ports, StepName: &stepName, StepID: &stepID,
				Step: &workflow.Step{Name: stepName, ID: stepID, Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo ok"}},
			}}},
			{Kind: service.WorkflowManualConnectEdge, Edge: &edit.EditableEdge{ID: "action-script-shell", EdgePatch: edit.EdgePatch{Source: &source, Target: &stepID, SourcePort: &output, TargetPort: &input, Kind: &edgeKind}}},
			{Kind: service.WorkflowManualConnectEdge, Edge: &edit.EditableEdge{ID: "script-shell-end", EdgePatch: edit.EdgePatch{Source: &stepID, Target: &target, SourcePort: &output, TargetPort: &input, Kind: &edgeKind}}},
		},
	}
	result, err := facade.Application().ExecuteManualCommand(context.Background(), request)
	if err != nil {
		t.Fatalf("ExecuteManualCommand(legacy workflow) = %#v, %v", result, err)
	}
	if result.Receipt.RevisionAfter == nil || result.Receipt.RevisionBefore.ResourceVersion == legacy.Revision.ResourceVersion {
		t.Fatalf("legacy baseline was not migrated before apply: legacy=%q receipt=%#v", legacy.Revision.ResourceVersion, result.Receipt)
	}
	committed, err := facade.ReadWorkflow(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if visualNodeByIDForFacadeTest(committed.Graph, stepID) == nil || visualEdgeByIDForFacadeTest(committed.Graph, "action-end") != nil || visualEdgeByIDForFacadeTest(committed.Graph, "action-script-shell") == nil || visualEdgeByIDForFacadeTest(committed.Graph, "script-shell-end") == nil {
		t.Fatalf("legacy command did not commit the connected node: %#v", committed.Graph)
	}
	snapshot, err := workflowService.WorkflowCommitStore().Snapshot(context.Background(), key)
	if err != nil || snapshot.Generation != 2 {
		t.Fatalf("legacy baseline plus manual commit snapshot = %#v, %v", snapshot, err)
	}
	replayed, err := facade.Application().ExecuteManualCommand(context.Background(), request)
	if err != nil || replayed.Receipt.TransactionID != result.Receipt.TransactionID {
		t.Fatalf("legacy command replay = %#v, %v", replayed, err)
	}
	afterReplay, err := workflowService.WorkflowCommitStore().Snapshot(context.Background(), key)
	if err != nil || afterReplay.Generation != 2 {
		t.Fatalf("legacy command replay changed transaction generation: %#v, %v", afterReplay, err)
	}
	stale := request
	stale.IdempotencyKey = "legacy-shell-stale-new-command"
	_, err = facade.Application().ExecuteManualCommand(context.Background(), stale)
	var conflict *edit.DomainError
	if !errors.As(err, &conflict) || conflict.Code != edit.ErrorStaleRevision {
		t.Fatalf("new command on the legacy revision error = %v, want stale revision", err)
	}
}

func visualNodeByIDForFacadeTest(graph visual.Graph, id string) *visual.Node {
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == id {
			return &graph.Nodes[index]
		}
	}
	return nil
}

func visualEdgeByIDForFacadeTest(graph visual.Graph, id string) *visual.Edge {
	for index := range graph.Edges {
		if graph.Edges[index].ID == id {
			return &graph.Edges[index]
		}
	}
	return nil
}

func TestWorkflowEditFacadeDefaultAdaptersFailClosedThenUseReferenceChecker(t *testing.T) {
	visualService, facade := newWorkflowEditFacadeTestServices(t)
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "guarded"}
	storageName, err := workflowStorageName(key)
	if err != nil {
		t.Fatal(err)
	}
	createWorkflowEditFacadeGraph(t, visualService, storageName, "uptime")
	snapshot, err := facade.ReadWorkflow(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	result, err := facade.Application().Validate(context.Background(), service.WorkflowEditValidateRequest{Key: key, Graph: snapshot.Graph})
	if err != nil {
		t.Fatal(err)
	}
	if result.Validation.Valid || !hasWorkflowEditIssueCode(result.Validation.Errors, "REFERENCE_GUARD_UNAVAILABLE") {
		t.Fatalf("missing reference guard was not fail-closed: %+v", result)
	}
	catalog, err := facade.Application().GetActionCatalog(context.Background(), key, service.WorkflowActionCatalogQuery{Names: []string{"script.shell"}})
	if err != nil || len(catalog.Items) != 1 || catalog.Items[0].Access.Allowed {
		t.Fatalf("high-risk action access was not fail-closed: %+v, %v", catalog, err)
	}

	checker := &workflowEditNoReferences{}
	facade.SetWorkflowReferenceChecker(checker)
	result, err = facade.Application().Validate(context.Background(), service.WorkflowEditValidateRequest{Key: key, Graph: snapshot.Graph})
	if err != nil || !result.Validation.Valid {
		t.Fatalf("real empty reference check did not enable valid result: %+v, %v", result, err)
	}
	if checker.workflowName != storageName || checker.workflowName == key.WorkflowID {
		t.Fatalf("reference guard received non-canonical name %q", checker.workflowName)
	}
}

type workflowEditNoReferences struct{ workflowName string }

func (checker *workflowEditNoReferences) ReferencesForWorkflow(_ context.Context, workflowName string) ([]service.WorkflowReference, error) {
	checker.workflowName = workflowName
	return nil, nil
}

func hasWorkflowEditIssueCode(issues []service.VisualWorkflowIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestWorkflowEditFacadeRejectsInvalidOrMismatchedKey(t *testing.T) {
	_, facade := newWorkflowEditFacadeTestServices(t)
	invalid := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "../escape"}
	if _, err := facade.GetGraph(context.Background(), invalid); err == nil {
		t.Fatal("GetGraph accepted invalid canonical key")
	}
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "bound"}
	graph := workflowEditFacadeTestGraph("different-storage-name", "true")
	if _, err := facade.Validate(context.Background(), key, graph); err == nil {
		t.Fatal("Validate accepted a graph bound to another workflow")
	}
}

func TestWorkflowEditFacadeDryRunExecutorBindsTrustedKeyGraphAndEnvironment(t *testing.T) {
	workflowService := service.NewWorkflowService(t.TempDir())
	visualService := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowService})
	facade := NewWorkflowEditFacade(visualService)
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "dry-run-boundary"}
	storageName, err := workflowStorageName(key)
	if err != nil {
		t.Fatal(err)
	}
	graph := workflowEditFacadeTestGraph(storageName, "true")
	graph.Workflow.ValidationEnv = "staging"
	if _, err := visualService.CreateGraph(context.Background(), graph, service.VisualWorkflowCreateOptions{}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := facade.GetGraph(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}

	result, err := facade.DryRunWorkflow(context.Background(), key, snapshot.Graph, service.WorkflowEditDryRunExecutionRequest{
		Mode: service.WorkflowDryRunModeDraftNonProduction, Environment: "staging",
	})
	if err != nil || result.Outcome != service.WorkflowPostCheckPassed || len(result.Diagnostics) == 0 || result.Diagnostics[0].Severity != "warning" {
		t.Fatalf("DryRunWorkflow() = %#v, %v", result, err)
	}
	record, err := workflowService.Get(context.Background(), storageName)
	if err != nil || record.Status != service.WorkflowStatusDraft {
		t.Fatalf("edit dry-run mutated the legacy lifecycle: %#v, %v", record, err)
	}

	for _, test := range []struct {
		name    string
		key     edit.WorkflowKey
		request service.WorkflowEditDryRunExecutionRequest
	}{
		{name: "forged key", key: edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: key.WorkflowID}, request: service.WorkflowEditDryRunExecutionRequest{Mode: service.WorkflowDryRunModeDraftNonProduction, Environment: "staging"}},
		{name: "wrong environment", key: key, request: service.WorkflowEditDryRunExecutionRequest{Mode: service.WorkflowDryRunModeDraftNonProduction, Environment: "production"}},
		{name: "wrong mode", key: key, request: service.WorkflowEditDryRunExecutionRequest{Mode: "execute", Environment: "staging"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := facade.DryRunWorkflow(context.Background(), test.key, snapshot.Graph, test.request); err == nil {
				t.Fatal("untrusted dry-run input was accepted")
			}
		})
	}

	invalid := snapshot.Graph
	invalid.Nodes, invalid.Edges = invalid.Nodes[:1], nil
	failed, err := facade.DryRunWorkflow(context.Background(), key, invalid, service.WorkflowEditDryRunExecutionRequest{
		Mode: service.WorkflowDryRunModeDraftNonProduction, Environment: "staging",
	})
	if err != nil || failed.Outcome != service.WorkflowPostCheckFailed || len(failed.Diagnostics) == 0 || failed.Diagnostics[0].Severity != "error" {
		t.Fatalf("invalid Visual dry-run projection = %#v, %v", failed, err)
	}
}

func newWorkflowEditFacadeTestServices(t *testing.T) (*service.VisualWorkflowService, *WorkflowEditFacade) {
	t.Helper()
	workflowService := service.NewWorkflowService(t.TempDir())
	visualService := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowService})
	return visualService, NewWorkflowEditFacade(visualService)
}

func createWorkflowEditFacadeGraph(t *testing.T, visualService *service.VisualWorkflowService, name, script string) {
	t.Helper()
	if _, err := visualService.CreateGraph(context.Background(), workflowEditFacadeTestGraph(name, script), service.VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func workflowEditFacadeTestGraph(name, script string) visual.Graph {
	return visual.Graph{
		Version:  visual.GraphVersion,
		Workflow: workflow.Workflow{Name: name, Version: "v0.1", Plan: workflow.Plan{Mode: "auto", Strategy: "sequential"}},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "action", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "action", Action: "script.shell", Args: map[string]any{"script": script}}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-action", Source: "start", Target: "action", Kind: visual.EdgeKindNext},
			{ID: "action-end", Source: "action", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}
