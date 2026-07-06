package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/specialinputmemory"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherExecutionScopeGuardAllowsBoundLowRiskHostExec(t *testing.T) {
	executor := &mockToolExecutor{result: "ok"}
	dispatcher := NewToolDispatcher(executionScopeGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithExecutionScopeGuard(ExecutionScopeGuardConfig{
			Enabled: true,
			Grants: []specialinputmemory.ExecutionScopeGrant{executionScopeGrant("host-a", []string{
				specialinputmemory.ActionInspect,
				specialinputmemory.ActionRead,
				specialinputmemory.ActionExecLowRisk,
			})},
		})

	result := dispatcher.Dispatch(context.Background(), "sess-scope", "turn-scope", ToolCall{
		ID:        "call-ok",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-a","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Blocked || result.Error != "" {
		t.Fatalf("dispatch result = %#v, want allowed low-risk host exec", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestToolDispatcherExecutionScopeGuardBlocksCrossHostToolCall(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(executionScopeGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithExecutionScopeGuard(ExecutionScopeGuardConfig{
			Enabled: true,
			Grants:  []specialinputmemory.ExecutionScopeGrant{executionScopeGrant("host-a", []string{specialinputmemory.ActionExecLowRisk})},
		})

	result := dispatcher.Dispatch(context.Background(), "sess-scope", "turn-scope", ToolCall{
		ID:        "call-cross-host",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-b","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "execution_scope_guard") {
		t.Fatalf("dispatch result = %#v, want execution scope guard denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherExecutionScopeGuardBlocksHostToolWithoutActiveGrant(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(executionScopeGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithExecutionScopeGuard(ExecutionScopeGuardConfig{Enabled: true})

	result := dispatcher.Dispatch(context.Background(), "sess-scope", "turn-scope", ToolCall{
		ID:        "call-missing-grant",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-a","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "no active execution scope grant") {
		t.Fatalf("dispatch result = %#v, want missing-grant denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherExecutionScopeGuardBlocksMutationWithoutMutationGrant(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(executionScopeGuardLookup(executor, tooling.ToolMetadata{Name: "host.mutate", Mutating: true, RiskLevel: tooling.ToolRiskHigh}), nil, &testMockEventEmitter{}).
		WithExecutionScopeGuard(ExecutionScopeGuardConfig{
			Enabled: true,
			Grants:  []specialinputmemory.ExecutionScopeGrant{executionScopeGrant("host-a", []string{specialinputmemory.ActionExecLowRisk})},
		})

	result := dispatcher.Dispatch(context.Background(), "sess-scope", "turn-scope", ToolCall{
		ID:        "call-mutate",
		Name:      "host.mutate",
		Arguments: json.RawMessage(`{"hostId":"host-a","command":"systemctl restart app"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "action mutate is not allowed") {
		t.Fatalf("dispatch result = %#v, want action mismatch denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherExecutionScopeGuardDisabledKeepsLegacyDispatch(t *testing.T) {
	executor := &mockToolExecutor{result: "ok"}
	dispatcher := NewToolDispatcher(executionScopeGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithExecutionScopeGuard(ExecutionScopeGuardConfig{Enabled: false})

	result := dispatcher.Dispatch(context.Background(), "sess-scope", "turn-scope", ToolCall{
		ID:        "call-legacy",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-b","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Blocked || result.Error != "" {
		t.Fatalf("dispatch result = %#v, want legacy dispatch when guard disabled", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
}

func TestRuntimeKernelIterationDispatcherUsesSnapshotSpecialInputReadPlan(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow},
		ExecuteFunc: func(ctx context.Context, args json.RawMessage) (tooling.ToolResult, error) {
			return executor.Execute(ctx, args)
		},
	}
	kernel := NewRuntimeKernel(RuntimeKernelConfig{Projector: &testMockEventEmitter{}})
	session := &SessionState{ID: "sess-scope-runtime", Type: SessionTypeHost, Mode: ModeExecute}
	snapshot := &TurnSnapshot{
		ID:          "turn-scope-runtime",
		SessionID:   session.ID,
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		SpecialInputReadPlan: &specialinputmemory.MemoryReadPlan{
			ActiveExecutionScope: ptrExecutionScopeGrant(executionScopeGrant("host-a", []string{specialinputmemory.ActionExecLowRisk})),
		},
	}

	dispatcher := kernel.newIterationDispatcher(session, snapshot, 0, []promptcompiler.Tool{toolDef}, RuntimeToolRouterSnapshot{})
	result := dispatcher.Dispatch(context.Background(), session.ID, snapshot.ID, ToolCall{
		ID:        "call-cross-runtime",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-b","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "execution_scope_guard") {
		t.Fatalf("dispatch result = %#v, want execution scope guard denial from snapshot read plan", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func executionScopeGuardLookup(executor ToolExecutor, meta tooling.ToolMetadata) *mockToolLookup {
	return &mockToolLookup{tools: map[string]mockToolEntry{
		meta.Name: {desc: ToolDescriptor{Metadata: meta}, executor: executor},
	}}
}

func executionScopeGrant(hostID string, actions []string) specialinputmemory.ExecutionScopeGrant {
	return specialinputmemory.ExecutionScopeGrant{
		ID:             "grant-" + hostID,
		ResourceKind:   specialinputmemory.ResourceKindHost,
		ResourceID:     hostID,
		CanonicalKey:   "host:" + hostID,
		AllowedActions: actions,
		Status:         specialinputmemory.GrantStatusActive,
		ExpiresAt:      time.Now().Add(time.Hour),
	}
}

func ptrExecutionScopeGrant(grant specialinputmemory.ExecutionScopeGrant) *specialinputmemory.ExecutionScopeGrant {
	return &grant
}
