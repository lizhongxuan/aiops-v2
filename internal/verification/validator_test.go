package verification

import "testing"

func TestValidateVerificationReportExecutionRequiredPassNeedsExecutionEvidence(t *testing.T) {
	report := validReport(StatusPass, VerificationExecutionRequired)
	report.Evidence = []EvidenceRecord{{
		Kind:   EvidenceAnalysis,
		Result: EvidenceResultPass,
		RawRef: "analysis-synthetic-1",
	}}

	decision := ValidateReport(report)
	if decision.Valid {
		t.Fatalf("Valid = true, want false")
	}
	if !hasReason(decision.Reasons, "missing_execution_evidence") {
		t.Fatalf("Reasons = %#v, want missing_execution_evidence", decision.Reasons)
	}
}

func TestValidateVerificationReportAnalysisAllowedPassCanUseAnalysisEvidence(t *testing.T) {
	report := validReport(StatusPass, VerificationAnalysisAllowed)
	report.Evidence = []EvidenceRecord{{
		Kind:   EvidenceAnalysis,
		Result: EvidenceResultPass,
		RawRef: "analysis-synthetic-1",
	}}

	decision := ValidateReport(report)
	if !decision.Valid {
		t.Fatalf("Valid = false, reasons = %#v", decision.Reasons)
	}
}

func TestValidateVerificationReportPassRejectsFailedProbeContractAndBlocker(t *testing.T) {
	cases := []struct {
		name   string
		report VerificationReport
		want   string
	}{
		{
			name: "failed probe",
			report: withReportMutation(validReport(StatusPass, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Probes = []ProbeResult{{
					Type:   ProbeBoundary,
					Result: EvidenceResultFail,
					RawRef: "probe-synthetic-1",
				}}
			}),
			want: "pass_has_failed_probe",
		},
		{
			name: "failed contract",
			report: withReportMutation(validReport(StatusPass, VerificationExecutionRequired), func(report *VerificationReport) {
				report.ContractChecks = []ContractCheck{{
					Name:    "synthetic contract",
					Checked: true,
					Result:  EvidenceResultFail,
				}}
			}),
			want: "pass_has_failed_contract",
		},
		{
			name: "unresolved blocker",
			report: withReportMutation(validReport(StatusPass, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Blockers = []VerificationBlocker{{
					Reason:     "permission missing",
					Source:     BlockerPermission,
					NextAction: "ask user",
				}}
			}),
			want: "pass_has_blocker",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := ValidateReport(tc.report)
			if decision.Valid {
				t.Fatalf("Valid = true, want false")
			}
			if !hasReason(decision.Reasons, tc.want) {
				t.Fatalf("Reasons = %#v, want %q", decision.Reasons, tc.want)
			}
		})
	}
}

