package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

type controlledToolFixture struct {
	kernel   *RuntimeKernel
	request  ControlledToolTurnRequest
	executed *atomic.Int32
	result   *tooling.ToolResult
	repo     SessionRepository
	recorder *RolloutRecorder
}

func newControlledToolFixture(t *testing.T, repo SessionRepository, recorder *RolloutRecorder) controlledToolFixture {
	t.Helper()
	var executed atomic.Int32
	executionResult := tooling.ToolResult{Content: `{"status":"applied","revision":"rv-2"}`, StructuredContent: json.RawMessage(`{"status":"applied","revision":"rv-2"}`)}
	registry := tooling.NewRegistry()
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "synthetic.apply", Description: "Apply one exact synthetic mutation",
			Mutating: true, RequiresApproval: true, RiskLevel: tooling.ToolRiskHigh,
			Discovery:     tooling.ToolDiscoveryMetadata{PermissionScope: "argument_scoped"},
			ResourceLocks: []tooling.ToolResourceLockKey{{ResourceType: "workflow", ResourceID: "tenant-a:wf-a", OperationKind: "apply"}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: []string{"verify workflow revision"},
			},
			Rollback: &tooling.ToolRollbackMetadata{Strategy: tooling.ToolRollbackStrategyAutomatic, Reference: "test://workflow/rollback"},
		},
		Visibility:   tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)}},
		ReadOnlyFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			// The controlled admission boundary must still create a pending
			// approval when a lower checker happens to return allow.
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			execution, ok := tooling.ToolExecutionContextFrom(ctx)
			if !ok || execution.VerifiedApproval == nil {
				return tooling.ToolResult{}, errors.New("missing verified approval context")
			}
			executed.Add(1)
			return executionResult, nil
		},
	}
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("register controlled test tool: %v", err)
	}
	artifact, err := runtimecontract.NewInlineTurnArtifact("workflow.candidate", map[string]any{
		"workflowId": "wf-a", "changeSetId": "changeset-a", "previewHash": "sha256:preview-a",
	})
	if err != nil {
		t.Fatalf("create controlled artifact: %v", err)
	}
	metadata := map[string]string{"profile": RuntimePromptProfileHostWorker, "toolProfile": RuntimePromptProfileHostWorker}
	turn := TurnRequest{
		SessionType: SessionTypeHost, Mode: ModeExecute, SessionID: "session-controlled", TurnID: "turn-controlled",
		HostID: "host-a", Metadata: metadata,
		TrustedIdentity: &runtimecontract.TurnIdentity{TenantID: "tenant-a", UserID: "user-a", SessionID: "session-controlled"},
		TurnArtifacts:   []runtimecontract.TurnArtifactEnvelope{artifact},
	}
	call := ToolCall{ID: "call-controlled", Name: "synthetic.apply", Arguments: json.RawMessage(`{"workflowId":"wf-a","expectedRevision":"rv-1"}`)}
	request := ControlledToolTurnRequest{
		Turn: turn, ToolCall: call, Source: "workflow_history_apply", IdempotencyKey: "history-undo-a",
		ExecutionContext: tooling.ToolExecutionContext{
			SessionID: turn.SessionID, TurnID: turn.TurnID, TenantID: "tenant-a", UserID: "user-a",
			ToolCallID: call.ID, ToolName: call.Name, HostID: turn.HostID,
			OriginalInput: append(json.RawMessage(nil), call.Arguments...), Metadata: cloneTurnMetadata(metadata),
			TurnArtifacts: cloneRuntimeTurnArtifacts(turn.TurnArtifacts),
		},
	}
	config := RuntimeKernelConfig{
		ToolSource: &testMockToolAssemblySource{registry: registry}, Projector: &testMockEventEmitter{},
		SessionRepo: repo, RolloutRecorder: recorder,
	}
	return controlledToolFixture{
		kernel: NewRuntimeKernel(config), request: request, executed: &executed, result: &executionResult, repo: repo, recorder: recorder,
	}
}

