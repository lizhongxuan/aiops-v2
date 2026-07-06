package resourcebinding

import (
	"sort"
	"strings"

	"aiops-v2/internal/tooling"
)

type CapabilityOptions struct {
	RequiresApproval bool
	PolicyHash       string
}

type ToolCapabilityInput struct {
	ToolName         string
	Capability       string
	RequiresApproval bool
	PolicyHash       string
	Hidden           bool
	HiddenReason     string
}

func ToolCapabilityInputsFromMetadata(metas []tooling.ToolMetadata, policyHash string) []ToolCapabilityInput {
	if len(metas) == 0 {
		return nil
	}
	inputs := make([]ToolCapabilityInput, 0, len(metas))
	for _, meta := range metas {
		if input, ok := ToolCapabilityInputFromMetadata(meta, policyHash); ok {
			inputs = append(inputs, input)
		}
	}
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].ToolName < inputs[j].ToolName
	})
	return inputs
}

func ToolCapabilityInputFromMetadata(meta tooling.ToolMetadata, policyHash string) (ToolCapabilityInput, bool) {
	toolName := strings.TrimSpace(meta.Name)
	if toolName == "" {
		return ToolCapabilityInput{}, false
	}
	discovery := meta.EffectiveDiscovery()
	capability := normalizeCapability(firstNonEmpty(discovery.CapabilityKind, firstCapability(discovery.Capabilities)))
	if capability == "" {
		capability = capabilityFromOperations(discovery.OperationKinds, meta.Mutating)
	}
	if capability == "" {
		return ToolCapabilityInput{}, false
	}
	governance := meta.EffectiveGovernance(4096)
	requiresApproval := governance.RequiresApproval
	if capability == CapabilityMutate {
		requiresApproval = true
	}
	input := ToolCapabilityInput{
		ToolName:         toolName,
		Capability:       capability,
		RequiresApproval: requiresApproval,
		PolicyHash:       strings.TrimSpace(policyHash),
	}
	if tooling.ToolHiddenFromPrompt(meta) || tooling.ToolHiddenFromDiscovery(meta) {
		input.Hidden = true
		input.HiddenReason = "hidden_tool"
	}
	return input, true
}

func BuildCapabilities(binding ResourceBindingSnapshot, inputs []ToolCapabilityInput) []ResourceCapability {
	if !binding.Verified() || len(inputs) == 0 {
		return nil
	}
	capabilities := make([]ResourceCapability, 0, len(inputs))
	for _, input := range inputs {
		if input.Hidden {
			continue
		}
		toolName := strings.TrimSpace(input.ToolName)
		capability := normalizeCapability(input.Capability)
		if toolName == "" || capability == "" {
			continue
		}
		projected := NewResourceCapability(binding, capability, []string{toolName}, CapabilityOptions{
			RequiresApproval: input.RequiresApproval,
			PolicyHash:       input.PolicyHash,
		})
		if !projected.Dispatchable() {
			continue
		}
		capabilities = append(capabilities, projected)
	}
	sort.Slice(capabilities, func(i, j int) bool {
		if capabilities[i].Capability != capabilities[j].Capability {
			return capabilities[i].Capability < capabilities[j].Capability
		}
		return strings.Join(capabilities[i].ToolNames, "\x00") < strings.Join(capabilities[j].ToolNames, "\x00")
	})
	return capabilities
}

func capabilityFromOperations(operations []string, mutating bool) string {
	if mutating {
		return CapabilityMutate
	}
	for _, op := range operations {
		switch normalizeToken(op) {
		case CapabilityExec, "execute", "command", "run":
			return CapabilityExec
		case CapabilityInspect, "list", "query", "diagnose", "rca":
			return CapabilityInspect
		case CapabilityRead, "get", "search":
			return CapabilityRead
		case CapabilityMutate, "write", "update", "delete", "restart", "repair":
			return CapabilityMutate
		}
	}
	return ""
}

func firstCapability(values []string) string {
	for _, value := range values {
		if normalizeCapability(value) != "" {
			return value
		}
	}
	return ""
}

func NewResourceCapability(binding ResourceBindingSnapshot, capability string, toolNames []string, options CapabilityOptions) ResourceCapability {
	normalizedCapability := normalizeCapability(capability)
	requiresApproval := options.RequiresApproval
	if normalizedCapability == CapabilityMutate {
		requiresApproval = true
	}
	out := ResourceCapability{
		ResourceRef:      NormalizeRef(binding.Ref),
		Capability:       normalizedCapability,
		ToolNames:        uniqueSorted(toolNames),
		RequiresApproval: requiresApproval,
		PolicyHash:       strings.TrimSpace(options.PolicyHash),
		BindingTraceHash: strings.TrimSpace(binding.TraceHash),
	}
	out.TraceHash = StableTraceHash("resource-capability", map[string]any{
		"resource":         out.ResourceRef,
		"capability":       out.Capability,
		"toolNames":        out.ToolNames,
		"requiresApproval": out.RequiresApproval,
		"policyHash":       out.PolicyHash,
		"bindingTraceHash": out.BindingTraceHash,
	})
	return out
}
