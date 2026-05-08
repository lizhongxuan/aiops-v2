package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
)

func appendAgentItem(snapshot *TurnSnapshot, item agentstate.TurnItem) {
	if snapshot == nil {
		return
	}
	state := agentStateFromSnapshot(snapshot)
	next, err := agentstate.AppendItem(state, item)
	if err != nil {
		return
	}
	snapshot.AgentItems = next.Items
	snapshot.UpdatedAt = time.Now()
}

func updateAgentItem(snapshot *TurnSnapshot, itemID string, status agentstate.ItemStatus, summary string) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	state := agentStateFromSnapshot(snapshot)
	next, err := agentstate.UpdateItem(state, itemID, func(item agentstate.TurnItem) (agentstate.TurnItem, error) {
		item.Status = status
		if strings.TrimSpace(summary) != "" {
			item.Payload.Summary = strings.TrimSpace(summary)
		}
		return item, nil
	})
	if err != nil {
		return
	}
	snapshot.AgentItems = next.Items
	snapshot.UpdatedAt = time.Now()
}

func removeAgentItem(snapshot *TurnSnapshot, itemID string) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	next := snapshot.AgentItems[:0]
	for _, item := range snapshot.AgentItems {
		if item.ID != itemID {
			next = append(next, item)
		}
	}
	snapshot.AgentItems = next
	snapshot.UpdatedAt = time.Now()
}

func hasAgentItemID(items []agentstate.TurnItem, itemID string) bool {
	for _, item := range items {
		if item.ID == itemID {
			return true
		}
	}
	return false
}

func agentStateFromSnapshot(snapshot *TurnSnapshot) agentstate.AgentState {
	return agentstate.AgentState{
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.ID,
		Phase:     agentPhaseFromLifecycle(snapshot.Lifecycle),
		Items:     append([]agentstate.TurnItem(nil), snapshot.AgentItems...),
	}
}

func agentPhaseFromLifecycle(lifecycle TurnLifecycleState) agentstate.AgentPhase {
	switch lifecycle {
	case TurnLifecycleCompleted:
		return agentstate.AgentPhaseFinished
	case TurnLifecycleFailed, TurnLifecycleCanceled:
		return agentstate.AgentPhaseFailed
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		return agentstate.AgentPhaseObserving
	default:
		return agentstate.AgentPhaseActing
	}
}

func newAgentItem(id string, typ agentstate.TurnItemType, status agentstate.ItemStatus, summary string, data any) agentstate.TurnItem {
	item := agentstate.TurnItem{
		ID:     id,
		Type:   typ,
		Status: status,
		Payload: agentstate.PayloadEnvelope{
			Summary: strings.TrimSpace(summary),
		},
	}
	if data != nil {
		if raw, err := json.Marshal(data); err == nil {
			item.Payload.Data = raw
		}
	}
	return item
}

func modelCallItemID(turnID string, iteration int) string {
	return fmt.Sprintf("%s-model-%d", strings.TrimSpace(turnID), iteration)
}

func toolCallItemID(turnID string, tc ToolCall) string {
	suffix := strings.TrimSpace(tc.ID)
	if suffix == "" {
		suffix = strings.TrimSpace(tc.Name)
	}
	if suffix == "" {
		suffix = "unknown"
	}
	return fmt.Sprintf("%s-tool-call-%s", strings.TrimSpace(turnID), suffix)
}

func toolResultItemID(turnID string, tc ToolCall) string {
	return toolCallItemID(turnID, tc) + "-result"
}

func errorItemID(turnID string, iteration int) string {
	if iteration >= 0 {
		return fmt.Sprintf("%s-error-%d", strings.TrimSpace(turnID), iteration)
	}
	return fmt.Sprintf("%s-error", strings.TrimSpace(turnID))
}

func planItemFromToolCall(turnID string, tc ToolCall) (agentstate.TurnItem, bool) {
	if !isUpdatePlanToolName(tc.Name) {
		return agentstate.TurnItem{}, false
	}
	plan, err := planning.DecodeUpdatePlan(tc.Arguments)
	if err != nil {
		return agentstate.TurnItem{}, false
	}
	return newAgentItem(
		planItemID(turnID, tc),
		agentstate.TurnItemTypePlan,
		agentstate.ItemStatusCompleted,
		planning.CompactSummary(plan),
		plan,
	), true
}

func planItemID(turnID string, tc ToolCall) string {
	suffix := strings.TrimSpace(tc.ID)
	if suffix == "" {
		suffix = "update_plan"
	}
	return fmt.Sprintf("%s-plan-%s", strings.TrimSpace(turnID), suffix)
}

func isUpdatePlanToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "update_plan", "plan_update", "set_plan":
		return true
	default:
		return false
	}
}

func toolResultItemStatus(result ToolResult) agentstate.ItemStatus {
	if strings.TrimSpace(result.Error) != "" {
		return agentstate.ItemStatusFailed
	}
	return agentstate.ItemStatusCompleted
}