func TestBeginControlledToolTurnCreatesExactPendingApprovalWithoutExecution(t *testing.T) {
	fixture := newControlledToolFixture(t, nil, nil)
	result, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("BeginControlledToolTurn() error = %v", err)
	}
	if result.Status != "blocked" || result.Approval == nil || fixture.executed.Load() != 0 {
		t.Fatalf("result = %#v, executed = %d; want pending approval/no execution", result, fixture.executed.Load())
	}
	if result.Approval.ToolCallID != fixture.request.ToolCall.ID || result.Approval.ToolName != fixture.request.ToolCall.Name || result.Approval.ArgumentsHash != toolArgumentsHash(fixture.request.ToolCall.Arguments) {
		t.Fatalf("approval binding = %#v, want exact tool call", result.Approval)
	}
	snapshot, err := fixture.kernel.sessions.GetSnapshot(fixture.request.Turn.SessionID)
	if err != nil || snapshot == nil || snapshot.CurrentTurn == nil {
		t.Fatalf("snapshot = %#v, %v", snapshot, err)
	}
	turn := snapshot.CurrentTurn
	if turn.ControlledToolTurn == nil || turn.Lifecycle != TurnLifecycleSuspended || turn.ResumeState != TurnResumeStatePendingApproval || len(turn.PendingApprovals) != 1 {
		t.Fatalf("controlled turn state = %#v", turn)
	}
	approval := turn.PendingApprovals[0]
	if approval.ActionToken == nil || approval.ActionToken.ToolCallID != fixture.request.ToolCall.ID || approval.ActionToken.ArgumentsHash != result.Approval.ArgumentsHash || approval.ActionToken.CheckpointID != turn.LatestCheckpoint.ID {
		t.Fatalf("ActionToken binding = %#v", approval.ActionToken)
	}
	if result.Approval.ExpiresAt == nil || len(approval.PreChangeEvidenceRefs) != 1 || approval.PreChangeEvidenceRefs[0] != fixture.request.Turn.TurnArtifacts[0].ID {
		t.Fatalf("approval evidence/expiry = %#v", approval)
	}
}

func TestControlledToolTurnApprovedExecutesExactlyOnceAndReplaysResult(t *testing.T) {
	fixture := newControlledToolFixture(t, nil, nil)
	pending, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: pending.Approval.ApprovalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || approved.Status != "completed" || fixture.executed.Load() != 1 {
		t.Fatalf("ResumeTurn() = %#v, %v, executed=%d", approved, err, fixture.executed.Load())
	}
	if _, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: pending.Approval.ApprovalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	}); err == nil || fixture.executed.Load() != 1 {
		t.Fatalf("approval replay error=%v executed=%d; want rejection/once", err, fixture.executed.Load())
	}
	replayed, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
	if err != nil || replayed.Status != "completed" || replayed.ToolResult == nil || fixture.executed.Load() != 1 {
		t.Fatalf("Begin replay = %#v, %v, executed=%d", replayed, err, fixture.executed.Load())
	}
}

func TestControlledToolTurnTypedToolFailureIsTerminalErrorAndReplaysWithoutExecution(t *testing.T) {
	fixture := newControlledToolFixture(t, nil, nil)
	*fixture.result = tooling.ToolResult{
		Content:           "Candidate 已过期，请重新预览。",
		StructuredContent: json.RawMessage(`{"schemaVersion":"aiops.workflow.edit.v2","kind":"error","code":"STALE_CANDIDATE"}`),
		IsError:           true,
	}
	pending, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: pending.Approval.ApprovalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || failed.Status != "error" || !strings.Contains(failed.Error, "Candidate 已过期") || fixture.executed.Load() != 1 {
		t.Fatalf("ResumeTurn() = %#v, %v, executed=%d", failed, err, fixture.executed.Load())
	}
	replayed, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
	if err != nil || replayed.Status != "error" || replayed.ToolResult == nil || !replayed.ToolResult.IsError || fixture.executed.Load() != 1 {
		t.Fatalf("Begin replay = %#v, %v, executed=%d", replayed, err, fixture.executed.Load())
	}
	state := fixture.kernel.sessions.Get(fixture.request.Turn.SessionID)
	if state.CurrentTurn == nil || state.CurrentTurn.Lifecycle != TurnLifecycleFailed || state.CurrentTurn.PendingApprovals != nil {
		t.Fatalf("controlled failure state = %#v", state.CurrentTurn)
	}
}

