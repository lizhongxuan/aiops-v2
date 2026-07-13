package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherPermissionBindingFailClosedForMutation(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		current  string
		want     string
	}{
		{name: "missing both hashes", want: "missing expected"},
		{name: "missing expected hash", current: "sha256:current", want: "missing expected"},
		{name: "missing current step hash", expected: "sha256:expected", want: "missing current"},
		{name: "current step hash mismatch", expected: "sha256:expected", current: "sha256:current", want: "mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &permissionBindingExecutor{}
			dispatcher := NewToolDispatcher(permissionBindingLookup(executor, true), nil, &testMockEventEmitter{}).
				WithPermissionBinding(tt.expected, tt.current)

			result := dispatcher.Dispatch(context.Background(), "sess-permission", "turn-permission", ToolCall{
				ID: "call-mutate", Name: "synthetic.mutate", Arguments: json.RawMessage(`{"value":"next"}`),
			}, SessionTypeHost, ModeExecute)

			if result.Error == "" || result.Outcome != "permission_binding_invalid" || !strings.Contains(result.Error, tt.want) {
				t.Fatalf("Dispatch() = %#v, want permission binding failure containing %q", result, tt.want)
			}
			if executor.calls != 0 {
				t.Fatalf("executor calls = %d, want 0", executor.calls)
			}
		})
	}
}

func TestToolDispatcherPermissionBindingKeepsReadOnlyCompatibility(t *testing.T) {
	executor := &permissionBindingExecutor{readOnly: true}
	dispatcher := NewToolDispatcher(permissionBindingLookup(executor, false), nil, &testMockEventEmitter{})

	result := dispatcher.Dispatch(context.Background(), "sess-permission-read", "turn-permission-read", ToolCall{
		ID: "call-read", Name: "synthetic.read", Arguments: json.RawMessage(`{"path":"/tmp/app.log"}`),
	}, SessionTypeHost, ModeInspect)

	if result.Error != "" || result.Content != "ok" || executor.calls != 1 {
		t.Fatalf("Dispatch() = %#v, executor calls = %d; want compatible read-only execution", result, executor.calls)
	}
}

func TestToolDispatcherPermissionSnapshotHashAloneCannotAuthorizeMutation(t *testing.T) {
	executor := &permissionBindingExecutor{}
	dispatcher := NewToolDispatcher(permissionBindingLookup(executor, true), nil, &testMockEventEmitter{}).
		WithPermissionSnapshotHash("sha256:current-only")

	result := dispatcher.Dispatch(context.Background(), "sess-permission-current-only", "turn-permission-current-only", ToolCall{
		ID: "call-mutate", Name: "synthetic.mutate", Arguments: json.RawMessage(`{"value":"next"}`),
	}, SessionTypeHost, ModeExecute)

	if result.Outcome != "permission_binding_invalid" || !strings.Contains(result.Error, "missing expected") || executor.calls != 0 {
		t.Fatalf("Dispatch() = %#v, executor calls = %d; want current-only binding rejected", result, executor.calls)
	}
}

func TestRunTurnPermissionBindingUsesFrozenStepHashAfterProvider(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-permission-binding", Type: "function",
			Function: schema.FunctionCall{Name: "synthetic.mutate", Arguments: `{"value":"next"}`},
		}}),
	}}
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "synthetic.mutate", Description: "Apply a synthetic state change",
			Mutating: true, RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: "argument_scoped"},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType: "synthetic", ResourceID: "state", OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: []string{"verify synthetic state"},
			},
		},
		Visibility:   tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		ReadOnlyFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval, Reason: "synthetic mutation requires approval",
				Approval: &tooling.PermissionApprovalPayload{
					ExpectedEffect: "update synthetic state", Rollback: "restore synthetic state", Validation: "verify synthetic state",
				},
			}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	intent := &runtimecontract.IntentFrame{
		Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite},
		Confidence: runtimecontract.ConfidenceHigh,
	}
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-permission-binding", TurnID: "turn-permission-binding",
		SessionType: SessionTypeHost, Mode: ModeExecute, HostID: "host-a", Input: "apply the synthetic change",
		IntentFrame: intent, PermissionProfile: RuntimePermissionProfileApprovalRequired,
		RollbackPolicy: RuntimeRollbackPolicyActionContractRequired,
		Metadata: map[string]string{
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "pre_change_snapshot",
			"aiops.userEvidence.signals":    "synthetic_state_before",
			"aiops.userEvidence.rawExcerpt": "synthetic state captured before change",
		},
	})
	if err != nil || result.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v; want pending approval after provider", result, err)
	}
	if len(model.inputs) != 1 || executed != 0 {
		t.Fatalf("provider calls = %d, executor calls = %d; want 1 then 0", len(model.inputs), executed)
	}
	session := kernel.sessions.Get("sess-permission-binding")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.TurnAssembly == nil ||
		session.CurrentTurn.ToolSurfaceSnapshot == nil || session.CurrentTurn.ToolSurfaceSnapshot.PolicySnapshot == nil ||
		len(session.PendingApprovals) != 1 {
		t.Fatalf("permission binding facts missing: %#v", session)
	}
	expected := kernel.runtimePermissionSnapshotHash(session.CurrentTurn.TurnAssembly, *session.CurrentTurn.ToolSurfaceSnapshot.PolicySnapshot)
	if got := session.PendingApprovals[0].PermissionSnapshotHash; got == "" || got != expected {
		t.Fatalf("pending permission hash = %q, want frozen step hash %q", got, expected)
	}
}

