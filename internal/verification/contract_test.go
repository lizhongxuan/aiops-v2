package verification

import "testing"

func TestContractReportFailRequiresCheckedContractOrContractUnavailableBlocker(t *testing.T) {
	report := validReport(StatusFail, VerificationExecutionRequired)
	report.ContractChecks = []ContractCheck{{
		Name:     "synthetic checked contract",
		Checked:  true,
		Expected: "operation returns a failed status",
		Actual:   "operation returned success",
		Result:   EvidenceResultFail,
	}}

	decision := ValidateReport(report)
	if !decision.Valid {
		t.Fatalf("Valid = false, reasons = %#v", decision.Reasons)
	}

	report.ContractChecks[0].Checked = false
	decision = ValidateReport(report)
	if decision.Valid {
		t.Fatalf("Valid = true, want false")
	}
	if !hasReason(decision.Reasons, "fail_missing_contract_check") {
		t.Fatalf("Reasons = %#v, want fail_missing_contract_check", decision.Reasons)
	}
}

func TestContractReportPartialRequiresBlockedScope(t *testing.T) {
	report := validReport(StatusPartial, VerificationExecutionRequired)
	report.Blockers = []VerificationBlocker{{
		Reason:     "synthetic tool unavailable",
		Source:     BlockerToolUnavailable,
		NextAction: "retry when tool is available",
	}}

	decision := ValidateReport(report)
	if decision.Valid {
		t.Fatalf("Valid = true, want false")
	}
	if !hasReason(decision.Reasons, "partial_blocker_missing_scope") {
		t.Fatalf("Reasons = %#v, want partial_blocker_missing_scope", decision.Reasons)
	}

	report.Blockers[0].BlockedScope = "synthetic probe execution"
	decision = ValidateReport(report)
	if !decision.Valid {
		t.Fatalf("Valid = false, reasons = %#v", decision.Reasons)
	}
}
