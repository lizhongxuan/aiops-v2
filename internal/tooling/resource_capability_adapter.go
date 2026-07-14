package tooling

import (
	"sort"
	"strings"

	"aiops-v2/internal/resourcebinding"
)

// ResourceCapabilityInputsFromMetadata projects tool-owned metadata into the
// resourcebinding input contract without making resourcebinding depend on the
// tool registry package.
func ResourceCapabilityInputsFromMetadata(metas []ToolMetadata, policyHash string) []resourcebinding.ToolCapabilityInput {
	if len(metas) == 0 {
		return nil
	}
	inputs := make([]resourcebinding.ToolCapabilityInput, 0, len(metas))
	for _, meta := range metas {
		if input, ok := ResourceCapabilityInputFromMetadata(meta, policyHash); ok {
			inputs = append(inputs, input)
		}
	}
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].ToolName < inputs[j].ToolName
	})
	return inputs
}

func ResourceCapabilityInputFromMetadata(meta ToolMetadata, policyHash string) (resourcebinding.ToolCapabilityInput, bool) {
	toolName := strings.TrimSpace(meta.Name)
	if toolName == "" {
		return resourcebinding.ToolCapabilityInput{}, false
	}
	discovery := meta.EffectiveDiscovery()
	capability := normalizeResourceCapability(firstResourceCapability(discovery.CapabilityKind, discovery.Capabilities))
	if capability == "" {
		capability = resourceCapabilityFromOperations(discovery.OperationKinds, meta.Mutating)
	}
	if capability == "" {
		return resourcebinding.ToolCapabilityInput{}, false
	}
	governance := meta.EffectiveGovernance(4096)
	requiresApproval := governance.RequiresApproval
	if capability == resourcebinding.CapabilityMutate {
		requiresApproval = true
	}
	input := resourcebinding.ToolCapabilityInput{
		ToolName:         toolName,
		Capability:       capability,
		RequiresApproval: requiresApproval,
		PolicyHash:       strings.TrimSpace(policyHash),
	}
	if ToolHiddenFromPrompt(meta) || ToolHiddenFromDiscovery(meta) {
		input.Hidden = true
		input.HiddenReason = "hidden_tool"
	}
	return input, true
}

func firstResourceCapability(primary string, values []string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	for _, value := range values {
		if normalizeResourceCapability(value) != "" {
			return value
		}
	}
	return ""
}

func resourceCapabilityFromOperations(operations []string, mutating bool) string {
	if mutating {
		return resourcebinding.CapabilityMutate
	}
	for _, operation := range operations {
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case resourcebinding.CapabilityExec, "execute", "command", "run":
			return resourcebinding.CapabilityExec
		case resourcebinding.CapabilityInspect, "list", "query", "diagnose", "rca":
			return resourcebinding.CapabilityInspect
		case resourcebinding.CapabilityRead, "get", "search":
			return resourcebinding.CapabilityRead
		case resourcebinding.CapabilityMutate, "write", "update", "delete", "restart", "repair":
			return resourcebinding.CapabilityMutate
		}
	}
	return ""
}

func normalizeResourceCapability(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case resourcebinding.CapabilityRead:
		return resourcebinding.CapabilityRead
	case resourcebinding.CapabilityInspect:
		return resourcebinding.CapabilityInspect
	case resourcebinding.CapabilityExec:
		return resourcebinding.CapabilityExec
	case resourcebinding.CapabilityMutate:
		return resourcebinding.CapabilityMutate
	default:
		return ""
	}
}
