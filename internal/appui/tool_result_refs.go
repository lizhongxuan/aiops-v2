package appui

import (
	"encoding/json"
	"strings"
)

const inlineToolResultBytes = 2 * 1024
const maxAgentEventPayloadBytes = 8 * 1024

func summarizeToolResultForEvent(turnID, toolCallID, result string) (summary string, preview json.RawMessage, rawRef string) {
	result = strings.TrimSpace(result)
	if result == "" {
		return "", nil, ""
	}
	summary = truncateAgentEventSummary(firstAgentSummaryLine(result), 180)
	size := len([]byte(result))
	if size <= inlineToolResultBytes {
		preview, _ = json.Marshal(result)
		return summary, preview, ""
	}
	rawRef = rawRefForAgentTool(turnID, toolCallID)
	previewText := truncateAgentEventBytes(result, inlineToolResultBytes)
	preview, _ = json.Marshal(previewText)
	return summary, preview, rawRef
}

func truncateAgentEventBytes(value string, limit int) string {
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
