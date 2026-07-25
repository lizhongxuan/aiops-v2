package workfloweditor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	"runner/workflow"
	contract "runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowReadToolsV2RegisterOnlyImplementedCoreTools(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2(app)
	if err != nil {
		t.Fatal(err)
	}
	tools := WorkflowReadToolsV2(app, resources)
	if got := workflowEditorToolNames(tools); strings.Join(got, ",") != "workflow.get_context,workflow.get_action_catalog" {
		t.Fatalf("registered tools = %v", got)
	}
	for _, tool := range tools {
		if !tool.IsReadOnly(nil) || tool.IsDestructive(nil) || !tool.IsConcurrencySafe(nil) {
			t.Fatalf("%s must be concurrency-safe read-only", tool.Metadata().Name)
		}
		if len(tool.InputSchema()) == 0 || len(tool.OutputSchema()) == 0 {
			t.Fatalf("%s must publish strict input and output schemas", tool.Metadata().Name)
		}
	}
	registry := mcp.NewRegistry()
	if err := Register(registry, app, resources); err != nil {
		t.Fatal(err)
	}
	if got := workflowEditorToolNames(registry.ListServerTools(ServerID)); strings.Join(got, ",") != "workflow.get_context,workflow.get_action_catalog,workflow.compile_plan,workflow.preview_patch,workflow.apply_patch,workflow.validate" {
		t.Fatalf("registry tools = %v", got)
	}
}

func TestWorkflowGetContextUsesTrustedScopeAndReturnsSummaryText(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	ctx := workflowTrustedContext("tenant-a", "user-a", "workflow-a")
	input := contextInput("selection", "old-revision", []string{"step"}, 1, 20)
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Content, `"nodes"`) || strings.HasPrefix(strings.TrimSpace(result.Content), "{") {
		t.Fatalf("TextContent must be a short human summary, got %q", result.Content)
	}
	var envelope contract.ToolResult[contract.WorkflowContext]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.StructuredContent.WorkflowKey.WorkflowID != "workflow-a" || !envelope.StructuredContent.SelectionContextStale {
		t.Fatalf("context = %#v", envelope.StructuredContent)
	}
	if envelope.StructuredContent.ResourceURI == "" || len(result.References) != 1 {
		t.Fatalf("resource uri/references = %q/%#v", envelope.StructuredContent.ResourceURI, result.References)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
		t.Fatalf("output contract: %v", err)
	}
}

func TestWorkflowGetContextRejectsModelOwnedTenantOrWorkflowFields(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	ctx := workflowTrustedContext("tenant-a", "user-a", "workflow-a")
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","view":"summary","workflowKey":{"tenantId":"tenant-b","workflowId":"workflow-a"}}`),
		json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","view":"summary","tenantId":"tenant-a","workflowId":"workflow-b"}`),
	} {
		if err := tool.ValidateInput(ctx, input); err == nil {
			t.Fatalf("model-owned identity was accepted by the read schema: %s", input)
		}
	}
	if _, err := tool.Execute(context.Background(), contextInput("summary", "", nil, 0, 10)); err == nil {
		t.Fatal("missing trusted execution context must fail closed")
	}
}

func TestWorkflowGetContextDefaultsToFrozenTurnSelectionAndRejectsExpansion(t *testing.T) {
	app, backend := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	trusted := appui.WorkflowTurnTrustedScope{TenantID: "tenant-a", WorkflowID: "workflow-a", ActorID: "user-a", SessionID: "session-a"}
	artifact, err := appui.FreezeWorkflowTurnContext(trusted, appui.WorkflowTurnContext{
		WorkflowKey: backend.snapshot.Key, Revision: backend.snapshot.Revision,
		Selection:   appui.WorkflowTurnSelection{NodeIDs: []string{"step"}, NeighborhoodIDs: []string{"start"}},
		Environment: appui.WorkflowTurnEnvironment{BindingVersion: "env-v1"},
		Scope:       appui.WorkflowTurnContextScopeBinding{ActorID: trusted.ActorID, SessionID: trusted.SessionID},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		TenantID: "tenant-a", UserID: "user-a", SessionID: "session-a",
		Metadata: map[string]string{"workflowId": "workflow-a"}, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{artifact},
	})
	input, _ := json.Marshal(workflowGetContextInputV2{
		SchemaVersion: contract.SchemaVersion,
		View:          "selection", TurnContextArtifactID: artifact.ID, Limit: 20,
	})
	result, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.WorkflowContext]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil || envelope.StructuredContent.SelectionContextStale {
		t.Fatalf("frozen context result = %#v, %v", envelope, err)
	}

	outOfScope, _ := json.Marshal(workflowGetContextInputV2{
		SchemaVersion: contract.SchemaVersion,
		View:          "selection", Revision: backend.snapshot.Revision.ResourceVersion,
		TurnContextArtifactID: artifact.ID, NodeIDs: []string{"end"}, Limit: 20,
	})
	if _, err := tool.Execute(ctx, outOfScope); err == nil || !strings.Contains(err.Error(), "outside the frozen") {
		t.Fatalf("out-of-scope selection error = %v", err)
	}
	forged := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		TenantID: "tenant-b", UserID: "user-a", SessionID: "session-a",
		Metadata: map[string]string{"workflowId": "workflow-a"}, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{artifact},
	})
	if _, err := tool.Execute(forged, input); err == nil || !strings.Contains(err.Error(), "trusted workflow scope") {
		t.Fatalf("forged runtime identity error = %v", err)
	}
}

