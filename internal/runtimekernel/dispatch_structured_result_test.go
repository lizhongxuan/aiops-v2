package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

type structuredResultExecutor struct{ result tooling.ToolResult }

func (e structuredResultExecutor) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return e.result, nil
}

type structuredPermissionResultExecutor struct{ structuredResultExecutor }

func (e structuredPermissionResultExecutor) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}

func TestToolDispatcherIsErrorPreservesStructuredDiagnostics(t *testing.T) {
	emitter := &testMockEventEmitter{}
	diagnostic := json.RawMessage(`{"code":"STALE_REVISION","retryHint":"reload"}`)
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"workflow.preview_patch": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "workflow.preview_patch", RiskLevel: tooling.ToolRiskLow}},
			executor: structuredResultExecutor{result: tooling.ToolResult{
				Content: "当前版本已变化，请重新加载。", StructuredContent: diagnostic, IsError: true,
			}},
		},
	}}, nil, emitter).WithPermissionBinding("sha256:test", "sha256:test")

	result := dispatcher.Dispatch(context.Background(), "session", "turn", ToolCall{ID: "call", Name: "workflow.preview_patch", Arguments: json.RawMessage(`{}`)}, SessionTypeWorkspace, ModeInspect)
	if result.Outcome != "tool_failed" || !result.Result.IsError || string(result.Result.StructuredContent) != string(diagnostic) {
		t.Fatalf("dispatch result = %#v", result)
	}
	if len(emitter.events) != 2 || emitter.events[1].Type != EventToolFailed || !strings.Contains(string(emitter.events[1].Payload), "STALE_REVISION") {
		t.Fatalf("events = %#v, want typed tool.failed", emitter.events)
	}
}

func TestToolDispatcherMutatingTypedDomainErrorIsNotSideEffectUnknown(t *testing.T) {
	emitter := &testMockEventEmitter{}
	diagnostic := json.RawMessage(`{"kind":"error","code":"APPROVAL_REQUIRED","retryHint":"request a new approval"}`)
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"workflow.apply_patch": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name: "workflow.apply_patch", Layer: tooling.ToolLayerMutation, Mutating: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{ResourceType: "workflow", ResourceID: "tenant:workflow", OperationKind: "apply"}},
				Idempotency:   tooling.ToolIdempotencyMetadata{Strategy: tooling.ToolIdempotencyStrategyArgumentsHash},
				Rollback:      &tooling.ToolRollbackMetadata{Strategy: tooling.ToolRollbackStrategyAutomatic, Reference: "workflow.inverse"},
			}},
			executor: structuredPermissionResultExecutor{structuredResultExecutor{result: tooling.ToolResult{
				Content: "active workflow grant and approval are required", StructuredContent: diagnostic, IsError: true,
			}}},
		},
	}}, nil, emitter).WithPermissionBinding("sha256:test", "sha256:test")

	result := dispatcher.Dispatch(context.Background(), "session", "turn", ToolCall{ID: "call", Name: "workflow.apply_patch", Arguments: json.RawMessage(`{}`)}, SessionTypeWorkspace, ModeExecute)
	if result.Outcome != "tool_failed" || !result.Result.IsError || string(result.Result.StructuredContent) != string(diagnostic) {
		t.Fatalf("dispatch result = %#v", result)
	}
	if strings.Contains(result.Error, "side_effect_unknown") {
		t.Fatalf("authoritative typed domain rejection was rewritten as uncertain side effect: %q", result.Error)
	}
}

func TestFailedToolResultForModelPreservesTypedDomainError(t *testing.T) {
	typed := json.RawMessage(`{"kind":"error","code":"APPROVAL_REQUIRED","retryHint":"request a new approval"}`)
	result := failedToolResultForModel(ToolCall{ID: "call-apply", Name: "workflow.apply_patch"}, DispatchResult{
		Error: "active workflow grant and approval are required", Outcome: "tool_failed", Source: "tool",
		Result: tooling.ToolResult{ToolCallID: "call-apply", Content: "active workflow grant and approval are required", StructuredContent: typed, IsError: true},
	})
	if !result.IsError || string(result.StructuredContent) != string(typed) {
		t.Fatalf("typed domain error was replaced by a generic classifier result: %#v", result)
	}
}

func TestToolDispatcherValidatesDeclaredOutputSchema(t *testing.T) {
	emitter := &testMockEventEmitter{}
	dispatcher := NewToolDispatcher(&mockToolLookup{tools: map[string]mockToolEntry{
		"workflow.get_context": {
			desc: ToolDescriptor{
				Metadata:     tooling.ToolMetadata{Name: "workflow.get_context", RiskLevel: tooling.ToolRiskLow},
				OutputSchema: json.RawMessage(`{"type":"object","required":["resourceVersion"],"properties":{"resourceVersion":{"type":"string"}}}`),
			},
			executor: structuredResultExecutor{result: tooling.ToolResult{StructuredContent: json.RawMessage(`{"resourceVersion":7}`)}},
		},
	}}, nil, emitter)

	result := dispatcher.Dispatch(context.Background(), "session", "turn", ToolCall{ID: "call", Name: "workflow.get_context", Arguments: json.RawMessage(`{}`)}, SessionTypeWorkspace, ModeInspect)
	if result.Outcome != "tool_failed" || !strings.Contains(result.Error, "output schema") {
		t.Fatalf("dispatch result = %#v", result)
	}
	if len(emitter.events) != 2 || emitter.events[1].Type != EventToolFailed {
		t.Fatalf("events = %#v, want started then failed", emitter.events)
	}
}

func TestMaterializeToolResultPreservesStructuredContent(t *testing.T) {
	snapshot := &TurnSnapshot{ID: "turn", SessionID: "session"}
	session := &SessionState{ID: "session", Type: SessionTypeWorkspace, Mode: ModeInspect, Context: ContextWindow{MaxTokens: DefaultMaxTokens}, CurrentTurn: snapshot}
	diagnostic := json.RawMessage(`{"code":"CONFIGURATION_REQUIRED"}`)
	result, err := (&RuntimeKernel{}).materializeToolResult(session, snapshot, 0, ToolCall{ID: "call", Name: "workflow.validate"}, tooling.ToolMetadata{Name: "workflow.validate"}, tooling.ToolResult{
		Content: "还需配置", StructuredContent: diagnostic, IsError: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || string(result.StructuredContent) != string(diagnostic) {
		t.Fatalf("materialized result = %#v", result)
	}
}
