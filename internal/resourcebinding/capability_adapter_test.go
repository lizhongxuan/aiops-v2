package resourcebinding

import (
	"testing"

	"aiops-v2/internal/tooling"
)

func TestBuildCapabilitiesForVerifiedBinding(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		VerifiedBy: HostVerifierHostopsResolver,
		TrustLevel: TrustLevelVerified,
	})

	capabilities := BuildCapabilities(binding, []ToolCapabilityInput{
		{ToolName: "host.read", Capability: CapabilityRead},
		{ToolName: "host.exec", Capability: CapabilityExec},
	})

	if len(capabilities) != 2 {
		t.Fatalf("capabilities = %+v, want 2", capabilities)
	}
	for _, capability := range capabilities {
		if capability.ResourceRef.IdentityHash() != binding.Ref.IdentityHash() {
			t.Fatalf("capability resource = %+v, want binding resource %+v", capability.ResourceRef, binding.Ref)
		}
		if !capability.Dispatchable() {
			t.Fatalf("capability = %+v, want dispatchable", capability)
		}
	}
}

func TestRejectedBindingBuildsNoDispatchCapabilities(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		TrustLevel: TrustLevelRejected,
	})

	capabilities := BuildCapabilities(binding, []ToolCapabilityInput{
		{ToolName: "host.read", Capability: CapabilityRead},
		{ToolName: "host.exec", Capability: CapabilityExec},
		{ToolName: "host.write", Capability: CapabilityMutate, RequiresApproval: true, PolicyHash: "sha256:policy"},
	})
	if len(capabilities) != 0 {
		t.Fatalf("rejected binding capabilities = %+v, want none", capabilities)
	}
}

func TestMutateCapabilityWithoutApprovalPolicyFailsClosed(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		VerifiedBy: HostVerifierHostopsResolver,
		TrustLevel: TrustLevelVerified,
	})

	capabilities := BuildCapabilities(binding, []ToolCapabilityInput{
		{ToolName: "host.write", Capability: CapabilityMutate, RequiresApproval: true},
	})
	if len(capabilities) != 0 {
		t.Fatalf("mutate capability without policy = %+v, want none", capabilities)
	}
}

func TestHiddenToolsDoNotProduceCapabilities(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		VerifiedBy: HostVerifierHostopsResolver,
		TrustLevel: TrustLevelVerified,
	})

	capabilities := BuildCapabilities(binding, []ToolCapabilityInput{
		{ToolName: "host.hidden", Capability: CapabilityRead, Hidden: true, HiddenReason: "profile_denied"},
	})
	if len(capabilities) != 0 {
		t.Fatalf("hidden tool capabilities = %+v, want none", capabilities)
	}
}

func TestToolCapabilityInputsFromMetadata(t *testing.T) {
	inputs := ToolCapabilityInputsFromMetadata([]tooling.ToolMetadata{{
		Name:             "host.exec",
		RequiresApproval: true,
		Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind: CapabilityExec,
			ResourceTypes:  []string{ResourceTypeHost},
		},
	}, {
		Name:     "host.write",
		Mutating: true,
		Discovery: tooling.ToolDiscoveryMetadata{
			OperationKinds: []string{"write"},
			ResourceTypes:  []string{ResourceTypeHost},
		},
	}, {
		Name: "host.hidden",
		Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind:   CapabilityRead,
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
			sawExec = input.Capability == CapabilityExec
		case "host.write":
			sawMutate = input.Capability == CapabilityMutate && input.RequiresApproval && input.PolicyHash == "sha256:policy"
		case "host.hidden":
			sawHidden = input.Hidden
		}
	}
	if !sawExec || !sawMutate || !sawHidden {
		t.Fatalf("metadata projection = %+v", inputs)
	}
}
