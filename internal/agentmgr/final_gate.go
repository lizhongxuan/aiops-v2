package agentmgr

import "strings"

type AgentNotification struct {
	AgentID    string     `json:"agentId"`
	Status     string     `json:"status"`
	Summary    string     `json:"summary,omitempty"`
	ResultRefs []string   `json:"resultRefs,omitempty"`
	Usage      AgentUsage `json:"usage,omitempty"`
	Error      string     `json:"error,omitempty"`
}

type AgentUsage struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	ToolCalls    int `json:"toolCalls,omitempty"`
}

type AgentFinalGateAction string

const (
	AgentFinalGateAllow                  AgentFinalGateAction = "allow"
	AgentFinalGateRequireWait            AgentFinalGateAction = "require_wait"
	AgentFinalGateRemoveUnverifiedClaims AgentFinalGateAction = "remove_unverified_claims"
)

type AgentFinalGateDecision struct {
	Action        AgentFinalGateAction `json:"action"`
	PendingAgents []string             `json:"pendingAgents,omitempty"`
	Reasons       []string             `json:"reasons,omitempty"`
}

type FinalGateInput struct {
	FinalText string
	Agents    []AgentNotification
}

func EvaluateAgentFinalGate(in FinalGateInput) AgentFinalGateDecision {
	text := strings.ToLower(in.FinalText)
	var pending []string
	var unverified []string
	for _, agent := range in.Agents {
		status := AgentStatus(agent.Status)
		referenced := referencesAgent(text, agent)
		if status == AgentStatusRunning || status == AgentStatusWaiting || status == AgentStatusIdle {
			if referenced && !statesUnconfirmed(text) {
				pending = append(pending, agent.AgentID)
			}
			continue
		}
		if status != AgentStatusCompleted && referencesEvidence(text, agent) {
			unverified = append(unverified, agent.AgentID)
		}
	}
	if len(pending) > 0 {
		return AgentFinalGateDecision{Action: AgentFinalGateRequireWait, PendingAgents: pending, Reasons: []string{"pending_worker_referenced_as_evidence"}}
	}
	if len(unverified) > 0 {
		return AgentFinalGateDecision{Action: AgentFinalGateRemoveUnverifiedClaims, PendingAgents: unverified, Reasons: []string{"non_completed_worker_evidence_referenced"}}
	}
	return AgentFinalGateDecision{Action: AgentFinalGateAllow}
}

func referencesAgent(text string, agent AgentNotification) bool {
	if agent.AgentID != "" && strings.Contains(text, strings.ToLower(agent.AgentID)) {
		return true
	}
	return referencesEvidence(text, agent)
}

func referencesEvidence(text string, agent AgentNotification) bool {
	if agent.Summary != "" && strings.Contains(text, strings.ToLower(agent.Summary)) {
		return true
	}
	for _, ref := range agent.ResultRefs {
		if ref != "" && strings.Contains(text, strings.ToLower(ref)) {
			return true
		}
	}
	return false
}

func statesUnconfirmed(text string) bool {
	for _, marker := range []string{"still running", "not confirmed", "unconfirmed", "pending", "尚未确认", "仍在运行"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
