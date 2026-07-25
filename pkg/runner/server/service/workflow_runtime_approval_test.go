package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowVerifiedRuntimeApprovalIssuesExactPersistentAuthorizationAndReplays(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	previewRequest := newWorkflowPreviewRequest(fixture.backend.snapshot, "candidate-runtime-approval", updatePosition("check", 180, 90))
	previewRequest.AgentSessionID, previewRequest.TurnID, previewRequest.ToolCallID = "runtime-session-1", "turn-runtime-apply", "call-runtime-preview"
	preview, err := fixture.app.Preview(ctx, previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	fixture.app.now = func() time.Time { return now }
	request := workflowVerifiedRuntimeAuthorizationRequest(preview, "grant-runtime", "approval-business", "approval-runtime", "sha256:"+repeatHex("1"), "sha256:"+repeatHex("2"))
	request.RuntimeToolName = workflowRuntimeApplyProviderAlias

	if _, err := fixture.app.ResolveTrustedApplyContext(ctx, WorkflowTrustedApplySelector{
		Key: request.Key, ActorID: request.ActorID, CanvasSessionID: previewRequest.CanvasSessionID,
		ChangeSetID: request.ChangeSetID, ApprovalID: request.ApplyApprovalID,
	}); err == nil {
		t.Fatal("runtime apply unexpectedly had a business approval before authorization")
	}
	trusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request)
	if err != nil {
		t.Fatalf("AuthorizeVerifiedRuntimeApply() error = %v", err)
	}
	if trusted.GrantID != request.CandidateGrantID || trusted.ApprovalID != request.ApplyApprovalID || trusted.ChangeSetID != request.ChangeSetID || trusted.PreviewHash != request.PreviewHash || trusted.CanvasSessionID != previewRequest.CanvasSessionID {
		t.Fatalf("trusted runtime authorization = %#v", trusted)
	}
	grant, err := fixture.approvals.GetGrant(ctx, request.Key, request.CandidateGrantID)
	if err != nil || grant.State != WorkflowApprovalActive || grant.Grant.MaxOperations != 1 || grant.Grant.AllowDeleteExisting || grant.RiskScope.MaxRisk != WorkflowRiskLow || grant.CanvasSessionID != previewRequest.CanvasSessionID {
		t.Fatalf("persisted exact runtime grant = %#v, %v", grant, err)
	}
	approval, err := fixture.approvals.GetApplyApproval(ctx, request.Key, request.ApplyApprovalID)
	if err != nil || approval.State != WorkflowApprovalActive || approval.Approval.CandidateID != trusted.CandidateID || approval.Approval.PreviewHash != request.PreviewHash || approval.CanvasSessionID != previewRequest.CanvasSessionID {
		t.Fatalf("persisted exact runtime approval = %#v, %v", approval, err)
	}

	replayedTrusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request)
	if err != nil || replayedTrusted != trusted {
		t.Fatalf("idempotent authorization replay = %#v, %v", replayedTrusted, err)
	}
	if _, err := fixture.app.ResolveTrustedApplyContext(ctx, WorkflowTrustedApplySelector{
		Key: request.Key, ActorID: request.ActorID, CanvasSessionID: "browser-declared-canvas",
		ChangeSetID: request.ChangeSetID, ApprovalID: request.ApplyApprovalID,
	}); err == nil {
		t.Fatal("browser-declared canvas resolved the persisted Runtime authorization")
	}
	applyRequest := workflowStoryApplyRequest(trusted, "runtime-approved-apply")
	forgedCanvasApply := applyRequest
	forgedCanvasApply.CanvasSessionID = "browser-declared-canvas"
	if _, err := fixture.app.Apply(ctx, trusted, forgedCanvasApply); err == nil {
		t.Fatal("browser-declared canvas applied the persisted Runtime authorization")
	}
	receipt, err := fixture.app.Apply(ctx, trusted, applyRequest)
	if err != nil {
		t.Fatalf("Apply(runtime-authorized) = %#v, %v", receipt, err)
	}
	fixture.syncBackend(t)
	fixture.restart(t)
	candidate, err := fixture.candidates.ResolveChangeSet(ctx, request.Key, request.ChangeSetID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.now = func() time.Time { return candidate.ExpiresAt.Add(time.Minute) }

	restartedTrusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request)
	if err != nil || restartedTrusted != trusted {
		t.Fatalf("consumed authorization replay after restart = %#v, %v", restartedTrusted, err)
	}
	replayedReceipt, err := fixture.app.Apply(ctx, restartedTrusted, applyRequest)
	if err != nil || replayedReceipt.TransactionID != receipt.TransactionID {
		t.Fatalf("runtime-approved Apply replay = %#v, %v", replayedReceipt, err)
	}
	snapshot, err := fixture.commits.Snapshot(ctx, request.Key)
	if err != nil || snapshot.Generation != 2 || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("runtime approval replay created another commit = %#v, %v", snapshot, err)
	}
}

