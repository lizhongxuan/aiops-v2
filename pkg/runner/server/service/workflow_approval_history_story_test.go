package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

// This story exercises the same service entry points used by the HTTP/MCP
// adapters. Undo and Redo remain reviewable Candidates; only the exact
// persisted approval can commit them.
func TestWorkflowApprovalHistoryStoryHighRiskDeleteReplayUndoRedoAndDiscard_SEC009_SEC015_TXN013_TXN014(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	source, sourcePort := "check", "next"
	target, targetPort := "end", "in"
	edgeKind := visual.EdgeKindSuccess
	preview, err := fixture.app.Preview(ctx, newWorkflowPreviewRequest(
		fixture.backend.snapshot,
		"candidate-delete-story",
		edit.NewOperation(&edit.DeleteNodeOperation{NodeID: "repair", CascadeEdges: true}),
		edit.NewOperation(&edit.AddEdgeOperation{ClientRef: "check-end", Edge: edit.EditableEdge{EdgePatch: edit.EdgePatch{
			Source: &source, SourcePort: &sourcePort, Target: &target, TargetPort: &targetPort, Kind: &edgeKind,
		}}}),
	))
	if err != nil || !preview.Risk.RequiresApproval || !preview.Risk.DeletesExisting || preview.Risk.Level != WorkflowRiskHigh {
		t.Fatalf("high-risk delete preview = %#v, %v", preview.Risk, err)
	}

	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	fixture.app.now = func() time.Time { return now }
	underScoped := issueWorkflowStoryApproval(t, fixture, preview, "grant-delete-low", "approval-delete-low", false, WorkflowRiskMedium, now)
	if _, err := fixture.app.ResolveTrustedApplyContext(ctx, underScoped); workflowCommitErrorCode(err) != edit.ErrorApprovalScopeExceeded {
		t.Fatalf("under-scoped delete approval error = %v", err)
	}
	before, err := fixture.commits.Snapshot(ctx, fixture.backend.snapshot.Key)
	if err != nil || before.Generation != 1 || len(before.ApprovalConsumptions) != 0 {
		t.Fatalf("rejected delete changed committed state = %#v, %v", before, err)
	}
	lowApproval, err := fixture.approvals.GetApplyApproval(ctx, preview.ChangeSet.Key, underScoped.ApprovalID)
	if err != nil || lowApproval.State != WorkflowApprovalActive {
		t.Fatalf("rejected delete consumed approval = %#v, %v", lowApproval, err)
	}

	exact := issueWorkflowStoryApproval(t, fixture, preview, "grant-delete-exact", "approval-delete-exact", true, WorkflowRiskHigh, now)
	trusted, err := fixture.app.ResolveTrustedApplyContext(ctx, exact)
	if err != nil {
		t.Fatalf("resolve exact delete approval: %v", err)
	}
	request := workflowStoryApplyRequest(trusted, "apply-delete-story")
	deleteReceipt, err := fixture.app.Apply(ctx, trusted, request)
	if err != nil || deleteReceipt.RevisionAfter == nil {
		t.Fatalf("apply exact delete = %#v, %v", deleteReceipt, err)
	}
	fixture.syncBackend(t)
	if visualNodeByID(fixture.backend.snapshot.Graph, "repair") != nil || visualEdgeByID(fixture.backend.snapshot.Graph, "check-end") == nil {
		t.Fatalf("approved delete graph = %#v", fixture.backend.snapshot.Graph)
	}
	exactApproval, err := fixture.approvals.GetApplyApproval(ctx, preview.ChangeSet.Key, exact.ApprovalID)
	if err != nil || exactApproval.State != WorkflowApprovalConsumed || exactApproval.ConsumptionID != deleteReceipt.TransactionID {
		t.Fatalf("exact delete approval = %#v, %v", exactApproval, err)
	}

	fixture.restart(t)
	restartedTrusted, err := fixture.app.ResolveTrustedApplyContext(ctx, exact)
	if err != nil {
		t.Fatalf("resolve consumed approval for exact replay: %v", err)
	}
	replayed, err := fixture.app.Apply(ctx, restartedTrusted, request)
	if err != nil || replayed.TransactionID != deleteReceipt.TransactionID {
		t.Fatalf("restart-safe exact apply replay = %#v, %v", replayed, err)
	}
	afterReplay, err := fixture.commits.Snapshot(ctx, preview.ChangeSet.Key)
	if err != nil || afterReplay.Generation != 2 || len(afterReplay.ApprovalConsumptions) != 1 {
		t.Fatalf("apply replay created a second commit = %#v, %v", afterReplay, err)
	}

	undo := workflowStoryHistoryPreview(t, fixture, WorkflowHistoryActionUndo, deleteReceipt.TransactionID, "candidate-delete-undo", "undo-delete-session")
	if visualNodeByID(undo.Graph, "repair") == nil || visualEdgeByID(undo.Graph, "check-end") != nil {
		t.Fatalf("undo preview did not restore the deleted branch = %#v", undo.Graph)
	}
	undoReceipt := fixture.applyPreview(t, undo, "apply-delete-undo")
	fixture.restart(t)

	redo := workflowStoryHistoryPreview(t, fixture, WorkflowHistoryActionRedo, undoReceipt.TransactionID, "candidate-delete-redo", "redo-delete-session")
	redoReceipt := fixture.applyPreview(t, redo, "apply-delete-redo")
	fixture.restart(t)
	if visualNodeByID(fixture.backend.snapshot.Graph, "repair") != nil || visualEdgeByID(fixture.backend.snapshot.Graph, "check-end") == nil {
		t.Fatalf("approved redo was not durable = %#v", fixture.backend.snapshot.Graph)
	}
	history, err := fixture.app.History(ctx, WorkflowEditHistoryRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.backend.snapshot.Key})
	if err != nil {
		t.Fatal(err)
	}
	redoFound := false
	for _, entry := range history.Entries {
		if entry.TransactionID == redoReceipt.TransactionID {
			redoFound = entry.ActionKind == WorkflowHistoryActionRedo && len(entry.TargetTransactionIDs) == 1 && entry.TargetTransactionIDs[0] == undoReceipt.TransactionID
		}
	}
	if !redoFound {
		t.Fatalf("redo lineage missing from durable history = %#v", history.Entries)
	}

	discardPreview := workflowStoryHistoryPreview(t, fixture, WorkflowHistoryActionUndo, redoReceipt.TransactionID, "candidate-delete-discard", "discard-delete-session")
	generation := fixture.currentGeneration(t)
	discard := WorkflowCandidateAbandonRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: discardPreview.ChangeSet.Key,
		CandidateID: discardPreview.Candidate.ID, CandidateRevision: discardPreview.CandidateRevision,
		ChangeSetID: discardPreview.ChangeSet.ID, PreviewHash: discardPreview.PreviewHash,
		Expected: discardPreview.ChangeSet.Base, IdempotencyKey: "discard-delete-undo",
		ActorID: "actor-1", CanvasSessionID: "canvas-1", SessionID: "discard-delete-session",
	}
	firstDiscard, err := fixture.app.AbandonCandidate(ctx, discard)
	if err != nil || firstDiscard.Candidate.State != WorkflowCandidateStateAbandoned {
		t.Fatalf("discard undo candidate = %#v, %v", firstDiscard, err)
	}
	replayedDiscard, err := fixture.app.AbandonCandidate(ctx, discard)
	if err != nil || replayedDiscard.Candidate != firstDiscard.Candidate || fixture.currentGeneration(t) != generation {
		t.Fatalf("discard replay changed committed graph = %#v, %v", replayedDiscard, err)
	}
}