func TestControlledToolTurnDeniedAndWrongBindingsNeverExecute(t *testing.T) {
	t.Run("denied", func(t *testing.T) {
		fixture := newControlledToolFixture(t, nil, nil)
		pending, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
			SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: pending.Approval.ApprovalID,
			Decision: "denied", ResumeState: TurnResumeStatePendingApproval,
		})
		if err != nil || result.Status != "completed" || fixture.executed.Load() != 0 {
			t.Fatalf("denied ResumeTurn() = %#v, %v, executed=%d", result, err, fixture.executed.Load())
		}
	})

	t.Run("wrong approval and turn", func(t *testing.T) {
		fixture := newControlledToolFixture(t, nil, nil)
		pending, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
		if err != nil {
			t.Fatal(err)
		}
		for _, resume := range []ResumeRequest{
			{SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: "approval-wrong", Decision: "approved", ResumeState: TurnResumeStatePendingApproval},
			{SessionID: pending.SessionID, TurnID: "turn-wrong", ApprovalID: pending.Approval.ApprovalID, Decision: "approved", ResumeState: TurnResumeStatePendingApproval},
		} {
			if _, err := fixture.kernel.ResumeTurn(context.Background(), resume); err == nil {
				t.Fatalf("ResumeTurn(%#v) succeeded", resume)
			}
		}
		if fixture.executed.Load() != 0 {
			t.Fatalf("executed = %d, want 0", fixture.executed.Load())
		}
	})
}

func TestControlledToolTurnStaleArgumentsAndExpiryReissueWithoutExecution(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SessionState)
	}{
		{name: "arguments hash", mutate: func(session *SessionState) {
			session.CurrentTurn.Iterations[0].ToolCalls[0].Arguments = json.RawMessage(`{"workflowId":"wf-a","expectedRevision":"rv-stale"}`)
		}},
		{name: "expired token", mutate: func(session *SessionState) {
			for _, approval := range []*PendingApproval{&session.PendingApprovals[0], &session.CurrentTurn.PendingApprovals[0]} {
				token := *approval.ActionToken
				token.ExpiresAt = time.Now().Add(-time.Minute)
				frozen, err := FreezeActionToken(token)
				if err != nil {
					panic(err)
				}
				approval.ActionToken = &frozen
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newControlledToolFixture(t, nil, nil)
			pending, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
			if err != nil {
				t.Fatal(err)
			}
			session := fixture.kernel.sessions.Get(pending.SessionID)
			test.mutate(session)
			result, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
				SessionID: pending.SessionID, TurnID: pending.TurnID, ApprovalID: pending.Approval.ApprovalID,
				Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
			})
			if fixture.executed.Load() != 0 {
				t.Fatalf("executed = %d, want 0", fixture.executed.Load())
			}
			if test.name == "arguments hash" {
				if err == nil {
					t.Fatalf("tampered controlled snapshot accepted: %#v", result)
				}
			} else if err != nil || result.Status != "blocked" {
				t.Fatalf("expired ResumeTurn() = %#v, %v; want fresh approval", result, err)
			}
		})
	}
}

