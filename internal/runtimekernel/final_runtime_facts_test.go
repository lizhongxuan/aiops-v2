package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/verification"
)

func TestFinalRuntimeFactsFailedPostcheckOverridesVerifiedAnswerClaim(t *testing.T) {
	snapshot := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-failed-postcheck",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusFail,
		Subject:     "synthetic mutation postcheck",
		Expected:    "service is healthy",
		Actual:      "service is unhealthy",
		RawRefs:     []string{"artifact://synthetic/postcheck-fail"},
		Evidence: []verification.EvidenceRecord{{
			Kind:    verification.EvidenceExecution,
			Command: "systemctl is-active demo.service",
			Result:  verification.EvidenceResultFail,
			RawRef:  "artifact://synthetic/postcheck-fail",
		}},
		ContractChecks: []verification.ContractCheck{{
			Name:     "systemctl is-active demo.service",
			Checked:  true,
			Expected: "active",
			Actual:   "failed",
			Result:   verification.EvidenceResultFail,
		}},
	})
	snapshot.TaskDepth = taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}
	snapshot.Iterations[0].ToolInvocations = []ToolInvocationState{{
		ToolCallID:            "call-mutate",
		ToolName:              "exec_command",
		Status:                ToolInvocationCompleted,
		Mutating:              true,
		RequiredPostCheckRefs: []string{"systemctl is-active demo.service"},
	}}

	facts := BuildFinalRuntimeFacts(snapshot, nil)
	contract := BuildFinalContract("已验证，全部检查通过。", facts)

	if facts.PostcheckStatus != FinalPostcheckStatusFailed {
		t.Fatalf("postcheck status = %q, want failed: %#v", facts.PostcheckStatus, facts)
	}
	if contract.Status != FinalContractStatusFailed && contract.Status != FinalContractStatusPartial {
		t.Fatalf("status = %q, want failed/partial from facts: %#v", contract.Status, contract)
	}
}

func TestFinalContractIgnoresAnswerClaimsWhenTypedCompletionSucceeded(t *testing.T) {
	snapshot := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-pass-facts",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     "synthetic read verification",
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultPass,
			RawRef: "artifact://synthetic/pass-facts",
		}},
	})
	snapshot.TaskDepth = taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}

	facts := BuildFinalRuntimeFacts(snapshot, nil)
	contract := BuildFinalContract("执行失败，验证未通过。", facts)

	if facts.CompletionStatus != FinalCompletionStatusSucceeded {
		t.Fatalf("completion status = %q, want succeeded: %#v", facts.CompletionStatus, facts)
	}
	if contract.Status != FinalContractStatusVerified {
		t.Fatalf("status = %q, want verified from facts: %#v", contract.Status, contract)
	}
}

func TestFinalContractIgnoresCitationLikeAnswerWithoutEvidenceFacts(t *testing.T) {
	facts := BuildFinalRuntimeFacts(&TurnSnapshot{
		ID:        "turn-no-evidence",
		SessionID: "session-no-evidence",
		TaskDepth: taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresEvidence: true},
	}, nil)

	contract := BuildFinalContract("已验证。[evidence://invented] (artifact://invented)", facts)

	if contract.Status == FinalContractStatusVerified {
		t.Fatalf("status = verified without typed evidence refs: %#v", contract)
	}
	if len(contract.CheckedEvidenceRefs) != 0 {
		t.Fatalf("checked evidence refs = %#v, want empty", contract.CheckedEvidenceRefs)
	}
}

func TestFinalRuntimeFactsAreInvariantAcrossAnswerClaims(t *testing.T) {
	snapshot := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-invariant",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     "synthetic invariant verification",
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultPass,
			RawRef: "artifact://synthetic/invariant",
		}},
	})
	snapshot.TaskDepth = taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}
	facts := BuildFinalRuntimeFacts(snapshot, nil)

	verifiedClaim := BuildFinalContract("已验证，成功。", facts)
	failedClaim := BuildFinalContract("失败，未验证。", facts)

	if verifiedClaim.Status != failedClaim.Status || verifiedClaim.Confidence != failedClaim.Confidence {
		t.Fatalf("answer claims changed business facts: verified=%#v failed=%#v", verifiedClaim, failedClaim)
	}
	if verifiedClaim.AnswerText == failedClaim.AnswerText {
		t.Fatal("display text should remain independent from business facts")
	}
}