func TestWorkflowHistorySameFieldConflictReturnsThreeWayRecoveryFacts_TXN015(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	first := fixture.applyOperations(t, "candidate-three-way-first", "session-a", updatePosition("check", 180, 90))
	fixture.applyOperations(t, "candidate-three-way-later", "session-b", updatePosition("check", 360, 180))

	_, err := fixture.app.Undo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.backend.snapshot.Key,
		TargetTransactionID: first.TransactionID, CandidateID: "candidate-three-way-undo",
		Expected: fixture.backend.snapshot.Revision, Actor: edit.ActorKind("user"),
		ActorID: "actor-1", CanvasSessionID: "canvas-1",
	})
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != edit.ErrorStaleRevision || domain.CurrentRevision != fixture.backend.snapshot.Revision.ResourceVersion ||
		domain.BaseRevision != first.RevisionAfter.ResourceVersion || domain.RetryHint == "" || !containsWorkflowPreviewPath(domain.ConflictingPaths, "/nodes/check/position") {
		t.Fatalf("three-way conflict facts = %#v, %v", domain, err)
	}
	var conflict edit.WorkflowConflict
	if json.Unmarshal(domain.Details, &conflict) != nil || conflict.Base.ResourceVersion != domain.BaseRevision || conflict.Current.ResourceVersion != domain.CurrentRevision ||
		!containsWorkflowPreviewPath(conflict.ConflictingPaths, "/nodes/check/position") {
		t.Fatalf("three-way conflict details = %#v raw=%s", conflict, domain.Details)
	}
}

