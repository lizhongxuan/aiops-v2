package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Runner loads cases, runs the configured agent, saves artifacts, and emits a report.
type Runner struct {
	CasesDir  string
	OutputDir string
	Agent     Agent
	AgentName string
	RunID     string

	BaselineReport *Report
}

// Run executes every case in CasesDir and writes artifacts to OutputDir.
func (r Runner) Run(ctx context.Context) (Report, error) {
	if r.Agent == nil {
		return Report{}, fmt.Errorf("agent is required")
	}
	if strings.TrimSpace(r.CasesDir) == "" {
		return Report{}, fmt.Errorf("cases dir is required")
	}
	if strings.TrimSpace(r.OutputDir) == "" {
		return Report{}, fmt.Errorf("output dir is required")
	}
	cases, err := LoadCases(r.CasesDir)
	if err != nil {
		return Report{}, err
	}
	if len(cases) == 0 {
		return Report{}, fmt.Errorf("no eval cases found in %s", r.CasesDir)
	}
	if err := os.MkdirAll(r.OutputDir, 0o755); err != nil {
		return Report{}, fmt.Errorf("create output dir: %w", err)
	}

	runID := strings.TrimSpace(r.RunID)
	if runID == "" {
		runID = time.Now().UTC().Format("20060102T150405Z")
	}
	startedAt := time.Now().UTC()
	report := Report{
		RunID:     runID,
		Agent:     strings.TrimSpace(r.AgentName),
		CasesDir:  r.CasesDir,
		OutputDir: r.OutputDir,
		StartedAt: startedAt,
		Cases:     make([]CaseScore, 0, len(cases)),
	}

	for _, c := range cases {
		output, runErr := r.Agent.Run(ctx, c)
		caseDir := filepath.Join(r.OutputDir, sanitizePathComponent(c.ID))
		if err := os.MkdirAll(caseDir, 0o755); err != nil {
			return Report{}, fmt.Errorf("create case output dir %s: %w", caseDir, err)
		}
		answerPath := filepath.Join(caseDir, "answer.txt")
		eventsPath := filepath.Join(caseDir, "agent_events.json")
		toolCallsPath := filepath.Join(caseDir, "tool_calls.json")
		turnItemsPath := filepath.Join(caseDir, "turn_items.json")

		if err := os.WriteFile(answerPath, []byte(output.Answer), 0o644); err != nil {
			return Report{}, fmt.Errorf("write answer for %s: %w", c.ID, err)
		}
		if err := writeJSONFile(eventsPath, output.Events); err != nil {
			return Report{}, fmt.Errorf("write events for %s: %w", c.ID, err)
		}
		if err := writeJSONFile(toolCallsPath, output.ToolCalls); err != nil {
			return Report{}, fmt.Errorf("write tool calls for %s: %w", c.ID, err)
		}
		if err := writeJSONFile(turnItemsPath, output.TurnItems); err != nil {
			return Report{}, fmt.Errorf("write turn items for %s: %w", c.ID, err)
		}

		score := ScoreCase(c, output)
		score.AnswerPath = answerPath
		score.EventsPath = eventsPath
		score.ToolCallsPath = toolCallsPath
		score.TurnItemsPath = turnItemsPath
		if runErr != nil {
			score.Passed = false
			score.Error = runErr.Error()
		}
		report.Cases = append(report.Cases, score)
	}

	report.CompletedAt = time.Now().UTC()
	report.Summary = summarizeCases(report.Cases)
	if r.BaselineReport != nil {
		comparison := CompareReports(*r.BaselineReport, report)
		report.BaselineComparison = &comparison
	}
	if err := SaveReport(filepath.Join(r.OutputDir, "report.json"), report); err != nil {
		return Report{}, err
	}
	if err := os.WriteFile(filepath.Join(r.OutputDir, "report.md"), []byte(RenderMarkdownReport(report)), 0o644); err != nil {
		return Report{}, fmt.Errorf("write markdown report: %w", err)
	}
	return report, nil
}

func summarizeCases(cases []CaseScore) ReportSummary {
	summary := ReportSummary{Total: len(cases)}
	if len(cases) == 0 {
		return summary
	}
	totalScore := 0.0
	for _, c := range cases {
		if c.Passed {
			summary.Passed++
		}
		totalScore += c.Score
	}
	summary.Failed = summary.Total - summary.Passed
	summary.AvgScore = totalScore / float64(len(cases))
	return summary
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

var unsafePathComponent = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizePathComponent(value string) string {
	value = unsafePathComponent.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "case"
	}
	return value
}
