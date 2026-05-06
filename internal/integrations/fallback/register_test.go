package fallback

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	core "aiops-v2/internal/fallback"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsAddsReadOnlyFallbackTools(t *testing.T) {
	service := core.NewService(actionproposal.NewSigner([]byte("fallback-secret"), time.Now), core.NewInMemoryStore(), time.Now)
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("host", "plan")
	if len(tools) != 2 {
		t.Fatalf("AssembleTools() len = %d, want 2", len(tools))
	}
	for _, tool := range tools {
		if !tool.IsReadOnly(nil) || tool.IsDestructive(nil) {
			t.Fatalf("%s should be read-only and non-destructive", tool.Metadata().Name)
		}
	}
	tool := toolByName(t, tools, "fallback.plan_exec")
	result, err := tool.Execute(context.Background(), json.RawMessage(`{
		"sessionId":"sess-1",
		"turnId":"turn-1",
		"incidentId":"inc-1",
		"goal":"检查磁盘",
		"whyNoRunbook":"no runbook",
		"evidenceRefs":["evidence:1"],
		"actions":[{"toolName":"exec_command","toolInput":{"command":"df","args":["-h"]},"reason":"检查磁盘"}]
	}`))
	if err != nil {
		t.Fatalf("fallback.plan_exec Execute() error = %v", err)
	}
	var body struct {
		Status string `json:"status"`
		Plan   struct {
			Actions []actionproposal.ActionProposal `json:"actions"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if body.Status != "ok" || len(body.Plan.Actions) != 1 {
		t.Fatalf("result = %#v, want ok plan with one action", body)
	}
}

func toolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return nil
}
