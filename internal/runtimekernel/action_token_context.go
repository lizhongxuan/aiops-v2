package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"

	"aiops-v2/internal/tooling"
)

func enrichToolExecutionContext(ctx context.Context, sessionID, turnID string, tc ToolCall, desc ToolDescriptor) (context.Context, json.RawMessage) {
	execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
	execCtx.SessionID = sessionID
	execCtx.TurnID = turnID
	execCtx.ToolCallID = tc.ID
	execCtx.ToolName = tc.Name
	execCtx.OriginalInput = append(json.RawMessage(nil), tc.Arguments...)

	token, incidentID, hostID, stripped, hasToken := extractActionToken(tc.Arguments)
	if hasToken {
		execCtx.ActionToken = token
	}
	if execCtx.IncidentID == "" {
		execCtx.IncidentID = incidentID
	}
	if execCtx.HostID == "" {
		execCtx.HostID = hostID
	}
	if len(stripped) == 0 {
		stripped = append(json.RawMessage(nil), tc.Arguments...)
	}
	execCtx.SanitizedInput = append(json.RawMessage(nil), stripped...)
	ctx = tooling.ContextWithToolExecution(ctx, execCtx)
	if hasToken && !schemaDeclaresActionToken(desc.InputSchema) {
		return ctx, stripped
	}
	return ctx, tc.Arguments
}

func extractActionToken(input json.RawMessage) (token string, incidentID string, hostID string, stripped json.RawMessage, hasToken bool) {
	stripped = append(json.RawMessage(nil), input...)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil || obj == nil {
		return "", "", "", stripped, false
	}
	token = stringField(obj, "actionToken")
	incidentID = firstStringField(obj, "incidentId", "incidentID")
	hostID = firstStringField(obj, "hostId", "hostID", "targetHost", "targetHostId")
	if token == "" {
		return "", incidentID, hostID, stripped, false
	}
	delete(obj, "actionToken")
	data, err := json.Marshal(obj)
	if err != nil {
		return token, incidentID, hostID, stripped, true
	}
	return token, incidentID, hostID, data, true
}

func schemaDeclaresActionToken(schema json.RawMessage) bool {
	var obj struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &obj); err != nil {
		return false
	}
	_, ok := obj.Properties["actionToken"]
	return ok
}

func firstStringField(obj map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value := stringField(obj, key); value != "" {
			return value
		}
	}
	return ""
}

func stringField(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}
