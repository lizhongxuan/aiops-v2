package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/runtimekernel/toolfailure"
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
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"cat config://service-a"},
				},
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
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "file",
					ResourceID:    "config://service-a",
					OperationKind: "write",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"cat config://service-a"},
				},
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

func TestToolDispatcherResourceLockMutationRetryGuardDeniesMutationWithoutSafetyMetadata(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "should-not-run"}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_config": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "write_config",
				Mutating:         true,
				RequiresApproval: true,
			}},
			executor: executor,
		},
	}}
	input := json.RawMessage(`{"path":"/etc/example.conf","content":"changed"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).WithSessionApprovalGrants([]SessionApprovalGrant{{
		ToolName:  "write_config",
		InputHash: hash,
	}})

	result := dispatcher.Dispatch(context.Background(), "sess-lock", "turn-lock", ToolCall{
		ID:        "call-lock",
		Name:      "write_config",
		Arguments: input,
	}, SessionTypeHost, ModeExecute)

	if result.Outcome != "tool_denied" || result.Source != "runtime" {
		t.Fatalf("dispatch result = %#v, want runtime tool_denied", result)
	}
	for _, want := range []string{"mutation_safety_guard", "resourceLocks", "idempotency"} {
		if !strings.Contains(result.Error, want) {
			t.Fatalf("dispatch error = %q, want %q", result.Error, want)
		}
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 when mutation safety metadata is missing", executor.calls)
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("emitted %s despite mutation safety guard denial", EventToolStarted)
		}
	}
}

func TestMutationRetryGuardFailureRequiresPostCheck(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{err: errors.New("context deadline exceeded after package manager started")}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"install_package": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:             "install_package",
				Mutating:         true,
				RequiresApproval: true,
				ResourceLocks: []tooling.ToolResourceLockKey{{
					ResourceType:  "host",
					ResourceID:    "host-a",
					OperationKind: "package_install",
				}},
				Idempotency: tooling.ToolIdempotencyMetadata{
					Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
					PostCheckRefs: []string{"psql --version"},
				},
			}},
			executor: executor,
		},
	}}
	input := json.RawMessage(`{"package":"postgresql"}`)
	hash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithResourceLockGate(&recordingToolResourceLockGate{}).
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{Enabled: true, MaxPerCall: 1, MaxPerTurn: 1}).
		WithSessionApprovalGrants([]SessionApprovalGrant{{
			ToolName:  "install_package",
			InputHash: hash,
		}})

	result := dispatcher.Dispatch(context.Background(), "sess-mutation", "turn-mutation", ToolCall{
		ID:        "call-install",
		Name:      "install_package",
		Arguments: input,
	}, SessionTypeHost, ModeExecute)

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want no automatic retry for mutation timeout", executor.calls)
	}
	if len(result.Attempts) != 0 {
		t.Fatalf("retry attempts = %#v, want none for mutation failure", result.Attempts)
	}
	if failureKindForDispatchResult(result) != string(toolfailure.KindSideEffectUnknown) {
		t.Fatalf("failure kind = %q, want side_effect_unknown", failureKindForDispatchResult(result))
	}
	toolResult := failedToolResultForModel(ToolCall{ID: "call-install", Name: "install_package"}, result)
	for _, want := range []string{"side_effect_unknown", "postCheckRequired", "psql --version"} {
		if !strings.Contains(toolResult.Content, want) {
			t.Fatalf("failed tool result = %s, want %q", toolResult.Content, want)
		}
	}
}
