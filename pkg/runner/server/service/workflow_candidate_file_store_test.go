package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowCandidateFileStoreRestartRecoveryEligibilityAndAbandon(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "restart-workflow"}
	createdAt := time.Date(2026, 7, 21, 7, 0, 0, 0, time.UTC)
	store := NewWorkflowCandidateFileStore(root)
	v1 := workflowCandidateTestRevision(key, "candidate-restart", 1, 0, "change-1", "preview-1", createdAt)
	v2 := workflowCandidateTestRevision(key, "candidate-restart", 2, 1, "change-2", "preview-2", createdAt.Add(time.Minute))
	v3 := workflowCandidateTestRevision(key, "candidate-restart", 3, 2, "change-3", "preview-3", createdAt.Add(2*time.Minute))
	if _, err := store.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revise(ctx, key, "candidate-restart", 1, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revise(ctx, key, "candidate-restart", 2, v3); err != nil {
		t.Fatal(err)
	}

	restarted := NewWorkflowCandidateFileStore(root)
	eligible, err := restarted.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: "candidate-restart", CandidateRevision: 3, ChangeSetID: "change-3", PreviewHash: "preview-3",
	}, createdAt.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("CheckApplyEligibility() after restart error = %v", err)
	}
	if eligible.CandidateRevision != 3 || eligible.ID != "change-3" {
		t.Fatalf("eligible revision = %+v", eligible)
	}

	_, err = restarted.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: "candidate-restart", CandidateRevision: 1, ChangeSetID: "change-1", PreviewHash: "preview-1",
	}, createdAt.Add(3*time.Minute))
	assertWorkflowCandidateDomainCode(t, err, edit.ErrorStaleChangeSet)
	_, err = restarted.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: "candidate-restart", CandidateRevision: 3, ChangeSetID: "change-3", PreviewHash: "preview-2",
	}, createdAt.Add(3*time.Minute))
	assertWorkflowCandidateDomainCode(t, err, edit.ErrorPreviewMismatch)

	if _, err := restarted.Abandon(ctx, key, "candidate-restart", 3, createdAt.Add(4*time.Minute)); err != nil {
		t.Fatalf("Abandon() error = %v", err)
	}
	afterAbandon := NewWorkflowCandidateFileStore(root)
	current, err := afterAbandon.GetCurrent(ctx, key, "candidate-restart")
	if err != nil {
		t.Fatal(err)
	}
	if current.State != WorkflowCandidateStateAbandoned || current.CurrentRevision != 3 {
		t.Fatalf("candidate after restart = %+v", current)
	}
	_, err = afterAbandon.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: "candidate-restart", CandidateRevision: 3, ChangeSetID: "change-3", PreviewHash: "preview-3",
	}, createdAt.Add(5*time.Minute))
	assertWorkflowCandidateDomainCode(t, err, edit.ErrorStaleChangeSet)
	revisions, err := afterAbandon.ListRevisions(ctx, key, "candidate-restart")
	if err != nil || len(revisions) != 3 {
		t.Fatalf("abandon deleted immutable history: revisions=%d err=%v", len(revisions), err)
	}

	manifestMatches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*", "current.json"))
	revisionMatches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*", "revisions", "*.json"))
	if len(manifestMatches) != 1 || len(revisionMatches) != 3 {
		t.Fatalf("persistent layout manifests=%v revisions=%v", manifestMatches, revisionMatches)
	}
	rawManifest, err := os.ReadFile(manifestMatches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawManifest), `"formatVersion": 1`) || !strings.Contains(string(rawManifest), `"1": 2`) || !strings.Contains(string(rawManifest), `"2": 3`) {
		t.Fatalf("manifest lacks version/supersession metadata: %s", rawManifest)
	}
}

func TestWorkflowCandidateFileStoreRejectsExpiredCurrentRevisionWithoutDeletingIt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant", WorkflowID: "expired-workflow"}
	createdAt := time.Date(2026, 7, 21, 1, 0, 0, 0, time.UTC)
	store := NewWorkflowCandidateFileStore(root)
	revision := workflowCandidateTestRevision(key, "candidate-expired", 1, 0, "change-expired", "preview-expired", createdAt)
	if _, err := store.Create(ctx, revision); err != nil {
		t.Fatal(err)
	}

	_, err := store.CheckApplyEligibility(ctx, WorkflowCandidateApplyBinding{
		Key: key, CandidateID: "candidate-expired", CandidateRevision: 1, ChangeSetID: "change-expired", PreviewHash: "preview-expired",
	}, createdAt.Add(2*time.Hour))
	assertWorkflowCandidateDomainCode(t, err, edit.ErrorStaleChangeSet)
	if _, err := store.GetRevision(ctx, key, "candidate-expired", 1); err != nil {
		t.Fatalf("TTL cleanup deleted immutable audit revision: %v", err)
	}
}

