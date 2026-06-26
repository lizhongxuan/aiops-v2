package runtimekernel

import (
	"context"
	"testing"
)

func TestNoopObserverDoesNotModifyContext(t *testing.T) {
	ctx := context.Background()
	observer := NoopObserver{}
	next, span := observer.StartTurn(ctx, TurnSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Input:     "check disk",
	})
	if next == nil {
		t.Fatal("StartTurn returned nil context")
	}
	if span == nil {
		t.Fatal("StartTurn returned nil span")
	}
	span.SetStatus("completed", "")
	span.SetAttributes(map[string]any{"turn.status": "completed"})
	span.End()
}

func TestNoopObserverNormalizesNilContext(t *testing.T) {
	observer := NoopObserver{}
	next, span := observer.StartModelCall(nil, ModelCallSpanAttrs{
		SessionID: "sess-1",
		TurnID:    "turn-1",
	})
	if next == nil {
		t.Fatal("StartModelCall returned nil context")
	}
	if span == nil {
		t.Fatal("StartModelCall returned nil span")
	}
}

func TestModelSpanAttrsCarryLocalTracePaths(t *testing.T) {
	attrs := ModelCallSpanAttrs{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		Iteration:        2,
		PromptStableHash: "abc123",
		TraceFile:        ".data/model-input-traces/sess-1/turn-1/iteration-002.md",
		TraceDiffFile:    ".data/model-input-traces/sess-1/turn-1/input.diff.md",
		VisibleTools:     []string{"read_file", "run_command"},
	}
	if attrs.TraceFile == "" || attrs.TraceDiffFile == "" {
		t.Fatalf("trace paths should be retained: %#v", attrs)
	}
	if len(attrs.VisibleTools) != 2 {
		t.Fatalf("visible tools length = %d, want 2", len(attrs.VisibleTools))
	}
}

func TestRuntimeKernelConfigAcceptsObserver(t *testing.T) {
	kernel := NewRuntimeKernel(RuntimeKernelConfig{
		Observer: NoopObserver{},
	})
	if kernel == nil {
		t.Fatal("NewRuntimeKernel returned nil")
	}
}
