package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditActionCatalogUsesRealSchemaPortsCapabilitiesAndAccess(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{denyHighRisk: true}, &workflowEditTestReferences{})
	includeSchema := true
	page, err := app.GetActionCatalog(context.Background(), workflowEditTestKey(), WorkflowActionCatalogQuery{
		Names:         []string{"script.shell"},
		IncludeSchema: &includeSchema,
		Limit:         10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("catalog items = %d; want 1", len(page.Items))
	}
	item := page.Items[0]
	if item.Spec.Action != "script.shell" || !json.Valid(item.Spec.ArgsSchema) || len(item.Spec.DefaultPorts.Inputs) == 0 || len(item.Spec.DefaultPorts.Outputs) == 0 || len(item.Spec.Capabilities) == 0 {
		t.Fatalf("catalog dropped real contract fields: %#v", item.Spec)
	}
	if item.Access.Allowed || len(item.Access.RequiredPermissions) == 0 {
		t.Fatalf("catalog access = %#v; want explicit denial", item.Access)
	}
	if page.CatalogDigest == "" || page.Total != 1 {
		t.Fatalf("catalog metadata = %#v", page)
	}
}

func TestWorkflowEditActionCatalogPaginatesAndRejectsStaleCursor(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	query := WorkflowActionCatalogQuery{Names: []string{"script.python", "script.shell"}, Limit: 1}
	first, err := app.GetActionCatalog(context.Background(), workflowEditTestKey(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first page = %#v", first)
	}
	query.Cursor = first.NextCursor
	second, err := app.GetActionCatalog(context.Background(), workflowEditTestKey(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Spec.Action == first.Items[0].Spec.Action {
		t.Fatalf("second page = %#v", second)
	}
	query.Names = []string{"script.shell"}
	if _, err := app.GetActionCatalog(context.Background(), workflowEditTestKey(), query); err != ErrWorkflowEditCursor {
		t.Fatalf("changed-query cursor error = %v", err)
	}
}

func TestWorkflowEditValidateCallsRealValidateCompileActionAndReferenceGuards(t *testing.T) {
	t.Parallel()
	graph := workflowEditTestGraph()
	backend := newWorkflowEditTestBackend(t, graph)
	references := &workflowEditTestReferences{issues: []VisualWorkflowIssue{{Severity: "error", Type: "reference", Code: "REFERENCE_LOCKED", Message: "referenced workflow is locked"}}}
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, references)

	result, err := app.Validate(context.Background(), WorkflowEditValidateRequest{Key: workflowEditTestKey(), Graph: graph, ExpectedRevision: backend.snapshot.Revision.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if result.Validation.Valid || !result.Compiled || backend.validateCalls != 1 || backend.compileCalls != 1 || references.calls != 1 {
		t.Fatalf("validate did not execute real gates: result=%#v validate=%d compile=%d refs=%d", result, backend.validateCalls, backend.compileCalls, references.calls)
	}
	if result.Revision != backend.snapshot.Revision {
		t.Fatalf("revision = %#v; want Runner revision %#v", result.Revision, backend.snapshot.Revision)
	}
}

func TestWorkflowEditValidateDoesNotInventValidWhenActionArgsFail(t *testing.T) {
	t.Parallel()
	graph := workflowEditTestGraph()
	graph.Nodes[1].Step.Args = nil
	backend := newWorkflowEditTestBackend(t, graph)
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})

	result, err := app.Validate(context.Background(), WorkflowEditValidateRequest{Key: workflowEditTestKey(), Graph: graph})
	if err != nil {
		t.Fatal(err)
	}
	if result.Validation.Valid || result.Compiled || backend.compileCalls != 0 || len(result.Validation.Errors) == 0 {
		t.Fatalf("invalid action was accepted: %#v", result)
	}
}

func TestWorkflowEditValidateReportsStaleExpectedRevision(t *testing.T) {
	t.Parallel()
	graph := workflowEditTestGraph()
	backend := newWorkflowEditTestBackend(t, graph)
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	result, err := app.Validate(context.Background(), WorkflowEditValidateRequest{Key: workflowEditTestKey(), Graph: graph, ExpectedRevision: "old-resource-version"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stale || result.Validation.Valid {
		t.Fatalf("stale revision result = %#v", result)
	}
}

type workflowEditTestBackend struct {
	mu            sync.Mutex
	snapshot      WorkflowEditSnapshot
	visual        *VisualWorkflowService
	validateCalls int
	compileCalls  int
}

func newWorkflowEditTestBackend(t *testing.T, graph visual.Graph) *workflowEditTestBackend {
	t.Helper()
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	return &workflowEditTestBackend{
		snapshot: WorkflowEditSnapshot{Key: workflowEditTestKey(), Revision: edit.Revision{ResourceVersion: "runner-rv-1", SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}, Graph: graph},
		visual:   NewVisualWorkflowService(VisualWorkflowServiceConfig{}),
	}
}

func (b *workflowEditTestBackend) ReadWorkflow(context.Context, edit.WorkflowKey) (WorkflowEditSnapshot, error) {
	return b.snapshot, nil
}

func (b *workflowEditTestBackend) ListWorkflowActions(ctx context.Context, _ edit.WorkflowKey, filter ActionCatalogFilter) ([]ActionSpec, error) {
	return b.visual.ListActions(ctx, filter), nil
}

func (b *workflowEditTestBackend) ValidateWorkflow(ctx context.Context, _ edit.WorkflowKey, graph visual.Graph) (*VisualWorkflowValidationResult, error) {
	b.mu.Lock()
	b.validateCalls++
	b.mu.Unlock()
	return b.visual.Validate(ctx, graph)
}

func (b *workflowEditTestBackend) CompileWorkflow(ctx context.Context, _ edit.WorkflowKey, graph visual.Graph) (*CompiledVisualWorkflow, error) {
	b.mu.Lock()
	b.compileCalls++
	b.mu.Unlock()
	return b.visual.Compile(ctx, graph)
}

type workflowEditTestAccess struct{ denyHighRisk bool }

func (a workflowEditTestAccess) ResolveWorkflowActionAccess(_ context.Context, _ edit.WorkflowKey, spec ActionSpec) (WorkflowActionAccess, error) {
	if a.denyHighRisk && spec.Risk == "high" {
		return WorkflowActionAccess{Allowed: false, RequiredPermissions: []string{"workflow.high_risk.use"}, Reason: "review required"}, nil
	}
	return WorkflowActionAccess{Allowed: true}, nil
}

type workflowEditTestReferences struct {
	mu     sync.Mutex
	issues []VisualWorkflowIssue
	calls  int
}

func (r *workflowEditTestReferences) ValidateWorkflowReferences(context.Context, edit.WorkflowKey, visual.Graph) ([]VisualWorkflowIssue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return append([]VisualWorkflowIssue(nil), r.issues...), nil
}

func newWorkflowEditTestApplication(t *testing.T, backend WorkflowEditReadBackend, access WorkflowEditActionAccessResolver, references WorkflowEditReferenceValidator) *WorkflowEditApplication {
	t.Helper()
	app, err := NewWorkflowEditApplication(backend, access, references)
	if err != nil {
		t.Fatal(err)
	}
	return app
}
