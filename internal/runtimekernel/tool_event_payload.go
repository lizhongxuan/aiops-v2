package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"
)

const inlineToolLifecycleResultBytes = 2 * 1024
const maxToolLifecyclePayloadBytes = 8 * 1024

func summarizeToolLifecycleResultForEvent(turnID, toolCallID, result string) (summary string, resultForEvent string, outputPreview json.RawMessage, rawRef string, resultBytes int, truncated bool) {
	result = strings.TrimSpace(result)
	if result == "" {
		return "", "", nil, "", 0, false
	}
	summary = truncateToolLifecycleSummary(firstToolLifecycleSummaryLine(result), 180)
	resultBytes = len([]byte(result))
	if resultBytes <= inlineToolLifecycleResultBytes {
		outputPreview, _ = json.Marshal(result)
		return summary, result, outputPreview, "", resultBytes, false
	}
	rawRef = rawToolLifecycleRef(turnID, toolCallID)
	truncated = true
	resultForEvent = summary
	preview := truncateToolLifecycleBytes(result, inlineToolLifecycleResultBytes)
	outputPreview, _ = json.Marshal(preview)
	return summary, resultForEvent, outputPreview, rawRef, resultBytes, truncated
}

func rawToolLifecycleRef(turnID, toolCallID string) string {
	turnID = strings.TrimSpace(turnID)
	toolCallID = strings.TrimSpace(toolCallID)
	if turnID == "" || toolCallID == "" {
		return ""
	}
	return fmt.Sprintf("tool-result://%s/%s", turnID, toolCallID)
}

func firstToolLifecycleSummaryLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(value)
}

func truncateToolLifecycleSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	return string([]rune(value)[:limit]) + "..."
}

func truncateToolLifecycleBytes(value string, limit int) string {
	if limit <= 0 || len([]byte(value)) <= limit {
		return value
	}
	var builder strings.Builder
	builder.Grow(limit + 3)
	used := 0
	for _, r := range value {
		part := string(r)
		partLen := len([]byte(part))
		if used+partLen > limit {
			break
		}
		builder.WriteString(part)
		used += partLen
	}
	return strings.TrimSpace(builder.String()) + "..."
}
