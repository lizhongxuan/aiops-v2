package agents

import "strings"

const maxWhenToUseChars = 360

// AgentDiscoveryMetadata contains short, prompt-safe routing metadata for an agent.
type AgentDiscoveryMetadata struct {
	WhenToUse       string   `json:"whenToUse,omitempty"`
	CapabilityKinds []string `json:"capabilityKinds,omitempty"`
	ResourceTypes   []string `json:"resourceTypes,omitempty"`
	OperationKinds  []string `json:"operationKinds,omitempty"`
	Modes           []string `json:"modes,omitempty"`
	ModelInvocable  bool     `json:"modelInvocable,omitempty"`
}

// AgentBudgetMetadata contains generic scheduling budget hints for an agent.
type AgentBudgetMetadata struct {
	MaxConcurrent int    `json:"maxConcurrent,omitempty"`
	CostClass     string `json:"costClass,omitempty"`
}

func cloneDiscoveryMetadata(in AgentDiscoveryMetadata) AgentDiscoveryMetadata {
	in.CapabilityKinds = append([]string(nil), in.CapabilityKinds...)
	in.ResourceTypes = append([]string(nil), in.ResourceTypes...)
	in.OperationKinds = append([]string(nil), in.OperationKinds...)
	in.Modes = append([]string(nil), in.Modes...)
	return in
}

func normalizeDiscoveryMetadata(in AgentDiscoveryMetadata) AgentDiscoveryMetadata {
	in.WhenToUse = truncateRunes(strings.TrimSpace(in.WhenToUse), maxWhenToUseChars)
	in.CapabilityKinds = cloneTrimmedStrings(in.CapabilityKinds)
	in.ResourceTypes = cloneTrimmedStrings(in.ResourceTypes)
	in.OperationKinds = cloneTrimmedStrings(in.OperationKinds)
	in.Modes = cloneTrimmedStrings(in.Modes)
	return in
}

func normalizeBudgetMetadata(in AgentBudgetMetadata) AgentBudgetMetadata {
	in.CostClass = strings.TrimSpace(in.CostClass)
	return in
}

func cloneTrimmedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(value) <= max {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
