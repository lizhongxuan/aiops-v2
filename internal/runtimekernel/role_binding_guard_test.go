package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherRoleBindingGuardBlocksCrossHostToolCall(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(roleBindingGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithRoleBindingGuard(RoleBindingGuardConfig{
			Enabled:         true,
			BoundHostID:     "host-a",
			BoundRole:       "pg_primary",
			RoleBindingHash: "role-hash-a",
			RoleBindings:    roleBindingGuardBindings(),
		})

	result := dispatcher.Dispatch(context.Background(), "sess-role", "turn-role", ToolCall{
		ID:        "call-cross-host",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-b","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "role_binding_guard") {
		t.Fatalf("dispatch result = %#v, want role binding guard denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherRoleBindingGuardBlocksWrongTargetRole(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(roleBindingGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithRoleBindingGuard(RoleBindingGuardConfig{
			Enabled:         true,
			BoundHostID:     "host-a",
			BoundRole:       "pg_primary",
			RoleBindingHash: "role-hash-a",
			RoleBindings:    roleBindingGuardBindings(),
		})

	result := dispatcher.Dispatch(context.Background(), "sess-role", "turn-role", ToolCall{
		ID:        "call-wrong-role",
		Name:      "host.exec",
		Arguments: json.RawMessage(`{"hostId":"host-a","targetRole":"pg_standby","command":"uptime"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "target role") {
		t.Fatalf("dispatch result = %#v, want wrong-role denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherRoleBindingGuardBlocksMutationWhenConflictExists(t *testing.T) {
	executor := &mockToolExecutor{result: "should-not-run"}
	dispatcher := NewToolDispatcher(roleBindingGuardLookup(executor, tooling.ToolMetadata{Name: "host.mutate", Mutating: true, RiskLevel: tooling.ToolRiskHigh}), nil, &testMockEventEmitter{}).
		WithRoleBindingGuard(RoleBindingGuardConfig{
			Enabled:         true,
			BoundHostID:     "host-a",
			BoundRole:       "pg_primary",
			RoleBindingHash: "role-hash-a",
			RoleBindings:    roleBindingGuardBindings(),
			RoleConflicts: []resourcebinding.RoleBindingConflict{{
				ResourceID: "host-a",
				Role:       "pg_primary",
				Reasons:    []string{"tool_evidence_conflict"},
			}},
		})

	result := dispatcher.Dispatch(context.Background(), "sess-role", "turn-role", ToolCall{
		ID:        "call-conflict-mutation",
		Name:      "host.mutate",
		Arguments: json.RawMessage(`{"hostId":"host-a","command":"promote"}`),
	}, SessionTypeHost, ModeExecute)

	if !result.Blocked || result.Outcome != "tool_denied" || !strings.Contains(result.Reason, "role conflict") {
		t.Fatalf("dispatch result = %#v, want role conflict mutation denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherRoleBindingGuardDisabledKeepsLegacyDispatch(t *testing.T) {
	executor := &mockToolExecutor{result: "ok"}
	dispatcher := NewToolDispatcher(roleBindingGuardLookup(executor, tooling.ToolMetadata{Name: "host.exec", RiskLevel: tooling.ToolRiskLow}), nil, &testMockEventEmitter{}).
		WithRoleBindingGuard(RoleBindingGuardConfig{
			Enabled:     false,
			BoundHostID: "host-a",
			BoundRole:   "pg_primary",
		})

	result := dispatcher.Dispatch(context.Background(), "sess-role", "turn-role", ToolCall{
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

func roleBindingGuardLookup(executor ToolExecutor, meta tooling.ToolMetadata) *mockToolLookup {
	return &mockToolLookup{tools: map[string]mockToolEntry{
		meta.Name: {desc: ToolDescriptor{Metadata: meta}, executor: executor},
	}}
}

func roleBindingGuardBindings() []resourcebinding.ResourceRoleBinding {
	return []resourcebinding.ResourceRoleBinding{
		resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
			ResourceRef:    resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
			Role:           "pg_primary",
			SourceTurnID:   "turn-source",
			Confidence:     1,
			ConflictPolicy: "fail_closed",
		}),
		resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
			ResourceRef:    resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"},
			Role:           "pg_standby",
			SourceTurnID:   "turn-source",
			Confidence:     1,
			ConflictPolicy: "fail_closed",
		}),
	}
}
