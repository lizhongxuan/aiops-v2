package agentassembly

import (
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func TestBuilderDoesNotFilterToolSurface(t *testing.T) {
	visible := []tooling.ToolMetadata{{Name: "a.read"}, {Name: "b.read"}}
	dispatchable := []tooling.ToolMetadata{{Name: "a.read"}, {Name: "b.read"}}
	snapshot := Build(BuildInput{
		AgentKind:         "worker",
		Profile:           "host_worker",
		RuntimeRole:       "host.inspect",
		ModelVisibleTools: visible,
		DispatchableTools: dispatchable,
	})

	if got := toolNames(snapshot.ToolSurface.ModelVisibleTools); len(got) != 2 || got[0] != "a.read" || got[1] != "b.read" {
		t.Fatalf("visible tools = %#v, want unchanged sorted visible tools", got)
	}
	if got := toolNames(snapshot.ToolSurface.DispatchableTools); len(got) != 2 || got[0] != "a.read" || got[1] != "b.read" {
		t.Fatalf("dispatchable tools = %#v, want unchanged sorted dispatchable tools", got)
	}
}

func TestProfileOnlyAgentLeavesToolSurfaceAndLoopHashStable(t *testing.T) {
	base := Build(BuildInput{
		AgentKind:         "writer",
		Profile:           "writer.v1",
		RuntimeRole:       "workspace.chat",
		ProfilePromptHash: "sha256:writer-v1",
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "material.read"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "material.read"}},
		LoopPolicy:        LoopPolicySnapshot{Lifecycle: LifecycleRequestScope, MaxIterations: 1, ToolCallPolicy: "none"},
	})
	next := Build(BuildInput{
		AgentKind:         "writer",
		Profile:           "writer.v2",
		RuntimeRole:       "workspace.chat",
		ProfilePromptHash: "sha256:writer-v2",
		ModelVisibleTools: []tooling.ToolMetadata{{Name: "material.read"}},
		DispatchableTools: []tooling.ToolMetadata{{Name: "material.read"}},
		LoopPolicy:        LoopPolicySnapshot{Lifecycle: LifecycleRequestScope, MaxIterations: 1, ToolCallPolicy: "none"},
	})

	if base.ToolSurface.Hash != next.ToolSurface.Hash {
		t.Fatalf("tool surface hash changed for profile-only agent: %q != %q", base.ToolSurface.Hash, next.ToolSurface.Hash)
	}
	if base.LoopPolicy.Hash != next.LoopPolicy.Hash {
		t.Fatalf("loop hash changed for profile-only agent: %q != %q", base.LoopPolicy.Hash, next.LoopPolicy.Hash)
	}
	if base.SpecHash == next.SpecHash {
		t.Fatalf("spec hash did not change for profile-only profile update")
	}
}

func TestBuilderPreservesAdvisorToolBoundary(t *testing.T) {
	snapshot := Build(BuildInput{
		AgentKind:         "advisor",
		Profile:           "advisor.v1",
		RuntimeRole:       "workspace.chat",
		ModelVisibleTools: []tooling.ToolMetadata{readOnlyTool("search_ops_manuals", "knowledge")},
		DispatchableTools: []tooling.ToolMetadata{readOnlyTool("search_ops_manuals", "knowledge")},
		HiddenTools:       []HiddenToolInput{{Name: "exec_command", Reason: "advisor_no_host_exec"}},
		LoopPolicy:        LoopPolicySnapshot{MaxIterations: 1, ToolCallPolicy: "read_only"},
	})

	assertVisibleTools(t, snapshot, []string{"search_ops_manuals"})
	assertDispatchableTools(t, snapshot, []string{"search_ops_manuals"})
	assertHiddenTool(t, snapshot, "exec_command", "advisor_no_host_exec")
}

func TestBuilderPreservesManagerDelegationToolBoundary(t *testing.T) {
	snapshot := Build(BuildInput{
		AgentKind:   "manager",
		Profile:     "host_manager.v1",
		RuntimeRole: "host.manager",
		ModelVisibleTools: []tooling.ToolMetadata{
			readOnlyTool("spawn_host_agent", "delegation"),
			readOnlyTool("wait_host_agents", "delegation"),
			readOnlyTool("summarize_child_reports", "delegation"),
		},
		DispatchableTools: []tooling.ToolMetadata{
			readOnlyTool("spawn_host_agent", "delegation"),
			readOnlyTool("wait_host_agents", "delegation"),
			readOnlyTool("summarize_child_reports", "delegation"),
		},
		HiddenTools: []HiddenToolInput{{Name: "exec_command", Reason: "manager_delegates_host_work"}},
		LoopPolicy:  LoopPolicySnapshot{MaxIterations: 8, ToolCallPolicy: "delegate_wait_summarize"},
	})

	assertVisibleTools(t, snapshot, []string{"spawn_host_agent", "summarize_child_reports", "wait_host_agents"})
	assertDispatchableTools(t, snapshot, []string{"spawn_host_agent", "summarize_child_reports", "wait_host_agents"})
	assertHiddenTool(t, snapshot, "exec_command", "manager_delegates_host_work")
}

