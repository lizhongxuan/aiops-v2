package runtimekernel

import (
	"strings"

	"aiops-v2/internal/promptinput"
)

func EvaluateRuntimeAgentFinalGate(finalText string, notifications []promptinput.AgentNotificationTrace) promptinput.AgentFinalGateDecisionTrace {
	text := strings.ToLower(finalText)
	var pending []string
	var unverified []string
	for _, notification := range notifications {
		status := strings.ToLower(strings.TrimSpace(notification.Status))
		referenced := runtimeFinalReferencesAgent(text, notification)
		switch status {
		case "running", "waiting", "idle":
			if referenced && !runtimeFinalStatesPending(text) {
				pending = append(pending, notification.AgentID)
			}
		case "completed":
			continue
		case "":
			if referenced {
				unverified = append(unverified, notification.AgentID)
			}
		default:
			if runtimeFinalReferencesEvidence(text, notification) {
				unverified = append(unverified, notification.AgentID)
			}
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

func runtimeFinalReferencesAgent(text string, notification promptinput.AgentNotificationTrace) bool {
	if notification.AgentID != "" && strings.Contains(text, strings.ToLower(notification.AgentID)) {
		return true
	}
	return runtimeFinalReferencesEvidence(text, notification)
}

func runtimeFinalReferencesEvidence(text string, notification promptinput.AgentNotificationTrace) bool {
	if notification.Summary != "" && strings.Contains(text, strings.ToLower(notification.Summary)) {
		return true
	}
	for _, ref := range notification.ResultRefs {
		if ref != "" && strings.Contains(text, strings.ToLower(ref)) {
			return true
		}
	}
	return false
}

func runtimeFinalStatesPending(text string) bool {
	for _, marker := range []string{"pending", "still running", "not confirmed", "unconfirmed", "尚未确认", "仍在运行"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