func TestFinalRuntimeFactsRejectEvidenceRefsFromFailedToolResult(t *testing.T) {
	snapshot := &TurnSnapshot{
		ID:        "turn-failed-ref",
		SessionID: "session-failed-ref",
		TaskDepth: taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresEvidence: true},
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{ID: "call-failed-ref", Name: "synthetic.read"}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-failed-ref",
				Error:      "synthetic read failed",
				Content:    `{"evidenceRefs":["artifact://must-not-count"]}`,
			}},
		}},
	}

	facts := BuildFinalRuntimeFacts(snapshot, nil)

	if len(facts.EvidenceRefs) != 0 {
		t.Fatalf("evidence refs = %#v, failed tool result must not satisfy coverage", facts.EvidenceRefs)
	}
	if facts.CompletionStatus == FinalCompletionStatusSucceeded {
		t.Fatalf("completion = succeeded with failed evidence result: %#v", facts)
	}
}

func TestFinalRuntimeFactsPostcheckPendingPassAndFailComeFromVerificationReport(t *testing.T) {
	makeSnapshot := func(report verification.VerificationReport) *TurnSnapshot {
		snapshot := verificationReportSnapshot(t, report)
		snapshot.TaskDepth = taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}
		snapshot.Iterations[0].ToolInvocations = []ToolInvocationState{{
			ToolCallID:            "call-mutate",
			ToolName:              "exec_command",
			Status:                ToolInvocationCompleted,
			Mutating:              true,
			RequiredPostCheckRefs: []string{"systemctl is-active demo.service"},
		}}
		contract := validActionRollbackContractForTest()
		snapshot.Iterations[0].PendingApprovals = []PendingApproval{{
			ID:               "approval-mutate",
			ToolCallID:       "call-mutate",
			ToolName:         "exec_command",
			Mutating:         true,
			Status:           "approved",
			Decision:         "approved",
			RollbackContract: contract,
		}}
		return snapshot
	}

	pass := makeSnapshot(verification.VerificationReport{
		ID:          "vr-postcheck-pass",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     "synthetic postcheck",
		Evidence: []verification.EvidenceRecord{{
			Kind:    verification.EvidenceExecution,
			Command: "systemctl is-active demo.service",
			Result:  verification.EvidenceResultPass,
			RawRef:  "artifact://synthetic/postcheck-pass",
		}},
	})
	passFacts := BuildFinalRuntimeFacts(pass, nil)
	if passFacts.PostcheckStatus != FinalPostcheckStatusPassed || passFacts.CompletionStatus != FinalCompletionStatusSucceeded {
		t.Fatalf("pass facts = %#v, want passed/succeeded", passFacts)
	}

	pending := makeSnapshot(verification.VerificationReport{
		ID:          "vr-postcheck-pending",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     "synthetic unrelated verification",
		Evidence: []verification.EvidenceRecord{{
			Kind:    verification.EvidenceExecution,
			Command: "echo unrelated",
			Result:  verification.EvidenceResultPass,
			RawRef:  "artifact://synthetic/unrelated-pass",
		}},
	})
	pendingFacts := BuildFinalRuntimeFacts(pending, nil)
	if pendingFacts.PostcheckStatus != FinalPostcheckStatusPending {
		t.Fatalf("pending facts = %#v, want pending", pendingFacts)
	}
	if status := BuildFinalContract("全部通过。", pendingFacts).Status; status != FinalContractStatusNeedsEvidence {
		t.Fatalf("pending postcheck contract status = %q, want needs_evidence", status)
	}
}

func TestFinalRuntimeFactsRollbackReadinessIgnoresRollbackSuccessProse(t *testing.T) {
	snapshot := &TurnSnapshot{
		ID:        "turn-rollback-readiness",
		SessionID: "session-rollback-readiness",
		Iterations: []IterationState{{ToolInvocations: []ToolInvocationState{{
			ToolCallID: "call-mutation",
			ToolName:   "exec_command",
			Status:     ToolInvocationCompleted,
			Mutating:   true,
		}}}},
	}
	facts := BuildFinalRuntimeFacts(snapshot, nil)
	if facts.RollbackStatus != FinalRollbackStatusMissing {
		t.Fatalf("rollback readiness = %q, want missing", facts.RollbackStatus)
	}
	successClaim := BuildFinalContract("rollback succeeded", facts)
	failureClaim := BuildFinalContract("rollback failed", facts)
	if successClaim.Status != failureClaim.Status || successClaim.Confidence != failureClaim.Confidence {
		t.Fatalf("rollback prose changed facts: success=%#v failure=%#v", successClaim, failureClaim)
	}
}

