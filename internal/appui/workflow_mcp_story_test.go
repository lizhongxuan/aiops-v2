package appui_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	contract "runner/workflow/edit"
)

func TestWorkflowMCPChatApplyStoryConformance(t *testing.T) {
	fixture := newWorkflowMCPStoryFixture(t, workflowStoryApprovalActive)

	visible := workflowStoryToolNames(fixture.assembler.AssembleToolsWithOptions("workspace", "execute", tooling.AssembleOptions{
		Profile: "workflow_agent", EnabledPacks: []string{"workflow_editor"},
		TenantID: workflowStoryTenantID, UserID: workflowStoryUserID,
	}))
	wantCore6 := []string{
		"workflow.apply_patch", "workflow.compile_plan", "workflow.get_action_catalog",
		"workflow.get_context", "workflow.preview_patch", "workflow.validate",
	}
	if !slices.Equal(visible, wantCore6) {
		t.Fatalf("workflow_agent scoped MCP surface = %v, want Core6 %v", visible, wantCore6)
	}

	before, err := fixture.commits.Snapshot(context.Background(), fixture.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	if before.TransactionID != "transaction-seed" || len(before.ApprovalConsumptions) != 0 {
		t.Fatalf("unexpected seed transaction: %#v", before)
	}
	if _, err := fixture.approvals.GetGrant(context.Background(), fixture.snapshot.Key, workflowStoryGrantID); err == nil {
		t.Fatal("active story unexpectedly preseeded a business grant")
	}
	if _, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.snapshot.Key, workflowStoryApprovalID); err == nil {
		t.Fatal("active story unexpectedly preseeded a business apply approval")
	}

	accepted, err := fixture.chat.SendMessage(fixture.baseContext, workflowStoryChatCommand(fixture))
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if accepted.Status != "accepted" || accepted.SessionID != workflowStorySessionID || accepted.TurnID == "" {
		t.Fatalf("SendMessage() = %#v, want accepted workflow turn", accepted)
	}

	blocked := waitForWorkflowStorySnapshot(t, fixture.sessions, workflowStorySessionID, func(turn *runtimekernel.TurnSnapshot) bool {
		return turn.ResumeState == runtimekernel.TurnResumeStatePendingApproval && len(turn.PendingApprovals) == 1
	})
	if blocked.SessionType != runtimekernel.SessionTypeWorkspace || blocked.Mode != runtimekernel.ModeExecute {
		t.Fatalf("workflow route = %s/%s, want workspace/execute", blocked.SessionType, blocked.Mode)
	}
	if blocked.Metadata["profile"] != "workflow_agent" || blocked.Metadata["enableToolPack"] != "workflow_editor" || blocked.Metadata["aiops.webSearch.policy"] != string(runtimekernel.WebSearchDisabled) || blocked.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("workflow route metadata = %#v", blocked.Metadata)
	}
	if blocked.LatestCheckpoint == nil || blocked.LatestCheckpoint.ResumeState != runtimekernel.TurnResumeStatePendingApproval {
		t.Fatalf("blocked turn checkpoint = %#v", blocked.LatestCheckpoint)
	}
	previewResult, found := workflowStoryToolResult(blocked, workflowStoryPreviewCallID)
	if !found || previewResult.IsError || len(previewResult.StructuredContent) == 0 {
		t.Fatalf("workflow.preview_patch did not execute successfully in the Runtime turn: %#v, found=%v", previewResult, found)
	}
	if _, found := workflowStoryToolResult(blocked, workflowStoryApplyCallID); found {
		t.Fatal("workflow.apply_patch executed before Runtime approval")
	}
	applyRequest, found := fixture.model.ApplyRequest()
	if !found || applyRequest.WorkflowKey != fixture.snapshot.Key || applyRequest.ChangeSetID == "" || applyRequest.PreviewHash == "" || applyRequest.ExpectedRevision != fixture.snapshot.Revision.ResourceVersion {
		t.Fatalf("model did not derive exact apply arguments from the typed preview: %#v", applyRequest)
	}
	if applyRequest.CandidateGrantID != workflowStoryGrantID || applyRequest.ApplyApprovalID != workflowStoryApprovalID {
		t.Fatalf("model apply authorization IDs = %q/%q", applyRequest.CandidateGrantID, applyRequest.ApplyApprovalID)
	}
	stillSeed, err := fixture.commits.Snapshot(context.Background(), fixture.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	if stillSeed.TransactionID != before.TransactionID || len(stillSeed.ApprovalConsumptions) != 0 {
		t.Fatalf("commit changed before approval: %#v", stillSeed)
	}

	pending := blocked.PendingApprovals[0]
	wantTargetRefs := []string{
		"type=workflow",
		"type=workflow id=" + workflowStoryTenantID + ":" + workflowStoryWorkflowID + " op=apply",
		"type=workflow_candidate",
	}
	if !slices.Equal(pending.TargetRefs, wantTargetRefs) {
		t.Fatalf("Runtime approval target refs = %#v, want exact dynamic Workflow target %#v", pending.TargetRefs, wantTargetRefs)
	}
	if _, err := fixture.approvals.GetGrant(context.Background(), fixture.snapshot.Key, workflowStoryGrantID); err == nil {
		t.Fatal("business grant materialized before the Runtime approval decision")
	}
	if _, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.snapshot.Key, workflowStoryApprovalID); err == nil {
		t.Fatal("business apply approval materialized before the Runtime approval decision")
	}
	resumed, err := fixture.kernel.ResumeTurn(fixture.baseContext, runtimekernel.ResumeRequest{
		SessionID: workflowStorySessionID, TurnID: blocked.ID,
		ApprovalID: pending.ID, ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Decision: "approved", Metadata: map[string]string{"approval.reason": "reviewed in workflow drawer"},
	})
	if err != nil {
		t.Fatalf("ResumeTurn() error = %v", err)
	}
	if resumed.Status == "blocked" || resumed.Status == "failed" || resumed.Error != "" {
		t.Fatalf("ResumeTurn() = %#v, want terminal non-error result", resumed)
	}
	completed := waitForWorkflowStorySnapshot(t, fixture.sessions, workflowStorySessionID, func(turn *runtimekernel.TurnSnapshot) bool {
		return turn.Lifecycle == runtimekernel.TurnLifecycleCompleted && turn.ResumeState == runtimekernel.TurnResumeStateNone
	})
	if len(completed.PendingApprovals) != 0 {
		t.Fatalf("completed turn retains pending approvals: %#v", completed.PendingApprovals)
	}

	toolResult, found := workflowStoryToolResult(completed, workflowStoryApplyCallID)
	if !found {
		t.Fatalf("completed story has no workflow.apply_patch result: %#v", completed.Iterations)
	}
	if toolResult.IsError || len(toolResult.StructuredContent) == 0 {
		t.Fatalf("workflow.apply_patch result = %#v", toolResult)
	}
	var envelope contract.ToolResult[contract.WorkflowMutationReceipt]
	if err := json.Unmarshal(toolResult.StructuredContent, &envelope); err != nil {
		t.Fatalf("decode workflow receipt: %v", err)
	}
	receipt := envelope.StructuredContent
	if envelope.IsError || receipt.Kind != "receipt" || receipt.TransactionID == "" || receipt.TransactionID == workflowStoryFakeFinalTxnID || receipt.ChangeSetID != applyRequest.ChangeSetID {
		t.Fatalf("typed workflow receipt = %#v", envelope)
	}

	committed, err := fixture.commits.Snapshot(context.Background(), fixture.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	if committed.TransactionID != receipt.TransactionID || committed.Receipt.TransactionID != receipt.TransactionID || len(committed.ApprovalConsumptions) != 1 {
		t.Fatalf("persisted transaction does not match typed receipt: %#v", committed)
	}
	businessGrant, err := fixture.approvals.GetGrant(context.Background(), fixture.snapshot.Key, applyRequest.CandidateGrantID)
	if err != nil {
		t.Fatal(err)
	}
	if businessGrant.Grant.ActorID != workflowStoryUserID || businessGrant.Grant.Key != fixture.snapshot.Key || businessGrant.Grant.MaxOperations != 1 || len(businessGrant.Grant.AllowedOps) != 1 || businessGrant.Grant.AllowedOps[0] != contract.OperationUpdateNode {
		t.Fatalf("Runtime materialized business grant = %#v", businessGrant)
	}
	businessApproval, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.snapshot.Key, applyRequest.ApplyApprovalID)
	if err != nil {
		t.Fatal(err)
	}
	if businessApproval.State != "consumed" || businessApproval.ConsumptionID != receipt.TransactionID {
		t.Fatalf("business approval consumption = %#v", businessApproval)
	}

	projected, err := appui.NewTransportProjector().ProjectTurnSnapshot(
		appui.NewAiopsTransportState(workflowStorySessionID, "workflow-story-thread"), completed,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.SchemaVersion != appui.AiopsTransportSchemaVersion || len(projected.PendingApprovals) != 0 {
		t.Fatalf("projected state envelope = %#v", projected)
	}
	projectedTurn, ok := projected.Turns[completed.ID]
	if !ok || projectedTurn.Status != appui.AiopsTransportTurnStatusCompleted {
		t.Fatalf("projected workflow turn = %#v", projectedTurn)
	}
	var typedReceiptFound, proseFinalFound bool
	for _, item := range projectedTurn.AgentItems {
		if item.Type == "tool_result" && strings.Contains(string(item.Payload.Data), receipt.TransactionID) && !strings.Contains(string(item.Payload.Data), workflowStoryFakeFinalTxnID) {
			typedReceiptFound = true
		}
	}
	for _, block := range projectedTurn.BlocksByID {
		if block.FinalContract != nil && strings.Contains(block.FinalContract.Text, workflowStoryFakeFinalTxnID) {
			proseFinalFound = true
		}
	}
	if !typedReceiptFound || !proseFinalFound {
		t.Fatalf("typed receipt/prose separation missing: typed=%v prose=%v agentItems=%d blocks=%d", typedReceiptFound, proseFinalFound, len(projectedTurn.AgentItems), len(projectedTurn.BlocksByID))
	}
	var previewArtifactFound, receiptArtifactFound bool
	for _, artifact := range projectedTurn.AgentUIArtifacts {
		raw, marshalErr := json.Marshal(artifact.InlineData)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(raw), workflowStoryFakeFinalTxnID) {
			t.Fatalf("final prose synthesized a typed transaction artifact: %#v", artifact)
		}
		switch artifact.Type {
		case "workflow_patch_preview":
			previewArtifactFound = true
		case "workflow_patch_result":
			receiptArtifactFound = strings.Contains(string(raw), receipt.TransactionID)
		}
	}
	if !previewArtifactFound || !receiptArtifactFound {
		t.Fatalf("typed tool-result artifacts missing: preview=%v receipt=%v artifacts=%#v", previewArtifactFound, receiptArtifactFound, projectedTurn.AgentUIArtifacts)
	}
	projectedJSON, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projectedJSON), workflowStorySecretMarker) || !strings.Contains(string(projectedJSON), "[REDACTED]") {
		t.Fatalf("projected transport did not redact sensitive tool arguments: %s", projectedJSON)
	}
	if !strings.Contains(string(projectedJSON), receipt.TransactionID) {
		t.Fatalf("typed receipt %q missing from projected transport", receipt.TransactionID)
	}
}

