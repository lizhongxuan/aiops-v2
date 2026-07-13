package runtimekernel

import (
	"strings"

	"aiops-v2/internal/promptinput"
)

func EvaluateRuntimeAgentFinalGate(_ string, notifications []promptinput.AgentNotificationTrace) promptinput.AgentFinalGateDecisionTrace {
	var pending []string
	var unverified []string
	for _, notification := range notifications {
		status := strings.ToLower(strings.TrimSpace(notification.Status))
		switch status {
		case "running", "waiting", "idle":
			pending = append(pending, notification.AgentID)
		case "completed":
			continue
		default:
			unverified = append(unverified, notification.AgentID)
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