func TestWorkflowVerifiedRuntimeApprovalUsesPersistedCanvasForUndoAndRedo(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	committed := fixture.applyOperations(t, "candidate-runtime-history-base", "runtime-history-base", updatePosition("check", 180, 90))

	undo, err := fixture.app.Undo(ctx, WorkflowEditHistoryActionRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.backend.snapshot.Key,
		TargetTransactionID: committed.TransactionID, CandidateID: "candidate-runtime-history-undo",
		Expected: fixture.backend.snapshot.Revision, Actor: edit.ActorKind("user"), ActorID: "actor-1",
		CanvasSessionID: "history-canvas-undo", AgentSessionID: "runtime-history-undo",
		TurnID: "turn-history-undo", ToolCallID: "call-history-undo-preview",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.now = func() time.Time { return undo.ChangeSet.CreatedAt.Add(time.Minute) }
	undoAuthorization := workflowVerifiedRuntimeAuthorizationRequest(
		undo, "grant-runtime-history-undo", "approval-runtime-history-undo", "runtime-history-undo-checkpoint",
		"sha256:"+repeatHex("c"), "sha256:"+repeatHex("d"),
	)
	undoAuthorization.RuntimeSessionID = "runtime-history-undo"
	undoAuthorization.RuntimeTurnID = "turn-history-undo"
	undoAuthorization.RuntimeToolCallID = "call-history-undo-apply"
	undoTrusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, undoAuthorization)
	if err != nil {
		t.Fatalf("AuthorizeVerifiedRuntimeApply(undo) error = %v", err)
	}
	assertWorkflowRuntimeHistoryCanvas(t, fixture, undoAuthorization, undoTrusted, "history-canvas-undo")
	undoReceipt, err := fixture.app.Apply(ctx, undoTrusted, workflowStoryApplyRequest(undoTrusted, "runtime-history-undo-apply"))
	if err != nil {
		t.Fatalf("Apply(runtime-authorized undo) error = %v", err)
	}
	fixture.syncBackend(t)

	redo, err := fixture.app.Redo(ctx, WorkflowEditHistoryActionRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.backend.snapshot.Key,
		TargetTransactionID: undoReceipt.TransactionID, CandidateID: "candidate-runtime-history-redo",
		Expected: fixture.backend.snapshot.Revision, Actor: edit.ActorKind("user"), ActorID: "actor-1",
		CanvasSessionID: "history-canvas-redo", AgentSessionID: "runtime-history-redo",
		ToolCallID: "call-history-redo-preview",
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.now = func() time.Time { return redo.ChangeSet.CreatedAt.Add(time.Minute) }
	redoAuthorization := workflowVerifiedRuntimeAuthorizationRequest(
		redo, "grant-runtime-history-redo", "approval-runtime-history-redo", "runtime-history-redo-checkpoint",
		"sha256:"+repeatHex("e"), "sha256:"+repeatHex("f"),
	)
	redoAuthorization.RuntimeSessionID = "runtime-history-redo"
	redoAuthorization.RuntimeTurnID = "turn-history-redo"
	redoAuthorization.RuntimeToolCallID = "call-history-redo-apply"
	redoTrusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, redoAuthorization)
	if err != nil {
		t.Fatalf("AuthorizeVerifiedRuntimeApply(redo) error = %v", err)
	}
	assertWorkflowRuntimeHistoryCanvas(t, fixture, redoAuthorization, redoTrusted, "history-canvas-redo")
	if _, err := fixture.app.Apply(ctx, redoTrusted, workflowStoryApplyRequest(redoTrusted, "runtime-history-redo-apply")); err != nil {
		t.Fatalf("Apply(runtime-authorized redo) error = %v", err)
	}
}

