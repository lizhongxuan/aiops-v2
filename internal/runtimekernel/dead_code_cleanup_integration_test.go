package runtimekernel

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
)

func TestRunTurnDeadCodeCleanupPreservesCanonicalNoToolPath(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("dead-code cleanup proof", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-dead-code-cleanup",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-dead-code-cleanup",
		Input:       "answer directly",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" || strings.TrimSpace(result.Output) != "dead-code cleanup proof" {
		t.Fatalf("result = %#v, want completed canonical answer", result)
	}

	session := kernel.sessions.Get("sess-dead-code-cleanup")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected persisted current turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	wantTypes := []agentstate.TurnItemType{
		agentstate.TurnItemTypeUserMessage,
		agentstate.TurnItemTypeModelCall,
		agentstate.TurnItemTypeAssistantMessage,
		agentstate.TurnItemTypeFinalResponse,
	}
	if got := agentItemTypes(session.CurrentTurn.AgentItems); !sameTurnItemTypes(got, wantTypes) {
		t.Fatalf("agent item types = %v, want %v", got, wantTypes)
	}
}
