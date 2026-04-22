package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

type captureToolExecutor struct {
	args   []json.RawMessage
	result string
	err    error
}

func (e *captureToolExecutor) Execute(_ context.Context, args json.RawMessage) (tooling.ToolResult, error) {
	e.args = append(e.args, append(json.RawMessage(nil), args...))
	return tooling.ToolResult{Content: e.result}, e.err
}

func TestToolDispatcher_PreToolHookRewritesInputBeforeExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{ModePolicy: make(map[string]policyengine.ModePolicy)}
	executor := &captureToolExecutor{result: "ok"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name: "disk_usage",
					},
				},
				executor: executor,
			},
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "rewrite-input",
		Stage: hooks.StagePreToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedInput = json.RawMessage(`{"path":"/tmp/after"}`)
			event.AdditionalContext = append(event.AdditionalContext, "input rewritten")
			event.WatchPaths = append(event.WatchPaths, "/tmp/after")
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	dispatcher := NewToolDispatcher(lookup, policy, emitter).WithHooks(registry)
	result := dispatcher.Dispatch(
		context.Background(), "sess-1", "turn-1",
		ToolCall{ID: "tc-1", Name: "disk_usage", Arguments: json.RawMessage(`{"path":"/tmp/before"}`)},
		SessionTypeHost, ModeInspect,
	)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(executor.args) != 1 || string(executor.args[0]) != `{"path":"/tmp/after"}` {
		t.Fatalf("executor args = %q", executor.args)
	}
}

func TestToolDispatcher_PreToolHookPermissionOverrideBlocksExecution(t *testing.T) {
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{ModePolicy: make(map[string]policyengine.ModePolicy)}
	executor := &captureToolExecutor{result: "ok"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name: "disk_usage",
					},
				},
				executor: executor,
			},
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "override-permission",
		Stage: hooks.StagePreToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedPermissions = &tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "hook approval required",
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	dispatcher := NewToolDispatcher(lookup, policy, emitter).WithHooks(registry)
	result := dispatcher.Dispatch(
		context.Background(), "sess-1", "turn-1",
		ToolCall{ID: "tc-1", Name: "disk_usage", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost, ModeInspect,
	)

	if !result.Blocked {
		t.Fatalf("expected blocked result, got %#v", result)
	}
	if result.Reason != "hook approval required" {
		t.Fatalf("blocked reason = %q", result.Reason)
	}
	if len(executor.args) != 0 {
		t.Fatalf("executor should not run, got %d calls", len(executor.args))
	}
}

func TestToolDispatcher_PostToolHookRewritesOutputAndPayload(t *testing.T) {
	emitter := &testMockEventEmitter{}
	policy := &policyengine.Engine{ModePolicy: make(map[string]policyengine.ModePolicy)}
	executor := &captureToolExecutor{result: "original"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"disk_usage": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name: "disk_usage",
					},
				},
				executor: executor,
			},
		},
	}

	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "rewrite-output",
		Stage: hooks.StagePostToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedMCPToolOutput = &tooling.ToolResult{ToolCallID: event.ToolCallID, Content: "rewritten"}
			event.AdditionalContext = append(event.AdditionalContext, "output rewritten")
			event.WatchPaths = append(event.WatchPaths, "/tmp/result.txt")
			event.HideTools = append(event.HideTools, "remote.write")
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}

	dispatcher := NewToolDispatcher(lookup, policy, emitter).WithHooks(registry)
	result := dispatcher.Dispatch(
		context.Background(), "sess-1", "turn-1",
		ToolCall{ID: "tc-1", Name: "disk_usage", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost, ModeInspect,
	)

	if result.Content != "rewritten" {
		t.Fatalf("Dispatch result content = %q", result.Content)
	}
	if len(result.HiddenTools) != 1 || result.HiddenTools[0] != "remote.write" {
		t.Fatalf("Dispatch hidden tools = %v", result.HiddenTools)
	}
	if len(emitter.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(emitter.events))
	}

	var payload map[string]any
	if err := json.Unmarshal(emitter.events[1].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["result"] != "rewritten" {
		t.Fatalf("payload result = %#v", payload["result"])
	}

	additionalContext, ok := payload["additionalContext"].([]any)
	if !ok || len(additionalContext) != 1 || additionalContext[0] != "output rewritten" {
		t.Fatalf("payload additionalContext = %#v", payload["additionalContext"])
	}
	watchPaths, ok := payload["watchPaths"].([]any)
	if !ok || len(watchPaths) != 1 || watchPaths[0] != "/tmp/result.txt" {
		t.Fatalf("payload watchPaths = %#v", payload["watchPaths"])
	}
	hiddenTools, ok := payload["hiddenTools"].([]any)
	if !ok || len(hiddenTools) != 1 || hiddenTools[0] != "remote.write" {
		t.Fatalf("payload hiddenTools = %#v", payload["hiddenTools"])
	}
}
