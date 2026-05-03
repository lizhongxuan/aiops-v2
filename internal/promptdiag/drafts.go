package promptdiag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/eval"
)

// WriteDraftCases creates local eval-case drafts for failed or regressed cases.
// It reuses eval.DraftCaseFromRunOutput so this is not a second case generator.
func WriteDraftCases(ctx context.Context, cfg Config, diagnosis RunDiagnosis, outDir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	outDir = strings.TrimSpace(outDir)
	if outDir == "" {
		return nil, nil
	}
	report, err := eval.LoadReport(cfg.ReportPath)
	if err != nil {
		return nil, err
	}
	casesByID, _ := loadCasesByID(cfg.CasesDir)
	selected := selectedDraftCaseIDs(diagnosis)
	if len(selected) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create draft cases dir: %w", err)
	}
	reportScores := caseScoreMap(report.Cases)
	var written []string
	for _, caseID := range selected {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		score, ok := reportScores[caseID]
		if !ok {
			continue
		}
		source := casesByID[caseID]
		answer, _ := readTextFile(score.AnswerPath)
		toolCalls, _ := readToolCallsFile(score.ToolCallsPath)
		turnItems, _ := readTurnItemsFile(score.TurnItemsPath)
		draft := eval.DraftCaseFromRunOutput(eval.DraftCaseInput{
			ID:       caseID,
			Category: firstNonEmpty(source.Category, score.Category, "agent-debug"),
			Input:    source.Input,
			Output: eval.RunOutput{
				Answer:    answer,
				ToolCalls: toolCalls,
				TurnItems: turnItems,
			},
		})
		if source.RootCauseCategory != "" {
			draft.Case.RootCauseCategory = source.RootCauseCategory
		}
		if source.Priority != "" {
			draft.Case.Priority = source.Priority
		}
		path := filepath.Join(outDir, sanitizeFileName(caseID)+".json")
		if err := writeDraft(path, draft); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

func selectedDraftCaseIDs(diagnosis RunDiagnosis) []string {
	var out []string
	for _, c := range diagnosis.Cases {
		if !c.Passed || c.Movement == eval.ComparisonWorse {
			out = appendUnique(out, c.CaseID)
		}
	}
	return out
}

func readTextFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readToolCallsFile(path string) ([]eval.ToolCall, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var calls []eval.ToolCall
	if err := readJSONFile(path, &calls); err != nil {
		return nil, err
	}
	return calls, nil
}

func readTurnItemsFile(path string) ([]agentstate.TurnItem, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var items []agentstate.TurnItem
	if err := readJSONFile(path, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func writeDraft(path string, draft eval.DraftCaseResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create draft dir: %w", err)
	}
	data, err := json.MarshalIndent(draft.Case, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal draft case: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write draft case: %w", err)
	}
	if err := os.WriteFile(path+".draft.md", []byte(draft.SidecarMarkdown), 0o644); err != nil {
		return fmt.Errorf("write draft sidecar: %w", err)
	}
	return nil
}