func TestRunTurnPermissionBindingFailClosedOnPostProviderPolicyDrift(t *testing.T) {
	inner := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-permission-drift", Type: "function",
			Function: schema.FunctionCall{Name: "synthetic.mutate", Arguments: `{"value":"next"}`},
		}}),
	}}
	model := &postProviderPermissionMutationModel{inner: inner}
	executed := 0
	toolDef := permissionBindingMutationTool(&executed)
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	model.after = func() {
		session := kernel.sessions.Get("sess-permission-drift")
		if session != nil && session.CurrentTurn != nil && session.CurrentTurn.ToolSurfaceSnapshot != nil && session.CurrentTurn.ToolSurfaceSnapshot.PolicySnapshot != nil {
			session.CurrentTurn.ToolSurfaceSnapshot.PolicySnapshot.PermissionHash = "sha256:post-provider-drift"
		}
	}
	_, err := kernel.RunTurn(context.Background(), permissionBindingMutationTurnRequest("sess-permission-drift", "turn-permission-drift"))
	if err == nil || !strings.Contains(err.Error(), "permission_binding_invalid") {
		t.Fatalf("RunTurn() error = %v, want post-provider permission drift failure", err)
	}
	if len(inner.inputs) != 1 || executed != 0 {
		t.Fatalf("provider calls = %d, executor calls = %d; want 1 then 0", len(inner.inputs), executed)
	}
}

type permissionBindingExecutor struct {
	readOnly bool
	calls    int
}

func (e *permissionBindingExecutor) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	e.calls++
	return tooling.ToolResult{Content: "ok"}, nil
}

func (e *permissionBindingExecutor) IsReadOnly(json.RawMessage) bool { return e.readOnly }

func permissionBindingLookup(executor ToolExecutor, mutating bool) *mockToolLookup {
	name := "synthetic.read"
	if mutating {
		name = "synthetic.mutate"
	}
	return &mockToolLookup{tools: map[string]mockToolEntry{
		name: {
			desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: name, Mutating: mutating}},
			executor: executor,
		},
	}}
}

type postProviderPermissionMutationModel struct {
	inner *sequentialLoopModel
	after func()
	once  sync.Once
}

func (m *postProviderPermissionMutationModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	response, err := m.inner.Generate(ctx, input, opts...)
	m.once.Do(m.after)
	return response, err
}

func (m *postProviderPermissionMutationModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	response, err := m.inner.Stream(ctx, input, opts...)
	m.once.Do(m.after)
	return response, err
}

func (m *postProviderPermissionMutationModel) BindTools(tools []*schema.ToolInfo) error {
	return m.inner.BindTools(tools)
}

func permissionBindingMutationTool(executed *int) *tooling.StaticTool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "synthetic.mutate", Description: "Apply a synthetic state change",
			Mutating: true, RequiresApproval: true,
			Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: "argument_scoped"},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType: "synthetic", ResourceID: "state", OperationKind: "write",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: []string{"verify synthetic state"},
			},
		},
		Visibility:   tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		ReadOnlyFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: "synthetic mutation requires approval"}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			(*executed)++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
}

func permissionBindingMutationTurnRequest(sessionID, turnID string) TurnRequest {
	return TurnRequest{
		SessionID: sessionID, TurnID: turnID, SessionType: SessionTypeHost, Mode: ModeExecute,
		HostID: "host-a", Input: "apply the synthetic change",
		IntentFrame: &runtimecontract.IntentFrame{
			Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite},
			Confidence: runtimecontract.ConfidenceHigh,
		},
		PermissionProfile: RuntimePermissionProfileApprovalRequired,
		RollbackPolicy:    RuntimeRollbackPolicyActionContractRequired,
	}
}
