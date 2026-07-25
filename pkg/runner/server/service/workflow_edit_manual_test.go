package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"runner/workflow"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditManualBatchAtomicallyInsertsConnectedNode(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	nodeType := visual.NodeTypeAction
	position := visual.Position{X: 520, Y: 180}
	repair, ping, end := "repair", "ping", "end"
	stepName := "ping"
	label := "ping"
	edgeKind := visual.EdgeKindSuccess
	ports := []visual.Port{{ID: "in", Type: "input"}, {ID: "next", Type: "output"}}
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-insert-ping-atomically",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session",
		Reason: "insert ping before end", Confirmed: true,
		Commands: []WorkflowManualCommand{
			{Kind: WorkflowManualDisconnectEdge, EdgeID: "repair-end"},
			{Kind: WorkflowManualAddNode, Node: &edit.EditableNode{ID: ping, NodePatch: edit.NodePatch{
				Type: &nodeType, Position: &position, Label: &label, Ports: &ports, Step: &workflow.Step{
					Name: stepName, Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo ping"},
				},
			}}},
			{Kind: WorkflowManualConnectEdge, Edge: &edit.EditableEdge{ID: "repair-ping", EdgePatch: edit.EdgePatch{Source: &repair, Target: &ping, Kind: &edgeKind}}},
			{Kind: WorkflowManualConnectEdge, Edge: &edit.EditableEdge{ID: "ping-end", EdgePatch: edit.EdgePatch{Source: &ping, Target: &end, Kind: &edgeKind}}},
		},
	}

	unconfirmed := request
	unconfirmed.IdempotencyKey, unconfirmed.Confirmed = "manual-insert-ping-unconfirmed", false
	_, err := fixture.app.ExecuteManualCommand(context.Background(), unconfirmed)
	var approval *edit.DomainError
	if !errors.As(err, &approval) || approval.Code != edit.ErrorApprovalRequired {
		t.Fatalf("unconfirmed batch error = %v", err)
	}
	beforeApply, snapshotErr := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if snapshotErr != nil || beforeApply.Generation != 1 {
		t.Fatalf("unconfirmed batch changed committed graph = %#v, %v", beforeApply, snapshotErr)
	}

	result, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil || result.Receipt.RevisionAfter == nil || result.Preview.Risk.OperationCount != 4 {
		t.Fatalf("ExecuteManualCommand(batch) = %#v, %v", result, err)
	}
	if result.Preview.ChangeSet.Intent != "manual canvas command batch" || strings.Contains(result.Preview.ChangeSet.Intent, request.Reason) {
		t.Fatalf("batch audit intent leaked request content = %q", result.Preview.ChangeSet.Intent)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if err != nil || snapshot.Generation != 2 {
		t.Fatalf("batch snapshot = %#v, %v", snapshot, err)
	}
	graph, err := parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
	if err != nil || visualNodeByID(graph, ping) == nil || visualEdgeByID(graph, "repair-end") != nil || visualEdgeByID(graph, "repair-ping") == nil || visualEdgeByID(graph, "ping-end") == nil {
		t.Fatalf("batch graph = %#v, %v", graph, err)
	}
	validation, err := fixture.app.Validate(context.Background(), WorkflowEditValidateRequest{Key: request.WorkflowKey, Graph: graph})
	if err != nil || !validation.Validation.Valid || !validation.Compiled {
		t.Fatalf("batch validation = %#v, %v", validation, err)
	}
	replayed, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil || replayed.Receipt.TransactionID != result.Receipt.TransactionID {
		t.Fatalf("batch replay = %#v, %v", replayed, err)
	}
	afterReplay, err := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if err != nil || afterReplay.Generation != 2 || len(afterReplay.ApprovalConsumptions) != 1 {
		t.Fatalf("batch replay mutated snapshot = %#v, %v", afterReplay, err)
	}
}

func TestWorkflowEditManualStandaloneOrphanAddRemainsFailClosed(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	nodeType := visual.NodeTypeAction
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-orphan-add-rejected",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session", Reason: "add orphan",
		Command: WorkflowManualCommand{Kind: WorkflowManualAddNode, Node: &edit.EditableNode{ID: "orphan", NodePatch: edit.NodePatch{
			Type: &nodeType, Step: &workflow.Step{Name: "orphan", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo orphan"}},
		}}},
	}

	_, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != edit.ErrorInvalidPatch {
		t.Fatalf("orphan add error = %v", err)
	}
	snapshot, snapshotErr := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if snapshotErr != nil || snapshot.Generation != 1 {
		t.Fatalf("orphan add changed committed graph = %#v, %v", snapshot, snapshotErr)
	}
}

