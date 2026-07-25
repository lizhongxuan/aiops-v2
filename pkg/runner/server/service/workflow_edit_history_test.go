package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditHistoryUndoRedoRemainDurableAcrossRestart_TXN013_TXN014_TXN016(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	originalCheck := nodePosition(fixture.backend.snapshot.Graph, "check")
	originalSourcePort := workflowEditHistoryEdgeSourcePort(fixture.backend.snapshot.Graph, "check-repair")
	first := fixture.applyOperations(t, "candidate-history-first", "agent-session-1", updatePosition("check", 180, 90))
	changedSourcePort := "next"
	second := fixture.applyOperations(t, "candidate-history-second", "agent-session-1", edit.NewOperation(&edit.UpdateEdgeOperation{EdgeID: "check-repair", Set: edit.EdgePatch{SourcePort: &changedSourcePort}}))

	fixture.restart(t)
	history, err := fixture.app.History(context.Background(), WorkflowEditHistoryRequest{
		SchemaVersion: edit.SchemaVersion,
		WorkflowKey:   fixture.backend.snapshot.Key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Entries) < 2 {
		t.Fatalf("History() entries = %#v", history.Entries)
	}
	assertHistoryEntry := func(transactionID string) WorkflowEditHistoryEntry {
		t.Helper()
		for _, entry := range history.Entries {
			if entry.TransactionID == transactionID {
				if !entry.Reversible || entry.CheckpointID == "" || entry.InverseTransactionID == "" || len(entry.TouchedPaths) == 0 || entry.AuditID == "" || entry.AgentSessionID != "agent-session-1" {
					t.Fatalf("incomplete durable history entry = %#v", entry)
				}
				return entry
			}
		}
		t.Fatalf("transaction %q not found in history %#v", transactionID, history.Entries)
		return WorkflowEditHistoryEntry{}
	}
	assertHistoryEntry(first.TransactionID)
	assertHistoryEntry(second.TransactionID)

	currentGeneration := fixture.currentGeneration(t)
	undo, err := fixture.app.Undo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion:        edit.SchemaVersion,
		WorkflowKey:          fixture.backend.snapshot.Key,
		TargetAgentSessionID: "agent-session-1",
		CandidateID:          "candidate-history-undo",
		Expected:             fixture.backend.snapshot.Revision,
		Actor:                edit.ActorKind("user"),
		ActorID:              "actor-1",
		CanvasSessionID:      "canvas-1",
		AgentSessionID:       "undo-session-1",
		TurnID:               "turn-undo-1",
		ToolCallID:           "tool-undo-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if nodePosition(undo.Graph, "check") != originalCheck || workflowEditHistoryEdgeSourcePort(undo.Graph, "check-repair") != originalSourcePort {
		t.Fatalf("session undo preview did not restore both transactions: check=%#v sourcePort=%q", nodePosition(undo.Graph, "check"), workflowEditHistoryEdgeSourcePort(undo.Graph, "check-repair"))
	}
	if fixture.currentGeneration(t) != currentGeneration {
		t.Fatal("Undo() committed the graph instead of returning a reviewable preview")
	}
	undoReceipt := fixture.applyPreview(t, undo, "apply-history-undo")

	redo, err := fixture.app.Redo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion:       edit.SchemaVersion,
		WorkflowKey:         fixture.backend.snapshot.Key,
		TargetTransactionID: undoReceipt.TransactionID,
		CandidateID:         "candidate-history-redo",
		Expected:            fixture.backend.snapshot.Revision,
		Actor:               edit.ActorKind("user"),
		ActorID:             "actor-1",
		CanvasSessionID:     "canvas-1",
		AgentSessionID:      "redo-session-1",
		TurnID:              "turn-redo-1",
		ToolCallID:          "tool-redo-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if nodePosition(redo.Graph, "check") != (visual.Position{X: 180, Y: 90}) || workflowEditHistoryEdgeSourcePort(redo.Graph, "check-repair") != changedSourcePort {
		t.Fatalf("redo preview did not restore the undone session: check=%#v sourcePort=%q", nodePosition(redo.Graph, "check"), workflowEditHistoryEdgeSourcePort(redo.Graph, "check-repair"))
	}
	if fixture.currentGeneration(t) != currentGeneration+1 {
		t.Fatal("Redo() committed the graph instead of returning a reviewable preview")
	}
}

