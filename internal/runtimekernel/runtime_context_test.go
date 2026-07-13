package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

func TestBuildRuntimeTurnContextFreezesRequestMetadata(t *testing.T) {
	req := TurnRequest{
		SessionType:     SessionTypeHost,
		Mode:            ModeChat,
		SessionID:       "session-1",
		TurnID:          "turn-1",
		ClientTurnID:    "client-turn-1",
		ClientMessageID: "client-message-1",
		HostID:          "host-a",
		Metadata: map[string]string{
			"profile":          RuntimePromptProfileHostWorker,
			"reasoningEffort":  "high",
			"approvalPolicy":   "on-request",
			"runtimeRoute":     "host",
			"contextBudgetKey": "host-default",
		},
	}
	session := &SessionState{ID: "session-1", Type: SessionTypeHost, Mode: ModeChat, HostID: "host-a"}

	ctx, err := BuildRuntimeTurnContext(req, session, RuntimeTurnContextOptions{
		Model: modelrouter.ModelCapabilities{Provider: "openai", Model: "gpt-4.1", MaxContextTokens: 200000},
		ContextBudget: RuntimeContextBudgetSnapshot{
			MaxTokens:    200000,
			TargetTokens: 120000,
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
	}

	req.Metadata["profile"] = "mutated-after-build"
	if ctx.Profile != RuntimePromptProfileHostWorker {
		t.Fatalf("Profile = %q, want frozen %q", ctx.Profile, RuntimePromptProfileHostWorker)
	}
	if ctx.SessionID != "session-1" || ctx.TurnID != "turn-1" || ctx.HostID != "host-a" {
		t.Fatalf("unexpected ids: %#v", ctx)
	}
	if ctx.Model.Model != "gpt-4.1" {
		t.Fatalf("model = %q, want gpt-4.1", ctx.Model.Model)
	}
	if ctx.Permission.ApprovalPolicy != "on-request" {
		t.Fatalf("approval policy = %q", ctx.Permission.ApprovalPolicy)
	}
}

func TestBuildRuntimeTurnContextBuildsShadowAdmissionFacts(t *testing.T) {
	frameJSON, err := json.Marshal(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindChange,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite},
	})
	if err != nil {
		t.Fatalf("json.Marshal(IntentFrame) error = %v", err)
	}
	binding := resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeHost,
		ID:   "host-a",
	}, resourcebinding.BindingOptions{
		Source:     resourcebinding.BindingSourceMention,
		VerifiedBy: "runtime-context-test",
		TrustLevel: resourcebinding.TrustLevelVerified,
	})
	req := TurnRequest{
		SessionType:      SessionTypeHost,
		Mode:             ModeExecute,
		SessionID:        "session-admission",
		TurnID:           "turn-admission",
		HostID:           "host-a",
		ResourceBindings: []resourcebinding.ResourceBindingSnapshot{binding},
		Metadata: map[string]string{
			runtimecontract.MetadataIntentFrame: string(frameJSON),
			runtimecontract.MetadataProfile:     RuntimePromptProfileHostWorker,
			"future.unregistered":               "trace-only",
		},
	}
	ctx, err := BuildRuntimeTurnContext(req, nil, RuntimeTurnContextOptions{
		Lineage: RuntimeLineageSnapshot{AgentKind: "host_worker"},
	})
	if err != nil {
		t.Fatalf("BuildRuntimeTurnContext() error = %v", err)
	}
	if ctx.AdmissionFacts.Hash == "" {
		t.Fatal("AdmissionFacts.Hash is empty")
	}
	if ctx.AdmissionFacts.Intent.Kind != runtimecontract.IntentKindChange {
		t.Fatalf("AdmissionFacts.Intent.Kind = %q, want change", ctx.AdmissionFacts.Intent.Kind)
	}
	if ctx.AdmissionFacts.SessionTarget.ID != "host-a" || ctx.AdmissionFacts.Profile != RuntimePromptProfileHostWorker {
		t.Fatalf("AdmissionFacts target/profile = %#v / %q", ctx.AdmissionFacts.SessionTarget, ctx.AdmissionFacts.Profile)
	}
	if len(ctx.AdmissionFacts.CompatibilityOnlyKeys) != 1 || ctx.AdmissionFacts.CompatibilityOnlyKeys[0] != "future.unregistered" {
		t.Fatalf("compatibility keys = %#v, want future.unregistered", ctx.AdmissionFacts.CompatibilityOnlyKeys)
	}

	req.ResourceBindings[0].Ref.ID = "mutated"
	req.Metadata["future.unregistered"] = "mutated"
	if ctx.AdmissionFacts.ResourceBindings[0].Ref.ID != "host-a" || ctx.AdmissionFacts.CompatibilityOnlyKeys[0] != "future.unregistered" {
		t.Fatalf("shadow admission facts mutated with request: %#v", ctx.AdmissionFacts)
	}
}

func TestBuildRuntimeTurnContextKeepsAdmissionValidationShadowOnly(t *testing.T) {
	ctx, err := BuildRuntimeTurnContext(TurnRequest{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		SessionID:   "session-shadow-invalid",
		TurnID:      "turn-shadow-invalid",
		Metadata: map[string]string{
			runtimecontract.MetadataIntentKind:       "secret-canary-value",
			runtimecontract.MetadataIntentRiskBudget: string(runtimecontract.ActionRiskWrite),
		},
	}, nil, RuntimeTurnContextOptions{})
	if err != nil {
		t.Fatalf("BuildRuntimeTurnContext() error = %v, want shadow-only validation", err)
	}
	if ctx.AdmissionError == "" || ctx.AdmissionFacts.Hash == "" {
		t.Fatalf("shadow admission result = %#v, want typed facts plus validation error", ctx)
	}
	if ctx.AdmissionError != "admission_facts_invalid" || strings.Contains(ctx.AdmissionError, "secret-canary-value") {
		t.Fatalf("AdmissionError = %q, want stable redacted code", ctx.AdmissionError)
	}
}

func TestRuntimeStepContextOwnsModelInputProviderRequestAndToolSurface(t *testing.T) {
	turn := RuntimeTurnContext{
		SessionID:   "session-1",
		TurnID:      "turn-1",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Profile:     RuntimePromptProfileHostWorker,
	}
	item := promptinput.ModelInputItem{
		ID:           "item-user",
		ProviderRole: promptinput.ProviderRoleUser,
		SemanticRole: "user_request",
		Content:      "check nginx",
	}
	step := RuntimeStepContext{
		Turn:             turn,
		TurnAssemblyHash: "assembly-hash",
		PermissionHash:   "permission-hash",
		Iteration:        2,
		Compiled:         promptcompiler.CompiledPrompt{},
		ModelInput:       []promptinput.ModelInputItem{item},
		ToolSurface: RuntimeToolRouterSnapshot{
			RegisteredTools:   []string{"exec_command"},
			ModelVisibleTools: []string{"exec_command"},
			DispatchableTools: []string{"exec_command"},
			PolicyHash:        "policy-hash",
			Fingerprint:       "tool-fp",
		},
	}
	var err error
	step, err = FreezeRuntimeStepContext(step)
	if err != nil {
		t.Fatalf("FreezeRuntimeStepContext() error = %v", err)
	}
	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := step.ModelInput[0].ID; got != "item-user" {
		t.Fatalf("model input id = %q", got)
	}
}