func TestWorkflowGetContextExternalizesLargeGraphButResourceKeepsPage(t *testing.T) {
	app, backend := newWorkflowReadFixture(t)
	backend.mu.Lock()
	for index := 0; index < 100; index++ {
		backend.snapshot.Graph.Nodes = append(backend.snapshot.Graph.Nodes, visual.Node{ID: fmt.Sprintf("extra-%03d", index), Type: visual.NodeTypeAction, Label: strings.Repeat("bounded-label-", 12)})
	}
	hashes, err := service.VisualWorkflowGraphHashes(backend.snapshot.Graph)
	if err != nil {
		backend.mu.Unlock()
		t.Fatal(err)
	}
	backend.snapshot.Revision.SemanticHash, backend.snapshot.Revision.LayoutHash = hashes.SemanticHash, hashes.LayoutHash
	backend.mu.Unlock()
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	result, err := tool.Execute(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), contextInput("full", "revision-a", nil, 0, 200))
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.WorkflowContext]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.StructuredContent.Graph) != 0 || len(result.References) != 1 {
		t.Fatalf("large inline graph/reference = %d/%#v", len(envelope.StructuredContent.Graph), result.References)
	}
	content, err := resources.Read(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), result.References[0].URI)
	if err != nil || !strings.Contains(string(content.Content), "extra-099") {
		t.Fatalf("resource read = %v, %s", err, content.Content)
	}
}

func TestWorkflowActionCatalogReadsAllowedRealSpecs(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_action_catalog")
	input := json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","names":["script.safe","script.denied"],"includeSchema":true,"includeExamples":true,"limit":20}`)
	result, err := tool.Execute(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), input)
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.ActionCatalogPage]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	page := envelope.StructuredContent
	if len(page.Entries) != 1 || page.Entries[0].Name != "script.safe" || page.CatalogVersion == "" {
		t.Fatalf("catalog = %#v", page)
	}
	if !strings.Contains(string(page.Entries[0].Spec), "args_schema") || !strings.Contains(string(page.Entries[0].Spec), "examples") {
		t.Fatalf("catalog spec = %s", page.Entries[0].Spec)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
		t.Fatalf("output contract: %v", err)
	}
}

func TestWorkflowActionCatalogExternalizesLargeSchemas(t *testing.T) {
	app, backend := newWorkflowReadFixture(t)
	backend.mu.Lock()
	backend.actions = []service.ActionSpec{{Action: "script.large", Title: "Large", Category: "script", ArgsSchema: json.RawMessage(`{"type":"object","description":"` + strings.Repeat("bounded schema ", 900) + `"}`)}}
	backend.mu.Unlock()
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_action_catalog")
	input := json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","includeSchema":true,"limit":20}`)
	result, err := tool.Execute(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), input)
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.ActionCatalogPage]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.StructuredContent.Entries) != 0 || len(result.References) != 1 {
		t.Fatalf("large catalog inline/reference = %#v/%#v", envelope.StructuredContent.Entries, result.References)
	}
	content, err := resources.Read(workflowTrustedContext("tenant-a", "user-a", "workflow-a"), result.References[0].URI)
	if err != nil || !strings.Contains(string(content.Content), "script.large") {
		t.Fatalf("catalog resource = %v, %s", err, content.Content)
	}
}

