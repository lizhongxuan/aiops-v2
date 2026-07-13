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
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: string(SessionTypeHost),
		Mode:        string(ModeChat),
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
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
		RuntimeStepControlFacts{TurnAssemblyHash: "assembly-hash", PermissionHash: "permission-hash", CheckpointRef: "checkpoint-1"},
		DefaultContextBudgetPolicy(32000, 8000).Thresholds(),
		"synthetic-model",
	)
	if err != nil {
		t.Fatalf("buildRuntimeStepContext() error = %v", err)
	}
	if len(promptBuild.Items) == 0 {
		t.Fatalf("missing prompt/model input: items=%d", len(promptBuild.Items))
	}
	if promptBuild.Items[0].Source.Layer != string(promptcompiler.LayerAbsoluteSystemCore) || promptBuild.Items[len(promptBuild.Items)-1].Source.Layer != string(promptcompiler.LayerCurrentUserInput) {
		t.Fatalf("model input does not span L0-L6: %#v", promptBuild.Items)
	}
	if step.ProviderRequest.ModelInputHash == "" || step.ProviderRequest.PromptCacheKey == "" {
		t.Fatalf("provider request missing hashes: %#v", step.ProviderRequest)
	}
	if step.ProviderRequest.PromptFingerprint.StablePrefixHash == "" || step.ProviderRequest.PromptFingerprint.TurnPrefixHash == "" || step.ProviderRequest.PromptFingerprint.ModelInputHash != step.ProviderRequest.ModelInputHash {
		t.Fatalf("provider request missing canonical prompt fingerprint: %#v", step.ProviderRequest.PromptFingerprint)
	}
	if step.Compiled.Fingerprint != step.ProviderRequest.PromptFingerprint {
		t.Fatalf("step compiled/provider fingerprints diverged: compiled=%#v provider=%#v", step.Compiled.Fingerprint, step.ProviderRequest.PromptFingerprint)
	}
	tamperedCompiledFingerprint := step
	tamperedCompiledFingerprint.Compiled.Fingerprint.ModelInputHash = "tampered"
	tamperedCompiledFingerprint.Hash = ComputeRuntimeStepContextHash(tamperedCompiledFingerprint)
	if err := tamperedCompiledFingerprint.Validate(); err == nil {
		t.Fatal("Validate() accepted compiled/provider prompt fingerprint drift")
	}
	if len(step.ProviderRequest.Input) != len(promptBuild.Items) {
		t.Fatalf("provider input len = %d, want %d", len(step.ProviderRequest.Input), len(promptBuild.Items))
	}
	if step.ToolSurface.PolicyHash != "policy-hash" || len(step.ToolSurface.ModelVisibleTools) != 1 {
		t.Fatalf("tool surface = %#v", step.ToolSurface)
	}
	if step.Hash == "" || step.TurnAssemblyHash != "assembly-hash" || step.PermissionHash != "permission-hash" {
		t.Fatalf("step control hashes = %#v", step)
	}
}