func TestControlledToolTurnConcurrentBeginAndApproveAtMostOnce(t *testing.T) {
	fixture := newControlledToolFixture(t, nil, nil)
	const attempts = 16
	results := make(chan ControlledToolTurnResult, attempts)
	errorsCh := make(chan error, attempts)
	var beginWG sync.WaitGroup
	for i := 0; i < attempts; i++ {
		beginWG.Add(1)
		go func() {
			defer beginWG.Done()
			result, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request)
			results <- result
			errorsCh <- err
		}()
	}
	beginWG.Wait()
	close(results)
	close(errorsCh)
	var approvalID string
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Begin error = %v", err)
		}
	}
	for result := range results {
		if result.Approval == nil {
			t.Fatalf("concurrent Begin result = %#v", result)
		}
		if approvalID == "" {
			approvalID = result.Approval.ApprovalID
		} else if result.Approval.ApprovalID != approvalID {
			t.Fatalf("approval ids differ: %q vs %q", result.Approval.ApprovalID, approvalID)
		}
	}
	if fixture.executed.Load() != 0 {
		t.Fatalf("Begin executed mutation %d times", fixture.executed.Load())
	}

	var approveWG sync.WaitGroup
	var completed atomic.Int32
	for i := 0; i < attempts; i++ {
		approveWG.Add(1)
		go func() {
			defer approveWG.Done()
			result, err := fixture.kernel.ResumeTurn(context.Background(), ResumeRequest{
				SessionID: fixture.request.Turn.SessionID, TurnID: fixture.request.Turn.TurnID,
				ApprovalID: approvalID, Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
			})
			if err == nil && result.Status == "completed" {
				completed.Add(1)
			}
		}()
	}
	approveWG.Wait()
	if fixture.executed.Load() != 1 || completed.Load() != 1 {
		t.Fatalf("executed=%d completed=%d, want exactly one", fixture.executed.Load(), completed.Load())
	}
}

type controlledSessionRepository struct {
	mu       sync.Mutex
	sessions map[string][]byte
}

func newControlledSessionRepository() *controlledSessionRepository {
	return &controlledSessionRepository{sessions: map[string][]byte{}}
}

func (r *controlledSessionRepository) GetSession(id string) (*SessionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw := r.sessions[id]
	if len(raw) == 0 {
		return nil, nil
	}
	var session SessionState
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *controlledSessionRepository) SaveSession(session *SessionState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	r.sessions[session.ID] = append([]byte(nil), raw...)
	return nil
}

func (r *controlledSessionRepository) ListSessions() ([]*SessionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*SessionState, 0, len(r.sessions))
	for _, raw := range r.sessions {
		var session SessionState
		if err := json.Unmarshal(raw, &session); err != nil {
			return nil, err
		}
		result = append(result, &session)
	}
	return result, nil
}

func (r *controlledSessionRepository) DeleteSession(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

func TestControlledToolTurnSnapshotRecoversAcrossKernelRestart(t *testing.T) {
	repo := newControlledSessionRepository()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: NewMemoryRolloutStore(), FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatal(err)
	}
	first := newControlledToolFixture(t, repo, recorder)
	pending, err := first.kernel.BeginControlledToolTurn(context.Background(), first.request)
	if err != nil {
		t.Fatal(err)
	}
	second := newControlledToolFixture(t, repo, recorder)
	second.request = first.request
	recovered, err := second.kernel.BeginControlledToolTurn(context.Background(), second.request)
	if err != nil || recovered.Approval == nil || recovered.Approval.ApprovalID != pending.Approval.ApprovalID {
		t.Fatalf("recovered Begin = %#v, %v", recovered, err)
	}
	completed, err := second.kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: recovered.SessionID, TurnID: recovered.TurnID, ApprovalID: recovered.Approval.ApprovalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || completed.Status != "completed" || second.executed.Load() != 1 {
		t.Fatalf("recovered Resume = %#v, %v, executed=%d", completed, err, second.executed.Load())
	}
}

func TestControlledToolTurnRejectsAuthorityAndIdempotencyDrift(t *testing.T) {
	fixture := newControlledToolFixture(t, nil, nil)
	authorized := fixture.request
	authorized.ExecutionContext.ActionToken = "caller-supplied"
	if _, err := fixture.kernel.BeginControlledToolTurn(context.Background(), authorized); err == nil {
		t.Fatal("caller-supplied ActionToken was accepted")
	}
	if _, err := fixture.kernel.BeginControlledToolTurn(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	drift := fixture.request
	drift.ToolCall.Arguments = json.RawMessage(`{"workflowId":"wf-a","expectedRevision":"rv-other"}`)
	drift.ExecutionContext.OriginalInput = append(json.RawMessage(nil), drift.ToolCall.Arguments...)
	if _, err := fixture.kernel.BeginControlledToolTurn(context.Background(), drift); err == nil {
		t.Fatal("same turn/idempotency key with different exact arguments was accepted")
	}
}
