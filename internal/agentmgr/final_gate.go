package agentmgr

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
	var pending []string
	var unverified []string
	for _, agent := range in.Agents {
		status := AgentStatus(agent.Status)
		if status == AgentStatusRunning || status == AgentStatusWaiting || status == AgentStatusIdle {
			pending = append(pending, agent.AgentID)
			continue
		}
		if status != AgentStatusCompleted {
			unverified = append(unverified, agent.AgentID)
		}
	}
	if len(pending) > 0 {
		return AgentFinalGateDecision{Action: AgentFinalGateRequireWait, PendingAgents: pending, Reasons: []string{"pending_worker_evidence"}}
	}
	if len(unverified) > 0 {
		return AgentFinalGateDecision{Action: AgentFinalGateRemoveUnverifiedClaims, PendingAgents: unverified, Reasons: []string{"non_completed_worker_evidence"}}
	}
	return AgentFinalGateDecision{Action: AgentFinalGateAllow}
}