func TestWorkflowEditUndoRejectsLaterSamePathModification_TXN015(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	first := fixture.applyOperations(t, "candidate-conflict-first", "agent-session-1", updatePosition("check", 180, 90))
	fixture.applyOperations(t, "candidate-conflict-later", "agent-session-2", updatePosition("check", 360, 180))
	generation := fixture.currentGeneration(t)

	_, err := fixture.app.Undo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion:       edit.SchemaVersion,
		WorkflowKey:         fixture.backend.snapshot.Key,
		TargetTransactionID: first.TransactionID,
		CandidateID:         "candidate-conflicting-undo",
		Expected:            fixture.backend.snapshot.Revision,
		Actor:               edit.ActorKind("user"),
		ActorID:             "actor-1",
		CanvasSessionID:     "canvas-1",
	})
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != edit.ErrorStaleRevision || !containsWorkflowPreviewPath(domain.ConflictingPaths, "/nodes/check/position") {
		t.Fatalf("Undo() conflict = %#v, %v", domain, err)
	}
	if fixture.currentGeneration(t) != generation {
		t.Fatal("conflicting undo changed the committed transaction generation")
	}
}

func TestWorkflowEditUndoPreservesLaterDisjointModification_TXN015(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	originalCheck := nodePosition(fixture.backend.snapshot.Graph, "check")
	first := fixture.applyOperations(t, "candidate-disjoint-first", "agent-session-1", updatePosition("check", 180, 90))
	laterRepair := visual.Position{X: 480, Y: 220}
	fixture.applyOperations(t, "candidate-disjoint-later", "agent-session-2", updatePosition("repair", laterRepair.X, laterRepair.Y))

	undo, err := fixture.app.Undo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion:       edit.SchemaVersion,
		WorkflowKey:         fixture.backend.snapshot.Key,
		TargetTransactionID: first.TransactionID,
		CandidateID:         "candidate-disjoint-undo",
		Expected:            fixture.backend.snapshot.Revision,
		Actor:               edit.ActorKind("user"),
		ActorID:             "actor-1",
		CanvasSessionID:     "canvas-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if nodePosition(undo.Graph, "check") != originalCheck || nodePosition(undo.Graph, "repair") != laterRepair {
		t.Fatalf("disjoint undo preview check=%#v repair=%#v", nodePosition(undo.Graph, "check"), nodePosition(undo.Graph, "repair"))
	}
}

type workflowEditHistoryFixture struct {
	backend       *workflowEditTestBackend
	app           *WorkflowEditApplication
	commits       *WorkflowCommitStore
	candidates    *WorkflowCandidateFileStore
	approvals     *WorkflowApprovalFileStore
	commitRoot    string
	candidateRoot string
	approvalRoot  string
	sequence      int
}

func newWorkflowEditHistoryFixture(t *testing.T) *workflowEditHistoryFixture {
	t.Helper()
	canonicalizer := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	compiled, err := canonicalizer.Compile(context.Background(), workflowEditTestGraph())
	if err != nil {
		t.Fatal(err)
	}
	graph, err := parseCommittedVisualGraph([]byte(compiled.YAML), "runner-rv-history-1")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &workflowEditHistoryFixture{
		backend: newWorkflowEditTestBackend(t, graph), commitRoot: t.TempDir(),
		candidateRoot: t.TempDir(), approvalRoot: t.TempDir(),
	}
	fixture.backend.snapshot.Revision.ResourceVersion = "runner-rv-history-1"
	fixture.restart(t)
	seed := workflowCommitInputFixture(fixture.backend.snapshot.Key, "", fixture.backend.snapshot.Revision.ResourceVersion, "transaction-history-seed", "idempotency-history-seed", "digest-history-seed")
	seed.GraphPayload = []byte(compiled.YAML)
	seed.Receipt.RevisionAfter.SemanticHash = fixture.backend.snapshot.Revision.SemanticHash
	seed.Receipt.RevisionAfter.LayoutHash = fixture.backend.snapshot.Revision.LayoutHash
	if _, err := fixture.commits.Commit(context.Background(), seed); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f *workflowEditHistoryFixture) restart(t *testing.T) {
	t.Helper()
	var err error
	f.commits, err = NewWorkflowCommitStore(f.commitRoot)
	if err != nil {
		t.Fatal(err)
	}
	f.candidates = NewWorkflowCandidateFileStore(f.candidateRoot)
	f.approvals = NewWorkflowApprovalFileStore(f.approvalRoot, f.commits)
	f.app, err = NewWorkflowEditMutationApplication(f.backend, workflowEditTestAccess{}, &workflowEditTestReferences{}, f.candidates, f.approvals, f.commits)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *workflowEditHistoryFixture) applyOperations(t *testing.T, candidateID, agentSessionID string, operations ...edit.WorkflowOperation) edit.WorkflowMutationReceipt {
	t.Helper()
	request := newWorkflowPreviewRequest(f.backend.snapshot, candidateID, operations...)
	request.AgentSessionID = agentSessionID
	request.TurnID = "turn-" + candidateID
	request.ToolCallID = "tool-" + candidateID
	preview, err := f.app.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return f.applyPreview(t, preview, "apply-"+candidateID)
}