func assertWorkflowRuntimeHistoryCanvas(t *testing.T, fixture *workflowEditHistoryFixture, request WorkflowVerifiedRuntimeApplyRequest, trusted WorkflowEditTrustedApplyContext, wantCanvas string) {
	t.Helper()
	if trusted.CanvasSessionID != wantCanvas || trusted.CanvasSessionID == request.RuntimeSessionID {
		t.Fatalf("trusted history canvas = %q, runtime session = %q, want persisted canvas %q", trusted.CanvasSessionID, request.RuntimeSessionID, wantCanvas)
	}
	candidate, err := fixture.candidates.ResolveChangeSet(context.Background(), request.Key, request.ChangeSetID)
	if err != nil || candidate.CanvasSessionID != wantCanvas || candidate.AgentSessionID != request.RuntimeSessionID || candidate.ToolCallID == request.RuntimeToolCallID {
		t.Fatalf("history candidate identity = %#v, %v", candidate, err)
	}
	grant, grantErr := fixture.approvals.GetGrant(context.Background(), request.Key, request.CandidateGrantID)
	approval, approvalErr := fixture.approvals.GetApplyApproval(context.Background(), request.Key, request.ApplyApprovalID)
	if grantErr != nil || approvalErr != nil || grant.CanvasSessionID != wantCanvas || approval.CanvasSessionID != wantCanvas {
		t.Fatalf("persisted history authorization canvas: grant=%#v (%v), approval=%#v (%v)", grant, grantErr, approval, approvalErr)
	}
}

func TestWorkflowVerifiedRuntimeApprovalDerivesHighRiskDeleteScope(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	source, sourcePort, target, targetPort := "check", "next", "end", "in"
	kind := visual.EdgeKindSuccess
	previewRequest := newWorkflowPreviewRequest(
		fixture.backend.snapshot, "candidate-runtime-delete",
		edit.NewOperation(&edit.DeleteNodeOperation{NodeID: "repair", CascadeEdges: true}),
		edit.NewOperation(&edit.AddEdgeOperation{ClientRef: "check-end", Edge: edit.EditableEdge{EdgePatch: edit.EdgePatch{
			Source: &source, SourcePort: &sourcePort, Target: &target, TargetPort: &targetPort, Kind: &kind,
		}}}),
	)
	previewRequest.AgentSessionID, previewRequest.TurnID, previewRequest.ToolCallID = "runtime-session-1", "turn-runtime-apply", "call-runtime-preview"
	preview, err := fixture.app.Preview(ctx, previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	fixture.app.now = func() time.Time { return now }
	request := workflowVerifiedRuntimeAuthorizationRequest(preview, "grant-runtime-delete", "approval-runtime-delete", "runtime-delete-checkpoint", "sha256:"+repeatHex("3"), "sha256:"+repeatHex("4"))
	if _, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request); err != nil {
		t.Fatal(err)
	}
	grant, err := fixture.approvals.GetGrant(ctx, request.Key, request.CandidateGrantID)
	if err != nil || grant.RiskScope.MaxRisk != WorkflowRiskHigh || !grant.Grant.AllowDeleteExisting || grant.Grant.MaxOperations != 2 {
		t.Fatalf("high-risk runtime grant = %#v, %v", grant, err)
	}
}

