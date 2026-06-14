package agentmgr

import (
	"context"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

type ToolResourceLockGate struct {
	manager *ResourceLockManager
}

func NewToolResourceLockGate(manager *ResourceLockManager) *ToolResourceLockGate {
	return &ToolResourceLockGate{manager: manager}
}

func (g *ToolResourceLockGate) AcquireToolResourceLocks(_ context.Context, req runtimekernel.ToolResourceLockRequest) (runtimekernel.ToolResourceLockDecision, func(), error) {
	if g == nil || g.manager == nil || len(req.Keys) == 0 {
		return runtimekernel.ToolResourceLockDecision{Action: "acquired"}, nil, nil
	}
	ownerID := strings.TrimSpace(req.OwnerID)
	if ownerID == "" {
		ownerID = strings.TrimSpace(req.ToolCall.ID)
	}
	if ownerID == "" {
		ownerID = strings.TrimSpace(req.ToolCall.Name)
	}
	acquired := make([]ResourceLockKey, 0, len(req.Keys))
	release := func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			g.manager.Release(acquired[i], ownerID)
		}
	}
	for _, key := range req.Keys {
		result, err := g.manager.TryAcquire(resourceLockKeyFromTooling(key), ownerID)
		if err != nil {
			release()
			return runtimekernel.ToolResourceLockDecision{Action: "denied", Reason: err.Error()}, nil, err
		}
		if !result.Acquired {
			release()
			reason := result.Reason
			if reason == "" {
				reason = "resource_lock_conflict"
			}
			return runtimekernel.ToolResourceLockDecision{
				Action: "denied",
				Reason: reason,
				Holder: result.BlockingAgentID,
			}, nil, nil
		}
		acquired = append(acquired, result.Key)
	}
	return runtimekernel.ToolResourceLockDecision{Action: "acquired"}, release, nil
}

func resourceLockKeyFromTooling(key tooling.ToolResourceLockKey) ResourceLockKey {
	return ResourceLockKey{
		ResourceType:  key.ResourceType,
		ResourceID:    key.ResourceID,
		OperationKind: key.OperationKind,
	}
}

var _ runtimekernel.ToolResourceLockGate = (*ToolResourceLockGate)(nil)