func TestWorkflowEditManualCommandRequestRejectsInvalidCommandSelection(t *testing.T) {
	valid := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: workflowEditTestKey(), ExpectedRevision: "rv-1",
		IdempotencyKey: "manual-command-selection", ActorID: "manual-user", CanvasSessionID: "manual-canvas",
		SessionID: "manual-session", Reason: "test strict command selection",
	}
	move := WorkflowManualCommand{Kind: WorkflowManualMove, Layout: &edit.LayoutPatch{Nodes: []edit.LayoutNodePatch{{NodeID: "check", Position: visual.Position{X: 20, Y: 40}}}}}
	tests := []struct {
		name    string
		mutate  func(*WorkflowManualCommandRequest)
		execute bool
	}{
		{name: "neither"},
		{name: "both", mutate: func(request *WorkflowManualCommandRequest) {
			request.Command, request.Commands = move, []WorkflowManualCommand{move}
		}},
		{name: "too many", mutate: func(request *WorkflowManualCommandRequest) {
			request.Commands = make([]WorkflowManualCommand, workflowManualCommandBatchLimit+1)
			for index := range request.Commands {
				request.Commands[index] = move
			}
		}},
		{name: "invalid batch command", mutate: func(request *WorkflowManualCommandRequest) {
			request.Commands = []WorkflowManualCommand{{Kind: WorkflowManualConnectEdge}}
		}, execute: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			if test.mutate != nil {
				test.mutate(&request)
			}
			var err error
			if test.execute {
				fixture := newWorkflowEditApplyFixture(t)
				request.WorkflowKey, request.ExpectedRevision = fixture.request.WorkflowKey, fixture.request.ExpectedRevision
				_, err = fixture.app.ExecuteManualCommand(context.Background(), request)
			} else {
				err = validateWorkflowManualRequest(request)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestWorkflowEditManualMoveUsesPreviewApprovalApplyAndIdempotentReplay(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	position := visual.Position{X: 320, Y: 180}
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-move-once",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session", Reason: "move the check node",
		Command: WorkflowManualCommand{Kind: WorkflowManualMove, Layout: &edit.LayoutPatch{
			Nodes: []edit.LayoutNodePatch{{NodeID: "check", Position: position}},
		}},
	}

	result, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil || result.Receipt.Status != "verifying" || result.Receipt.RevisionAfter == nil {
		t.Fatalf("ExecuteManualCommand() = %#v, %v", result, err)
	}
	if result.Preview.Risk.Level != WorkflowRiskLow || result.Preview.ChangeSet.Actor != edit.ActorKind("user") {
		t.Fatalf("manual preview = %#v", result.Preview)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if err != nil || snapshot.Generation != 2 || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("manual snapshot = %#v, %v", snapshot, err)
	}
	committed, err := parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
	if err != nil || nodePosition(committed, "check") != position {
		t.Fatalf("manual position = %#v, %v", nodePosition(committed, "check"), err)
	}

	// A committed command remains replayable after its Candidate TTL. The
	// persisted receipt wins before renewal, so a retry cannot create a second
	// mutation merely because the reviewed Candidate is now old.
	fixture.app.now = func() time.Time { return result.Preview.ChangeSet.ExpiresAt.Add(time.Nanosecond) }
	replayed, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil || replayed.Receipt.TransactionID != result.Receipt.TransactionID {
		t.Fatalf("manual replay = %#v, %v", replayed, err)
	}
	afterReplay, _ := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if afterReplay.Generation != 2 || len(afterReplay.ApprovalConsumptions) != 1 {
		t.Fatalf("manual replay mutated snapshot = %#v", afterReplay)
	}
}

func TestWorkflowEditManualExpiredUnappliedCandidateCreatesFreshCandidate(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	position := visual.Position{X: 360, Y: 220}
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-move-after-expiry",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session", Reason: "move after an abandoned preview",
		Command: WorkflowManualCommand{Kind: WorkflowManualMove, Layout: &edit.LayoutPatch{
			Nodes: []edit.LayoutNodePatch{{NodeID: "check", Position: position}},
		}},
	}
	operation, err := workflowManualOperation(request.Command)
	if err != nil {
		t.Fatal(err)
	}
	batch := workflowCommandBatchRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: request.WorkflowKey, ExpectedRevision: request.ExpectedRevision,
		IdempotencyKey: request.IdempotencyKey, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID,
		SessionID: request.SessionID, Reason: request.Reason, Intent: "manual canvas command: " + string(request.Command.Kind),
		Operations: []edit.WorkflowOperation{operation},
	}
	candidateID, err := workflowCommandBatchCandidateID(batch)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := fixture.app.resolveOrCreateManualPreview(context.Background(), batch, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.now = func() time.Time { return stale.ChangeSet.ExpiresAt.Add(time.Nanosecond) }

	result, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil {
		t.Fatalf("ExecuteManualCommand() after candidate expiry error = %v", err)
	}
	if result.Preview.Candidate.ID == stale.Candidate.ID {
		t.Fatalf("expired candidate %q was reused", stale.Candidate.ID)
	}
	if result.Receipt.Status != "verifying" || result.Receipt.RevisionAfter == nil {
		t.Fatalf("manual result after candidate expiry = %#v", result)
	}
}

func TestWorkflowEditManualDeleteRequiresExplicitConfirmation(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-delete-unconfirmed",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session", Reason: "remove the repair node",
		Command: WorkflowManualCommand{Kind: WorkflowManualDeleteNode, NodeID: "repair", CascadeEdges: true},
	}
	_, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != edit.ErrorApprovalRequired {
		t.Fatalf("unconfirmed delete error = %v", err)
	}
	snapshot, snapshotErr := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if snapshotErr != nil || snapshot.Generation != 1 || len(snapshot.ApprovalConsumptions) != 0 {
		t.Fatalf("unconfirmed delete mutated snapshot = %#v, %v", snapshot, snapshotErr)
	}
}