func TestBuilderPreservesHostWorkerBoundHostTools(t *testing.T) {
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeHost,
		ID:   "host-a",
	}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	execTool := tooling.ToolMetadata{
		Name:        "exec_command",
		Domain:      "host",
		Mutating:    true,
		RiskLevel:   tooling.ToolRiskHigh,
		Description: "Execute an approved command on the bound host.",
		Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind: "mutate",
			ResourceTypes:  []string{resourcebinding.ResourceTypeHost},
		},
	}
	snapshot := Build(BuildInput{
		AgentKind:         "worker",
		Profile:           "host_worker.v1",
		RuntimeRole:       "host.worker",
		ResourceBindings:  []resourcebinding.ResourceBindingSnapshot{binding},
		ModelVisibleTools: []tooling.ToolMetadata{execTool},
		DispatchableTools: []tooling.ToolMetadata{execTool},
		PolicyHash:        "sha256:host-mutation-policy",
		LoopPolicy:        LoopPolicySnapshot{MaxIterations: 6, ToolCallPolicy: "bound_host_only"},
	})

	assertVisibleTools(t, snapshot, []string{"exec_command"})
	assertDispatchableTools(t, snapshot, []string{"exec_command"})
	if got := snapshot.ToolSurface.ModelVisibleTools[0].ResourceBindingHash; got != binding.TraceHash {
		t.Fatalf("exec_command binding hash = %q, want %q", got, binding.TraceHash)
	}
	if !snapshot.ToolSurface.ModelVisibleTools[0].RequiresApproval {
		t.Fatalf("exec_command should require approval in host worker surface")
	}
}

func TestBuilderPreservesVerifierToolBoundary(t *testing.T) {
	snapshot := Build(BuildInput{
		AgentKind:         "verifier",
		Profile:           "verifier.v1",
		RuntimeRole:       "workspace.verify",
		ModelVisibleTools: []tooling.ToolMetadata{readOnlyTool("read_evidence", "evidence")},
		DispatchableTools: []tooling.ToolMetadata{readOnlyTool("read_evidence", "evidence")},
		HiddenTools:       []HiddenToolInput{{Name: "exec_command", Reason: "verifier_no_mutation"}},
		LoopPolicy:        LoopPolicySnapshot{MaxIterations: 2, ToolCallPolicy: "verify_read_only"},
	})

	assertVisibleTools(t, snapshot, []string{"read_evidence"})
	assertDispatchableTools(t, snapshot, []string{"read_evidence"})
	assertHiddenTool(t, snapshot, "exec_command", "verifier_no_mutation")
}

func TestBuilderPreservesWriterToolBoundary(t *testing.T) {
	snapshot := Build(BuildInput{
		AgentKind:         "writer",
		Profile:           "writer.v1",
		RuntimeRole:       "workspace.write",
		ModelVisibleTools: []tooling.ToolMetadata{readOnlyTool("read_structured_material", "material")},
		DispatchableTools: []tooling.ToolMetadata{readOnlyTool("read_structured_material", "material")},
		HiddenTools:       []HiddenToolInput{{Name: "exec_command", Reason: "writer_consumes_materials_only"}},
		LoopPolicy:        LoopPolicySnapshot{MaxIterations: 1, ToolCallPolicy: "consume_structured_material"},
	})

	assertVisibleTools(t, snapshot, []string{"read_structured_material"})
	assertDispatchableTools(t, snapshot, []string{"read_structured_material"})
	assertHiddenTool(t, snapshot, "exec_command", "writer_consumes_materials_only")
}

func toolNames(items []ToolSurfaceItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

func readOnlyTool(name, domain string) tooling.ToolMetadata {
	return tooling.ToolMetadata{
		Name:        name,
		Domain:      domain,
		Description: name,
		Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind: "read",
		},
	}
}

func assertVisibleTools(t *testing.T, snapshot AgentAssemblySnapshot, want []string) {
	t.Helper()
	if got := toolNames(snapshot.ToolSurface.ModelVisibleTools); !sameStrings(got, want) {
		t.Fatalf("visible tools = %#v, want %#v", got, want)
	}
}

func assertDispatchableTools(t *testing.T, snapshot AgentAssemblySnapshot, want []string) {
	t.Helper()
	if got := toolNames(snapshot.ToolSurface.DispatchableTools); !sameStrings(got, want) {
		t.Fatalf("dispatchable tools = %#v, want %#v", got, want)
	}
}

func assertHiddenTool(t *testing.T, snapshot AgentAssemblySnapshot, name, reason string) {
	t.Helper()
	for _, item := range snapshot.ToolSurface.HiddenTools {
		if item.Name == name {
			if item.HiddenReason != reason {
				t.Fatalf("hidden tool %q reason = %q, want %q", name, item.HiddenReason, reason)
			}
			return
		}
	}
	t.Fatalf("hidden tool %q not found in %#v", name, snapshot.ToolSurface.HiddenTools)
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
