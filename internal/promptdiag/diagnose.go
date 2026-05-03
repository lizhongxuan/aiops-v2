package promptdiag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/eval"
)

func Diagnose(ctx context.Context, cfg Config) (RunDiagnosis, error) {
	if err := ctx.Err(); err != nil {
		return RunDiagnosis{}, err
	}
	reportPath := strings.TrimSpace(cfg.ReportPath)
	if reportPath == "" {
		return RunDiagnosis{}, fmt.Errorf("report path is required")
	}
	report, err := eval.LoadReport(reportPath)
	if err != nil {
		return RunDiagnosis{}, err
	}
	var baseline *eval.Report
	if strings.TrimSpace(cfg.BaselinePath) != "" {
		base, err := eval.LoadReport(cfg.BaselinePath)
		if err != nil {
			return RunDiagnosis{}, fmt.Errorf("load baseline: %w", err)
		}
		baseline = &base
	}
	comparison := report.BaselineComparison
	if comparison == nil && baseline != nil {
		diff := eval.CompareReports(*baseline, report)
		comparison = &diff
	}
	casesByID, warnings := loadCasesByID(cfg.CasesDir)
	traceIndex, err := BuildTraceIndex(cfg.TraceDir)
	if err != nil {
		return RunDiagnosis{}, err
	}
	generatedAt := cfg.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	diagnosis := RunDiagnosis{
		RunID:       report.RunID,
		Agent:       report.Agent,
		ReportPath:  reportPath,
		Baseline:    strings.TrimSpace(cfg.BaselinePath),
		CasesDir:    strings.TrimSpace(cfg.CasesDir),
		TraceDir:    traceIndex.RootDir,
		GeneratedAt: generatedAt,
		Summary: DiagnosisSummary{
			Total:           report.Summary.Total,
			Passed:          report.Summary.Passed,
			Failed:          report.Summary.Failed,
			AvgScore:        report.Summary.AvgScore,
			RootCauseCounts: map[string]int{},
		},
		Warnings: warnings,
	}
	if comparison != nil {
		diagnosis.Summary.Better = comparison.Summary.Better
		diagnosis.Summary.Worse = comparison.Summary.Worse
		diagnosis.Summary.Same = comparison.Summary.Same
		diagnosis.Summary.New = comparison.Summary.New
		diagnosis.Summary.Missing = comparison.Summary.Missing
	}
	if baseline != nil {
		diagnosis.Summary.BaselineFailed = baseline.Summary.Failed
		diagnosis.Summary.BaselineAvgScore = baseline.Summary.AvgScore
		diagnosis.Summary.FailedIncreased = report.Summary.Failed > baseline.Summary.Failed
		diagnosis.Summary.AvgScoreDropped = report.Summary.AvgScore < baseline.Summary.AvgScore
	}
	movementByID := movementMap(comparison)
	baselineByID := caseScoreMap(nil)
	if baseline != nil {
		baselineByID = caseScoreMap(baseline.Cases)
	}
	for _, score := range report.Cases {
		if err := ctx.Err(); err != nil {
			return RunDiagnosis{}, err
		}
		expected := casesByID[score.CaseID]
		artifacts := loadArtifacts(score)
		caseDiag := diagnoseCase(score, expected, artifacts, traceIndex, movementByID[score.CaseID], baselineByID[score.CaseID], cfg.PromptSizeWarning)
		diagnosis.Cases = append(diagnosis.Cases, caseDiag)
		diagnosis.TraceLinks = append(diagnosis.TraceLinks, caseDiag.Evidence.TraceFiles...)
		for _, suggestion := range caseDiag.Suggestions {
			diagnosis.Suggestions = appendSuggestion(diagnosis.Suggestions, suggestion)
		}
		if root := strings.TrimSpace(caseDiag.LikelyRootCause); root != "" {
			diagnosis.Summary.RootCauseCounts[root]++
		}
	}
	diagnosis.Summary.AvgModelCalls, diagnosis.Summary.AvgToolCalls, diagnosis.Summary.AvgIterations = averageCaseOperationalStats(diagnosis.Cases)
	diagnosis.Summary.StableHashChanged = fingerprintChanged(baseline, report, "stableHash")
	diagnosis.Summary.DeveloperHashChanged = fingerprintChanged(baseline, report, "developerHash")
	diagnosis.Summary.ToolRegistryHashChanged = fingerprintChanged(baseline, report, "toolRegistryHash")
	diagnosis.Summary.PromptChanged = diagnosis.Summary.StableHashChanged || diagnosis.Summary.DeveloperHashChanged || diagnosis.Summary.ToolRegistryHashChanged
	if cfg.LLMSuggestions {
		suggestions, err := GenerateLLMSuggestions(ctx, cfg, diagnosis)
		if err != nil {
			diagnosis.Warnings = append(diagnosis.Warnings, "llm suggestions: "+err.Error())
		} else {
			for _, suggestion := range suggestions {
				diagnosis.Suggestions = appendSuggestion(diagnosis.Suggestions, suggestion)
			}
		}
	}
	return diagnosis, nil
}

