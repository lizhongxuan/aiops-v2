package runtimekernel

import (
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestVerificationAgentGateRequiresFreshVerifierReport(t *testing.T) {
	decision := EvaluateVerificationAgentGate(VerificationAgentGateInput{
		RequiresVerifier: true,
		HasFinalClaim:    true,
	})
	if decision.Action != "require_verifier" {
		t.Fatalf("action = %q, want require_verifier: %#v", decision.Action, decision)
	}
}

func TestVerificationAgentGateBlocksFailedVerifier(t *testing.T) {
	decision := EvaluateVerificationAgentGate(VerificationAgentGateInput{
		RequiresVerifier: true,
		HasFinalClaim:    true,
		Report:           &promptinput.VerificationAgentReportTrace{Status: "FAIL", Summary: "countercheck failed"},
	})
	if decision.Action != "block_success_final" {
		t.Fatalf("action = %q, want block_success_final: %#v", decision.Action, decision)
	}
}

func TestVerificationAgentGateAllowsPartialOnlyWithBlockers(t *testing.T) {
	withoutBlockers := EvaluateVerificationAgentGate(VerificationAgentGateInput{
		RequiresVerifier: true,
		HasFinalClaim:    true,
		Report:           &promptinput.VerificationAgentReportTrace{Status: "PARTIAL"},
	})
	if withoutBlockers.Action != "block_success_final" {
		t.Fatalf("action without blockers = %q, want block_success_final", withoutBlockers.Action)
	}
	withBlockers := EvaluateVerificationAgentGate(VerificationAgentGateInput{
		RequiresVerifier: true,
		HasFinalClaim:    true,
		Report:           &promptinput.VerificationAgentReportTrace{Status: "PARTIAL", Blockers: []string{"synthetic tool unavailable"}},
	})
	if withBlockers.Action != "allow" {
		t.Fatalf("action with blockers = %q, want allow", withBlockers.Action)
	}
}
