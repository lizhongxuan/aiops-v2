package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

type recordingToolResourceLockGate struct {
	decision ToolResourceLockDecision
	requests []ToolResourceLockRequest
	released int
}

func (g *recordingToolResourceLockGate) AcquireToolResourceLocks(_ context.Context, req ToolResourceLockRequest) (ToolResourceLockDecision, func(), error) {
	g.requests = append(g.requests, req)
	decision := g.decision
	if decision.Action == "" {
		decision.Action = "acquired"
	}
	return decision, func() { g.released++ }, nil
}

func TestToolDispatcherAcquiresAndReleasesResourceLockForMutatingTool(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "changed"}
	gate := &recordingToolResourceLockGate{}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:     "write_config",
				Mutating: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithResourceLockGate(gate)

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: json.RawMessage(`{"path":"/etc/example.conf"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Content != "changed" || result.Error != "" {
		t.Fatalf("dispatch result = %#v, want successful tool result", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 after lock acquired", executor.calls)
	}
	if len(gate.requests) != 1 {
		t.Fatalf("lock requests = %d, want 1", len(gate.requests))
	}
	if gate.released != 1 {
		t.Fatalf("lock releases = %d, want 1", gate.released)
	}
	req := gate.requests[0]
	if req.OwnerID == "" || req.ToolCall.ID != "call-lock" || len(req.Keys) != 1 {
		t.Fatalf("lock request = %#v, want owner, tool call, and one key", req)
	}
	if len(result.ResourceLocks) != 1 || result.ResourceLocks[0].Action != "acquired" {
		t.Fatalf("resource lock trace = %#v, want acquired trace", result.ResourceLocks)
	}
}

func TestToolDispatcherBlocksMutatingToolOnResourceLockConflict(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	gate := &recordingToolResourceLockGate{decision: ToolResourceLockDecision{
		Action: "denied",
		Reason: "resource_lock_conflict",
		Holder: "worker-a",
	}}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:     "write_config",
				Mutating: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithResourceLockGate(gate)

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: json.RawMessage(`{"path":"/etc/example.conf"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Outcome != "tool_failed" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_failed", result)
	}
	if !strings.Contains(result.Error, "resource_lock_conflict") || !strings.Contains(result.Error, "worker-a") {
		t.Fatalf("dispatch error = %q, want structured resource lock conflict with holder", result.Error)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when lock conflicts", executor.calls)
	}
	if gate.released != 0 {
		t.Fatalf("lock releases = %d, want 0 for unacquired lock", gate.released)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s despite resource lock conflict", EventToolStarted)
		}
	}
	if len(result.ResourceLocks) != 1 || result.ResourceLocks[0].Action != "denied" || result.ResourceLocks[0].Holder != "worker-a" {
		t.Fatalf("resource lock trace = %#v, want denied holder trace", result.ResourceLocks)
	}
}
