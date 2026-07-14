package agentassembly

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func TestStableHashIgnoresMapAndSliceOrder(t *testing.T) {
	a := StableHash("assembly", map[string]any{
		"tags": map[string]string{"z": "last", "a": "first"},
		"tools": []string{
			"b",
			"a",
		},
	})
	b := StableHash("assembly", map[string]any{
		"tools": []string{"a", "b"},
		"tags":  map[string]string{"a": "first", "z": "last"},
	})
	if a == "" || a != b {
		t.Fatalf("hashes differ: %q != %q", a, b)
	}
}

func TestToolSurfaceHashUnaffectedByProfilePromptChange(t *testing.T) {
	tools := []tooling.ToolMetadata{{Name: "host.read", Description: "read host"}}
	first := Build(BuildInput{
		AgentKind:         "worker",
		Profile:           "host_worker",
		RuntimeRole:       "host.execute",
		ProfilePromptHash: "sha256:old-profile",
		ModelVisibleTools: tools,
		DispatchableTools: tools,
	})
	second := Build(BuildInput{
		AgentKind:         "worker",
		Profile:           "host_worker",
		RuntimeRole:       "host.execute",
		ProfilePromptHash: "sha256:new-profile",
		ModelVisibleTools: tools,
		DispatchableTools: tools,
	})

	if first.ToolSurface.Hash == "" {
		t.Fatalf("tool surface hash is empty")
	}
	if first.ToolSurface.Hash != second.ToolSurface.Hash {
		t.Fatalf("tool surface hash changed with profile text: %q != %q", first.ToolSurface.Hash, second.ToolSurface.Hash)
	}
	if first.SpecHash == second.SpecHash {
		t.Fatalf("assembly spec hash did not change after profile prompt hash changed")
	}
}

func TestPromptSectionSnapshotSortsByStableID(t *testing.T) {
	snapshot := PromptSectionSnapshotFromTrace([]promptcompiler.PromptSectionTrace{
		{ID: "profile.host", Source: "profile", Hash: "sha256:b"},
		{ID: "base.contract", Source: "compiler", Hash: "sha256:a"},
	})

	if len(snapshot.Sections) != 2 {
		t.Fatalf("sections = %#v, want 2", snapshot.Sections)
	}
	if snapshot.Sections[0].ID != "base.contract" || snapshot.Sections[1].ID != "profile.host" {
		t.Fatalf("sections not sorted by id: %#v", snapshot.Sections)
	}
	if snapshot.Hash == "" {
		t.Fatalf("section snapshot hash is empty")
	}
}

func TestBuildAgentAssemblySnapshotAllowsEmptyResources(t *testing.T) {
	snapshot := Build(BuildInput{
		AgentKind:         "advisor",
		Profile:           "advisor",
		RuntimeRole:       "workspace.chat",
		RouteReason:       []string{"advisory"},
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "search_docs", Description: "search docs"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "search_docs", Description: "search docs"}},
		PromptSections:    []promptcompiler.PromptSectionTrace{{ID: "base.contract", Source: "compiler", Hash: "sha256:base"}},
		TraceTags:         map[string]string{"route": "advisory"},
	})

	if snapshot.AgentKind != "advisor" || snapshot.Profile != "advisor" || snapshot.RuntimeRole != "workspace.chat" {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if len(snapshot.ResourceBindings) != 0 {
		t.Fatalf("resource bindings = %#v, want empty", snapshot.ResourceBindings)
	}
	if snapshot.SpecHash == "" {
		t.Fatalf("SpecHash is empty")
	}
}

func TestBuildAgentAssemblySnapshotCarriesResourceAndRoleBindings(t *testing.T) {
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	role := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeDatabase, ID: "pg-a"},
		Role:         "primary",
		SourceTurnID: "turn-1",
	})

	snapshot := Build(BuildInput{
		AgentKind:        "worker",
		Profile:          "host_worker",
		RuntimeRole:      "host.execute",
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{binding},
		RoleBindings:     []resourcebinding.ResourceRoleBinding{role},
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "host.exec", Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind: resourcebinding.CapabilityExec,
		}}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "host.exec", Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind: resourcebinding.CapabilityExec,
		}}},
	})

	if len(snapshot.ResourceBindings) != 1 || snapshot.ResourceBindings[0].Ref.ID != "host-a" {
		t.Fatalf("resource bindings = %#v", snapshot.ResourceBindings)
	}
	if len(snapshot.RoleBindings) != 1 || snapshot.RoleBindings[0].Role != "primary" {
		t.Fatalf("role bindings = %#v", snapshot.RoleBindings)
	}
	if snapshot.ToolSurface.ModelVisibleTools[0].ResourceBindingHash == "" {
		t.Fatalf("tool surface item missing resource binding hash: %#v", snapshot.ToolSurface.ModelVisibleTools[0])
	}
}
