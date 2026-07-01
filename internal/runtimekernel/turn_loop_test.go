package runtimekernel

import (
	"testing"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

func TestBuildRuntimeStepContextCreatesProviderRequestSnapshot(t *testing.T) {
	kernel := NewRuntimeKernel(RuntimeKernelConfig{})
	session := &SessionState{
		ID:     "session-step",
		Type:   SessionTypeHost,
		Mode:   ModeChat,
		HostID: "host-a",
		ActiveTurn: ActiveTurnState{
			TurnID: "turn-step",
		},
		Context: ContextWindow{MaxTokens: 32000},
	}
	compiled := promptcompiler.CompiledPrompt{
		System: promptcompiler.SystemPrompt{Content: "system layer"},
		Envelope: promptcompiler.PromptEnvelope{Sections: []promptcompiler.PromptCompiledSection{{
			ID:        "base.contract",
			Layer:     promptcompiler.PromptSectionKindStable,
			Role:      "system",
			Content:   "system layer",
			Stability: promptcompiler.PromptSectionKindStable,
			Source:    "base",
		}}},
	}
	step, promptBuild, err := kernel.buildRuntimeStepContext(
		TurnRequest{
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			SessionID:   "session-step",
			TurnID:      "turn-step",
			Input:       "check nginx errors",
			Metadata:    map[string]string{"reasoningEffort": "high"},
		},
		session,
		modelrouter.AgentKindWorker,
		1,
		ContextPipelineResult{},
		[]Message{{Role: "user", Content: "check nginx errors"}},
		compiled,
		runtimeToolRouterSnapshotFromCompile([]promptcompiler.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_logs", Description: "Read logs"}},
		}, []string{"read_logs"}, "tool-fingerprint", tooling.ToolSurfacePolicySnapshot{Hash: "policy-hash"}),
		DefaultContextBudgetPolicy(32000, 8000).Thresholds(),
		"synthetic-model",
	)
	if err != nil {
		t.Fatalf("buildRuntimeStepContext() error = %v", err)
	}
	if len(promptBuild.Items) == 0 {
		t.Fatalf("missing prompt/model input: items=%d", len(promptBuild.Items))
	}
	if step.ProviderRequest.ModelInputHash == "" || step.ProviderRequest.PromptCacheKey == "" {
		t.Fatalf("provider request missing hashes: %#v", step.ProviderRequest)
	}
	if len(step.ProviderRequest.Input) != len(promptBuild.Items) {
		t.Fatalf("provider input len = %d, want %d", len(step.ProviderRequest.Input), len(promptBuild.Items))
	}
	if step.ToolSurface.PolicyHash != "policy-hash" || len(step.ToolSurface.ModelVisibleTools) != 1 {
		t.Fatalf("tool surface = %#v", step.ToolSurface)
	}
}
