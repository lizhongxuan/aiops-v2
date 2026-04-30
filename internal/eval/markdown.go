package eval

import (
	"fmt"
	"strings"
)

// RenderMarkdownReport renders a compact human-readable eval report.
func RenderMarkdownReport(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Eval Report `%s`\n\n", report.RunID)
	fmt.Fprintf(&b, "- Summary: %d/%d passed, avg score %.2f\n", report.Summary.Passed, report.Summary.Total, report.Summary.AvgScore)
	if report.Agent != "" {
		fmt.Fprintf(&b, "- Agent: `%s`\n", report.Agent)
	}
	if report.OutputDir != "" {
		fmt.Fprintf(&b, "- Output: `%s`\n", report.OutputDir)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "## Cases")
	for _, c := range report.Cases {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "- `%s` %s %.2f (%d/%d checks)\n", c.CaseID, status, c.Score, c.PassedChecks, c.TotalChecks)
		if c.Error != "" {
			fmt.Fprintf(&b, "  Error: %s\n", c.Error)
		}
	}
	if report.BaselineComparison != nil {
		s := report.BaselineComparison.Summary
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Baseline Comparison")
		fmt.Fprintf(&b, "- better=%d worse=%d same=%d new=%d missing=%d\n", s.Better, s.Worse, s.Same, s.New, s.Missing)
		for _, c := range report.BaselineComparison.Cases {
			if c.Status == ComparisonSame && len(c.RegressedChecks) == 0 && len(c.ImprovedChecks) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- `%s`: %s %.2f -> %.2f (delta %.2f)\n", c.CaseID, c.Status, c.BaselineScore, c.CurrentScore, c.Delta)
			if len(c.RegressedChecks) > 0 {
				fmt.Fprintf(&b, "  Regressed checks: `%s`\n", strings.Join(c.RegressedChecks, "`, `"))
			}
			if len(c.ImprovedChecks) > 0 {
				fmt.Fprintf(&b, "  Improved checks: `%s`\n", strings.Join(c.ImprovedChecks, "`, `"))
			}
		}
	}
	return b.String()
}
