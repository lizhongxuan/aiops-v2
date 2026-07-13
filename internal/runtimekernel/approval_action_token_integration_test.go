package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestApprovalActionTokenRejectsCurrentWorldDriftBeforeExecution(t *testing.T) {
	tests := []struct {
		name      string
		mismatch  string
		reapprove bool
		mutate    func(*RuntimeKernel, *SessionState)
	}{
		{
			name: "arguments", mismatch: "arguments",
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				last := latestIteration(session.CurrentTurn)
				last.ToolCalls[0].Arguments = json.RawMessage(`{"path":"/tmp/changed","content":"secret-value"}`)
			},
		},
		{
			name: "target", mismatch: "target",
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				session.HostID = "host-b"
			},
		},
		{
			name: "tool router", mismatch: "tool_router",
			mutate: func(kernel *RuntimeKernel, _ *SessionState) {
				source := kernel.tools.(*testMockToolAssemblySource)
				err := source.registry.Register(&tooling.StaticTool{
					Meta:       tooling.ToolMetadata{Name: "new_runtime_tool", Description: "New runtime tool"},
					Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
					ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
						return tooling.ToolResult{Content: "unused"}, nil
					},
				})
				if err != nil {
					t.Fatalf("Register(new runtime tool) error = %v", err)
				}
			},
		},
		{
			name: "legacy arguments", mismatch: "arguments",
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				session.CurrentTurn.PendingApprovals[0].ActionToken = nil
				session.PendingApprovals[0].ActionToken = nil
				last := latestIteration(session.CurrentTurn)
				last.ToolCalls[0].Arguments = json.RawMessage(`{"path":"/tmp/legacy-changed","content":"legacy-secret"}`)
			},
		},
		{
			name: "permission", mismatch: "permission",
			mutate: func(kernel *RuntimeKernel, _ *SessionState) {
				kernel.permissions = permissions.NewEngine([]permissions.Rule{{
					Name: "permission-world-changed", Action: permissions.ActionDeny,
					Matcher: permissions.Matcher{ToolNames: []string{"write_file"}},
				}})
			},
		},
		{
			name: "rollback", mismatch: "rollback",
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				session.CurrentTurn.PendingApprovals[0].RollbackContract.PostCheck = "changed server postcheck"
				session.PendingApprovals[0].RollbackContract.PostCheck = "changed server postcheck"
			},
		},
		{
			name: "checkpoint", mismatch: "checkpoint",
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				session.CurrentTurn.LatestCheckpoint.ID = "checkpoint-world-changed"
				session.LatestCheckpoint.ID = "checkpoint-world-changed"
			},
		},
		{
			name: "expiry", mismatch: "expiry", reapprove: true,
			mutate: func(_ *RuntimeKernel, session *SessionState) {
				for _, approval := range []*PendingApproval{&session.CurrentTurn.PendingApprovals[0], &session.PendingApprovals[0]} {
					token := *approval.ActionToken
					token.ExpiresAt = time.Now().Add(-time.Minute)
					frozen, err := FreezeActionToken(token)
					if err != nil {
						t.Fatalf("FreezeActionToken(expired) error = %v", err)
					}
					approval.ActionToken = &frozen
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &sequentialLoopModel{responses: []*schema.Message{
				schema.AssistantMessage("", []schema.ToolCall{{
					ID: "call-binding", Type: "function",
					Function: schema.FunctionCall{Name: "write_file", Arguments: `{"path":"/tmp/a","content":"original"}`},
				}}),
				schema.AssistantMessage("approved action completed", nil),
			}}
			var executed int
			toolDef := &tooling.StaticTool{
				Meta: tooling.ToolMetadata{Name: "write_file", Description: "Write a file"},
				Visibility: tooling.Visibility{
					SessionTypes: []string{string(SessionTypeHost)},
					Modes:        []string{string(ModeExecute)},
				},
				ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
					executed++
					return tooling.ToolResult{Content: "should-not-run"}, nil
				},
			}
			kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
			blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
				SessionID: "sess-binding", SessionType: SessionTypeHost, Mode: ModeExecute,
				TurnID: "turn-binding", HostID: "host-a", Input: "write the file",
			})
			if err != nil || blocked.Status != "blocked" {
				t.Fatalf("RunTurn() = %#v, %v, want pending approval", blocked, err)
			}
			session := kernel.sessions.Get("sess-binding")
			if session == nil || session.CurrentTurn == nil || len(session.PendingApprovals) != 1 || len(session.CurrentTurn.PendingApprovals) != 1 {
				t.Fatalf("pending approval missing: %#v", session)
			}
			if session.PendingApprovals[0].ActionToken == nil || session.CurrentTurn.ToolSurfaceSnapshot == nil || session.CurrentTurn.ToolSurfaceSnapshot.StepRouter == nil {
				t.Fatalf("server binding facts missing: %#v", session.PendingApprovals[0])
			}
			approvalID := session.PendingApprovals[0].ID
			tt.mutate(kernel, session)

			resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
				SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: approvalID,
				Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
			})
			if err != nil {
				t.Fatalf("ResumeTurn() error = %v", err)
			}
			if executed != 0 {
				t.Fatalf("executor calls = %d, want 0", executed)
			}
			if resumed.Status != "blocked" || !strings.Contains(resumed.Error, ApprovalContextStaleCode) || !strings.Contains(resumed.Error, `"`+tt.mismatch+`"`) {
				t.Fatalf("ResumeTurn() = %#v, want stale %q blocker", resumed, tt.mismatch)
			}
			if strings.Contains(resumed.Error, "/tmp/") || strings.Contains(resumed.Error, "secret-value") || strings.Contains(resumed.Error, "legacy-secret") {
				t.Fatalf("stale trace leaked raw arguments: %s", resumed.Error)
			}
			session = kernel.sessions.Get("sess-binding")
			if len(session.PendingApprovals) != 1 || session.CurrentTurn.ResumeState != TurnResumeStatePendingApproval {
				t.Fatalf("stale approval was consumed: %#v", session.PendingApprovals)
			}
			freshApproval := session.PendingApprovals[0]
			if freshApproval.ID == approvalID || freshApproval.ActionToken == nil {
				t.Fatalf("stale approval was not reissued with a fresh server binding: %#v", freshApproval)
			}
			if tt.reapprove {
				second, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
					SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: freshApproval.ID,
					Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
				})
				if err != nil || second.Status != "completed" || executed != 1 {
					t.Fatalf("second ResumeTurn() = %#v, %v, executed=%d; want fresh approval to execute once", second, err, executed)
				}
			}
		})
	}
}

