package runtimekernel

import (
	"context"
	"encoding/json"
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