func loadCasesByID(casesDir string) (map[string]eval.Case, []string) {
	out := map[string]eval.Case{}
	casesDir = strings.TrimSpace(casesDir)
	if casesDir == "" {
		return out, nil
	}
	cases, err := eval.LoadCases(casesDir)
	if err != nil {
		return out, []string{fmt.Sprintf("load cases: %v", err)}
	}
	for _, c := range cases {
		out[c.ID] = c
	}
	return out, nil
}

func diagnoseCase(score eval.CaseScore, expected eval.Case, artifacts artifactBundle, traces TraceIndex, movement string, baselineScore eval.CaseScore, promptSizeWarning int) CaseDiagnosis {
	evidence := EvidenceSummary{
		AnswerPath:            score.AnswerPath,
		AnswerPreview:         artifacts.Stats.AnswerPreview,
		AnswerCharCount:       artifacts.Stats.AnswerCharCount,
		AnswerLineCount:       artifacts.Stats.AnswerLineCount,
		EventsPath:            score.EventsPath,
		ToolCallsPath:         score.ToolCallsPath,
		TurnItemsPath:         score.TurnItemsPath,
		ToolCalls:             artifacts.Stats.ToolCalls,
		VisibleTools:          artifacts.Stats.VisibleTools,
		ExpectedTools:         cleanStrings(expected.Expected.ExpectedToolCalls),
		PromptFingerprints:    score.PromptFingerprints,
		ModelCallCount:        artifacts.Stats.ModelCallCount,
		ToolCallCount:         artifacts.Stats.ToolCallCount,
		ToolResultCount:       artifacts.Stats.ToolResultCount,
		FailedToolResultCount: artifacts.Stats.FailedToolResultCount,
		FailedToolNames:       artifacts.Stats.FailedToolNames,
		PlanCount:             artifacts.Stats.PlanCount,
		EvidenceCount:         artifacts.Stats.EvidenceCount,
		MaxIterationObserved:  artifacts.Stats.MaxIterationObserved,
		Error:                 score.Error,
	}
	if last := lastFingerprint(score.PromptFingerprints); last != nil {
		evidence.StableHash = strings.TrimSpace(last["stableHash"])
		evidence.DeveloperHash = strings.TrimSpace(last["developerHash"])
		evidence.ToolRegistryHash = strings.TrimSpace(last["toolRegistryHash"])
	}
	evidence.MissingExpectedTools = missingStrings(evidence.ExpectedTools, evidence.ToolCalls)
	tracePaths := append([]string(nil), artifacts.Stats.TracePaths...)
	for _, diff := range artifacts.Stats.DiffPaths {
		if diff != "" {
			tracePaths = append(tracePaths, diff)
		}
	}
	for _, path := range tracePaths {
		if trace, ok := traces.Lookup(path); ok {
			trace.CaseID = score.CaseID
			evidence.TraceFiles = appendTrace(evidence.TraceFiles, trace)
		}
	}
	if len(evidence.TraceFiles) == 0 {
		for _, trace := range traces.FindByCaseID(score.CaseID) {
			trace.CaseID = score.CaseID
			evidence.TraceFiles = appendTrace(evidence.TraceFiles, trace)
		}
	}
	if len(evidence.TraceFiles) == 0 {
		for _, trace := range traces.FindByFingerprint(score.PromptFingerprints) {
			trace.CaseID = firstNonEmpty(trace.CaseID, score.CaseID)
			evidence.TraceFiles = appendTrace(evidence.TraceFiles, trace)
		}
	}
	for _, trace := range evidence.TraceFiles {
		evidence.VisibleTools = appendUnique(evidence.VisibleTools, trace.VisibleTools...)
		if trace.HasUserMessage {
			evidence.HasUserMessage = true
		}
		if trace.MessageCount > evidence.MessageCount {
			evidence.MessageCount = trace.MessageCount
		}
		if trace.PromptSizeChars > evidence.PromptSizeChars {
			evidence.PromptSizeChars = trace.PromptSizeChars
		}
	}
	evidence.TraceTurns = traceTurnSummaries(evidence.TraceFiles)
	evidence.TraceTurnCount = len(evidence.TraceTurns)
	evidence.TraceIterationCount = traceIterationCount(evidence.TraceTurns)
	checks := failedChecks(score)
	caseDiag := CaseDiagnosis{
		CaseID:       score.CaseID,
		Category:     firstNonEmpty(score.Category, expected.Category),
		Priority:     firstNonEmpty(score.Priority, expected.Priority),
		Passed:       score.Passed,
		Score:        score.Score,
		Movement:     movement,
		FailedChecks: checks,
		Evidence:     evidence,
		Artifacts: map[string]string{
			"answer":    score.AnswerPath,
			"events":    score.EventsPath,
			"toolCalls": score.ToolCallsPath,
			"turnItems": score.TurnItemsPath,
		},
	}
	caseDiag.RuleHits = applyRules(score, expected, evidence, movement, baselineScore, promptSizeWarning)
	caseDiag.LikelyRootCause = likelyRootCause(caseDiag.RuleHits, score.Passed, movement)
	caseDiag.Suggestions = suggestionsFor(caseDiag.RuleHits)
	return caseDiag
}

