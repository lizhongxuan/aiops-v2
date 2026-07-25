package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func ScoreWorkflowGenerationReport(report Report, cases []Case, options WorkflowGenerationScoreOptions) (Report, error) {
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	if mode == "" || mode == "auto" {
		mode = "recorded"
	}
	if mode != "recorded" && mode != "live" {
		return report, fmt.Errorf("unsupported workflow eval mode %q", options.Mode)
	}
	if mode == "live" && report.Repetitions < 3 {
		return report, fmt.Errorf("live workflow eval requires at least 3 repetitions")
	}
	byID := map[string]Case{}
	for _, c := range cases {
		if c.WorkflowGeneration != nil {
			byID[c.ID] = c
		}
	}
	wr := WorkflowGenerationReport{SchemaVersion: WorkflowGenerationReportSchemaVersion, Mode: mode, Repetitions: report.Repetitions, Gate: WorkflowGenerationGate{FinalValidThreshold: WorkflowFinalValidThreshold, FirstPreviewValidThreshold: WorkflowFirstPreviewValidThreshold}}
	var allScores []float64
	var preview, validate, feedback []int64
	totals := WorkflowEvalMetrics{}
	attemptsN := 0
	for i := range report.Cases {
		c, ok := byID[report.Cases[i].CaseID]
		if !ok {
			continue
		}
		attempts := c.WorkflowGeneration.RecordedAttempts
		if mode == "live" {
			var err error
			attempts, err = loadLiveWorkflowAttempts(report.Cases[i])
			if err != nil {
				return report, err
			}
		}
		score, err := ScoreWorkflowGenerationCase(c.ID, *c.WorkflowGeneration, attempts)
		if err != nil {
			return report, err
		}
		wr.Cases = append(wr.Cases, score)
		report.Cases[i].Passed = score.Passed
		report.Cases[i].Score = score.MeanScore
		report.Cases[i].AvgScore = score.MeanScore
		report.Cases[i].MinScore = minFloat64(score.AttemptScores)
		report.Cases[i].Iterations = len(attempts)
		report.Cases[i].IterationScores = score.AttemptScores
		report.Cases[i].Checks = score.Checks
		report.Cases[i].TotalChecks = len(score.Checks)
		report.Cases[i].PassedChecks = countPassed(score.Checks)
		wr.Metrics.PlaceholderCount += score.PlaceholderCount
		wr.Metrics.FirstPreviewValidRate += float64(score.FirstPreviewValid)
		wr.Metrics.FinalValidRate += float64(score.FinalValid)
		wr.Metrics.MeanScore += sum(score.AttemptScores)
		attemptsN += len(attempts)
		allScores = append(allScores, score.AttemptScores...)
		failureStart := len(wr.FailureSamples)
		for n, a := range attempts {
			if c.WorkflowGeneration.PerformanceFixture == "W4" {
				preview = append(preview, a.Metrics.PreviewMS)
				validate = append(validate, a.Metrics.ValidateMS)
				feedback = append(feedback, a.Metrics.FirstFeedbackMS)
			}
			totals.InputTokens += a.Metrics.InputTokens
			totals.OutputTokens += a.Metrics.OutputTokens
			totals.ContextTokens += a.Metrics.ContextTokens
			if a.Metrics.RepairCount > wr.Metrics.MaxRepairCount {
				wr.Metrics.MaxRepairCount = a.Metrics.RepairCount
			}
			if !workflowGraphTextBoundary(a).Passed {
				wr.Metrics.GraphCopiesInTextContent++
			}
			checks, _ := scoreWorkflowAttempt(*c.WorkflowGeneration, a)
			if !allWorkflowChecksPass(checks) {
				wr.FailureSamples = append(wr.FailureSamples, WorkflowGenerationFailureSample{CaseID: c.ID, Iteration: n + 1, FailedChecks: failedWorkflowChecks(checks)})
			}
		}
		if !score.Passed && len(wr.FailureSamples) == failureStart {
			wr.FailureSamples = append(wr.FailureSamples, WorkflowGenerationFailureSample{CaseID: c.ID, Iteration: 0, FailedChecks: failedWorkflowChecks(score.Checks)})
		}
	}
	eligible := 0
	for _, c := range wr.Cases {
		eligible += c.RateEligible
	}
	if eligible > 0 {
		wr.Metrics.FirstPreviewValidRate /= float64(eligible)
		wr.Metrics.FinalValidRate /= float64(eligible)
	}
	if attemptsN > 0 {
		wr.Metrics.MeanScore /= float64(attemptsN)
		wr.Metrics.MeanInputTokens = float64(totals.InputTokens) / float64(attemptsN)
		wr.Metrics.MeanOutputTokens = float64(totals.OutputTokens) / float64(attemptsN)
		wr.Metrics.MeanContextTokens = float64(totals.ContextTokens) / float64(attemptsN)
	}
	_, wr.Metrics.ScoreVariance = meanVariance(allScores)
	wr.Metrics.PreviewP95MS = p95(preview)
	wr.Metrics.ValidateP95MS = p95(validate)
	wr.Metrics.FirstFeedbackP95MS = p95(feedback)
	if eligible > 0 {
		if wr.Metrics.FinalValidRate < WorkflowFinalValidThreshold {
			wr.Gate.Failures = append(wr.Gate.Failures, "final_valid_rate")
		}
		if wr.Metrics.FirstPreviewValidRate < WorkflowFirstPreviewValidThreshold {
			wr.Gate.Failures = append(wr.Gate.Failures, "first_preview_valid_rate")
		}
	}
	if wr.Metrics.PlaceholderCount > 0 {
		wr.Gate.Failures = append(wr.Gate.Failures, "placeholder_count")
	}
	if wr.Metrics.GraphCopiesInTextContent > 0 {
		wr.Gate.Failures = append(wr.Gate.Failures, "graph_text_boundary")
	}
	for _, c := range wr.Cases {
		if !c.Passed {
			wr.Gate.Failures = append(wr.Gate.Failures, "case:"+c.CaseID)
		}
	}
	wr.Gate.Failures = uniqueStrings(wr.Gate.Failures)
	wr.Gate.Passed = len(wr.Gate.Failures) == 0
	report.WorkflowGeneration = &wr
	report.Summary = summarizeCases(report.Cases)
	return report, nil
}

