package resourcebinding

import (
	"sort"
	"strings"
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