func TestFinalRuntimeFactsPendingApprovalCannotBeBypassedByAnswerText(t *testing.T) {
	snapshot := &TurnSnapshot{ID: "turn-pending-approval", SessionID: "session-pending-approval"}
	session := &SessionState{ID: snapshot.SessionID, PendingApprovals: []PendingApproval{{
		ID: "approval-pending", Status: "pending",
	}}}
	facts := BuildFinalRuntimeFacts(snapshot, session)
	if facts.CompletionStatus != FinalCompletionStatusBlocked {
		t.Fatalf("completion = %q, want blocked: %#v", facts.CompletionStatus, facts)
	}
	claimedDone := BuildFinalContract("审批已经完成。", facts)
	declaredPending := BuildFinalContract("审批仍在等待。", facts)
	if claimedDone.Status != declaredPending.Status || claimedDone.Status != FinalContractStatusBlocked {
		t.Fatalf("approval prose changed status: done=%#v pending=%#v", claimedDone, declaredPending)
	}
}

func TestFinalRuntimeFactsTypedPartialResultPrecedesMissingVerificationReport(t *testing.T) {
	facts := FinalRuntimeFacts{
		EvidenceState: FinalEvidenceState{FailedTools: []FailedToolImpact{{
			ToolName:     "wait_host_agents",
			FailureClass: "partial_result",
		}}},
	}
	completion := VerificationCompletionDecision{
		Action:  VerificationCompletionActionBlockSuccessFinal,
		Reasons: []string{"execution_required", "missing_verification_report"},
	}

	if got := finalRuntimeCompletionStatus(completion, facts); got != FinalCompletionStatusPartial {
		t.Fatalf("completion = %q, want partial from typed aggregate child outcome", got)
	}
}

func TestFinalRuntimeFactsTypedPartialResultCannotOverrideHardBlockers(t *testing.T) {
	tests := []struct {
		name             string
		failureCodes     []string
		evidenceDecision FinalEvidenceVerification
	}{
		{
			name: "mutation target binding",
			evidenceDecision: FinalEvidenceVerification{
				Action:  FinalEvidenceActionBlock,
				Reasons: []string{"mutation_intent_requires_explicit_target_binding", "no_explicit_target_binding"},
			},
		},
		{name: "approval denied", failureCodes: []string{"approval_denied"}},
		{name: "approval pending", failureCodes: []string{"approval_pending"}},
		{name: "plan completion", failureCodes: []string{"plan_completion_blocked"}},
		{name: "coverage", failureCodes: []string{"coverage_incomplete"}},
		{name: "completion policy deny", failureCodes: []string{"completion_policy_deny"}},
		{name: "completion policy evidence", failureCodes: []string{"completion_policy_need_evidence"}},
		{name: "runtime approval gate", failureCodes: []string{"missing_runtime_approval_gate"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts := FinalRuntimeFacts{
				FailureCodes:     tc.failureCodes,
				EvidenceDecision: tc.evidenceDecision,
				EvidenceState: FinalEvidenceState{FailedTools: []FailedToolImpact{{
					ToolName:     "wait_host_agents",
					FailureClass: "partial_result",
				}}},
			}
			completion := VerificationCompletionDecision{Action: VerificationCompletionActionAllow}

			if got := finalRuntimeCompletionStatus(completion, facts); got != FinalCompletionStatusBlocked {
				t.Fatalf("completion = %q, want blocked because %s is a hard blocker: %#v", got, tc.name, facts)
			}
		})
	}
}

func TestFinalRuntimeFactsIgnoresRejectedApprovalsFromOtherTurns(t *testing.T) {
	snapshot := &TurnSnapshot{ID: "turn-current", SessionID: "session-rejection-history"}
	session := &SessionState{
		ID: snapshot.SessionID,
		RejectedApprovals: []RejectedApproval{{
			ID:       "approval-from-history",
			TurnID:   "turn-history",
			Decision: "denied",
			Reason:   "historical operator decision",
		}},
	}

	facts := BuildFinalRuntimeFacts(snapshot, session)

	if containsFinalRuntimeCode(facts.FailureCodes, "approval_denied") {
		t.Fatalf("historical rejection polluted current failure codes: %#v", facts.FailureCodes)
	}
	if facts.CompletionStatus == FinalCompletionStatusBlocked {
		t.Fatalf("historical rejection blocked current turn: %#v", facts)
	}
	if len(facts.ApprovalOutcomes) != 0 {
		t.Fatalf("current-turn approval outcomes include history: %#v", facts.ApprovalOutcomes)
	}
}