func TestWorkflowMCPChatRejectsExpiredOrRevokedAuthorizationIDCollisionStoryConformance(t *testing.T) {
	for _, approvalMode := range []workflowStoryApprovalMode{workflowStoryApprovalExpired, workflowStoryApprovalRevoked} {
		t.Run(string(approvalMode), func(t *testing.T) {
			fixture := newWorkflowMCPStoryFixture(t, approvalMode)
			collision, err := fixture.approvals.GetGrant(context.Background(), fixture.snapshot.Key, workflowStoryGrantID)
			if err != nil {
				t.Fatal(err)
			}
			if approvalMode == workflowStoryApprovalRevoked && collision.State != "revoked" {
				t.Fatalf("revoked collision fixture = %#v", collision)
			}
			accepted, err := fixture.chat.SendMessage(fixture.baseContext, workflowStoryChatCommand(fixture))
			if err != nil || accepted.Status != "accepted" {
				t.Fatalf("SendMessage() = %#v, %v", accepted, err)
			}
			blocked := waitForWorkflowStorySnapshot(t, fixture.sessions, workflowStorySessionID, func(turn *runtimekernel.TurnSnapshot) bool {
				return turn.ResumeState == runtimekernel.TurnResumeStatePendingApproval && len(turn.PendingApprovals) == 1
			})
			pending := blocked.PendingApprovals[0]
			wantTargetRefs := []string{
				"type=workflow",
				"type=workflow id=" + workflowStoryTenantID + ":" + workflowStoryWorkflowID + " op=apply",
				"type=workflow_candidate",
			}
			if !slices.Equal(pending.TargetRefs, wantTargetRefs) {
				t.Fatalf("Runtime approval target refs = %#v, want %#v", pending.TargetRefs, wantTargetRefs)
			}
			applyRequest, found := fixture.model.ApplyRequest()
			if !found {
				t.Fatal("model did not derive apply request from Runtime preview")
			}
			resumed, err := fixture.kernel.ResumeTurn(fixture.baseContext, runtimekernel.ResumeRequest{
				SessionID: workflowStorySessionID, TurnID: blocked.ID, ApprovalID: pending.ID,
				ResumeState: runtimekernel.TurnResumeStatePendingApproval, Decision: "approved",
			})
			if err != nil {
				t.Fatalf("ResumeTurn() error = %v", err)
			}
			if resumed.Status == "blocked" {
				t.Fatalf("Runtime approval remained blocked instead of returning typed business error: %#v", resumed)
			}
			completed := waitForWorkflowStorySnapshot(t, fixture.sessions, workflowStorySessionID, func(turn *runtimekernel.TurnSnapshot) bool {
				return turn.Lifecycle == runtimekernel.TurnLifecycleCompleted
			})
			toolResult, found := workflowStoryToolResult(completed, workflowStoryApplyCallID)
			if !found || !toolResult.IsError {
				t.Fatalf("%s grant result = %#v, found=%v", approvalMode, toolResult, found)
			}
			var envelope contract.ToolResult[contract.DomainError]
			if err := json.Unmarshal(toolResult.StructuredContent, &envelope); err != nil {
				t.Fatal(err)
			}
			if !envelope.IsError || envelope.StructuredContent.Kind != "error" || envelope.StructuredContent.Code != contract.ErrorApprovalRequired {
				t.Fatalf("%s typed error = %#v", approvalMode, envelope)
			}
			if _, err := fixture.approvals.GetApplyApproval(context.Background(), fixture.snapshot.Key, applyRequest.ApplyApprovalID); err == nil {
				t.Fatalf("%s collision was overwritten with a business approval", approvalMode)
			}
			persistedCollision, err := fixture.approvals.GetGrant(context.Background(), fixture.snapshot.Key, workflowStoryGrantID)
			if err != nil {
				t.Fatal(err)
			}
			if approvalMode == workflowStoryApprovalRevoked && persistedCollision.State != "revoked" {
				t.Fatalf("revoked collision changed state: %#v", persistedCollision)
			}
			if approvalMode == workflowStoryApprovalExpired && (!persistedCollision.Grant.ExpiresAt.Before(time.Now().UTC()) || persistedCollision.Grant.ExpiresAt != collision.Grant.ExpiresAt) {
				t.Fatalf("expired collision expiry was overwritten: %#v", persistedCollision)
			}
			committed, err := fixture.commits.Snapshot(context.Background(), fixture.snapshot.Key)
			if err != nil {
				t.Fatal(err)
			}
			if committed.TransactionID != "transaction-seed" || len(committed.ApprovalConsumptions) != 0 {
				t.Fatalf("%s grant changed committed workflow: %#v", approvalMode, committed)
			}
		})
	}
}
