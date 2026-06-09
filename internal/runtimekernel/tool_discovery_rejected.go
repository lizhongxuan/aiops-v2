package runtimekernel

import (
	"encoding/json"
	"strings"
	"time"
)

func recordRejectedToolCallFromDispatch(session *SessionState, turnID string, tc ToolCall, result DispatchResult, now time.Time) {
	if session == nil || strings.TrimSpace(result.Error) == "" {
		return
	}
	call, ok := rejectedToolCallFromDispatchError(turnID, tc, result)
	if !ok {
		return
	}
	session.ToolDiscovery.AddRejectedCall(call, now)
}

func rejectedToolCallFromDispatchError(turnID string, tc ToolCall, result DispatchResult) (DeferredToolRejectedCall, bool) {
	var payload struct {
		ErrorType            string `json:"errorType"`
		ToolName             string `json:"toolName"`
		Reason               string `json:"reason"`
		RequiredAction       string `json:"requiredAction"`
		SuggestedSearchQuery string `json:"suggestedSearchQuery"`
	}
	if err := json.Unmarshal([]byte(result.Error), &payload); err != nil {
		return DeferredToolRejectedCall{}, false
	}
	payload.ErrorType = strings.TrimSpace(payload.ErrorType)
	if !isRecoverableToolDiscoveryError(payload.ErrorType) {
		return DeferredToolRejectedCall{}, false
	}
	toolName := strings.TrimSpace(payload.ToolName)
	if toolName == "" {
		toolName = strings.TrimSpace(tc.Name)
	}
	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = strings.TrimSpace(result.Reason)
	}
	if reason == "" {
		reason = strings.TrimSpace(result.Error)
	}
	return DeferredToolRejectedCall{
		ToolName:             toolName,
		ErrorType:            payload.ErrorType,
		Reason:               reason,
		RequiredAction:       strings.TrimSpace(payload.RequiredAction),
		SuggestedSearchQuery: strings.TrimSpace(payload.SuggestedSearchQuery),
		TurnID:               strings.TrimSpace(turnID),
		ToolCallID:           strings.TrimSpace(tc.ID),
	}, true
}

func isRecoverableToolDiscoveryError(errorType string) bool {
	switch strings.TrimSpace(errorType) {
	case "tool_unloaded", "tool_hidden_by_policy", "tool_not_found", "dedicated_tool_preferred":
		return true
	default:
		return false
	}
}