func issueWorkflowStoryApproval(t *testing.T, fixture *workflowEditHistoryFixture, preview WorkflowEditPreviewResult, grantID, approvalID string, allowDelete bool, maxRisk WorkflowRiskLevel, now time.Time) WorkflowTrustedApplySelector {
	t.Helper()
	candidate, err := fixture.candidates.GetRevision(context.Background(), preview.ChangeSet.Key, preview.Candidate.ID, preview.CandidateRevision)
	if err != nil {
		t.Fatal(err)
	}
	planHash := workflowApprovalHash(grantID)
	grant := WorkflowEditGrantRecord{Grant: edit.WorkflowEditGrant{
		ID: grantID, Key: candidate.Key, ActorID: candidate.ActorID, PlanHash: planHash,
		BaseSemanticHash: candidate.Base.SemanticHash, AllowedOps: workflowManualAllowedOperations(candidate.NormalizedOperations),
		MaxOperations: len(candidate.NormalizedOperations), MaxAddedNodes: preview.Risk.AddedNodeCount,
		AllowDeleteExisting: allowDelete, ExpiresAt: now.Add(time.Hour),
	}, CanvasSessionID: candidate.CanvasSessionID, RiskScope: WorkflowGrantRiskScope{MaxRisk: maxRisk}, State: WorkflowApprovalActive, IssuedAt: now}
	if err := fixture.approvals.IssueGrant(context.Background(), grant); err != nil {
		t.Fatal(err)
	}
	approval := WorkflowApplyApprovalRecord{Approval: edit.WorkflowApplyApproval{
		ID: approvalID, ActorID: candidate.ActorID, Key: candidate.Key, CandidateID: candidate.CandidateID,
		ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion,
		ExpiresAt: now.Add(time.Hour),
	}, GrantID: grantID, CanvasSessionID: candidate.CanvasSessionID, PlanHash: planHash, State: WorkflowApprovalActive, IssuedAt: now}
	if err := fixture.approvals.IssueApplyApproval(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	return WorkflowTrustedApplySelector{Key: candidate.Key, ActorID: candidate.ActorID, CanvasSessionID: candidate.CanvasSessionID, ChangeSetID: candidate.ID, ApprovalID: approvalID}
}

func workflowStoryApplyRequest(trusted WorkflowEditTrustedApplyContext, idempotencyKey string) WorkflowEditApplyRequest {
	return WorkflowEditApplyRequest{ApplyPatchRequest: edit.ApplyPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: trusted.Key, ChangeSetID: trusted.ChangeSetID,
		PreviewHash: trusted.PreviewHash, ExpectedRevision: trusted.ExpectedRevision, IdempotencyKey: idempotencyKey,
		CandidateGrantID: trusted.GrantID, ApplyApprovalID: trusted.ApprovalID, Reason: "apply reviewed workflow story candidate",
	}, CandidateID: trusted.CandidateID, CandidateRevision: trusted.CandidateRevision, ActorID: trusted.ActorID, CanvasSessionID: trusted.CanvasSessionID}
}

func workflowStoryHistoryPreview(t *testing.T, fixture *workflowEditHistoryFixture, action WorkflowHistoryActionKind, transactionID, candidateID, sessionID string) WorkflowEditPreviewResult {
	t.Helper()
	request := WorkflowEditHistoryActionRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.backend.snapshot.Key,
		TargetTransactionID: transactionID, CandidateID: candidateID, Expected: fixture.backend.snapshot.Revision,
		Actor: edit.ActorKind("user"), ActorID: "actor-1", CanvasSessionID: "canvas-1", AgentSessionID: sessionID,
	}
	var result WorkflowEditPreviewResult
	var err error
	if action == WorkflowHistoryActionUndo {
		result, err = fixture.app.Undo(context.Background(), request)
	} else {
		result, err = fixture.app.Redo(context.Background(), request)
	}
	if err != nil {
		t.Fatal(err)
	}
	return result
}
