package workfloweditor

import (
	"context"
	"testing"

	runnerapp "runner/server/app"
	"runner/server/service"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

func TestRunnerAdapterUsesCanonicalWorkflowKey(t *testing.T) {
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	fake := &fakeRunnerWorkflowFacade{
		snapshot: runnerapp.WorkflowGraphSnapshot{Key: key, Revision: edit.Revision{ResourceVersion: "r17", SemanticHash: "semantic", LayoutHash: "layout"}},
	}
	adapter, err := NewRunnerAdapter(fake)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.GetGraph(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Key != key || fake.lastKey != key {
		t.Fatalf("adapter lost canonical key: snapshot=%+v delegated=%+v", snapshot.Key, fake.lastKey)
	}
	if _, err := adapter.GetHashes(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ListActions(context.Background(), key, service.ActionCatalogFilter{}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Validate(context.Background(), key, visual.Graph{}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Compile(context.Background(), key, visual.Graph{}); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerAdapterRejectsMissingFacade(t *testing.T) {
	if _, err := NewRunnerAdapter(nil); err == nil {
		t.Fatal("NewRunnerAdapter accepted nil facade")
	}
}

func TestRunnerAdapterRejectsInvalidKeyBeforeDelegation(t *testing.T) {
	fake := &fakeRunnerWorkflowFacade{}
	adapter, err := NewRunnerAdapter(fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.GetGraph(context.Background(), edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "../escape"}); err == nil {
		t.Fatal("adapter accepted invalid workflow key")
	}
	if fake.lastKey != (edit.WorkflowKey{}) {
		t.Fatalf("adapter delegated an invalid key: %+v", fake.lastKey)
	}
}

type fakeRunnerWorkflowFacade struct {
	snapshot runnerapp.WorkflowGraphSnapshot
	lastKey  edit.WorkflowKey
}

func (f *fakeRunnerWorkflowFacade) GetGraph(_ context.Context, key edit.WorkflowKey) (runnerapp.WorkflowGraphSnapshot, error) {
	f.lastKey = key
	return f.snapshot, nil
}

func (f *fakeRunnerWorkflowFacade) GetHashes(_ context.Context, key edit.WorkflowKey) (edit.Revision, error) {
	f.lastKey = key
	return f.snapshot.Revision, nil
}

func (f *fakeRunnerWorkflowFacade) ListActions(_ context.Context, key edit.WorkflowKey, _ service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	f.lastKey = key
	return []service.ActionSpec{{Action: "fixture.action"}}, nil
}

func (f *fakeRunnerWorkflowFacade) Validate(_ context.Context, key edit.WorkflowKey, _ visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	f.lastKey = key
	return &service.VisualWorkflowValidationResult{Valid: true}, nil
}

func (f *fakeRunnerWorkflowFacade) Compile(_ context.Context, key edit.WorkflowKey, _ visual.Graph) (*service.CompiledVisualWorkflow, error) {
	f.lastKey = key
	return &service.CompiledVisualWorkflow{}, nil
}
