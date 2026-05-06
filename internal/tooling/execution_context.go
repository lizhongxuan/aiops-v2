package tooling

import (
	"context"
	"encoding/json"
)

type toolExecutionContextKey struct{}

type ToolExecutionContext struct {
	SessionID      string          `json:"sessionId,omitempty"`
	TurnID         string          `json:"turnId,omitempty"`
	ToolCallID     string          `json:"toolCallId,omitempty"`
	ToolName       string          `json:"toolName,omitempty"`
	HostID         string          `json:"hostId,omitempty"`
	IncidentID     string          `json:"incidentId,omitempty"`
	ActionToken    string          `json:"actionToken,omitempty"`
	OriginalInput  json.RawMessage `json:"originalInput,omitempty"`
	SanitizedInput json.RawMessage `json:"sanitizedInput,omitempty"`
}

func ContextWithToolExecution(ctx context.Context, execCtx ToolExecutionContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolExecutionContextKey{}, execCtx)
}

func ToolExecutionContextFrom(ctx context.Context) (ToolExecutionContext, bool) {
	if ctx == nil {
		return ToolExecutionContext{}, false
	}
	execCtx, ok := ctx.Value(toolExecutionContextKey{}).(ToolExecutionContext)
	return execCtx, ok
}
