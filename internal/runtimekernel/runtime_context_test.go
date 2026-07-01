package runtimekernel

import (
	"testing"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
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
		Turn:       turn,
		Iteration:  2,
		Compiled:   promptcompiler.CompiledPrompt{},
		ModelInput: []promptinput.ModelInputItem{item},
		ToolSurface: RuntimeToolRouterSnapshot{
			RegisteredTools:   []string{"exec_command"},
			ModelVisibleTools: []string{"exec_command"},
			DispatchableTools: []string{"exec_command"},
			PolicyHash:        "policy-hash",
			Fingerprint:       "tool-fp",
		},
	}
	if err := step.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := step.ModelInput[0].ID; got != "item-user" {
		t.Fatalf("model input id = %q", got)
	}
}
