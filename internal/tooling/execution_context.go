package tooling

import (
	"context"
	"encoding/json"

	"aiops-v2/internal/runtimecontract"
)

type toolExecutionContextKey struct{}

// VerifiedApprovalContext contains only facts projected by RuntimeKernel after
// an ActionToken has been verified against the current tool call. Tool input
// and transport metadata must never populate this value.
type VerifiedApprovalContext struct {
	ApprovalID    string   `json:"approvalId"`
	TurnID        string   `json:"turnId"`
	ToolCallID    string   `json:"toolCallId"`
	ToolName      string   `json:"toolName"`
	ArgumentsHash string   `json:"argumentsHash"`
	TargetRefs    []string `json:"targetRefs,omitempty"`
	BindingHash   string   `json:"-"`
}

type ToolExecutionContext struct {
	SessionID        string                                 `json:"sessionId,omitempty"`
	TurnID           string                                 `json:"turnId,omitempty"`
	TenantID         string                                 `json:"tenantId,omitempty"`
	UserID           string                                 `json:"userId,omitempty"`
	ToolCallID       string                                 `json:"toolCallId,omitempty"`
	ToolName         string                                 `json:"toolName,omitempty"`
	HostID           string                                 `json:"hostId,omitempty"`
	IncidentID       string                                 `json:"incidentId,omitempty"`
	ActionToken      string                                 `json:"actionToken,omitempty"`
	OriginalInput    json.RawMessage                        `json:"originalInput,omitempty"`
	SanitizedInput   json.RawMessage                        `json:"sanitizedInput,omitempty"`
	Metadata         map[string]string                      `json:"metadata,omitempty"`
	TurnArtifacts    []runtimecontract.TurnArtifactEnvelope `json:"-"`
	VerifiedApproval *VerifiedApprovalContext               `json:"-"`
}

func ContextWithToolExecution(ctx context.Context, execCtx ToolExecutionContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	execCtx.TurnArtifacts = cloneToolExecutionArtifacts(execCtx.TurnArtifacts)
	execCtx.VerifiedApproval = cloneVerifiedApprovalContext(execCtx.VerifiedApproval)
	if artifactContext, err := runtimecontract.ContextWithTurnArtifacts(ctx, execCtx.TurnArtifacts); err == nil {
		ctx = artifactContext
	} else {
		execCtx.TurnArtifacts = nil
	}
	return context.WithValue(ctx, toolExecutionContextKey{}, execCtx)
}

func ToolExecutionContextFrom(ctx context.Context) (ToolExecutionContext, bool) {
	if ctx == nil {
		return ToolExecutionContext{}, false
	}
	execCtx, ok := ctx.Value(toolExecutionContextKey{}).(ToolExecutionContext)
	execCtx.TurnArtifacts = cloneToolExecutionArtifacts(execCtx.TurnArtifacts)
	execCtx.VerifiedApproval = cloneVerifiedApprovalContext(execCtx.VerifiedApproval)
	return execCtx, ok
}

func cloneVerifiedApprovalContext(value *VerifiedApprovalContext) *VerifiedApprovalContext {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.TargetRefs = append([]string(nil), value.TargetRefs...)
	return &cloned
}

func cloneToolExecutionArtifacts(values []runtimecontract.TurnArtifactEnvelope) []runtimecontract.TurnArtifactEnvelope {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]runtimecontract.TurnArtifactEnvelope, len(values))
	for index := range values {
		cloned[index] = values[index].Clone()
	}
	return cloned
}
