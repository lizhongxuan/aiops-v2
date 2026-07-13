package runtimekernel

import "testing"

func TestCompletionReadinessIgnoresSafeTerminalAnswerMarkers(t *testing.T) {
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
	}{
		{name: "insufficient_evidence", answer: "insufficient_evidence: metrics evidence is missing, so I cannot confirm health."},
		{name: "user_denied_action", answer: "user_denied_action: approval denied, no command was executed."},
		{name: "tool_unavailable", answer: "tool_unavailable: metrics tool unavailable; next action is to restore that evidence source."},
		{name: "multi_host_partial", answer: "multi_host_partial: host-a completed, some hosts are unknown and need follow-up."},
		{name: "partial_mutation", answer: "partial_mutation: what may have partially executed: service restart request reached the tool; known evidence refs: evidence-1; unknown state: service process state; required post-check: systemctl status."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateCompletionReadiness(snapshot, tc.answer)
			if decision.Action != "block_success_final" || !containsString(decision.Reasons, "missing_coverage_dimension") {
				t.Fatalf("Action = %q reasons=%v, want typed coverage block", decision.Action, decision.Reasons)
			}
		})
	}
}

func TestFinalEvidenceGatePartialMutationRequiresPostCheckFields(t *testing.T) {
	missing := EvaluateSafeTerminalFinal("partial_mutation: command may have partially executed.")
	if missing.Valid || !containsString(missing.Reasons, "partial_mutation_missing_required_fields") {
		t.Fatalf("missing partial mutation fields decision = %#v, want block", missing)
	}

	complete := EvaluateSafeTerminalFinal("partial_mutation: what may have partially executed: package install; known evidence refs: evidence-1; unknown state: package database lock; required post-check: rpm -q package.")
	if !complete.Valid || !containsString(complete.Reasons, "partial_mutation") {
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
		t.Fatal("typed verification decision should allow display commit")
	}
	if !verificationCompletionGateAllowsFinal("partial_mutation: maybe ran.", decision, nil) {
		t.Fatal("answer wording must not change display commit decision")
	}
}