func TestWorkflowEditManualConfirmedHighRiskCommandUsesSingleUseApproval(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	ports := []visual.Port{{ID: "in", Type: "input"}, {ID: "next", Type: "output"}}
	request := WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "manual-ports-confirmed",
		ActorID: "manual-user", CanvasSessionID: "manual-canvas", SessionID: "manual-session",
		Reason: "confirm the check node ports", Confirmed: true,
		Command: WorkflowManualCommand{Kind: WorkflowManualUpdateNode, NodeID: "check", NodePatch: &edit.NodePatch{Ports: &ports}},
	}
	result, err := fixture.app.ExecuteManualCommand(context.Background(), request)
	if err != nil || result.Preview.Risk.Level != WorkflowRiskHigh || result.Receipt.TransactionID == "" {
		t.Fatalf("confirmed high-risk manual command = %#v, %v", result, err)
	}
	approvalID := "manual-approval-" + strings.TrimPrefix(mustWorkflowManualDigest(t, []any{edit.SchemaVersion, "manual", request.WorkflowKey, request.ActorID, request.CanvasSessionID, request.IdempotencyKey}), "sha256:")[:32]
	approval, err := fixture.approvals.GetApplyApproval(context.Background(), request.WorkflowKey, approvalID)
	if err != nil || approval.State != WorkflowApprovalConsumed {
		t.Fatalf("confirmed manual approval = %#v, %v", approval, err)
	}
}

func TestWorkflowManualCommandTranslationIsStrictAndMoveIsLayoutOnly(t *testing.T) {
	position := visual.Position{X: 10, Y: 20}
	source, target := "start", "check"
	edgeKind := visual.EdgeKindSuccess
	tests := []struct {
		name string
		in   WorkflowManualCommand
		kind edit.OperationKind
	}{
		{"add", WorkflowManualCommand{Kind: WorkflowManualAddNode, Node: &edit.EditableNode{ID: "new", NodePatch: edit.NodePatch{Type: nodeTypePointer(visual.NodeTypeAction), Position: &position}}}, edit.OperationAddNode},
		{"update", WorkflowManualCommand{Kind: WorkflowManualUpdateNode, NodeID: "check", NodePatch: &edit.NodePatch{Position: &position}}, edit.OperationUpdateNode},
		{"delete", WorkflowManualCommand{Kind: WorkflowManualDeleteNode, NodeID: "check", CascadeEdges: true}, edit.OperationDeleteNode},
		{"connect", WorkflowManualCommand{Kind: WorkflowManualConnectEdge, Edge: &edit.EditableEdge{ID: "new-edge", EdgePatch: edit.EdgePatch{Source: &source, Target: &target}}}, edit.OperationAddEdge},
		{"update edge", WorkflowManualCommand{Kind: WorkflowManualUpdateEdge, EdgeID: "start-check", EdgePatch: &edit.EdgePatch{Kind: &edgeKind}}, edit.OperationUpdateEdge},
		{"disconnect", WorkflowManualCommand{Kind: WorkflowManualDisconnectEdge, EdgeID: "start-check"}, edit.OperationDeleteEdge},
		{"move", WorkflowManualCommand{Kind: WorkflowManualMove, Layout: &edit.LayoutPatch{Nodes: []edit.LayoutNodePatch{{NodeID: "check", Position: position}}}}, edit.OperationUpdateLayout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation, err := workflowManualOperation(test.in)
			if err != nil || operation.Value.Kind() != test.kind {
				t.Fatalf("workflowManualOperation() = %#v, %v", operation, err)
			}
		})
	}
	if _, err := workflowManualOperation(WorkflowManualCommand{Kind: WorkflowManualMove, NodeID: "check"}); err == nil {
		t.Fatal("move without a typed layout was accepted")
	}
}

func nodeTypePointer(value visual.NodeType) *visual.NodeType { return &value }

func mustWorkflowManualDigest(t *testing.T, value any) string {
	t.Helper()
	digest, err := workflowQueryDigest(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