func averageCaseOperationalStats(cases []CaseDiagnosis) (float64, float64, float64) {
	if len(cases) == 0 {
		return 0, 0, 0
	}
	var modelCalls, toolCalls, iterations int
	for _, c := range cases {
		modelCalls += c.Evidence.ModelCallCount
		toolCalls += c.Evidence.ToolCallCount
		if c.Evidence.TraceIterationCount > 0 {
			iterations += c.Evidence.TraceIterationCount
		} else if c.Evidence.MaxIterationObserved > 0 || c.Evidence.ModelCallCount > 0 {
			iterations += c.Evidence.MaxIterationObserved + 1
		}
	}
	count := float64(len(cases))
	return float64(modelCalls) / count, float64(toolCalls) / count, float64(iterations) / count
}

func movementMap(comparison *eval.ComparisonReport) map[string]string {
	out := map[string]string{}
	if comparison == nil {
		return out
	}
	for _, c := range comparison.Cases {
		out[c.CaseID] = c.Status
	}
	return out
}

func caseScoreMap(scores []eval.CaseScore) map[string]eval.CaseScore {
	out := map[string]eval.CaseScore{}
	for _, score := range scores {
		out[score.CaseID] = score
	}
	return out
}

func failedChecks(score eval.CaseScore) []string {
	var out []string
	for _, check := range score.Checks {
		if !check.Passed {
			out = append(out, check.Name)
		}
	}
	return out
}

func lastFingerprint(fps []map[string]string) map[string]string {
	if len(fps) == 0 {
		return nil
	}
	return fps[len(fps)-1]
}

func fingerprintChanged(baseline *eval.Report, current eval.Report, key string) bool {
	if baseline == nil {
		return false
	}
	baseValues := map[string]string{}
	for _, c := range baseline.Cases {
		if fp := lastFingerprint(c.PromptFingerprints); fp != nil {
			baseValues[c.CaseID] = fp[key]
		}
	}
	for _, c := range current.Cases {
		if fp := lastFingerprint(c.PromptFingerprints); fp != nil {
			if base, ok := baseValues[c.CaseID]; ok && strings.TrimSpace(base) != "" && strings.TrimSpace(fp[key]) != "" && base != fp[key] {
				return true
			}
		}
	}
	return false
}