func TestExpiredPendingEvidenceReissuesNormalApprovalAndCanResume(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-expired-evidence", Type: "function",
			Function: schema.FunctionCall{Name: "inspect_runtime_evidence", Arguments: `{"scope":"service"}`},
		}}),
		schema.AssistantMessage("evidence accepted", nil),
	}}
	permissionChecks := 0
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "inspect_runtime_evidence", Description: "Inspect runtime evidence", RiskLevel: tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: "read"},
		},
		Visibility:   tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			permissionChecks++
			if permissionChecks == 1 {
				return tooling.PermissionDecision{Action: tooling.PermissionActionNeedEvidence, Reason: "operator evidence acceptance required"}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: `{"status":"healthy"}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-expired-evidence", SessionType: SessionTypeHost, Mode: ModeInspect,
		TurnID: "turn-expired-evidence", Input: "inspect service health",
	})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v, want pending evidence", blocked, err)
	}
	session := kernel.sessions.Get("sess-expired-evidence")
	oldEvidenceID := session.PendingEvidence[0].ID
	old := time.Now().Add(-16 * time.Minute)
	session.PendingEvidence[0].CreatedAt = old
	session.CurrentTurn.PendingEvidence[0].CreatedAt = old

	stale, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: oldEvidenceID,
		Decision: "approved", ResumeState: TurnResumeStatePendingEvidence,
	})
	if err != nil || stale.Status != "blocked" || !strings.Contains(stale.Error, `"expiry"`) || executed != 0 {
		t.Fatalf("expired evidence ResumeTurn() = %#v, %v, executed=%d", stale, err, executed)
	}
	session = kernel.sessions.Get(session.ID)
	if len(session.PendingEvidence) != 0 || len(session.CurrentTurn.PendingEvidence) != 0 || len(session.PendingApprovals) != 1 {
		t.Fatalf("reissued state = evidence=%#v turnEvidence=%#v approvals=%#v", session.PendingEvidence, session.CurrentTurn.PendingEvidence, session.PendingApprovals)
	}
	fresh := session.PendingApprovals[0]
	if fresh.ID == oldEvidenceID || fresh.Source == "pending_evidence" || session.CurrentTurn.ResumeState != TurnResumeStatePendingApproval {
		t.Fatalf("fresh approval = %#v, resume=%s", fresh, session.CurrentTurn.ResumeState)
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: fresh.ID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || resumed.Status != "completed" || executed != 1 {
		t.Fatalf("fresh approval ResumeTurn() = %#v, %v, executed=%d", resumed, err, executed)
	}
}
