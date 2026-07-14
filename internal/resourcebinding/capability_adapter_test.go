package resourcebinding

import "testing"

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
