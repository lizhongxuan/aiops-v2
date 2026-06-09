package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"

	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

type ToolResourceLockRequest struct {
	SessionID string
	TurnID    string
	OwnerID   string
	ToolCall  ToolCall
	Tool      tooling.ToolMetadata
	Keys      []tooling.ToolResourceLockKey
	Context   tooling.ToolExecutionContext
}

type ToolResourceLockDecision struct {
	Action string
	Reason string
	Holder string
}

type ToolResourceLockGate interface {
	AcquireToolResourceLocks(ctx context.Context, req ToolResourceLockRequest) (ToolResourceLockDecision, func(), error)
}

func normalizeToolResourceLockDecision(decision ToolResourceLockDecision) ToolResourceLockDecision {
	decision.Action = strings.TrimSpace(strings.ToLower(decision.Action))
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.Holder = strings.TrimSpace(decision.Holder)
	if decision.Action == "" {
		decision.Action = "acquired"
	}
	return decision
}

func toolResourceLockTrace(req ToolResourceLockRequest, decision ToolResourceLockDecision) []promptinput.ResourceLockTrace {
	if len(req.Keys) == 0 {
		return nil
	}
	decision = normalizeToolResourceLockDecision(decision)
	out := make([]promptinput.ResourceLockTrace, 0, len(req.Keys))
	for _, key := range req.Keys {
		out = append(out, promptinput.ResourceLockTrace{
			LeaseID: "",
			AgentID: req.OwnerID,
			Action:  decision.Action,
			Reason:  decision.Reason,
			Holder:  decision.Holder,
			Key: promptinput.ResourceLockKeyTrace{
				ResourceType:  strings.TrimSpace(key.ResourceType),
				ResourceID:    strings.TrimSpace(key.ResourceID),
				OperationKind: strings.TrimSpace(key.OperationKind),
			},
		})
	}
	return out
}

func resourceLockFailurePayload(toolName string, decision ToolResourceLockDecision) string {
	decision = normalizeToolResourceLockDecision(decision)
	reason := decision.Reason
	if reason == "" {
		reason = "resource_lock_conflict"
	}
	payload := map[string]string{
		"errorType":      "resource_lock_conflict",
		"toolName":       strings.TrimSpace(toolName),
		"reason":         reason,
		"requiredAction": "wait for the current holder to finish, retry later, or split work onto non-overlapping resource keys",
	}
	if decision.Holder != "" {
		payload["holder"] = decision.Holder
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func toolResourceLockOwnerID(sessionID, turnID string, tc ToolCall, execCtx tooling.ToolExecutionContext) string {
	parts := []string{
		strings.TrimSpace(execCtx.SessionID),
		strings.TrimSpace(execCtx.TurnID),
		strings.TrimSpace(execCtx.ToolCallID),
	}
	if parts[0] == "" {
		parts[0] = strings.TrimSpace(sessionID)
	}
	if parts[1] == "" {
		parts[1] = strings.TrimSpace(turnID)
	}
	if parts[2] == "" {
		parts[2] = strings.TrimSpace(tc.ID)
	}
	owner := strings.Join(parts, ":")
	if strings.Trim(owner, ":") == "" {
		return strings.TrimSpace(tc.Name)
	}
	return owner
}
