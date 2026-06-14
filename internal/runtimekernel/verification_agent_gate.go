package runtimekernel

import (
	"strings"

	"aiops-v2/internal/promptinput"
)

type VerificationAgentGateDecision struct {
	Action  string   `json:"action"` // allow | require_verifier | block_success_final
	Reasons []string `json:"reasons,omitempty"`
}

type VerificationAgentGateInput struct {
	RequiresVerifier bool
	HasFinalClaim    bool
	Report           *promptinput.VerificationAgentReportTrace
}

func EvaluateVerificationAgentGate(in VerificationAgentGateInput) VerificationAgentGateDecision {
	if !in.RequiresVerifier || !in.HasFinalClaim {
		return VerificationAgentGateDecision{Action: "allow"}
	}
	if in.Report == nil {
		return VerificationAgentGateDecision{Action: "require_verifier", Reasons: []string{"missing_fresh_context_verifier_report"}}
	}
	switch strings.ToUpper(strings.TrimSpace(in.Report.Status)) {
	case "PASS":
		return VerificationAgentGateDecision{Action: "allow"}
	case "PARTIAL":
		if len(in.Report.Blockers) > 0 {
			return VerificationAgentGateDecision{Action: "allow", Reasons: []string{"partial_verification_with_blockers"}}
		}
		return VerificationAgentGateDecision{Action: "block_success_final", Reasons: []string{"partial_verification_missing_blockers"}}
	case "FAIL":
		return VerificationAgentGateDecision{Action: "block_success_final", Reasons: []string{"verification_failed"}}
	default:
		return VerificationAgentGateDecision{Action: "require_verifier", Reasons: []string{"invalid_verifier_report_status"}}
	}
}
