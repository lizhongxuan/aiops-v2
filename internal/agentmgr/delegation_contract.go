package agentmgr

import "strings"

type DelegationDecisionAction string

const (
	DelegationSpawnNew         DelegationDecisionAction = "spawn_new"
	DelegationContinueExisting DelegationDecisionAction = "continue_existing"
	DelegationHandleInline     DelegationDecisionAction = "handle_inline"
	DelegationAskClarification DelegationDecisionAction = "ask_clarification"
	DelegationBlocked          DelegationDecisionAction = "blocked"
)

type DelegationDecision struct {
	Action          DelegationDecisionAction `json:"action"`
	Reason          string                   `json:"reason"`
	CandidateAgent  string                   `json:"candidateAgent,omitempty"`
	ExistingAgentID string                   `json:"existingAgentId,omitempty"`
	RequiredFields  []string                 `json:"requiredFields,omitempty"`
}

type DelegationEvaluationInput struct {
	Objective            string
	Scope                string
	EvidenceRequirement  string
	ReadOnly             bool
	Simple               bool
	BlockedReason        string
	EvidenceSurfaces     []EvidenceSurface
	AvailableAgents      []CandidateAgent
	ExistingAgents       []ExistingAgentContext
	RequiredCapability   string
	RequiredResourceType string
}

type EvidenceSurface struct {
	CapabilityKind string
	ResourceType   string
	OperationKind  string
}

type CandidateAgent struct {
	Name            string
	CapabilityKinds []string
	ResourceTypes   []string
	OperationKinds  []string
}

type ExistingAgentContext struct {
	AgentID         string
	Status          AgentStatus
	CapabilityKinds []string
	ResourceTypes   []string
	OperationKinds  []string
}

func EvaluateDelegationDecision(in DelegationEvaluationInput) DelegationDecision {
	if strings.TrimSpace(in.BlockedReason) != "" {
		return DelegationDecision{Action: DelegationBlocked, Reason: strings.TrimSpace(in.BlockedReason)}
	}
	missing := missingDelegationFields(in)
	if len(missing) > 0 && !in.Simple {
		return DelegationDecision{Action: DelegationAskClarification, Reason: "missing_delegation_fields", RequiredFields: missing}
	}
	if in.Simple && len(in.EvidenceSurfaces) == 0 && strings.TrimSpace(in.EvidenceRequirement) == "" {
		return DelegationDecision{Action: DelegationHandleInline, Reason: "simple_no_independent_evidence_required"}
	}
	if existing := matchExistingAgent(in.ExistingAgents, in.RequiredCapability, in.RequiredResourceType); existing != "" {
		return DelegationDecision{Action: DelegationContinueExisting, Reason: "same_capability_resource_followup", ExistingAgentID: existing}
	}
	if in.ReadOnly && independentSurfaceCount(in.EvidenceSurfaces) > 1 {
		return DelegationDecision{Action: DelegationSpawnNew, Reason: "independent_evidence_surface", CandidateAgent: matchCandidateAgent(in.AvailableAgents, in.RequiredCapability, in.RequiredResourceType)}
	}
	if len(missing) > 0 {
		return DelegationDecision{Action: DelegationAskClarification, Reason: "missing_delegation_fields", RequiredFields: missing}
	}
	return DelegationDecision{Action: DelegationHandleInline, Reason: "no_independent_delegation_benefit"}
}

func missingDelegationFields(in DelegationEvaluationInput) []string {
	var missing []string
	if strings.TrimSpace(in.Objective) == "" {
		missing = append(missing, "objective")
	}
	if strings.TrimSpace(in.Scope) == "" {
		missing = append(missing, "scope")
	}
	if strings.TrimSpace(in.EvidenceRequirement) == "" && len(in.EvidenceSurfaces) == 0 {
		missing = append(missing, "evidenceRequirement")
	}
	return missing
}

func independentSurfaceCount(surfaces []EvidenceSurface) int {
	seen := map[string]struct{}{}
	for _, surface := range surfaces {
		key := strings.TrimSpace(surface.CapabilityKind) + "\x00" + strings.TrimSpace(surface.ResourceType) + "\x00" + strings.TrimSpace(surface.OperationKind)
		if key == "\x00\x00" {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

func matchExistingAgent(agents []ExistingAgentContext, capability, resourceType string) string {
	for _, agent := range agents {
		if agent.AgentID == "" || (agent.Status != AgentStatusFailed && agent.Status != AgentStatusCompleted) {
			continue
		}
		if matchesCapabilityResource(agent.CapabilityKinds, agent.ResourceTypes, capability, resourceType) {
			return agent.AgentID
		}
	}
	return ""
}

func matchCandidateAgent(agents []CandidateAgent, capability, resourceType string) string {
	for _, agent := range agents {
		if agent.Name == "" {
			continue
		}
		if matchesCapabilityResource(agent.CapabilityKinds, agent.ResourceTypes, capability, resourceType) {
			return agent.Name
		}
	}
	if len(agents) > 0 {
		return agents[0].Name
	}
	return ""
}

func matchesCapabilityResource(capabilities, resources []string, capability, resourceType string) bool {
	capability = strings.TrimSpace(capability)
	resourceType = strings.TrimSpace(resourceType)
	capabilityOK := capability == "" || containsString(capabilities, capability)
	resourceOK := resourceType == "" || containsString(resources, resourceType)
	return capabilityOK && resourceOK
}
