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
		meta := ""
		if c.RootCauseCategory != "" {
			meta = fmt.Sprintf(" rootCause=%s", c.RootCauseCategory)
		}
		if c.Priority != "" {
			meta += fmt.Sprintf(" priority=%s", c.Priority)
		}
		fmt.Fprintf(&b, "- `%s` %s %.2f (%d/%d checks)%s\n", c.CaseID, status, c.Score, c.PassedChecks, c.TotalChecks, meta)
		if c.Error != "" {
			fmt.Fprintf(&b, "  Error: %s\n", c.Error)
		}
		renderFailedChecks(&b, c)
		if len(c.PromptFingerprints) > 0 {
			last := c.PromptFingerprints[len(c.PromptFingerprints)-1]
			if stable := strings.TrimSpace(last["stableHash"]); stable != "" {
				fmt.Fprintf(&b, "  Prompt stable hash: `%s`\n", stable)
			}
			if developer := strings.TrimSpace(last["developerHash"]); developer != "" {
				fmt.Fprintf(&b, "  Developer hash: `%s`\n", developer)
			}
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

func renderFailedChecks(b *strings.Builder, c CaseScore) {
	if c.Passed {
		return
	}
	var failed []CheckResult
	for _, check := range c.Checks {
		if !check.Passed {
			failed = append(failed, check)
		}
	}
	if len(failed) == 0 {
		return
	}
	fmt.Fprintln(b, "  Failure details:")
	for _, check := range failed {
		detail := strings.TrimSpace(check.Detail)
		if detail == "" {
			detail = "check failed"
		}
		fmt.Fprintf(b, "  - `%s`: %s\n", check.Name, detail)
		if len(check.Missing) > 0 {
			fmt.Fprintf(b, "    Missing: %s\n", markdownInlineList(check.Missing))
		}
		if len(check.Unexpected) > 0 {
			fmt.Fprintf(b, "    Unexpected: %s\n", markdownInlineList(check.Unexpected))
		}
		if hint := failedCheckFixHint(check.Name); hint != "" {
			fmt.Fprintf(b, "    Fix logic: %s\n", hint)
		}
	}
}

func markdownInlineList(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, fmt.Sprintf("`%s`", strings.ReplaceAll(value, "`", "'")))
	}
	return strings.Join(out, ", ")
}

func failedCheckFixHint(name string) string {
	switch name {
	case "mustInclude":
		return "make the answer explicitly cover every missing required phrase; if the phrase depends on local implementation, gather local evidence before answering."
	case "mustNotInclude":
		return "remove forbidden wording from the answer; for simple user-facing answers, avoid leaking internal tool names or unnecessary implementation detail."
	case "expectedToolCalls":
		return "make the agent call every expected canonical tool; use maxToolCalls when extra tools should be treated as inefficient."
	case "expectedTurnItems":
		return "ensure runtime emits the expected TurnItem, or remove the expectation when the case input cannot trigger that runtime state."
	case "planPresence", "expectedPlanStatuses":
		return "for tasks that explicitly require structured planning, call the planning tool and keep step statuses such as in_progress visible."
	case "expectedApprovals":
		return "route approval-sensitive work through the approval event path instead of only describing the rule."
	case "expectedEvidence":
		return "capture evidence TurnItems from tool results before finalizing."
	case "mustMentionFiles":
		return "cite the missing local file path in the answer, ideally after verifying it with a read-only local inspection."
	case "hasVerification":
		return "add an explicit verification command, smoke check, or concrete confirmation method."
	default:
		return ""
	}
}