func TestFinalRuntimeFactsUsesInjectedCompletionEvaluator(t *testing.T) {
	tests := []struct {
		name       string
		decision   policyengine.PolicyDecision
		wantCode   string
		wantReason string
	}{
		{
			name:       "deny",
			decision:   policyengine.PolicyDecision{Action: policyengine.PolicyActionDeny, Reason: "synthetic completion deny"},
			wantCode:   "completion_policy_deny",
			wantReason: "synthetic completion deny",
		},
		{
			name:       "need evidence",
			decision:   policyengine.PolicyDecision{Action: policyengine.PolicyActionNeedEvidence, Reason: "synthetic completion evidence"},
			wantCode:   "completion_policy_need_evidence",
			wantReason: "synthetic completion evidence",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := &recordingFinalCompletionEvaluator{decision: tc.decision}
			snapshot := &TurnSnapshot{ID: "turn-injected-completion", SessionID: "session-injected-completion"}

			facts := BuildFinalRuntimeFacts(snapshot, nil, evaluator)

			if evaluator.calls != 1 {
				t.Fatalf("completion evaluator calls = %d, want 1", evaluator.calls)
			}
			if evaluator.state.SessionID != snapshot.SessionID || evaluator.state.TurnID != snapshot.ID || !evaluator.state.Completed {
				t.Fatalf("completion evaluator state = %#v, want current completed turn", evaluator.state)
			}
			if !containsFinalRuntimeCode(facts.FailureCodes, tc.wantCode) || !containsFinalRuntimeCode(facts.FailureCodes, tc.wantReason) {
				t.Fatalf("failure codes = %#v, want %q and %q", facts.FailureCodes, tc.wantCode, tc.wantReason)
			}
			if facts.CompletionStatus != FinalCompletionStatusBlocked {
				t.Fatalf("completion = %q, want blocked from injected evaluator: %#v", facts.CompletionStatus, facts)
			}
		})
	}
}

type recordingFinalCompletionEvaluator struct {
	decision policyengine.PolicyDecision
	calls    int
	state    policyengine.TurnState
	ctxValue string
}

type finalCompletionContextKey struct{}

func (e *recordingFinalCompletionEvaluator) CheckCompletion(ctx context.Context, state policyengine.TurnState) policyengine.PolicyDecision {
	e.calls++
	e.state = state
	e.ctxValue, _ = ctx.Value(finalCompletionContextKey{}).(string)
	return e.decision
}

func TestRunTurnFinalContractUsesKernelCompletionPolicy(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("执行已经完成。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	evaluator := &recordingFinalCompletionEvaluator{decision: policyengine.PolicyDecision{
		Action: policyengine.PolicyActionDeny,
		Reason: "synthetic kernel completion deny",
	}}
	kernel.policy.CompletionPolicy = evaluator
	spanSource := &mockSpanStreamSource{}
	kernel.spanSource = spanSource
	runCtx := context.WithValue(context.Background(), finalCompletionContextKey{}, "runtime-final-context")

	result, err := kernel.RunTurn(runCtx, TurnRequest{
		SessionID: "session-kernel-completion", SessionType: SessionTypeWorkspace, Mode: ModeInspect,
		TurnID: "turn-kernel-completion", Input: "explain current state",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("turn status = %q, want blocked from configured completion policy", result.Status)
	}
	session := kernel.sessions.Get("session-kernel-completion")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("current turn is missing")
	}
	var contract FinalContract
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type != agentstate.TurnItemTypeFinalResponse {
			continue
		}
		var payload struct {
			FinalContract FinalContract `json:"finalContract"`
		}
		if json.Unmarshal(item.Payload.Data, &payload) == nil {
			contract = payload.FinalContract
		}
	}
	if contract.Status != FinalContractStatusBlocked || !containsFinalRuntimeCode(contract.Limitations, "completion_policy_deny") {
		t.Fatalf("final contract = %#v, want same configured policy blocker as TurnResult", contract)
	}
	if evaluator.calls != 1 {
		t.Fatalf("completion evaluator calls = %d, want one authoritative finalization decision", evaluator.calls)
	}
	if evaluator.ctxValue != "runtime-final-context" {
		t.Fatalf("completion evaluator context value = %q, want RunTurn context", evaluator.ctxValue)
	}
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatalf("projector = %T, want test event emitter", kernel.projector)
	}
	foundTurnComplete := false
	for _, event := range emitter.events {
		if event.Type == EventTurnComplete && event.TurnID == "turn-kernel-completion" {
			foundTurnComplete = true
		}
	}
	if !foundTurnComplete {
		t.Fatalf("events = %#v, want terminal EventTurnComplete", emitter.events)
	}
	if len(spanSource.failedIDs) != 1 || spanSource.failedIDs[0] != "turn-span-turn-kernel-completion" || len(spanSource.completedIDs) != 0 {
		t.Fatalf("span failed=%#v completed=%#v, want one terminal failure", spanSource.failedIDs, spanSource.completedIDs)
	}
}
