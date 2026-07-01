package promptdiag

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteOutputs(outDir string, diagnosis RunDiagnosis) error {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return fmt.Errorf("output dir is required")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := writeJSON(filepath.Join(outDir, "diagnosis.json"), diagnosis); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "failed-cases.json"), failedCases(diagnosis)); err != nil {
		return err
	}
	files := map[string]string{
		"diagnosis.zh.md":   RenderDiagnosisMarkdown(diagnosis),
		"compare.zh.md":     RenderCompareMarkdown(diagnosis),
		"trace-links.md":    RenderTraceLinksMarkdown(diagnosis),
		"suggestions.zh.md": RenderSuggestionsMarkdown(diagnosis),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(outDir, name), []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func RenderDiagnosisMarkdown(d RunDiagnosis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt Diagnose 报告\n\n")
	fmt.Fprintf(&b, "- Run: `%s`\n", fallback(d.RunID, "unknown"))
	if d.Agent != "" {
		fmt.Fprintf(&b, "- Agent: `%s`\n", d.Agent)
	}
	fmt.Fprintf(&b, "- Generated: `%s`\n", d.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&b, "- Report: `%s`\n", d.ReportPath)
	if d.Baseline != "" {
		fmt.Fprintf(&b, "- Baseline: `%s`\n", d.Baseline)
	}
	fmt.Fprintf(&b, "\n## 总览\n\n")
	fmt.Fprintf(&b, "- 通过率：%d/%d，失败 %d，平均分 %.2f\n", d.Summary.Passed, d.Summary.Total, d.Summary.Failed, d.Summary.AvgScore)
	if d.Summary.Better+d.Summary.Worse+d.Summary.Same+d.Summary.New+d.Summary.Missing > 0 {
		fmt.Fprintf(&b, "- 实验对比：better=%d worse=%d same=%d new=%d missing=%d\n", d.Summary.Better, d.Summary.Worse, d.Summary.Same, d.Summary.New, d.Summary.Missing)
	}
	if d.Summary.BaselineFailed != 0 || d.Summary.BaselineAvgScore != 0 {
		fmt.Fprintf(&b, "- Baseline：failed=%d avgScore=%.2f，failedIncreased=%t avgScoreDropped=%t\n", d.Summary.BaselineFailed, d.Summary.BaselineAvgScore, d.Summary.FailedIncreased, d.Summary.AvgScoreDropped)
	}
	if d.Summary.AvgModelCalls != 0 || d.Summary.AvgToolCalls != 0 || d.Summary.AvgIterations != 0 {
		fmt.Fprintf(&b, "- 运行成本：avgModelCalls=%.1f avgToolCalls=%.1f avgIterations=%.1f\n", d.Summary.AvgModelCalls, d.Summary.AvgToolCalls, d.Summary.AvgIterations)
	}
	fmt.Fprintf(&b, "- Prompt hash 变化：stable=%t developer=%t tools=%t\n", d.Summary.StableHashChanged, d.Summary.DeveloperHashChanged, d.Summary.ToolRegistryHashChanged)
	if len(d.Summary.RootCauseCounts) > 0 {
		fmt.Fprintf(&b, "\n## 根因分布\n\n")
		for _, key := range sortedKeys(d.Summary.RootCauseCounts) {
			fmt.Fprintf(&b, "- `%s`: %d\n", key, d.Summary.RootCauseCounts[key])
		}
	}
	if len(d.Warnings) > 0 {
		fmt.Fprintf(&b, "\n## 诊断警告\n\n")
		for _, warning := range d.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	fmt.Fprintf(&b, "\n## Case 诊断\n\n")
	for _, c := range d.Cases {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "### `%s` %s %.2f\n\n", c.CaseID, status, c.Score)
		if c.Priority != "" || c.Category != "" || c.Movement != "" {
			fmt.Fprintf(&b, "- Meta：priority=`%s` category=`%s` movement=`%s`\n", fallback(c.Priority, "-"), fallback(c.Category, "-"), fallback(c.Movement, "-"))
		}
		fmt.Fprintf(&b, "- Likely root cause：`%s`\n", fallback(c.LikelyRootCause, "-"))
		if len(c.FailedChecks) > 0 {
			fmt.Fprintf(&b, "- Failed checks：`%s`\n", strings.Join(c.FailedChecks, "`, `"))
		}
		if c.Evidence.AnswerCharCount > 0 {
			fmt.Fprintf(&b, "- Answer：chars=%d lines=%d preview=`%s`\n", c.Evidence.AnswerCharCount, c.Evidence.AnswerLineCount, fallback(c.Evidence.AnswerPreview, "-"))
		}
		fmt.Fprintf(&b, "- Evidence：modelCalls=%d toolCalls=%d toolResults=%d failedToolResults=%d promptSize=%d messages=%d userMessage=%t\n",
			c.Evidence.ModelCallCount, c.Evidence.ToolCallCount, c.Evidence.ToolResultCount, c.Evidence.FailedToolResultCount, c.Evidence.PromptSizeChars, c.Evidence.MessageCount, c.Evidence.HasUserMessage)
		if c.Evidence.TraceTurnCount > 0 {
			fmt.Fprintf(&b, "- Trace turns：turns=%d iterations=%d\n", c.Evidence.TraceTurnCount, c.Evidence.TraceIterationCount)
		}
		if c.Evidence.StableHash != "" {
			fmt.Fprintf(&b, "- Fingerprint：stable `%s` developer `%s` tools `%s`\n", shortHash(c.Evidence.StableHash), shortHash(c.Evidence.DeveloperHash), shortHash(c.Evidence.ToolRegistryHash))
		}
		if len(c.RuleHits) > 0 {
			fmt.Fprintf(&b, "\n规则命中：\n\n")
			for _, hit := range c.RuleHits {
				fmt.Fprintf(&b, "- `%s` [%s/%s] %s", hit.RuleID, hit.Severity, hit.RootCause, hit.Message)
				if hit.Evidence != "" {
					fmt.Fprintf(&b, " 证据：%s", hit.Evidence)
				}
				fmt.Fprintln(&b)
			}
		}
		if len(c.Suggestions) > 0 {
			fmt.Fprintf(&b, "\n建议：\n\n")
			for _, suggestion := range c.Suggestions {
				fmt.Fprintf(&b, "- `%s`: %s%s\n", suggestion.Area, suggestion.Action, llmMarker(suggestion))
			}
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func RenderCompareMarkdown(d RunDiagnosis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt Experiment 对比\n\n")
	if d.Summary.Better+d.Summary.Worse+d.Summary.Same+d.Summary.New+d.Summary.Missing == 0 {
		fmt.Fprintf(&b, "没有 baseline/current 对比。运行 `prompt-diagnose -baseline <report.json>` 或让 `agent-eval -baseline` 生成 comparison。\n")
		return b.String()
	}
	fmt.Fprintf(&b, "- better=%d\n- worse=%d\n- same=%d\n- new=%d\n- missing=%d\n", d.Summary.Better, d.Summary.Worse, d.Summary.Same, d.Summary.New, d.Summary.Missing)
	fmt.Fprintf(&b, "\n## 需要关注\n\n")
	for _, c := range d.Cases {
		if c.Movement == "same" || c.Movement == "" {
			continue
		}
		fmt.Fprintf(&b, "- `%s`: `%s`, score=%.2f, rootCause=`%s`", c.CaseID, c.Movement, c.Score, fallback(c.LikelyRootCause, "-"))
		if len(c.FailedChecks) > 0 {
			fmt.Fprintf(&b, ", failed=`%s`", strings.Join(c.FailedChecks, "`, `"))
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func RenderTraceLinksMarkdown(d RunDiagnosis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt Trace Links\n\n")
	if len(d.TraceLinks) == 0 {
		fmt.Fprintf(&b, "没有找到可关联的 model input trace。确认运行时配置页已开启 Debug / Model Input Trace，且 `-trace-dir` 指向正确目录。\n")
		return b.String()
	}
	for _, trace := range d.TraceLinks {
		link := promptTraceDeepLink(trace)
		if link != "" {
			fmt.Fprintf(&b, "- [打开 Prompt Trace](%s) case=`%s` session=`%s` turn=`%s` iteration=%d json=`%s` md=`%s` diff=`%s` stable=`%s`\n",
				link, fallback(trace.CaseID, "-"), fallback(trace.SessionID, "-"), fallback(trace.TurnID, "-"), trace.Iteration, fallback(trace.JSONPath, "-"), fallback(trace.MarkdownPath, "-"), fallback(trace.DiffPath, "-"), shortHash(trace.StableHash))
			continue
		}
		fmt.Fprintf(&b, "- case=`%s` session=`%s` turn=`%s` iteration=%d json=`%s` md=`%s` diff=`%s` stable=`%s`\n",
			fallback(trace.CaseID, "-"), fallback(trace.SessionID, "-"), fallback(trace.TurnID, "-"), trace.Iteration, fallback(trace.JSONPath, "-"), fallback(trace.MarkdownPath, "-"), fallback(trace.DiffPath, "-"), shortHash(trace.StableHash))
	}
	return b.String()
}

func RenderSuggestionsMarkdown(d RunDiagnosis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Prompt 优化建议\n\n")
	if len(d.Suggestions) == 0 {
		fmt.Fprintf(&b, "没有自动建议。若仍有失败，请打开 `diagnosis.zh.md` 中对应 case 的 trace 和 artifacts 人工判断。\n")
		return b.String()
	}
	for _, suggestion := range d.Suggestions {
		fmt.Fprintf(&b, "- `%s`: %s%s", suggestion.Area, suggestion.Action, llmMarker(suggestion))
		if suggestion.Rationale != "" {
			fmt.Fprintf(&b, "  \n  原因：%s", suggestion.Rationale)
		}
		fmt.Fprintln(&b)
	}
	return b.String()
}

func promptTraceDeepLink(trace TraceLink) string {
	path := firstNonEmpty(trace.JSONPath, trace.MarkdownPath)
	if strings.TrimSpace(path) == "" {
		return ""
	}
	query := "trace=" + urlQueryEscape(path)
	if trace.CaseID != "" {
		query += "&caseId=" + urlQueryEscape(trace.CaseID)
	}
	return "/debug/prompts?" + query
}

func llmMarker(suggestion Suggestion) string {
	if suggestion.LLMAssisted {
		return "（llm_assisted=true）"
	}
	return ""
}

func urlQueryEscape(value string) string {
	return url.QueryEscape(value)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func failedCases(d RunDiagnosis) []CaseDiagnosis {
	var out []CaseDiagnosis
	for _, c := range d.Cases {
		if !c.Passed || c.Movement == "worse" {
			out = append(out, c)
		}
	}
	return out
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 16 {
		return value
	}
	return value[:8] + "..." + value[len(value)-6:]
}
