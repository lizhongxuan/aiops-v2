package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/tooling"
)

type UnexpectedStateSignal struct {
	Status        string `json:"status"`
	SourceTool    string `json:"sourceTool,omitempty"`
	ToolCallID    string `json:"toolCallId,omitempty"`
	ResourceType  string `json:"resourceType,omitempty"`
	ResourceID    string `json:"resourceId,omitempty"`
	ResourcePath  string `json:"resourcePath,omitempty"`
	Summary       string `json:"summary,omitempty"`
	Resolved      bool   `json:"resolved,omitempty"`
	ApprovalScope string `json:"approvalScope,omitempty"`
}

type UnexpectedStateGateDecision struct {
	Action  string   `json:"action"` // allow | require_inspect | require_user_confirm | block_mutation
	Reasons []string `json:"reasons,omitempty"`
}

const (
	UnexpectedStateActionAllow              = "allow"
	UnexpectedStateActionRequireInspect     = "require_inspect"
	UnexpectedStateActionRequireUserConfirm = "require_user_confirm"
	UnexpectedStateActionBlockMutation      = "block_mutation"
)

var unexpectedStateStatuses = map[string]bool{
	"unexpected_state":    true,
	"conflict":            true,
	"drift":               true,
	"precondition_failed": true,
	"stale_assumption":    true,
}

func DetectUnexpectedStateFromToolResult(toolName string, result ToolResult) []UnexpectedStateSignal {
	var out []UnexpectedStateSignal
	out = append(out, detectUnexpectedStateFromJSON(toolName, result.ToolCallID, []byte(result.Content))...)
	if result.Display != nil && len(result.Display.Data) > 0 {
		out = append(out, detectUnexpectedStateFromJSON(toolName, result.ToolCallID, result.Display.Data)...)
	}
	if len(out) == 0 && strings.TrimSpace(result.Error) != "" && containsUnexpectedStateText(result.Error) {
		out = append(out, UnexpectedStateSignal{
			Status:     normalizeUnexpectedStateStatus(result.Error),
			SourceTool: toolName,
			ToolCallID: result.ToolCallID,
			Summary:    truncateString(result.Error, 180),
		})
	}
	return dedupeUnexpectedStateSignals(out)
}

func EvaluateUnexpectedStateGate(signals []UnexpectedStateSignal, tc ToolCall, meta tooling.ToolMetadata) UnexpectedStateGateDecision {
	var unresolved []UnexpectedStateSignal
	for _, signal := range signals {
		if !signal.Resolved && unexpectedStateStatuses[normalizeUnexpectedStateStatus(signal.Status)] {
			unresolved = append(unresolved, signal)
		}
	}
	if len(unresolved) == 0 {
		return UnexpectedStateGateDecision{Action: UnexpectedStateActionAllow}
	}
	governance := meta.EffectiveGovernance(0)
	mutating := governance.Mutating || governance.RequiresApproval || meta.Layer == tooling.ToolLayerMutation
	if !mutating || tooling.IsPlanArtifactTool(firstNonEmpty(tc.Name, meta.Name)) {
		return UnexpectedStateGateDecision{Action: UnexpectedStateActionRequireInspect, Reasons: []string{"unresolved_unexpected_state_requires_inspect_or_plan_update"}}
	}
	reasons := []string{"unresolved_unexpected_state_blocks_mutation"}
	for _, signal := range unresolved {
		if signal.Status != "" {
			reasons = append(reasons, normalizeUnexpectedStateStatus(signal.Status))
		}
	}
	return UnexpectedStateGateDecision{Action: UnexpectedStateActionBlockMutation, Reasons: uniqueRuntimeStrings(reasons)}
}

func collectUnexpectedStateSignalsFromSession(session *SessionState) []UnexpectedStateSignal {
	if session == nil {
		return nil
	}
	var signals []UnexpectedStateSignal
	for _, msg := range session.Messages {
		if msg.ToolResult == nil {
			continue
		}
		toolName := ""
		if msg.Metadata != nil {
			toolName = msg.Metadata["toolName"]
		}
		signals = append(signals, DetectUnexpectedStateFromToolResult(toolName, *msg.ToolResult)...)
	}
	if session.CurrentTurn != nil {
		for _, iter := range session.CurrentTurn.Iterations {
			for _, result := range iter.ToolResults {
				signals = append(signals, DetectUnexpectedStateFromToolResult("", result)...)
			}
		}
	}
	return dedupeUnexpectedStateSignals(signals)
}

func detectUnexpectedStateFromJSON(toolName, toolCallID string, data []byte) []UnexpectedStateSignal {
	if len(data) == 0 || strings.TrimSpace(string(data)) == "" {
		return nil
	}
	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		if containsUnexpectedStateText(string(data)) {
			return []UnexpectedStateSignal{{Status: normalizeUnexpectedStateStatus(string(data)), SourceTool: toolName, ToolCallID: toolCallID, Summary: truncateString(string(data), 180)}}
		}
		return nil
	}
	var out []UnexpectedStateSignal
	walkUnexpectedStatePayload(payload, func(item map[string]any) {
		status := normalizeUnexpectedStateStatus(firstMapString(item, "status", "state", "errorType", "error_type", "condition", "reason"))
		if !unexpectedStateStatuses[status] {
			return
		}
		out = append(out, UnexpectedStateSignal{
			Status:       status,
			SourceTool:   firstNonBlankRuntimeString(toolName, firstMapString(item, "tool", "sourceTool", "source_tool")),
			ToolCallID:   firstNonBlankRuntimeString(toolCallID, firstMapString(item, "toolCallId", "tool_call_id")),
			ResourceType: firstMapString(item, "resourceType", "resource_type", "type", "kind"),
			ResourceID:   firstMapString(item, "resourceId", "resource_id", "id", "target", "name"),
			ResourcePath: firstMapString(item, "resourcePath", "resource_path", "path"),
			Summary:      truncateString(firstMapString(item, "summary", "message", "detail"), 180),
			Resolved:     firstMapBool(item, "resolved"),
		})
	})
	return out
}

func walkUnexpectedStatePayload(value any, visit func(map[string]any)) {
	switch typed := value.(type) {
	case map[string]any:
		visit(typed)
		for _, child := range typed {
			walkUnexpectedStatePayload(child, visit)
		}
	case []any:
		for _, child := range typed {
			walkUnexpectedStatePayload(child, visit)
		}
	}
}

func containsUnexpectedStateText(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for status := range unexpectedStateStatuses {
		if strings.Contains(text, status) || strings.Contains(text, strings.ReplaceAll(status, "_", " ")) {
			return true
		}
	}
	return false
}

func normalizeUnexpectedStateStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	for status := range unexpectedStateStatuses {
		if strings.Contains(value, status) {
			return status
		}
	}
	return value
}

func firstMapString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			case fmt.Stringer:
				if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func firstMapBool(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if typed, ok := value.(bool); ok {
				return typed
			}
		}
	}
	return false
}

func dedupeUnexpectedStateSignals(signals []UnexpectedStateSignal) []UnexpectedStateSignal {
	if len(signals) == 0 {
		return nil
	}
	out := make([]UnexpectedStateSignal, 0, len(signals))
	seen := map[string]bool{}
	for _, signal := range signals {
		if strings.TrimSpace(signal.Status) == "" {
			continue
		}
		key := strings.Join([]string{signal.Status, signal.SourceTool, signal.ToolCallID, signal.ResourceType, signal.ResourceID, signal.ResourcePath}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, signal)
	}
	return out
}

func uniqueRuntimeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
