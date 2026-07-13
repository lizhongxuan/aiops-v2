package runtimekernel

import (
	"testing"

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