func TestValidateVerificationReportPartialRequiresAllowedBlocker(t *testing.T) {
	cases := []struct {
		name   string
		report VerificationReport
		want   string
	}{
		{
			name:   "missing blocker",
			report: validReport(StatusPartial, VerificationExecutionRequired),
			want:   "partial_missing_blocker",
		},
		{
			name: "unknown source",
			report: withReportMutation(validReport(StatusPartial, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Blockers = []VerificationBlocker{{
					Reason:     "synthetic blocker",
					Source:     "policy",
					NextAction: "ask user",
				}}
			}),
			want: "partial_invalid_blocker_source",
		},
		{
			name: "missing next action",
			report: withReportMutation(validReport(StatusPartial, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Blockers = []VerificationBlocker{{
					Reason: "synthetic blocker",
					Source: BlockerPermission,
				}}
			}),
			want: "partial_blocker_missing_next_action",
		},
		{
			name: "allowed contract blocker",
			report: withReportMutation(validReport(StatusPartial, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Blockers = []VerificationBlocker{{
					Reason:       "contract source unavailable",
					Source:       BlockerContract,
					BlockedScope: "contract verification",
					NextAction:   "ask user for source contract",
				}}
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := ValidateReport(tc.report)
			if tc.want == "" {
				if !decision.Valid {
					t.Fatalf("Valid = false, reasons = %#v", decision.Reasons)
				}
				return
			}
			if decision.Valid {
				t.Fatalf("Valid = true, want false")
			}
			if !hasReason(decision.Reasons, tc.want) {
				t.Fatalf("Reasons = %#v, want %q", decision.Reasons, tc.want)
			}
		})
	}
}

func TestValidateVerificationReportFailRequiresExpectedActualAndFailedEvidence(t *testing.T) {
	cases := []struct {
		name   string
		report VerificationReport
		want   string
	}{
		{
			name: "missing expected actual",
			report: withReportMutation(validReport(StatusFail, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Expected = ""
				report.Actual = ""
				report.Evidence = []EvidenceRecord{{
					Kind:   EvidenceExecution,
					Result: EvidenceResultFail,
					RawRef: "trace-synthetic-1",
				}}
				report.ContractChecks = []ContractCheck{{
					Name:    "synthetic contract",
					Checked: true,
					Result:  EvidenceResultFail,
				}}
			}),
			want: "fail_missing_expected_actual",
		},
		{
			name: "missing raw ref",
			report: withReportMutation(validReport(StatusFail, VerificationExecutionRequired), func(report *VerificationReport) {
				report.RawRefs = nil
				report.Evidence = []EvidenceRecord{{
					Kind:   EvidenceExecution,
					Result: EvidenceResultFail,
				}}
				report.ContractChecks = []ContractCheck{{
					Name:    "synthetic contract",
					Checked: true,
					Result:  EvidenceResultFail,
				}}
			}),
			want: "fail_missing_raw_ref",
		},
		{
			name: "missing failed evidence or contract",
			report: withReportMutation(validReport(StatusFail, VerificationExecutionRequired), func(report *VerificationReport) {
				report.Evidence = []EvidenceRecord{{
					Kind:   EvidenceExecution,
					Result: EvidenceResultPass,
					RawRef: "trace-synthetic-1",
				}}
				report.ContractChecks = nil
			}),
			want: "fail_missing_failed_evidence",
		},
		{
			name: "missing checked contract",
			report: withReportMutation(validReport(StatusFail, VerificationExecutionRequired), func(report *VerificationReport) {
				report.ContractChecks = nil
			}),
			want: "fail_missing_contract_check",
		},
		{
			name: "contract unavailable blocker allowed",
			report: withReportMutation(validReport(StatusFail, VerificationExecutionRequired), func(report *VerificationReport) {
				report.ContractChecks = nil
				report.Blockers = []VerificationBlocker{{
					Reason:       "no contract source was provided",
					Source:       BlockerContract,
					BlockedScope: "contract verification",
					NextAction:   "ask user for the contract source",
				}}
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := ValidateReport(tc.report)
			if tc.want == "" {
				if !decision.Valid {
					t.Fatalf("Valid = false, reasons = %#v", decision.Reasons)
				}
				return
			}
			if decision.Valid {
				t.Fatalf("Valid = true, want false")
			}
			if !hasReason(decision.Reasons, tc.want) {
				t.Fatalf("Reasons = %#v, want %q", decision.Reasons, tc.want)
			}
		})
	}
}

func validReport(status ReportStatus, requirement VerificationRequirement) VerificationReport {
	report := VerificationReport{
		ID:          "report-synthetic-1",
		Requirement: requirement,
		Status:      status,
		Subject:     "synthetic verification subject",
		Expected:    "synthetic expected result",
		Actual:      "synthetic actual result",
		RawRefs:     []string{"trace-synthetic-1"},
		Evidence: []EvidenceRecord{{
			Kind:   EvidenceExecution,
			Result: EvidenceResultPass,
			RawRef: "trace-synthetic-1",
		}},
	}
	if status == StatusFail {
		report.Evidence[0].Result = EvidenceResultFail
		report.ContractChecks = []ContractCheck{{
			Name:     "synthetic contract",
			Checked:  true,
			Expected: "synthetic expected result",
			Actual:   "synthetic actual result",
			Result:   EvidenceResultFail,
		}}
	}
	return report
}

func withReportMutation(report VerificationReport, mutate func(*VerificationReport)) VerificationReport {
	mutate(&report)
	return report
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
