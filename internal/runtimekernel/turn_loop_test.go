package runtimekernel

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
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
	if !step.PromptShadowParity.Passed || len(step.PromptShadowParity.GateViolations) != 0 || len(step.PromptShadowParity.Layers) != 7 {
		t.Fatalf("prompt shadow parity = %#v, want passed L0-L6 report", step.PromptShadowParity)
	}
	if step.PromptShadowParity.LegacyFacts.ToolVisibilityHash != step.PromptShadowParity.V2Facts.ToolVisibilityHash || step.PromptShadowParity.LegacyFacts.PolicyHash != "policy-hash" {
		t.Fatalf("prompt shadow control facts = legacy %#v v2 %#v", step.PromptShadowParity.LegacyFacts, step.PromptShadowParity.V2Facts)
	}
	shadowJSON, _ := json.Marshal(step.PromptShadowParity)
	if strings.Contains(string(shadowJSON), "check nginx errors") {
		t.Fatalf("prompt shadow parity leaked current user input: %s", shadowJSON)
	}
	tracePath, err := writeRuntimeStepTrace(modeltrace.Config{Enabled: true, RootDir: t.TempDir()}, step, RuntimeTraceDebugRequest{})
	if err != nil {
		t.Fatalf("writeRuntimeStepTrace() error = %v", err)
	}
	traceJSON, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	if !strings.Contains(string(traceJSON), `"promptShadowParity"`) || !strings.Contains(string(traceJSON), `"passed": true`) {
		t.Fatalf("trace missing passed prompt shadow parity: %s", traceJSON)
	}
	t.Run("deprecated failed parity is read-only", func(t *testing.T) {
		tamperedShadow := step
		tamperedShadow.PromptShadowParity.GateViolations = []string{"deprecated_shadow_drift"}
		tamperedShadow.PromptShadowParity.Passed = false
		if _, err := tamperedShadow.ValidatedProviderRequest(); err != nil {
			t.Fatalf("deprecated failed prompt shadow parity blocked provider request: %v", err)
		}
	})
	t.Run("deprecated partial parity is read-only", func(t *testing.T) {
		partialShadow := step
		partialShadow.PromptShadowParity.SchemaVersion = ""
		if _, err := partialShadow.ValidatedProviderRequest(); err != nil {
			t.Fatalf("deprecated partial prompt shadow parity blocked provider request: %v", err)
		}
	})
	t.Run("deprecated legacy shadow build error is isolated", func(t *testing.T) {
		if _, err := step.ValidatedProviderRequest(); err != nil {
			t.Fatalf("canonical V2 provider request is not valid before legacy shadow: %v", err)
		}
		invalidLegacyHistory := []Message{
			{Role: "system", Content: "legacy-only derived context", Metadata: map[string]string{"runtime.context.kind": "unsupported_legacy_shadow"}},
			{Role: "user", Content: "check nginx errors"},
		}
		legacyShadow := buildRuntimePromptShadowParity(invalidLegacyHistory, compiled, step.ProviderRequest.Input, 0, nil, step.ToolSurface, step.ProviderRequest.Tools)
		if !legacyShadow.IsZero() {
			t.Fatalf("legacy shadow build failure report = %#v, want empty read-only trace", legacyShadow)
		}
	})
	t.Run("canonical provider request remains fail closed", func(t *testing.T) {
		tamperedProvider, err := cloneRuntimeStepContext(step)
		if err != nil {
			t.Fatalf("cloneRuntimeStepContext() error = %v", err)
		}
		tamperedProvider.ProviderRequest.Input[0].Content = "tampered canonical V2 input"
		if _, err := tamperedProvider.ValidatedProviderRequest(); err == nil {
			t.Fatal("ValidatedProviderRequest() accepted canonical V2/provider tamper")
		}
	})
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
