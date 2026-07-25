package service

import (
	"context"
	"testing"

	"runner/workflow/edit"
)

func TestWorkflowResolveCandidateScopeReturnsCurrentImmutableOperations(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowPreviewTestApplication(t, backend, NewWorkflowCandidateFileStore(t.TempDir()))
	request := newWorkflowPreviewRequest(backend.snapshot, "candidate-scope", updatePosition("check", 120, 80))
	preview, err := app.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	scope, err := app.ResolveCandidateScope(context.Background(), request.WorkflowKey, preview.ChangeSet.ID)
	if err != nil {
		t.Fatalf("ResolveCandidateScope() error = %v", err)
	}
	if scope.Key != request.WorkflowKey || scope.ChangeSetID != preview.ChangeSet.ID || scope.CandidateID != request.CandidateID || scope.CandidateRevision != 1 || scope.Base != preview.ChangeSet.Base || len(scope.NormalizedOperations) != len(preview.ChangeSet.Operations) {
		t.Fatalf("candidate scope = %#v", scope)
	}
}

func TestWorkflowResolveCandidateScopeRejectsSupersededRevision(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowPreviewTestApplication(t, backend, NewWorkflowCandidateFileStore(t.TempDir()))
	request := newWorkflowPreviewRequest(backend.snapshot, "candidate-scope-old", updatePosition("check", 120, 80))
	first, err := app.Preview(context.Background(), request)
	if err != nil {
		t.Fatalf("Preview(v1) error = %v", err)
	}
	request = newWorkflowPreviewRequest(backend.snapshot, "candidate-scope-old")
	request.BaseChangeSetID = first.ChangeSet.ID
	request.AdditionalOperations = []edit.WorkflowOperation{updatePosition("repair", 220, 80)}
	if _, err := app.Preview(context.Background(), request); err != nil {
		t.Fatalf("Preview(v2) error = %v", err)
	}
	if _, err := app.ResolveCandidateScope(context.Background(), request.WorkflowKey, first.ChangeSet.ID); err == nil {
		t.Fatal("ResolveCandidateScope(v1) error = nil, want superseded rejection")
	}
}
