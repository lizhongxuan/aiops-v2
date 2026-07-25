package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowEditApplicationCandidateQueriesFailClosedWithoutStore(t *testing.T) {
	application := &WorkflowEditApplication{}
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	if _, err := application.GetCurrent(context.Background(), key, "candidate-a"); !errors.Is(err, ErrWorkflowEditConfiguration) {
		t.Fatalf("GetCurrent error = %v", err)
	}
	if _, err := application.GetRevision(context.Background(), key, "candidate-a", 1); !errors.Is(err, ErrWorkflowEditConfiguration) {
		t.Fatalf("GetRevision error = %v", err)
	}
	if _, err := application.CheckApplyEligibility(context.Background(), WorkflowCandidateApplyBinding{Key: key}, time.Now()); !errors.Is(err, ErrWorkflowEditConfiguration) {
		t.Fatalf("CheckApplyEligibility error = %v", err)
	}
}

func TestWorkflowEditApplicationCandidateQueriesDelegateToOwnedStore(t *testing.T) {
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	revision := workflowCandidateTestRevision(key, "candidate-a", 1, 0, "change-a", "preview-a", time.Now().UTC())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	if _, err := store.Create(context.Background(), revision); err != nil {
		t.Fatal(err)
	}
	application := &WorkflowEditApplication{candidates: store}

	current, err := application.GetCurrent(context.Background(), key, revision.CandidateID)
	if err != nil || current.CurrentChangeSetID != revision.ID {
		t.Fatalf("GetCurrent = %#v, %v", current, err)
	}
	got, err := application.GetRevision(context.Background(), key, revision.CandidateID, revision.CandidateRevision)
	if err != nil || got.ID != revision.ID {
		t.Fatalf("GetRevision = %#v, %v", got, err)
	}
	eligible, err := application.CheckApplyEligibility(context.Background(), WorkflowCandidateApplyBinding{
		Key: key, CandidateID: revision.CandidateID, CandidateRevision: revision.CandidateRevision,
		ChangeSetID: revision.ID, PreviewHash: revision.PreviewHash,
	}, revision.CreatedAt)
	if err != nil || eligible.ID != revision.ID {
		t.Fatalf("CheckApplyEligibility = %#v, %v", eligible, err)
	}
}
