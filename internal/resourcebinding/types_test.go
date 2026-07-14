package resourcebinding

import (
	"strings"
	"testing"
)

func TestResourceRefIdentityHashStableAcrossDisplayName(t *testing.T) {
	a := ResourceRef{Type: ResourceTypeHost, ID: "hostA", DisplayName: "primary db"}
	b := ResourceRef{Type: " HOST ", ID: "hostA", DisplayName: "renamed host"}

	if a.IdentityHash() == "" {
		t.Fatal("IdentityHash is empty")
	}
	if a.IdentityHash() != b.IdentityHash() {
		t.Fatalf("identity hash changed with display name: %q != %q", a.IdentityHash(), b.IdentityHash())
	}
}

func TestResourceRefNormalizesCommonTypes(t *testing.T) {
	tests := []ResourceRef{
		{Type: ResourceTypeHost, ID: "host-a"},
		{Type: ResourceTypeService, ID: "svc-a"},
		{Type: ResourceTypeDatabase, ID: "db-a"},
		{Type: ResourceTypeIncident, ID: "inc-a"},
	}
	for _, ref := range tests {
		normalized := NormalizeRef(ref)
		if normalized.Type == "" || normalized.ID == "" {
			t.Fatalf("NormalizeRef(%+v) = %+v, want type and id", ref, normalized)
		}
		if !strings.HasPrefix(normalized.IdentityHash(), "sha256:") {
			t.Fatalf("IdentityHash(%+v) = %q, want sha256 prefix", normalized, normalized.IdentityHash())
		}
	}
}

func TestEmptyResourceRefIsZeroAndHashable(t *testing.T) {
	var ref ResourceRef
	if !ref.IsZero() {
		t.Fatalf("empty ref IsZero = false")
	}
	if got := ref.IdentityHash(); got != "" {
		t.Fatalf("empty ref IdentityHash = %q, want empty", got)
	}
}

func TestStableTraceHashIgnoresMapAndStringSliceOrder(t *testing.T) {
	a := StableTraceHash("resource-capability", map[string]any{
		"toolNames": []string{"b", "a"},
		"labels": map[string]string{
			"z": "last",
			"a": "first",
		},
	})
	b := StableTraceHash("resource-capability", map[string]any{
		"labels": map[string]string{
			"a": "first",
			"z": "last",
		},
		"toolNames": []string{"a", "b"},
	})
	if a == "" || a != b {
		t.Fatalf("stable hashes differ: %q != %q", a, b)
	}
}

func TestRejectedBindingFailsClosed(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		TrustLevel: TrustLevelRejected,
	})

	if !binding.FailClosed {
		t.Fatalf("rejected binding FailClosed = false")
	}
	if binding.Verified() {
		t.Fatalf("rejected binding Verified = true")
	}
}

func TestMutateCapabilityRequiresApprovalAndPolicyHash(t *testing.T) {
	binding := NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{
		Source:     BindingSourceMention,
		VerifiedBy: HostVerifierHostopsResolver,
		TrustLevel: TrustLevelVerified,
	})

	capability := NewResourceCapability(binding, CapabilityMutate, []string{"host.write"}, CapabilityOptions{})
	if !capability.RequiresApproval {
		t.Fatalf("mutate capability RequiresApproval = false")
	}
	if capability.Dispatchable() {
		t.Fatalf("mutate capability without policy hash is dispatchable")
	}

	withPolicy := NewResourceCapability(binding, CapabilityMutate, []string{"host.write"}, CapabilityOptions{
		RequiresApproval: true,
		PolicyHash:       "sha256:policy",
	})
	if !withPolicy.Dispatchable() {
		t.Fatalf("mutate capability with approval policy is not dispatchable")
	}
}

func TestRoleBindingTraceHashIgnoresAliasOrder(t *testing.T) {
	a := NewRoleBinding(RoleBindingInput{
		ResourceRef:  ResourceRef{Type: ResourceTypeDatabase, ID: "pg-a"},
		Role:         "primary",
		RoleAlias:    []string{"主节点", "primary"},
		SourceTurnID: "turn-1",
	})
	b := NewRoleBinding(RoleBindingInput{
		ResourceRef:  ResourceRef{Type: ResourceTypeDatabase, ID: "pg-a"},
		Role:         "primary",
		RoleAlias:    []string{"primary", "主节点"},
		SourceTurnID: "turn-1",
	})

	if a.TraceHash == "" || a.TraceHash != b.TraceHash {
		t.Fatalf("role binding trace hash changed with alias order: %q != %q", a.TraceHash, b.TraceHash)
	}
}
