package eval

import (
	"strings"
	"testing"
)

func TestRenderMarkdownReportIncludesFailureDetailsAndFixLogic(t *testing.T) {
	report := Report{
		RunID: "run-1",
		Summary: ReportSummary{
			Total:    1,
			Failed:   1,
			AvgScore: 0.5,
		},
		Cases: []CaseScore{{
			CaseID:       "tool-calling",
			Passed:       false,
			Score:        0.5,
			PassedChecks: 1,
			TotalChecks:  2,
			Checks: []CheckResult{
				{Name: "mustInclude", Passed: true},
				{
					Name:    "expectedToolCalls",
					Passed:  false,
					Detail:  "0/1 expected tools called",
					Missing: []string{"exec_command"},
				},
			},
		}},
	}

	got := RenderMarkdownReport(report)
	for _, want := range []string{
		"Failure details:",
		"`expectedToolCalls`: 0/1 expected tools called",
		"Missing: `exec_command`",
		"Fix logic: make the agent call every expected canonical tool",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("markdown report missing %q:\n%s", want, got)
		}
	}
}
