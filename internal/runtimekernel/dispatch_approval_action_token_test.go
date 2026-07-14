package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/tooling"
)

func TestToolDispatcherActionTokenRejectsMissingAuthorization(t *testing.T) {
	dispatcher, executor := actionTokenTestDispatcher(t, nil)
	result := dispatcher.DispatchApproved(
		context.Background(), "sess-action", "turn-action",
		ToolCall{ID: "call-action", Name: "write_file", Arguments: json.RawMessage(`{"path":"/tmp/a"}`)},
		SessionTypeHost, ModeExecute, VerifiedActionToken{},
	)
	if !result.Blocked || !strings.Contains(result.Error, ApprovalContextStaleCode) {
		t.Fatalf("DispatchApproved() = %#v, want stale blocker", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherActionTokenRejectsHookArgumentDrift(t *testing.T) {
	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name: "rewrite-approved-input", Stage: hooks.StagePreToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedInput = json.RawMessage(`{"path":"/tmp/changed"}`)
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool() error = %v", err)
	}
	dispatcher, executor := actionTokenTestDispatcher(t, registry)
	call := ToolCall{ID: "call-action", Name: "write_file", Arguments: json.RawMessage(`{"path":"/tmp/a"}`)}
	verified := verifiedActionTokenForDispatcherTest(t, dispatcher, "turn-action", call)
	result := dispatcher.DispatchApproved(context.Background(), "sess-action", "turn-action", call, SessionTypeHost, ModeExecute, verified)
	if !result.Blocked || !strings.Contains(result.Error, "approval_context_stale: arguments") {
		t.Fatalf("DispatchApproved() = %#v, want argument stale blocker", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestToolDispatcherActionTokenDoesNotBypassDenyOrEvidence(t *testing.T) {
	tests := []struct {
		name    string
		action  tooling.PermissionAction
		outcome string
	}{
		{name: "deny", action: tooling.PermissionActionDeny, outcome: "tool_denied"},
		{name: "evidence", action: tooling.PermissionActionNeedEvidence, outcome: "evidence_needed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &permissionCheckingExecutor{decision: tooling.PermissionDecision{Action: tt.action, Reason: tt.name}}
			lookup := &mockToolLookup{tools: map[string]mockToolEntry{
				"write_file": {desc: ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "write_file"}}, executor: executor},
			}}
			dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).WithToolSurfaceFingerprint("sha256:router")
			call := ToolCall{ID: "call-action", Name: "write_file", Arguments: json.RawMessage(`{"path":"/tmp/a"}`)}
			verified := verifiedActionTokenForDispatcherTest(t, dispatcher, "turn-action", call)
			result := dispatcher.DispatchApproved(context.Background(), "sess-action", "turn-action", call, SessionTypeHost, ModeExecute, verified)
			if executor.calls != 0 || result.Outcome != tt.outcome {
				t.Fatalf("DispatchApproved() = %#v, executor calls = %d", result, executor.calls)
			}
		})
	}
}

func actionTokenTestDispatcher(t *testing.T, registry *hooks.Registry) (*ToolDispatcher, *mockToolExecutor) {
	t.Helper()
	executor := &mockToolExecutor{result: "wrote"}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"write_file": {
			desc:     ToolDescriptor{Metadata: tooling.ToolMetadata{Name: "write_file"}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, &testMockEventEmitter{}).
		WithToolSurfaceFingerprint("sha256:router")
	if registry != nil {
		dispatcher = dispatcher.WithHooks(registry)
	}
	return dispatcher, executor
}

func verifiedActionTokenForDispatcherTest(t *testing.T, dispatcher *ToolDispatcher, turnID string, call ToolCall) VerifiedActionToken {
	t.Helper()
	permissionHash := dispatcher.effectivePermissionSnapshotHash()
	token := mustFreezeActionTokenForTest(t, ActionToken{
		ApprovalID: "approval-action", TurnID: turnID, ToolCallID: call.ID, ToolName: call.Name,
		ArgumentsHash: toolArgumentsHash(call.Arguments), TargetRefs: []string{"host:host-a"},
		ToolSurfaceFingerprint: dispatcher.toolSurfaceFP, PermissionHash: permissionHash,
		RollbackHash: "sha256:rollback", CheckpointID: "checkpoint-action", ExpiresAt: time.Now().Add(time.Hour),
	})
	verified, err := VerifyActionToken(token, ActionTokenCurrentFacts{
		ApprovalID: token.ApprovalID, TurnID: token.TurnID, ToolCallID: token.ToolCallID, ToolName: token.ToolName,
		ArgumentsHash: token.ArgumentsHash, TargetRefs: token.TargetRefs, ToolSurfaceFingerprint: token.ToolSurfaceFingerprint,
		PermissionHash: token.PermissionHash, RollbackHash: token.RollbackHash, CheckpointID: token.CheckpointID,
	}, time.Now())
	if err != nil {
		t.Fatalf("VerifyActionToken() error = %v", err)
	}
	return verified
}
