package promptdiag

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WriteRunMetadata(outDir, historyPath string, diagnosis RunDiagnosis) error {
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return fmt.Errorf("output dir is required")
	}
	metadata := RunMetadata{
		RunID:         diagnosis.RunID,
		Agent:         diagnosis.Agent,
		GeneratedAt:   diagnosis.GeneratedAt,
		ReportPath:    diagnosis.ReportPath,
		BaselinePath:  diagnosis.Baseline,
		Total:         diagnosis.Summary.Total,
		Passed:        diagnosis.Summary.Passed,
		Failed:        diagnosis.Summary.Failed,
		AvgScore:      diagnosis.Summary.AvgScore,
		Better:        diagnosis.Summary.Better,
		Worse:         diagnosis.Summary.Worse,
		Same:          diagnosis.Summary.Same,
		New:           diagnosis.Summary.New,
		Missing:       diagnosis.Summary.Missing,
		PromptChanged: diagnosis.Summary.PromptChanged,
		AvgModelCalls: diagnosis.Summary.AvgModelCalls,
		AvgToolCalls:  diagnosis.Summary.AvgToolCalls,
		AvgIterations: diagnosis.Summary.AvgIterations,
	}
	if err := writeJSON(filepath.Join(outDir, "run-metadata.json"), metadata); err != nil {
		return err
	}
	historyPath = strings.TrimSpace(historyPath)
	if historyPath == "" {
		historyPath = filepath.Join(filepath.Dir(outDir), "history.json")
	}
	if err := os.MkdirAll(filepath.Dir(historyPath), 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	var history []RunMetadata
	if data, err := os.ReadFile(historyPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &history); err != nil {
			return fmt.Errorf("decode history metadata: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read history metadata: %w", err)
	}
	history = append(history, metadata)
	const maxHistory = 200
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	if err := writeJSON(historyPath, history); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "trend.zh.md"), []byte(RenderTrendMarkdown(history)), 0o644)
}

func RenderTrendMarkdown(history []RunMetadata) string {
	var b strings.Builder
	b.WriteString("# Prompt 优化趋势\n\n")
	if len(history) == 0 {
		b.WriteString("暂无历史 run metadata。\n")
		return b.String()
	}
	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}
	b.WriteString("| Run | Agent | Passed | Failed | Avg Score | Worse | Avg Iter | Avg Tool Calls |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, item := range history[start:] {
		fmt.Fprintf(&b, "| `%s` | `%s` | %d/%d | %d | %.2f | %d | %.1f | %.1f |\n",
			fallback(item.RunID, "-"), fallback(item.Agent, "-"), item.Passed, item.Total, item.Failed, item.AvgScore, item.Worse, item.AvgIterations, item.AvgToolCalls)
	}
	return b.String()
}
