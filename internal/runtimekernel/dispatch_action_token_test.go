package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

type captureExecutor struct {
	args json.RawMessage
	ctx  tooling.ToolExecutionContext
}

func (e *captureExecutor) Execute(ctx context.Context, args json.RawMessage) (tooling.ToolResult, error) {
	e.args = append(json.RawMessage(nil), args...)
	e.ctx, _ = tooling.ToolExecutionContextFrom(ctx)
	return tooling.ToolResult{Content: "ok"}, nil
}

func TestToolDispatcherExtractsActionTokenIntoExecutionContextAndStripsUnknownSchemaField(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &captureExecutor{}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"exec_command": {
			desc: ToolDescriptor{
				Metadata:    tooling.ToolMetadata{Name: "exec_command"},
				InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
			},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{HostID: "host-a"})

	result := dispatcher.Dispatch(ctx, "sess-1", "turn-1", ToolCall{
		ID:        "call-1",
		Name:      "exec_command",
		Arguments: json.RawMessage(`{"command":"date","actionToken":"tok-1","incidentId":"inc-1"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Error != "" {
		t.Fatalf("Dispatch() error = %s", result.Error)
	}
	if string(executor.args) != `{"command":"date","incidentId":"inc-1"}` {
		t.Fatalf("executor args = %s, want actionToken stripped", executor.args)
	}
	if executor.ctx.SessionID != "sess-1" || executor.ctx.TurnID != "turn-1" || executor.ctx.ToolCallID != "call-1" || executor.ctx.HostID != "host-a" || executor.ctx.IncidentID != "inc-1" || executor.ctx.ActionToken != "tok-1" {
		t.Fatalf("execution context = %#v", executor.ctx)
	}
}

func TestToolDispatcherKeepsActionTokenWhenSchemaDeclaresIt(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &captureExecutor{}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"token_aware": {
			desc: ToolDescriptor{
				Metadata:    tooling.ToolMetadata{Name: "token_aware"},
				InputSchema: json.RawMessage(`{"type":"object","properties":{"actionToken":{"type":"string"},"value":{"type":"string"}}}`),
			},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(context.Background(), "sess-1", "turn-1", ToolCall{
		ID:        "call-1",
		Name:      "token_aware",
		Arguments: json.RawMessage(`{"value":"x","actionToken":"tok-1"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Error != "" {
		t.Fatalf("Dispatch() error = %s", result.Error)
	}
	if string(executor.args) != `{"value":"x","actionToken":"tok-1"}` {
		t.Fatalf("executor args = %s, want actionToken preserved", executor.args)
	}
	if executor.ctx.ActionToken != "tok-1" {
		t.Fatalf("context action token = %q, want tok-1", executor.ctx.ActionToken)
	}
}
