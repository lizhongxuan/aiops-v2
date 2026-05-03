package promptdiag

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/eval"
)

type artifactBundle struct {
	ToolCalls []eval.ToolCall
	TurnItems []agentstate.TurnItem
	Stats     artifactStats
	Warnings  []string
}

type artifactStats struct {
	AnswerPreview         string
	AnswerCharCount       int
	AnswerLineCount       int
	ToolCalls             []string
	VisibleTools          []string
	TracePaths            []string
	DiffPaths             []string
	FailedToolNames       []string
	ModelCallCount        int
	ToolCallCount         int
	ToolResultCount       int
	FailedToolResultCount int
	PlanCount             int
	EvidenceCount         int
	MaxIterationObserved  int
}

func loadArtifacts(score eval.CaseScore) artifactBundle {
	var out artifactBundle
	if strings.TrimSpace(score.AnswerPath) != "" {
		answer, err := os.ReadFile(score.AnswerPath)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %v", score.AnswerPath, err))
		} else {
			out.Stats.AnswerPreview, out.Stats.AnswerCharCount, out.Stats.AnswerLineCount = summarizeAnswer(string(answer))
		}
	}
	if strings.TrimSpace(score.ToolCallsPath) != "" {
		if err := readJSONFile(score.ToolCallsPath, &out.ToolCalls); err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %v", score.ToolCallsPath, err))
		}
	}
	if strings.TrimSpace(score.TurnItemsPath) != "" {
		if err := readJSONFile(score.TurnItemsPath, &out.TurnItems); err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %v", score.TurnItemsPath, err))
		}
	}
	answerPreview := out.Stats.AnswerPreview
	answerCharCount := out.Stats.AnswerCharCount
	answerLineCount := out.Stats.AnswerLineCount
	out.Stats = summarizeArtifacts(out.ToolCalls, out.TurnItems)
	out.Stats.AnswerPreview = answerPreview
	out.Stats.AnswerCharCount = answerCharCount
	out.Stats.AnswerLineCount = answerLineCount
	return out
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func summarizeArtifacts(toolCalls []eval.ToolCall, items []agentstate.TurnItem) artifactStats {
	var stats artifactStats
	for _, call := range toolCalls {
		name := strings.TrimSpace(call.Name)
		if name != "" {
			stats.ToolCalls = appendUnique(stats.ToolCalls, name)
		}
	}
	for _, item := range items {
		switch item.Type {
		case agentstate.TurnItemTypeModelCall:
			stats.ModelCallCount++
			data := turnItemData(item)
			stats.VisibleTools = appendUnique(stats.VisibleTools, stringSliceValue(data, "visibleTools")...)
			if traceFile := stringValue(data, "traceFile"); traceFile != "" {
				stats.TracePaths = appendUnique(stats.TracePaths, traceFile)
			}
			if traceFile := stringValue(data, "modelInputTraceFile"); traceFile != "" {
				stats.TracePaths = appendUnique(stats.TracePaths, traceFile)
			}
			if diffFile := stringValue(data, "traceDiffFile"); diffFile != "" {
				stats.DiffPaths = appendUnique(stats.DiffPaths, diffFile)
			}
			if iter, ok := intValue(data, "iteration"); ok && iter > stats.MaxIterationObserved {
				stats.MaxIterationObserved = iter
			}
		case agentstate.TurnItemTypeToolCall:
			stats.ToolCallCount++
			data := turnItemData(item)
			if name := firstNonEmpty(stringValue(data, "toolName"), strings.TrimSpace(item.Payload.Summary)); name != "" {
				stats.ToolCalls = appendUnique(stats.ToolCalls, name)
			}
		case agentstate.TurnItemTypeToolResult:
			stats.ToolResultCount++
			data := turnItemData(item)
			name := firstNonEmpty(stringValue(data, "toolName"), strings.TrimSpace(item.Payload.Summary))
			if item.Status == agentstate.ItemStatusFailed {
				stats.FailedToolResultCount++
				if name != "" {
					stats.FailedToolNames = appendUnique(stats.FailedToolNames, name)
				}
			}
		case agentstate.TurnItemTypePlan:
			stats.PlanCount++
		case agentstate.TurnItemTypeEvidence:
			stats.EvidenceCount++
		}
	}
	return stats
}

func summarizeAnswer(answer string) (string, int, int) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", 0, 0
	}
	lines := strings.Split(answer, "\n")
	previewParts := make([]string, 0, 2)
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		previewParts = append(previewParts, line)
		if len(previewParts) >= 2 {
			break
		}
	}
	preview := strings.Join(previewParts, " / ")
	const maxPreview = 180
	if len([]rune(preview)) > maxPreview {
		runes := []rune(preview)
		preview = string(runes[:maxPreview]) + "..."
	}
	return preview, len([]rune(answer)), len(lines)
}

func turnItemData(item agentstate.TurnItem) map[string]json.RawMessage {
	if len(item.Payload.Data) == 0 {
		return nil
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(item.Payload.Data, &data); err != nil {
		return nil
	}
	return data
}

func stringValue(data map[string]json.RawMessage, key string) string {
	if data == nil {
		return ""
	}
	raw, ok := data[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func stringSliceValue(data map[string]json.RawMessage, key string) []string {
	if data == nil {
		return nil
	}
	raw, ok := data[key]
	if !ok {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func intValue(data map[string]json.RawMessage, key string) (int, bool) {
	if data == nil {
		return 0, false
	}
	raw, ok := data[key]
	if !ok {
		return 0, false
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
}
