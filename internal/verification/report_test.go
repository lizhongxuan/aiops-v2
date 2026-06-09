package verification

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVerificationReportSchemaUsesStableJSONNames(t *testing.T) {
	report := VerificationReport{
		ID:          "report-synthetic-1",
		PlanID:      "plan-synthetic-1",
		TaskID:      "task-synthetic-1",
		Requirement: VerificationExecutionRequired,
		Status:      StatusPass,
		Subject:     "verify synthetic deployment",
		Evidence: []EvidenceRecord{{
			Kind:     EvidenceExecution,
			Command:  "go test ./internal/verification -v",
			Result:   EvidenceResultPass,
			Expected: "tests pass",
			Actual:   "tests pass",
			RawRef:   "trace-synthetic-1",
		}},
		Probes: []ProbeResult{{
			Type:     ProbeIdempotency,
			Expected: "second run has no drift",
			Actual:   "second run has no drift",
			Result:   EvidenceResultPass,
			RawRef:   "probe-synthetic-1",
		}},
		ContractChecks: []ContractCheck{{
			Name:     "schema_status_enum",
			Checked:  true,
			Expected: "PASS/PARTIAL/FAIL",
			Actual:   "PASS",
			Result:   EvidenceResultPass,
		}},
		RawRefs: []string{"trace-synthetic-1"},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for _, name := range []string{
		"id",
		"planId",
		"taskId",
		"requirement",
		"status",
		"subject",
		"evidence",
		"probes",
		"contractChecks",
		"rawRefs",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("json field %q missing from %s", name, string(data))
		}
	}
}

func TestVerificationReportNormalizeTrimsDeduplicatesAndRedacts(t *testing.T) {
	longSubject := strings.Repeat("a", 700)
	report := VerificationReport{
		ID:          "  report-synthetic-1  ",
		Requirement: " EXECUTION_REQUIRED ",
		Status:      " pass ",
		Subject:     "  " + longSubject + "  ",
		Expected:    " token=synthetic-secret should not appear ",
		Actual:      " password=synthetic-secret should not appear ",
		RawRefs:     []string{" trace-synthetic-1 ", "trace-synthetic-1", "", "sk-synthetic123456789012"},
		Evidence: []EvidenceRecord{{
			Kind:    " EXECUTION ",
			Command: "  go test ./internal/verification -v  ",
			Result:  " PASS ",
			RawRef:  " trace-synthetic-1 ",
			Actual:  "token=synthetic-secret",
		}},
		Probes: []ProbeResult{{
			Type:   " IDEMPOTENCY ",
			Result: " PASS ",
			RawRef: " trace-synthetic-1 ",
		}},
		ContractChecks: []ContractCheck{{
			Name:    " status enum ",
			Checked: true,
			Result:  " PASS ",
			Actual:  "password=synthetic-secret",
		}},
		Blockers: []VerificationBlocker{{
			Reason:     " permission missing ",
			Source:     " PERMISSION ",
			NextAction: " ask user ",
		}},
	}

	normalized := report.Normalize()

	if normalized.ID != "report-synthetic-1" {
		t.Fatalf("ID = %q", normalized.ID)
	}
	if normalized.Requirement != VerificationExecutionRequired {
		t.Fatalf("Requirement = %q", normalized.Requirement)
	}
	if normalized.Status != StatusPass {
		t.Fatalf("Status = %q", normalized.Status)
	}
	if len(normalized.Subject) != maxReportFieldLength {
		t.Fatalf("Subject length = %d, want %d", len(normalized.Subject), maxReportFieldLength)
	}
	if got := normalized.RawRefs; len(got) != 2 || got[0] != "trace-synthetic-1" || got[1] != "[redacted]" {
		t.Fatalf("RawRefs = %#v", got)
	}
	if normalized.Expected != "[redacted]" || normalized.Actual != "[redacted]" {
		t.Fatalf("Expected/Actual not redacted: %q / %q", normalized.Expected, normalized.Actual)
	}
	if normalized.Evidence[0].Kind != EvidenceExecution || normalized.Evidence[0].Result != EvidenceResultPass {
		t.Fatalf("evidence normalized = %#v", normalized.Evidence[0])
	}
	if normalized.Evidence[0].Actual != "[redacted]" {
		t.Fatalf("evidence actual = %q, want redacted", normalized.Evidence[0].Actual)
	}
	if normalized.Probes[0].Type != ProbeIdempotency || normalized.Probes[0].Result != EvidenceResultPass {
		t.Fatalf("probe normalized = %#v", normalized.Probes[0])
	}
	if normalized.ContractChecks[0].Name != "status enum" || normalized.ContractChecks[0].Actual != "[redacted]" {
		t.Fatalf("contract normalized = %#v", normalized.ContractChecks[0])
	}
	if normalized.Blockers[0].Source != BlockerPermission {
		t.Fatalf("blocker source = %q", normalized.Blockers[0].Source)
	}
}

func TestVerificationReportRejectsUnknownStatusAndMissingCoreFields(t *testing.T) {
	cases := []struct {
		name   string
		report VerificationReport
		want   string
	}{
		{
			name: "unknown status",
			report: VerificationReport{
				ID:          "report-synthetic-1",
				Requirement: VerificationAnalysisAllowed,
				Status:      "DONE",
				Subject:     "synthetic read-only check",
				Evidence: []EvidenceRecord{{
					Kind:   EvidenceAnalysis,
					Result: EvidenceResultPass,
				}},
			},
			want: "unknown_status",
		},
		{
			name: "empty subject",
			report: VerificationReport{
				ID:          "report-synthetic-1",
				Requirement: VerificationAnalysisAllowed,
				Status:      StatusPass,
				Evidence: []EvidenceRecord{{
					Kind:   EvidenceAnalysis,
					Result: EvidenceResultPass,
				}},
			},
			want: "missing_subject",
		},
		{
			name: "empty evidence",
			report: VerificationReport{
				ID:          "report-synthetic-1",
				Requirement: VerificationAnalysisAllowed,
				Status:      StatusPass,
				Subject:     "synthetic read-only check",
			},
			want: "missing_evidence",
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
