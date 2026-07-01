package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"

	evidencecore "aiops-v2/internal/evidence"
	"aiops-v2/internal/tooling"
)

func (k *RuntimeKernel) autoRecordToolResultEvidence(ctx context.Context, session *SessionState, turnID string, tc ToolCall, meta tooling.ToolMetadata, result tooling.ToolResult) tooling.ToolResult {
	if k == nil || k.evidenceService == nil {
		return result
	}
	if len(evidenceRefsFromToolResultContent(result.Content)) > 0 {
		return result
	}
	if !meta.RecordEvidence && !toolingResultHasRawRef(result) {
		return result
	}
	sourceTool := firstNonEmpty(meta.Name, tc.Name)
	summary := firstNonEmpty(toolingResultSummary(result), meta.Description, sourceTool+" result")
	rec, err := k.evidenceService.Record(ctx, evidencecore.RecordRequest{
		SourceTool: sourceTool,
		Source:     firstNonEmpty(meta.Domain, contentStringField(result.Content, "source")),
		Kind:       evidencecore.KindOther,
		Summary:    summary,
		Data:       evidenceRecordData(result),
		SessionID:  sessionIDForEvidence(session),
		TurnID:     turnID,
		ToolCallID: tc.ID,
	})
	if err != nil {
		return result
	}
	return appendEvidenceRefsToToolingResult(result, []string{rec.Ref})
}

func sessionIDForEvidence(session *SessionState) string {
	if session == nil {
		return ""
	}
	return session.ID
}

func toolingResultHasRawRef(result tooling.ToolResult) bool {
	if jsonMapHasRawRef(jsonMapFromString(result.Content)) {
		return true
	}
	if result.Display != nil {
		return jsonMapHasRawRef(jsonMapFromBytes(result.Display.Data))
	}
	return false
}

func toolingResultSummary(result tooling.ToolResult) string {
	if value := contentStringField(result.Content, "summary"); value != "" {
		return value
	}
	if value := contentStringField(result.Content, "message"); value != "" {
		return value
	}
	summary, _, _, _, _, _ := summarizeToolLifecycleResultForEvent("", "", result.Content)
	return summary
}

func contentStringField(content, field string) string {
	return stringValueFromAny(jsonMapFromString(content)[field])
}

func evidenceRecordData(result tooling.ToolResult) map[string]any {
	data := map[string]any{}
	if content := jsonMapFromString(result.Content); len(content) > 0 {
		data["content"] = content
	} else if strings.TrimSpace(result.Content) != "" {
		data["content"] = strings.TrimSpace(result.Content)
	}
	if result.Display != nil && len(result.Display.Data) > 0 {
		if display := jsonMapFromBytes(result.Display.Data); len(display) > 0 {
			data["display"] = display
		}
	}
	return data
}

func appendEvidenceRefsToToolingResult(result tooling.ToolResult, refs []string) tooling.ToolResult {
	refs = cleanEvidenceRefs(refs)
	if len(refs) == 0 {
		return result
	}
	if content := jsonMapFromString(result.Content); len(content) > 0 {
		content["evidenceRefs"] = mergeEvidenceRefs(anyStringSlice(content["evidenceRefs"]), refs)
		if data, ok := content["data"].(map[string]any); ok {
			data["evidenceRefs"] = mergeEvidenceRefs(anyStringSlice(data["evidenceRefs"]), refs)
		}
		if encoded, err := json.Marshal(content); err == nil {
			result.Content = string(encoded)
		}
	}
	if result.Display != nil && len(result.Display.Data) > 0 {
		if display := jsonMapFromBytes(result.Display.Data); len(display) > 0 {
			display["evidenceRefs"] = mergeEvidenceRefs(anyStringSlice(display["evidenceRefs"]), refs)
			if encoded, err := json.Marshal(display); err == nil {
				result.Display.Data = encoded
			}
		}
	}
	return result
}

func mergeEvidenceRefs(existing, refs []string) []string {
	return cleanEvidenceRefs(append(existing, refs...))
}

func jsonMapHasRawRef(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	if _, ok := payload["rawRef"]; ok {
		return true
	}
	if rawRefs := anySlice(payload["rawRefs"]); len(rawRefs) > 0 {
		return true
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return jsonMapHasRawRef(data)
	}
	return false
}

func jsonMapFromString(content string) map[string]any {
	content = strings.TrimSpace(content)
	if content == "" || !strings.HasPrefix(content, "{") {
		return nil
	}
	return jsonMapFromBytes([]byte(content))
}

func jsonMapFromBytes(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload
}

func stringValueFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

func anyStringSlice(value any) []string {
	items := anySlice(value)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := stringValueFromAny(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}