func (f *workflowEditHistoryFixture) applyPreview(t *testing.T, preview WorkflowEditPreviewResult, identity string) edit.WorkflowMutationReceipt {
	t.Helper()
	f.sequence++
	candidate, err := f.candidates.GetRevision(context.Background(), preview.ChangeSet.Key, preview.Candidate.ID, preview.CandidateRevision)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	f.app.now = func() time.Time { return now }
	planHash := workflowApprovalHash("history-plan-" + identity)
	allowedOps := make([]edit.OperationKind, 0, len(candidate.NormalizedOperations))
	seenOps := map[edit.OperationKind]bool{}
	for _, operation := range candidate.NormalizedOperations {
		if kind := operation.Value.Kind(); !seenOps[kind] {
			seenOps[kind] = true
			allowedOps = append(allowedOps, kind)
		}
	}
	grantID := fmt.Sprintf("grant-history-%d", f.sequence)
	approvalID := fmt.Sprintf("approval-history-%d", f.sequence)
	grant := WorkflowEditGrantRecord{Grant: edit.WorkflowEditGrant{ID: grantID, Key: preview.ChangeSet.Key, ActorID: candidate.ActorID, PlanHash: planHash, BaseSemanticHash: preview.ChangeSet.Base.SemanticHash, AllowedOps: allowedOps, MaxOperations: len(candidate.NormalizedOperations), MaxAddedNodes: len(candidate.NormalizedOperations), AllowDeleteExisting: true, ExpiresAt: now.Add(time.Hour)}, CanvasSessionID: candidate.CanvasSessionID, RiskScope: WorkflowGrantRiskScope{MaxRisk: WorkflowRiskHigh}, State: WorkflowApprovalActive, IssuedAt: now}
	if err := f.approvals.IssueGrant(context.Background(), grant); err != nil {
		t.Fatal(err)
	}
	approval := WorkflowApplyApprovalRecord{Approval: edit.WorkflowApplyApproval{ID: approvalID, ActorID: candidate.ActorID, Key: preview.ChangeSet.Key, CandidateID: candidate.CandidateID, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion, ExpiresAt: now.Add(time.Hour)}, GrantID: grantID, CanvasSessionID: candidate.CanvasSessionID, PlanHash: planHash, State: WorkflowApprovalActive, IssuedAt: now}
	if err := f.approvals.IssueApplyApproval(context.Background(), approval); err != nil {
		t.Fatal(err)
	}
	request := WorkflowEditApplyRequest{ApplyPatchRequest: edit.ApplyPatchRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: candidate.Key, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion, IdempotencyKey: "idempotency-" + identity, CandidateGrantID: grantID, ApplyApprovalID: approvalID, Reason: "apply reviewed history candidate"}, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, ActorID: candidate.ActorID, CanvasSessionID: candidate.CanvasSessionID}
	trusted := WorkflowEditTrustedApplyContext{Key: candidate.Key, ActorID: candidate.ActorID, CanvasSessionID: candidate.CanvasSessionID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion, GrantID: grantID, ApprovalID: approvalID, PlanHash: planHash, BaseSemanticHash: candidate.Base.SemanticHash, ReferenceSnapshotHash: candidate.Validation.Basis.ReferenceSnapshotHash}
	receipt, err := f.app.Apply(context.Background(), trusted, request)
	if err != nil {
		t.Fatal(err)
	}
	f.syncBackend(t)
	return receipt
}

func (f *workflowEditHistoryFixture) syncBackend(t *testing.T) {
	t.Helper()
	snapshot, err := f.commits.Snapshot(context.Background(), f.backend.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	f.backend.snapshot = WorkflowEditSnapshot{Key: snapshot.Key, Revision: *snapshot.Receipt.RevisionAfter, Graph: graph}
}

func (f *workflowEditHistoryFixture) currentGeneration(t *testing.T) uint64 {
	t.Helper()
	snapshot, err := f.commits.Snapshot(context.Background(), f.backend.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Generation
}

func workflowEditHistoryEdgeSourcePort(graph visual.Graph, edgeID string) string {
	for _, edge := range graph.Edges {
		if edge.ID == edgeID {
			return edge.SourcePort
		}
	}
	return ""
}
