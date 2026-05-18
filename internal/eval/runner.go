package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	RunPhase  string
	Priority  string
	// Repetitions runs each case multiple times and aggregates average/min score.
	// Values <= 0 are treated as 1 for backward compatibility.
	Repetitions int
	Metadata    map[string]string

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
	if priority := strings.TrimSpace(r.Priority); priority != "" {
		normalized := normalizePriority(priority)
		if _, ok := allowedPriorities[normalized]; !ok {
			return Report{}, fmt.Errorf("invalid priority filter %q", r.Priority)
		}
		cases = filterCasesByPriority(cases, normalized)
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
	repetitions := r.Repetitions
	if repetitions <= 0 {
		repetitions = 1
	}
	startedAt := time.Now().UTC()
	report := Report{
		RunID:       runID,
		RunPhase:    strings.TrimSpace(r.RunPhase),
		Agent:       strings.TrimSpace(r.AgentName),
		CasesDir:    r.CasesDir,
		OutputDir:   r.OutputDir,
		Repetitions: repetitions,
		Metadata:    cloneStringMap(r.Metadata),
		StartedAt:   startedAt,
		Cases:       make([]CaseScore, 0, len(cases)),
	}

	for _, c := range cases {
		var iterationScores []CaseScore
		for iteration := 1; iteration <= repetitions; iteration++ {
			score, err := r.runCaseIteration(ctx, c, iteration, repetitions)
			if err != nil {
				return Report{}, err
			}
			iterationScores = append(iterationScores, score)
		}
		report.Cases = append(report.Cases, aggregateRepeatedCaseScores(c, iterationScores))
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

func (r Runner) runCaseIteration(ctx context.Context, c Case, iteration, repetitions int) (CaseScore, error) {
	output, runErr := r.Agent.Run(ctx, c)
	caseDir := filepath.Join(r.OutputDir, sanitizePathComponent(c.ID))
	if repetitions > 1 {
		caseDir = filepath.Join(caseDir, strconv.Itoa(iteration))
	}
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		return CaseScore{}, fmt.Errorf("create case output dir %s: %w", caseDir, err)
	}
	answerPath := filepath.Join(caseDir, "answer.txt")
	eventsPath := filepath.Join(caseDir, "agent_events.json")
	toolCallsPath := filepath.Join(caseDir, "tool_calls.json")
	turnItemsPath := filepath.Join(caseDir, "turn_items.json")

	if err := os.WriteFile(answerPath, []byte(output.Answer), 0o644); err != nil {
		return CaseScore{}, fmt.Errorf("write answer for %s: %w", c.ID, err)
	}
	if err := writeJSONFile(eventsPath, output.Events); err != nil {
		return CaseScore{}, fmt.Errorf("write events for %s: %w", c.ID, err)
	}
	if err := writeJSONFile(toolCallsPath, output.ToolCalls); err != nil {
		return CaseScore{}, fmt.Errorf("write tool calls for %s: %w", c.ID, err)
	}
	if err := writeJSONFile(turnItemsPath, output.TurnItems); err != nil {
		return CaseScore{}, fmt.Errorf("write turn items for %s: %w", c.ID, err)
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
	return score, nil
}

func aggregateRepeatedCaseScores(c Case, scores []CaseScore) CaseScore {
	if len(scores) == 0 {
		return CaseScore{CaseID: c.ID, Category: c.Category, RootCauseCategory: c.RootCauseCategory, Priority: c.Priority}
	}
	worst := scores[0]
	total := 0.0
	minScore := math.MaxFloat64
	passed := true
	iterationScores := make([]float64, 0, len(scores))
	artifacts := make([]IterationArtifact, 0, len(scores))
	for idx, score := range scores {
		total += score.Score
		iterationScores = append(iterationScores, score.Score)
		if score.Score < minScore {
			minScore = score.Score
			worst = score
		}
		if !score.Passed {
			passed = false
		}
		artifacts = append(artifacts, IterationArtifact{
			Iteration:     idx + 1,
			Score:         score.Score,
			Passed:        score.Passed,
			AnswerPath:    score.AnswerPath,
			EventsPath:    score.EventsPath,
			ToolCallsPath: score.ToolCallsPath,
			TurnItemsPath: score.TurnItemsPath,
			Error:         score.Error,
		})
	}
	avg := total / float64(len(scores))
	out := worst
	out.CaseID = c.ID
	out.Category = c.Category
	out.RootCauseCategory = c.RootCauseCategory
	out.Priority = c.Priority
	out.Passed = passed
	out.Score = avg
	out.AvgScore = avg
	out.MinScore = minScore
	out.Iterations = len(scores)
	out.IterationScores = iterationScores
	out.IterationArtifacts = artifacts
	if len(scores) == 1 {
		out.AvgScore = out.Score
		out.MinScore = out.Score
		out.Iterations = 1
		out.IterationScores = nil
		out.IterationArtifacts = nil
	}
	return out
}

func filterCasesByPriority(cases []Case, priority string) []Case {
	if strings.TrimSpace(priority) == "" {
		return cases
	}
	priority = normalizePriority(priority)
	filtered := make([]Case, 0, len(cases))
	for _, c := range cases {
		if normalizePriority(c.Priority) == priority {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func summarizeCases(cases []CaseScore) ReportSummary {
	summary := ReportSummary{Total: len(cases)}
	if len(cases) == 0 {
		return summary
	}
	totalScore := 0.0
	totalMinScore := 0.0
	minScore := math.MaxFloat64
	for _, c := range cases {
		if c.Passed {
			summary.Passed++
		}
		totalScore += c.Score
		caseMin := c.Score
		if c.Iterations > 1 {
			caseMin = c.MinScore
		}
		totalMinScore += caseMin
		if caseMin < minScore {
			minScore = caseMin
		}
	}
	summary.Failed = summary.Total - summary.Passed
	summary.AvgScore = totalScore / float64(len(cases))
	summary.LowestScoreAverage = totalMinScore / float64(len(cases))
	if minScore != math.MaxFloat64 {
		summary.MinScore = minScore
	}
	return summary
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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
