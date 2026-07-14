package resourcebinding

import "testing"

func TestExtractRoleBindingsForPGPrimaryAndStandby(t *testing.T) {
	bindings := []ResourceBindingSnapshot{
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a", DisplayName: "hostA"}, BindingOptions{Source: BindingSourceMention, VerifiedBy: HostVerifierHostopsResolver, TrustLevel: TrustLevelVerified}),
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-b", DisplayName: "hostB"}, BindingOptions{Source: BindingSourceMention, VerifiedBy: HostVerifierHostopsResolver, TrustLevel: TrustLevelVerified}),
	}
	extraction := ExtractRoleBindings("@hostA 设置为 PG 主节点，@hostB 设置为 PG 从节点", RoleCandidatesFromBindings(bindings), "turn-1")

	if len(extraction.Bindings) != 2 {
		t.Fatalf("bindings = %#v, want 2", extraction.Bindings)
	}
	rolesByHost := map[string]string{}
	for _, binding := range extraction.Bindings {
		rolesByHost[binding.ResourceRef.ID] = binding.Role
	}
	if rolesByHost["host-a"] != RolePGPrimary || rolesByHost["host-b"] != RolePGStandby {
		t.Fatalf("rolesByHost = %#v, want host-a primary and host-b standby", rolesByHost)
	}
	if len(extraction.Conflicts) != 0 {
		t.Fatalf("conflicts = %#v, want none", extraction.Conflicts)
	}
}

func TestDetectRoleBindingConflicts(t *testing.T) {
	conflicts := DetectRoleBindingConflicts([]ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-b"}, Role: RolePGPrimary}),
	})
	if len(conflicts) != 1 || conflicts[0].Role != RolePGPrimary {
		t.Fatalf("conflicts = %#v, want duplicate primary conflict", conflicts)
	}
}

func TestSameResourceMutuallyExclusiveRolesConflict(t *testing.T) {
	conflicts := DetectRoleBindingConflicts([]ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGStandby}),
	})
	if len(conflicts) == 0 {
		t.Fatalf("conflicts empty, want primary/standby same resource conflict")
	}
}

func TestResolveUniqueRoleBindingResolvesPrimaryAndStandby(t *testing.T) {
	bindings := []ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-b"}, Role: RolePGStandby}),
	}

	primary := ResolveUniqueRoleBinding(bindings, nil, "主节点")
	if primary.Status != RoleBindingResolutionResolved || primary.ResourceRef.ID != "host-a" || primary.Role != RolePGPrimary {
		t.Fatalf("primary resolution = %#v, want host-a pg_primary", primary)
	}
	standby := ResolveUniqueRoleBinding(bindings, nil, "从节点")
	if standby.Status != RoleBindingResolutionResolved || standby.ResourceRef.ID != "host-b" || standby.Role != RolePGStandby {
		t.Fatalf("standby resolution = %#v, want host-b pg_standby", standby)
	}
}

func TestResolveUniqueRoleBindingFailsClosedOnAmbiguousOrConflict(t *testing.T) {
	ambiguous := ResolveUniqueRoleBinding([]ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-b"}, Role: RolePGPrimary}),
	}, nil, "primary")
	if ambiguous.Status != RoleBindingResolutionAmbiguous {
		t.Fatalf("ambiguous resolution = %#v, want ambiguous", ambiguous)
	}

	conflicted := ResolveUniqueRoleBinding([]ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
	}, []RoleBindingConflict{{Role: RolePGPrimary, Reasons: []string{"unique_role_bound_to_multiple_resources"}}}, "主节点")
	if conflicted.Status != RoleBindingResolutionConflict {
		t.Fatalf("conflicted resolution = %#v, want conflict", conflicted)
	}
}

func TestResolveUniqueRoleBindingReportsToolEvidenceConflict(t *testing.T) {
	conflicted := ResolveUniqueRoleBinding([]ResourceRoleBinding{
		NewRoleBinding(RoleBindingInput{ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, Role: RolePGPrimary}),
	}, []RoleBindingConflict{{
		ResourceID: "host-b",
		Role:       RolePGPrimary,
		Reasons:    []string{"tool_evidence_conflict"},
	}}, "主节点")

	if conflicted.Status != RoleBindingResolutionConflict || conflicted.Reason != "role_conflict" {
		t.Fatalf("conflicted resolution = %#v, want role_conflict", conflicted)
	}
}
