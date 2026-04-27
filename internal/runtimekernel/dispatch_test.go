package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcher_DoesNotEmitStartedBeforeApproval(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"restart_service": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "restart_service",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, &policyengine.Engine{
		ModePolicy: map[string]policyengine.ModePolicy{},
	}, emitter).WithPermissions(permissions.NewEngine([]permissions.Rule{
		{
			Name:   "ask-before-restart",
			Action: permissions.ActionAsk,
			Reason: "service restart needs explicit approval",
			Matcher: permissions.Matcher{
				ToolNames: []string{"restart_service"},
			},
		},
	}))

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-approval",
		"turn-approval",
		ToolCall{
			ID:        "tool-restart-1",
			Name:      "restart_service",
			Arguments: json.RawMessage(`{"service":"redis"}`),
		},
		SessionTypeHost,
		ModeExecute,
	)

	if !result.Blocked || result.Outcome != "approval_needed" {
		t.Fatalf("dispatch result = %#v, want blocked approval_needed", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before approval", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s before approval was resolved", EventToolStarted)
		}
	}
}

func TestToolDispatcher_DispatchApprovedEmitsStartedAfterApprovalGate(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "restarted"}
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"restart_service": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "restart_service",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: executor,
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithPermissions(permissions.NewEngine([]permissions.Rule{
		{
			Name:   "ask-before-restart",
			Action: permissions.ActionAsk,
			Reason: "service restart needs explicit approval",
			Matcher: permissions.Matcher{
				ToolNames: []string{"restart_service"},
			},
		},
	}))

	result := dispatcher.DispatchApproved(
		context.Background(),
		"sess-approval",
		"turn-approval",
		ToolCall{
			ID:        "tool-restart-1",
			Name:      "restart_service",
			Arguments: json.RawMessage(`{"service":"redis"}`),
		},
		SessionTypeHost,
		ModeExecute,
	)

	if result.Content != "restarted" {
		t.Fatalf("dispatch result content = %q, want restarted", result.Content)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 after approval", executor.calls)
	}
	if len(emitter.events) != 2 {
		t.Fatalf("events = %d, want started and completed", len(emitter.events))
	}
	if emitter.events[0].Type != EventToolStarted || emitter.events[1].Type != EventToolCompleted {
		t.Fatalf("events = %q then %q, want tool.started then tool.completed", emitter.events[0].Type, emitter.events[1].Type)
	}
}

func TestToolDispatcher_CompletedPayloadFitsBudgetForLargeResult(t *testing.T) {
	emitter := &testMockEventEmitter{}
	largeResult := strings.Repeat("x", 20*1024)
	lookup := &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_log": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:   "read_log",
						Origin: tooling.ToolOriginBuiltin,
					},
				},
				executor: &mockToolExecutor{result: largeResult},
			},
		},
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter)

	result := dispatcher.Dispatch(
		context.Background(),
		"sess-budget",
		"turn-budget",
		ToolCall{
			ID:        "tool-large-1",
			Name:      "read_log",
			Arguments: json.RawMessage(`{"path":"/var/log/app.log"}`),
		},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Content != largeResult {
		t.Fatal("dispatch result should keep full content for model context")
	}
	var completed *LifecycleEvent
	for i := range emitter.events {
		if emitter.events[i].Type == EventToolCompleted {
			completed = &emitter.events[i]
			break
		}
	}
	if completed == nil {
		t.Fatal("expected tool.completed event")
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed lifecycle event: %v", err)
	}
	if len(encoded) > 8192 {
		t.Fatalf("tool event payload = %d bytes, want <= 8192", len(encoded))
	}
	var payload map[string]any
	if err := json.Unmarshal(completed.Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload["result"] == largeResult {
		t.Fatal("tool.completed payload should not include the full large result")
	}
	if payload["rawRef"] == "" {
		t.Fatal("tool.completed payload should include rawRef for large results")
	}
	if payload["outputPreview"] != nil {
		t.Fatal("large tool result should omit outputPreview")
	}
}
