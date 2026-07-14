package runtimekernel

import (
	"testing"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func TestAgentAssemblyTraceCarriesAssemblyInputsInOneSnapshot(t *testing.T) {
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeHost,
		ID:   "host-a",
	}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: resourcebinding.HostVerifierHostopsResolver,
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	roleBinding := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  binding.Ref,
		Role:         "pg_primary",
		SourceTurnID: "turn-1",
		Confidence:   0.98,
	})
	sessionTarget := resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
		HostIDs:          []string{"host-a"},
		SourceTurnID:     "turn-1",
		SourceMentionIDs: []string{"m-host-a"},
		Confidence:       1,
	})

	snapshot := buildAgentAssemblySnapshotForTrace(agentAssemblyTraceInput{
		AgentKind:   modelrouter.AgentKindWorker,
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		Metadata: map[string]string{
			"aiops.route.mode":         "host_bound_ops",
			"aiops.route.activeSource": "mention",
			"aiops.target.binding":     "session_target",
		},
		CompileContext: promptcompiler.CompileContext{
			Profile:        promptcompiler.PromptProfileHostWorker,
			ToolBudget:     "model_input=20000 tool_result=8000",
			EvidencePolicy: "evidence_required",
			AssembledTools: []promptcompiler.Tool{
				&testMockTool{name: "host.read", description: "Read host state", readOnly: true},
			},
		},
		Compiled: promptcompiler.CompiledPrompt{PromptSections: []promptcompiler.PromptSectionTrace{{
			ID:             "runtime.state",
			Kind:           promptcompiler.PromptSectionKindDynamic,
			Source:         "runtime",
			Hash:           "sha256:runtime-state",
			Bytes:          12,
			TokensEstimate: 3,
		}, {
			ID:             "dynamic.context",
			Kind:           promptcompiler.PromptSectionKindDynamic,
			Source:         "dynamic",
			Hash:           "sha256:dynamic-context",
			Bytes:          16,
			TokensEstimate: 4,
		}}},
		ToolSurfacePolicy: tooling.ToolSurfacePolicySnapshot{
			Hash: "sha256:tool-policy",
			HiddenTools: []tooling.ToolHiddenReason{{
				Name:   "exec_command",
				Reason: "mutation_requires_approval",
			}},
		},
		ToolSurfaceFingerprint: "surface-fingerprint",
		ResourceBindings:       []resourcebinding.ResourceBindingSnapshot{binding},
		SessionTargetSnapshot:  sessionTarget,
		RoleBindings:           []resourcebinding.ResourceRoleBinding{roleBinding},
	})

	if snapshot == nil {
		t.Fatal("assembly snapshot is nil")
	}
	if snapshot.AgentKind != "worker" || snapshot.Profile != promptcompiler.PromptProfileHostWorker || snapshot.RuntimeRole != "host.execute" {
		t.Fatalf("agent identity = %#v, want worker host_worker host.execute", snapshot)
	}
	assertTraceStrings(t, "route reasons", snapshot.RouteReason, []string{
		"aiops.route.activeSource=mention",
		"aiops.route.mode=host_bound_ops",
		"aiops.target.binding=session_target",
	})
	if len(snapshot.ResourceBindings) != 1 || snapshot.ResourceBindings[0].TraceHash != binding.TraceHash {
		t.Fatalf("resource bindings = %#v, want bound host-a", snapshot.ResourceBindings)
	}
	if len(snapshot.SessionTargets) != 1 || snapshot.SessionTargets[0].ID != "host-a" {
		t.Fatalf("session targets = %#v, want host-a", snapshot.SessionTargets)
	}
	if len(snapshot.RoleBindings) != 1 || snapshot.RoleBindings[0].TraceHash != roleBinding.TraceHash {
		t.Fatalf("role bindings = %#v, want role binding trace", snapshot.RoleBindings)
	}
	assertTraceStrings(t, "visible tools", toolSurfaceNames(snapshot.ToolSurface.ModelVisibleTools), []string{"host.read"})
	assertTraceStrings(t, "dispatch tools", toolSurfaceNames(snapshot.ToolSurface.DispatchableTools), []string{"host.read"})
	assertTraceStrings(t, "hidden tools", toolSurfaceNames(snapshot.ToolSurface.HiddenTools), []string{"exec_command"})
	if snapshot.ToolSurface.PolicyHash != "sha256:tool-policy" || snapshot.ToolSurface.Fingerprint != "surface-fingerprint" {
		t.Fatalf("tool surface = %#v, want policy hash and fingerprint from same assembly input", snapshot.ToolSurface)
	}
	if snapshot.ContextSelector.Budget != "model_input=20000 tool_result=8000" || snapshot.ContextSelector.Policy != "evidence_required" {
		t.Fatalf("context selector = %#v, want budget and evidence policy", snapshot.ContextSelector)
	}
	assertTraceStrings(t, "prompt sections", promptSectionIDs(snapshot.PromptSections.Sections), []string{"dynamic.context", "runtime.state"})
	if snapshot.SpecHash == "" {
		t.Fatal("assembly snapshot missing spec hash")
	}
}

func toolSurfaceNames(items []agentassembly.ToolSurfaceItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

func promptSectionIDs(items []agentassembly.PromptSectionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func assertTraceStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %#v, want %#v", label, got, want)
		}
	}
}