func TestWorkflowCandidateTransactionRecoversManifestAndEventProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-recovery", WorkflowID: "workflow-candidate"}
	createdAt := time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)
	store := NewWorkflowCandidateFileStore(root)
	v1 := workflowCandidateTestRevision(key, "candidate-atomic", 1, 0, "change-1", "preview-1", createdAt)
	v2 := workflowCandidateTestRevision(key, "candidate-atomic", 2, 1, "change-2", "preview-2", createdAt.Add(time.Minute))
	if _, err := store.Create(ctx, v1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revise(ctx, key, "candidate-atomic", 1, v2); err != nil {
		t.Fatal(err)
	}

	log, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := log.Replay(ctx, key, 0, 10)
	if err != nil || len(replay.Events) != 2 {
		t.Fatalf("candidate event projection = %#v, err=%v", replay, err)
	}
	for index, event := range replay.Events {
		if event.Sequence != uint64(index+1) || event.CandidateRevision != uint64(index+1) || event.ChangeSetID != []string{"change-1", "change-2"}[index] {
			t.Fatalf("candidate event %d = %#v", index, event)
		}
	}

	dir, err := store.candidateDir(key, "candidate-atomic")
	if err != nil {
		t.Fatal(err)
	}
	fact, err := readWorkflowCandidateTransaction(workflowCandidateRevisionPath(dir, 2))
	if err != nil || fact.Revision.ID != "change-2" || fact.Event.ChangeSetID != "change-2" || fact.Event.TransactionID == "" {
		t.Fatalf("atomic candidate fact = %#v, err=%v", fact, err)
	}
	if err := os.Remove(filepath.Join(dir, "current.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workflowStatePath(root, "events", key)); err != nil {
		t.Fatal(err)
	}

	restartedLog, err := NewFileWorkflowEventLog(root)
	if err != nil {
		t.Fatal(err)
	}
	recoveredEvents, err := restartedLog.Replay(ctx, key, 0, 10)
	if err != nil || len(recoveredEvents.Events) != 2 || recoveredEvents.Events[1].Sequence != 2 {
		t.Fatalf("recovered candidate events = %#v, err=%v", recoveredEvents, err)
	}
	restartedStore := NewWorkflowCandidateFileStore(root)
	current, err := restartedStore.GetCurrent(ctx, key, "candidate-atomic")
	if err != nil || current.CurrentRevision != 2 || current.CurrentChangeSetID != "change-2" {
		t.Fatalf("recovered candidate manifest = %#v, err=%v", current, err)
	}
	if _, err := restartedStore.Revise(ctx, key, "candidate-atomic", 1, v2); err != nil {
		t.Fatalf("idempotent candidate retry error = %v", err)
	}
	afterRetry, err := restartedLog.Replay(ctx, key, 0, 10)
	if err != nil || len(afterRetry.Events) != 2 {
		t.Fatalf("idempotent event replay = %#v, err=%v", afterRetry, err)
	}
}

func TestWorkflowCandidateResolveChangeSetIsKeyBoundDurableAndUnique(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := edit.WorkflowKey{TenantID: "tenant-resolve", WorkflowID: "workflow-resolve"}
	otherTenant := edit.WorkflowKey{TenantID: "tenant-other", WorkflowID: key.WorkflowID}
	createdAt := time.Date(2026, 7, 21, 8, 30, 0, 0, time.UTC)
	store := NewWorkflowCandidateFileStore(root)
	revision := workflowCandidateTestRevision(key, "candidate-a", 1, 0, "changeset-canonical", "preview-a", createdAt)
	if _, err := store.Create(ctx, revision); err != nil {
		t.Fatal(err)
	}
	resolved, err := NewWorkflowCandidateFileStore(root).ResolveChangeSet(ctx, key, revision.ID)
	if err != nil || resolved.CandidateID != "candidate-a" || resolved.Key != key {
		t.Fatalf("ResolveChangeSet() = %#v, %v", resolved, err)
	}
	if _, err := store.ResolveChangeSet(ctx, otherTenant, revision.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant ResolveChangeSet() error = %v", err)
	}
	duplicate := workflowCandidateTestRevision(key, "candidate-b", 1, 0, revision.ID, "preview-b", createdAt.Add(time.Minute))
	if _, err := store.Create(ctx, duplicate); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveChangeSet(ctx, key, revision.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate ResolveChangeSet() error = %v", err)
	}
}

func assertWorkflowCandidateDomainCode(t *testing.T, err error, code edit.ErrorCode) {
	t.Helper()
	var domainErr *edit.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != code {
		t.Fatalf("error = %T %v, want %s", err, err, code)
	}
}
