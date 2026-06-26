package runtimekernel

import "testing"

func TestUXModelGeneralityAllowsSafeTerminalFinalStates(t *testing.T) {
	snapshot := &TurnSnapshot{
		ID: "turn-safe-terminal",
		Metadata: map[string]string{
			"coverage.requiredDimensions": "logs,metrics",
			"coverage.coveredDimensions":  "logs",
		},
	}
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{name: "insufficient_evidence", answer: "insufficient_evidence: metrics evidence is missing, so I cannot confirm health.", want: "insufficient_evidence"},
		{name: "user_denied_action", answer: "user_denied_action: approval denied, no command was executed.", want: "user_denied_action"},
		{name: "tool_unavailable", answer: "tool_unavailable: metrics tool unavailable; next action is to restore that evidence source.", want: "tool_unavailable"},
		{name: "multi_host_partial", answer: "multi_host_partial: host-a completed, some hosts are unknown and need follow-up.", want: "multi_host_partial"},
		{name: "partial_mutation", answer: "partial_mutation: what may have partially executed: service restart request reached the tool; known evidence refs: evidence-1; unknown state: service process state; required post-check: systemctl status.", want: "partial_mutation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateCompletionReadiness(snapshot, tc.answer)
			if decision.Action != "allow_blocker_final" {
				t.Fatalf("Action = %q reasons=%v, want allow_blocker_final", decision.Action, decision.Reasons)
			}
			if !containsString(decision.Reasons, tc.want) {
				t.Fatalf("Reasons = %v, missing %q", decision.Reasons, tc.want)
			}
		})
	}
}

func TestFinalEvidenceGatePartialMutationRequiresPostCheckFields(t *testing.T) {
	state := FinalEvidenceState{Confidence: FinalEvidenceConfidenceLow}
	missing := VerifyFinalEvidence("partial_mutation: command may have partially executed.", state)
	if missing.Action != FinalEvidenceActionBlock || !containsString(missing.Reasons, "partial_mutation_missing_required_fields") {
		t.Fatalf("missing partial mutation fields decision = %#v, want block", missing)
	}

	complete := VerifyFinalEvidence("partial_mutation: what may have partially executed: package install; known evidence refs: evidence-1; unknown state: package database lock; required post-check: rpm -q package.", state)
	if complete.Action != FinalEvidenceActionAllow || !containsString(complete.Reasons, "partial_mutation") {
		t.Fatalf("complete partial mutation decision = %#v, want allow safe terminal", complete)
	}
}

func TestFinalVerificationGateAllowsSafeTerminalBlockerFinal(t *testing.T) {
	decision := VerificationCompletionDecision{
		Action:  VerificationCompletionActionBlockSuccessFinal,
		Reasons: []string{"missing_verification_report"},
	}
	answer := "tool_unavailable: verification tool unavailable, cannot claim completion; next action is restoring tool access."
	if !verificationCompletionGateAllowsFinal(answer, decision, nil) {
		t.Fatal("safe terminal blocker final should be allowed")
	}
	if verificationCompletionGateAllowsFinal("partial_mutation: maybe ran.", decision, nil) {
		t.Fatal("partial mutation without required fields should not be allowed")
	}
}
