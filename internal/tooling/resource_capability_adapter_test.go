package tooling

import (
	"testing"

	"aiops-v2/internal/resourcebinding"
)

func TestResourceCapabilityInputsFromMetadata(t *testing.T) {
	inputs := ResourceCapabilityInputsFromMetadata([]ToolMetadata{{
		Name:             "host.exec",
		RequiresApproval: true,
		Discovery: ToolDiscoveryMetadata{
			CapabilityKind: resourcebinding.CapabilityExec,
			ResourceTypes:  []string{resourcebinding.ResourceTypeHost},
		},
	}, {
		Name:     "host.write",
		Mutating: true,
		Discovery: ToolDiscoveryMetadata{
			OperationKinds: []string{"write"},
			ResourceTypes:  []string{resourcebinding.ResourceTypeHost},
		},
	}, {
		Name: "host.hidden",
		Discovery: ToolDiscoveryMetadata{
			CapabilityKind:   resourcebinding.CapabilityRead,
			HiddenFromPrompt: true,
		},
	}}, "sha256:policy")

	if len(inputs) != 3 {
		t.Fatalf("inputs = %+v, want 3", inputs)
	}
	var sawExec, sawMutate, sawHidden bool
	for _, input := range inputs {
		switch input.ToolName {
		case "host.exec":
			sawExec = input.Capability == resourcebinding.CapabilityExec
		case "host.write":
			sawMutate = input.Capability == resourcebinding.CapabilityMutate && input.RequiresApproval && input.PolicyHash == "sha256:policy"
		case "host.hidden":
			sawHidden = input.Hidden
		}
	}
	if !sawExec || !sawMutate || !sawHidden {
		t.Fatalf("metadata projection = %+v", inputs)
	}
}

func TestResourceCapabilityInputPreservesInvalidPrimaryFallbackOrder(t *testing.T) {
	input, ok := ResourceCapabilityInputFromMetadata(ToolMetadata{
		Name: "host.write",
		Discovery: ToolDiscoveryMetadata{
			CapabilityKind: "future-unknown",
			Capabilities:   []string{resourcebinding.CapabilityRead},
			OperationKinds: []string{"write"},
		},
	}, "sha256:policy")
	if !ok || input.Capability != resourcebinding.CapabilityMutate {
		t.Fatalf("input = %#v, ok = %t, want operation fallback mutate", input, ok)
	}
}