func TestWorkflowReadToolsV2RejectUnknownFieldsAndUntrustedPermissions(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	tool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	unknown := json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","view":"summary","unknown":true}`)
	if err := tool.ValidateInput(context.Background(), unknown); err == nil {
		t.Fatal("strict input must reject unknown fields")
	}
	if decision := tool.CheckPermissions(context.Background(), contextInput("summary", "", nil, 0, 10)); decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("permission decision = %#v", decision)
	}
}

func contextInput(view, revision string, nodeIDs []string, depth, limit int) json.RawMessage {
	raw, _ := json.Marshal(workflowGetContextInputV2{SchemaVersion: contract.SchemaVersion, Revision: revision, View: view, NodeIDs: nodeIDs, EdgeDepth: depth, Limit: limit})
	return raw
}

func workflowTrustedContext(tenant, user, workflowID string) context.Context {
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: tenant, UserID: user, SessionID: "session-a", Metadata: map[string]string{"workflowId": workflowID}})
}

type workflowReadBackend struct {
	mu       sync.RWMutex
	snapshot service.WorkflowEditSnapshot
	actions  []service.ActionSpec
}

func (b *workflowReadBackend) ReadWorkflow(_ context.Context, key contract.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if key != b.snapshot.Key {
		return service.WorkflowEditSnapshot{}, service.ErrWorkflowEditSnapshot
	}
	return b.snapshot, nil
}
func (b *workflowReadBackend) ListWorkflowActions(context.Context, contract.WorkflowKey, service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]service.ActionSpec(nil), b.actions...), nil
}
func (*workflowReadBackend) ValidateWorkflow(context.Context, contract.WorkflowKey, visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	return &service.VisualWorkflowValidationResult{Valid: true, Errors: []service.VisualWorkflowIssue{}, Warnings: []service.VisualWorkflowIssue{}}, nil
}
func (*workflowReadBackend) CompileWorkflow(context.Context, contract.WorkflowKey, visual.Graph) (*service.CompiledVisualWorkflow, error) {
	return &service.CompiledVisualWorkflow{YAML: "workflow: safe"}, nil
}

type workflowReadAccess struct{}

func (workflowReadAccess) ResolveWorkflowActionAccess(_ context.Context, _ contract.WorkflowKey, spec service.ActionSpec) (service.WorkflowActionAccess, error) {
	return service.WorkflowActionAccess{Allowed: spec.Action != "script.denied"}, nil
}

type workflowReadReferences struct{}

func (workflowReadReferences) ValidateWorkflowReferences(context.Context, contract.WorkflowKey, visual.Graph) ([]service.VisualWorkflowIssue, error) {
	return nil, nil
}

func newWorkflowReadFixture(t *testing.T) (*service.WorkflowEditApplication, *workflowReadBackend) {
	t.Helper()
	step := workflow.Step{ID: "step", Name: "step", Action: "script.safe"}
	graph := visual.Graph{Version: visual.GraphVersion, Workflow: workflow.Workflow{Version: "v1", Name: "workflow-a", Steps: []workflow.Step{step}}, Nodes: []visual.Node{{ID: "start", Type: visual.NodeTypeStart}, {ID: "step", Type: visual.NodeTypeAction, StepID: "step", Step: &step}, {ID: "end", Type: visual.NodeTypeEnd}}, Edges: []visual.Edge{{ID: "a", Source: "start", Target: "step", Kind: visual.EdgeKindNext}, {ID: "b", Source: "step", Target: "end", Kind: visual.EdgeKindNext}}}
	hashes, err := service.VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	backend := &workflowReadBackend{snapshot: service.WorkflowEditSnapshot{Key: contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}, Revision: contract.Revision{ResourceVersion: "revision-a", SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}, Graph: graph}, actions: []service.ActionSpec{{Action: "script.safe", Title: "Safe", Category: "script", ArgsSchema: json.RawMessage(`{"type":"object"}`), Examples: []service.ActionExample{{Title: "safe"}}}, {Action: "script.denied", Title: "Denied", Category: "script"}}}
	app, err := service.NewWorkflowEditApplication(backend, workflowReadAccess{}, workflowReadReferences{})
	if err != nil {
		t.Fatal(err)
	}
	return app, backend
}
