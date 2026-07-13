package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/agentruntime"
	"aiops-v2/internal/promptinput"
)

const runtimeAgentFinalGateRetryMetadataKey = "agentFinalGateRetry"

func EvaluateRuntimeAgentFinalGate(_ string, notifications []promptinput.AgentNotificationTrace) promptinput.AgentFinalGateDecisionTrace {
	var pending []string
	var unverified []string
	for _, notification := range notifications {
		status := strings.ToLower(strings.TrimSpace(notification.Status))
		switch status {
		case "completed":
			continue
		case "failed", "cancelled", "killed":
			unverified = append(unverified, notification.AgentID)
		default:
			pending = append(pending, notification.AgentID)
		}
	}
	if len(pending) > 0 {
		return promptinput.AgentFinalGateDecisionTrace{
			Action:        "require_wait",
			PendingAgents: pending,
			Reasons:       []string{"pending_worker_evidence"},
		}
	}
	if len(unverified) > 0 {
		return promptinput.AgentFinalGateDecisionTrace{
			Action:        "remove_unverified_claims",
			PendingAgents: unverified,
			Reasons:       []string{"non_completed_worker_evidence"},
		}
	}
	return promptinput.AgentFinalGateDecisionTrace{Action: "allow"}
}

func runtimeAgentNotifications(snapshot *TurnSnapshot) []promptinput.AgentNotificationTrace {
	latest := map[string]promptinput.AgentNotificationTrace{}
	if snapshot == nil {
		return nil
	}
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			toolName := runtimeAgentResultToolName(iteration, result.ToolCallID)
			var data []byte
			if result.Display != nil && len(result.Display.Data) > 0 {
				data = result.Display.Data
			} else {
				data = []byte(result.Content)
			}
			facts, ok := agentruntime.ParseWorkerStatusContract(toolName, data)
			if !ok {
				continue
			}
			for _, child := range facts {
				id := strings.TrimSpace(child.AgentID)
				status := strings.ToLower(strings.TrimSpace(child.Status))
				if id != "" && status != "" {
					latest[id] = promptinput.AgentNotificationTrace{AgentID: id, Status: status}
				}
			}
		}
	}
	ids := make([]string, 0, len(latest))
	for id := range latest {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]promptinput.AgentNotificationTrace, 0, len(ids))
	for _, id := range ids {
		out = append(out, latest[id])
	}
	return out
}

func runtimeAgentResultToolName(iteration IterationState, toolCallID string) string {
	toolCallID = strings.TrimSpace(toolCallID)
	for _, call := range iteration.ToolCalls {
		if strings.TrimSpace(call.ID) == toolCallID {
			return strings.TrimSpace(call.Name)
		}
	}
	for _, invocation := range iteration.ToolInvocations {
		if strings.TrimSpace(invocation.ToolCallID) == toolCallID {
			return strings.TrimSpace(invocation.ToolName)
		}
	}
	return ""
}

func runtimeAgentFinalGateRetryPrompt(decision promptinput.AgentFinalGateDecisionTrace) string {
	return fmt.Sprintf("## Worker completion gate\nPending worker IDs: %s. Continue the task and use the available orchestration status tool until every worker reaches a typed terminal state before producing a final answer.", strings.Join(decision.PendingAgents, ", "))
}