func TestWorkflowVerifiedRuntimeApprovalRejectsDriftAndIDCollision(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	previewRequest := newWorkflowPreviewRequest(fixture.backend.snapshot, "candidate-runtime-drift", updatePosition("check", 180, 90))
	previewRequest.AgentSessionID, previewRequest.TurnID, previewRequest.ToolCallID = "runtime-session-1", "turn-runtime-apply", "call-runtime-preview"
	preview, err := fixture.app.Preview(ctx, previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	fixture.app.now = func() time.Time { return now }
	request := workflowVerifiedRuntimeAuthorizationRequest(preview, "grant-runtime-drift", "approval-runtime-drift", "runtime-drift-checkpoint", "sha256:"+repeatHex("5"), "sha256:"+repeatHex("6"))
	if _, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*WorkflowVerifiedRuntimeApplyRequest)
	}{
		{"actor", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.ActorID = "other-actor" }},
		{"runtime session", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.RuntimeSessionID = "other-runtime-session" }},
		{"runtime turn", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.RuntimeTurnID = "other-runtime-turn" }},
		{"runtime tool call", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.RuntimeToolCallID = "other-runtime-tool-call" }},
		{"arguments hash", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeArgumentsHash = "sha256:" + repeatHex("7")
		}},
		{"binding hash", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeBindingHash = "sha256:" + repeatHex("8")
		}},
		{"runtime approval", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.RuntimeApprovalID = "another-runtime-approval" }},
		{"target", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeTargetRefs = []string{"type=workflow id=tenant-a:other op=apply"}
		}},
		{"host target", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeTargetRefs = append([]string{"host:host-a"}, value.RuntimeTargetRefs...)
		}},
		{"other operation", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeTargetRefs[1] = "type=workflow id=tenant-a:workflow-a op=publish"
		}},
		{"duplicate", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeTargetRefs = append([]string{value.RuntimeTargetRefs[0]}, value.RuntimeTargetRefs...)
		}},
		{"unsorted", func(value *WorkflowVerifiedRuntimeApplyRequest) {
			value.RuntimeTargetRefs[0], value.RuntimeTargetRefs[2] = value.RuntimeTargetRefs[2], value.RuntimeTargetRefs[0]
		}},
		{"preview", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.PreviewHash = "sha256:" + repeatHex("9") }},
		{"forged runtime tool alias", func(value *WorkflowVerifiedRuntimeApplyRequest) { value.RuntimeToolName = "workflow-apply-patch" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drifted := request
			drifted.RuntimeTargetRefs = append([]string(nil), request.RuntimeTargetRefs...)
			test.mutate(&drifted)
			if _, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, drifted); err == nil {
				t.Fatal("drifted verified Runtime approval was accepted")
			}
		})
	}
	approval, err := fixture.approvals.GetApplyApproval(ctx, request.Key, request.ApplyApprovalID)
	if err != nil || approval.State != WorkflowApprovalActive {
		t.Fatalf("drift changed exact approval = %#v, %v", approval, err)
	}
}

func TestWorkflowVerifiedRuntimeApprovalConcurrentExactAuthorizationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	fixture := newWorkflowEditHistoryFixture(t)
	previewRequest := newWorkflowPreviewRequest(fixture.backend.snapshot, "candidate-runtime-concurrent", updatePosition("check", 180, 90))
	previewRequest.AgentSessionID, previewRequest.TurnID, previewRequest.ToolCallID = "runtime-session-1", "turn-runtime-apply", "call-runtime-preview"
	preview, err := fixture.app.Preview(ctx, previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	now := preview.ChangeSet.CreatedAt.Add(time.Minute)
	fixture.app.now = func() time.Time { return now }
	request := workflowVerifiedRuntimeAuthorizationRequest(preview, "grant-runtime-concurrent", "approval-runtime-concurrent", "runtime-concurrent-checkpoint", "sha256:"+repeatHex("a"), "sha256:"+repeatHex("b"))
	start := make(chan struct{})
	results := make(chan struct {
		trusted WorkflowEditTrustedApplyContext
		err     error
	}, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			trusted, err := fixture.app.AuthorizeVerifiedRuntimeApply(ctx, request)
			results <- struct {
				trusted WorkflowEditTrustedApplyContext
				err     error
			}{trusted: trusted, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var want WorkflowEditTrustedApplyContext
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if want.CandidateID != "" && result.trusted != want {
			t.Fatalf("concurrent authorization mismatch = %#v, want %#v", result.trusted, want)
		}
		want = result.trusted
	}
}

func workflowVerifiedRuntimeAuthorizationRequest(preview WorkflowEditPreviewResult, grantID, approvalID, runtimeApprovalID, argumentsHash, bindingHash string) WorkflowVerifiedRuntimeApplyRequest {
	return WorkflowVerifiedRuntimeApplyRequest{
		SchemaVersion: edit.SchemaVersion, Key: preview.ChangeSet.Key, ActorID: "actor-1", RuntimeSessionID: "runtime-session-1",
		ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash,
		ExpectedRevision: preview.ChangeSet.Base.ResourceVersion,
		CandidateGrantID: grantID, ApplyApprovalID: approvalID,
		RuntimeApprovalID: runtimeApprovalID, RuntimeTurnID: "turn-runtime-apply",
		RuntimeToolCallID: "call-runtime-apply", RuntimeToolName: "workflow.apply_patch",
		RuntimeArgumentsHash: argumentsHash, RuntimeBindingHash: bindingHash,
		RuntimeTargetRefs: []string{
			"type=workflow",
			"type=workflow id=" + preview.ChangeSet.Key.TenantID + ":" + preview.ChangeSet.Key.WorkflowID + " op=apply",
			"type=workflow_candidate",
		},
	}
}

func repeatHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