func loadLiveWorkflowAttempts(c CaseScore) ([]WorkflowGenerationAttempt, error) {
	artifacts := c.IterationArtifacts
	if len(artifacts) == 0 {
		artifacts = []IterationArtifact{{Iteration: 1, AnswerPath: c.AnswerPath, EventsPath: c.EventsPath, ToolCallsPath: c.ToolCallsPath, TurnItemsPath: c.TurnItemsPath}}
	}
	out := make([]WorkflowGenerationAttempt, 0, len(artifacts))
	for _, a := range artifacts {
		attempt, ok := findWorkflowAttemptInFiles(a.AnswerPath, a.TurnItemsPath, a.EventsPath, a.ToolCallsPath)
		if !ok {
			out = append(out, WorkflowGenerationAttempt{
				SchemaVersion: WorkflowGenerationOutputSchemaVersion,
				FirstPreview:  WorkflowEvalSnapshot{Validation: WorkflowEvalValidation{Status: "invalid", Valid: false, Diagnostics: []string{"versioned workflow eval output not found"}}},
				Final:         WorkflowEvalSnapshot{Validation: WorkflowEvalValidation{Status: "invalid", Valid: false, Diagnostics: []string{"versioned workflow eval output not found"}}, Risk: "unknown"},
			})
			continue
		}
		out = append(out, attempt)
	}
	return out, nil
}
func findWorkflowAttemptInFiles(paths ...string) (WorkflowGenerationAttempt, bool) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var value any
		if json.Unmarshal(data, &value) == nil {
			if attempt, ok := findWorkflowAttempt(value); ok {
				return attempt, true
			}
		}
	}
	return WorkflowGenerationAttempt{}, false
}
func findWorkflowAttempt(value any) (WorkflowGenerationAttempt, bool) {
	if object, ok := value.(map[string]any); ok {
		if object["schemaVersion"] == WorkflowGenerationOutputSchemaVersion {
			data, _ := json.Marshal(object)
			var a WorkflowGenerationAttempt
			if json.Unmarshal(data, &a) == nil {
				return a, true
			}
		}
		for _, child := range object {
			if a, ok := findWorkflowAttempt(child); ok {
				return a, true
			}
		}
	}
	if array, ok := value.([]any); ok {
		for _, child := range array {
			if a, ok := findWorkflowAttempt(child); ok {
				return a, true
			}
		}
	}
	return WorkflowGenerationAttempt{}, false
}

var workflowCredentialJSON = regexp.MustCompile(`(?i)("(?:authorization|proxy-authorization|password|passwd|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|secret)"\s*:\s*")[^"]*(")`)
var workflowBearer = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/-]+`)
var workflowCredentialText = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|passwd)\s*[=:]\s*[^\s,"'}&]+`)

func RedactWorkflowEvalText(text string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 6 {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	text = workflowCredentialJSON.ReplaceAllString(text, "${1}[REDACTED]${2}")
	text = workflowCredentialText.ReplaceAllString(text, "${1}=[REDACTED]")
	return workflowBearer.ReplaceAllString(text, "${1}[REDACTED]")
}
func RedactWorkflowEvalArtifacts(report Report, secrets []string) error {
	seen := map[string]bool{}
	for _, c := range report.Cases {
		paths := []string{c.AnswerPath, c.EventsPath, c.ToolCallsPath, c.TurnItemsPath}
		for _, a := range c.IterationArtifacts {
			paths = append(paths, a.AnswerPath, a.EventsPath, a.ToolCallsPath, a.TurnItemsPath)
		}
		for _, path := range paths {
			if path == "" || seen[path] {
				continue
			}
			seen[path] = true
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(RedactWorkflowEvalText(string(data), secrets)), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}
func RedactWorkflowEvalReport(report Report, secrets []string) Report {
	for i := range report.Cases {
		report.Cases[i].Error = RedactWorkflowEvalText(report.Cases[i].Error, secrets)
		for n := range report.Cases[i].IterationArtifacts {
			report.Cases[i].IterationArtifacts[n].Error = RedactWorkflowEvalText(report.Cases[i].IterationArtifacts[n].Error, secrets)
		}
		for n := range report.Cases[i].Checks {
			report.Cases[i].Checks[n].Detail = RedactWorkflowEvalText(report.Cases[i].Checks[n].Detail, secrets)
			report.Cases[i].Checks[n].Missing = redactWorkflowStrings(report.Cases[i].Checks[n].Missing, secrets)
			report.Cases[i].Checks[n].Unexpected = redactWorkflowStrings(report.Cases[i].Checks[n].Unexpected, secrets)
		}
	}
	if report.Metadata != nil {
		for key, value := range report.Metadata {
			report.Metadata[key] = RedactWorkflowEvalText(value, secrets)
		}
	}
	return report
}

func redactWorkflowStrings(values []string, secrets []string) []string {
	for i := range values {
		values[i] = RedactWorkflowEvalText(values[i], secrets)
	}
	return values
}
func WorkflowEvalSecretValues(environ []string) []string {
	var out []string
	for _, entry := range environ {
		key, value, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if value != "" && (strings.Contains(upper, "SECRET") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "API_KEY")) {
			out = append(out, value)
		}
	}
	return out
}
