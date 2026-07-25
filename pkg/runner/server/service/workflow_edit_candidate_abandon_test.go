package service

import (
	"context"
	"errors"
	"testing"

	"runner/workflow/edit"
)

func TestWorkflowCandidateAbandonIsScopedIdempotentAndDoesNotCommit(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)
	beforeRevision := backend.snapshot.Revision
	previewRequest := newWorkflowPreviewRequest(backend.snapshot, "candidate-discard", updatePosition("check", 240, 120))
	previewRequest.Actor = edit.ActorKind("assistant")
	previewRequest.AgentSessionID = "session-1"
	preview, err := app.Preview(context.Background(), previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	request := WorkflowCandidateAbandonRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: backend.snapshot.Key, CandidateID: preview.Candidate.ID,
		CandidateRevision: preview.CandidateRevision, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash,
		Expected: preview.ChangeSet.Base, IdempotencyKey: "discard-once", ActorID: previewRequest.ActorID,
		CanvasSessionID: previewRequest.CanvasSessionID, SessionID: previewRequest.AgentSessionID,
	}
	first, err := app.AbandonCandidate(context.Background(), request)
	if err != nil || first.Candidate.State != WorkflowCandidateStateAbandoned {
		t.Fatalf("first abandon = %#v err=%v", first, err)
	}
	replayed, err := app.AbandonCandidate(context.Background(), request)
	if err != nil || replayed.Candidate != first.Candidate {
		t.Fatalf("replayed abandon = %#v err=%v", replayed, err)
	}
	if backend.snapshot.Revision != beforeRevision {
		t.Fatalf("discard changed committed revision: before=%#v after=%#v", beforeRevision, backend.snapshot.Revision)
	}
	if _, err := store.CheckApplyEligibility(context.Background(), WorkflowCandidateApplyBinding{
		Key: request.WorkflowKey, CandidateID: request.CandidateID, CandidateRevision: request.CandidateRevision,
		ChangeSetID: request.ChangeSetID, PreviewHash: request.PreviewHash,
	}, app.now()); err == nil {
		t.Fatal("abandoned candidate remained apply eligible")
	}

	wrongScope := request
	wrongScope.ActorID = "another-user"
	_, err = app.AbandonCandidate(context.Background(), wrongScope)
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != edit.ErrorApprovalRequired {
		t.Fatalf("cross-scope abandon error = %v", err)
	}
}
