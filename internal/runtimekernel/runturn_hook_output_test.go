package runtimekernel

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/promptcompiler"
)

type captureCompileContextCompiler struct {
	last promptcompiler.CompileContext
}

func (c *captureCompileContextCompiler) Compile(ctx promptcompiler.CompileContext) (promptcompiler.CompiledPrompt, error) {
	c.last = ctx
	return promptcompiler.CompiledPrompt{
		System:    promptcompiler.SystemPrompt{Content: "system"},
		Developer: promptcompiler.DeveloperInstructions{Content: "dev"},
		Tools:     promptcompiler.ToolPromptSet{Content: "tools"},
		Policy:    promptcompiler.RuntimePolicyPrompt{Content: "policy"},
	}, nil
}

func (c *captureCompileContextCompiler) CompileForEino(_ promptcompiler.CompileContext) ([]*schema.Message, error) {
	return []*schema.Message{{Role: schema.System, Content: "compiled"}}, nil
}

func TestRunTurn_PreTurnHookCanRewriteInput(t *testing.T) {
	registry := hooks.NewRegistry()
	if err := registry.RegisterTurn(hooks.TurnRegistration{
		Name:  "rewrite-input",
		Stage: hooks.StagePreTurn,
		Hook: func(_ context.Context, event *hooks.TurnEvent) error {
			event.UpdatedInput = "rewritten by hook"
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTurn failed: %v", err)
	}

	kernel := newTestKernelWithHooks(nil, registry)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hook-input",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-hook-input",
		Input:       "original input",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q", result.Status)
	}

	session := kernel.sessions.Get("sess-hook-input")
	if session == nil || len(session.Messages) == 0 {
		t.Fatalf("expected session messages to be recorded")
	}
	found := false
	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].Role != "user" {
			continue
		}
		if session.Messages[i].Content == "rewritten by hook" {
			found = true
		}
		break
	}
	if !found {
		t.Fatalf("expected latest user message to be rewritten, got %+v", session.Messages)
	}
}

func TestRunTurn_PreTurnHookCanInjectAdditionalContext(t *testing.T) {
	registry := hooks.NewRegistry()
	if err := registry.RegisterTurn(hooks.TurnRegistration{
		Name:  "inject-context",
		Stage: hooks.StagePreTurn,
		Hook: func(_ context.Context, event *hooks.TurnEvent) error {
			event.AdditionalContext = append(event.AdditionalContext, "hook context fragment")
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTurn failed: %v", err)
	}

	compiler := &captureCompileContextCompiler{}
	kernel := newTestKernelWithHooks(compiler, registry)
	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-hook-context",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-hook-context",
		Input:       "hello",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	found := false
	for _, item := range compiler.last.SkillPromptAssets {
		if item == "hook context fragment" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected additional context in compile context, got %v", compiler.last.SkillPromptAssets)
	}
}
