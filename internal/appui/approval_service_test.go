package appui

import (
	"context"
	"errors"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type approvalRuntimeStub struct {
	resumeReq    runtimekernel.ResumeRequest
	resumeResult runtimekernel.TurnResult
	resumeErr    error
	runReq       runtimekernel.TurnRequest
	runReqCh     chan runtimekernel.TurnRequest
}

func (s *approvalRuntimeStub) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	s.runReq = req
	if s.runReqCh != nil {
		s.runReqCh <- req
	}
	return runtimekernel.TurnResult{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (s *approvalRuntimeStub) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	s.resumeReq = req
	if s.resumeErr != nil {
		return runtimekernel.TurnResult{}, s.resumeErr
	}
	if s.resumeResult.Status != "" {
		return s.resumeResult, nil
	}
	status := "completed"
	errorText := ""
	if req.Decision == "denied" {
		status = "blocked"
		errorText = "approval denied"
	}
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      status,
		Error:       errorText,
	}, nil
}

func (s *approvalRuntimeStub) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type blockingApprovalRuntimeStub struct {
	reqCh     chan runtimekernel.ResumeRequest
	releaseCh chan struct{}
}

func (s *blockingApprovalRuntimeStub) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (s *blockingApprovalRuntimeStub) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	s.reqCh <- req
	<-s.releaseCh
	return runtimekernel.TurnResult{
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (s *blockingApprovalRuntimeStub) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestApprovalService_DecideResumesMatchingApproval(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:          "chk-approval",
			SessionID:   session.ID,
			TurnID:      "turn-approval",
			Iteration:   1,
			Sequence:    1,
			Kind:        "approval_needed",
			Lifecycle:   runtimekernel.TurnLifecycleSuspended,
			ResumeState: runtimekernel.TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-1",
			SessionID: session.ID,
			TurnID:    "turn-approval",
			Iteration: 1,
			ToolName:  "exec_command",
			HostID:    "host-a",
			Reason:    "needs approval",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	result, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-1",
		Decision: "accept",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if runtime.resumeReq.SessionID != session.ID {
		t.Fatalf("ResumeTurn sessionId = %q, want %q", runtime.resumeReq.SessionID, session.ID)
	}
	if runtime.resumeReq.TurnID != "turn-approval" {
		t.Fatalf("ResumeTurn turnId = %q, want turn-approval", runtime.resumeReq.TurnID)
	}
	if runtime.resumeReq.ApprovalID != "approval-1" {
		t.Fatalf("ResumeTurn approvalId = %q, want approval-1", runtime.resumeReq.ApprovalID)
	}
	if runtime.resumeReq.Decision != "approved" {
		t.Fatalf("ResumeTurn decision = %q, want approved", runtime.resumeReq.Decision)
	}
	if result.Status != "completed" {
		t.Fatalf("ActionResult status = %q, want completed", result.Status)
	}
}

func TestApprovalDecisionUsesResumeTurnForApproveAndDeny(t *testing.T) {
	for _, tc := range []struct {
		name         string
		input        string
		wantDecision string
	}{
		{name: "approve", input: "approve", wantDecision: "approved"},
		{name: "deny", input: "deny", wantDecision: "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			sessions := runtimekernel.NewSessionManager()
			session := sessions.GetOrCreate("sess-"+tc.name, runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
			session.CurrentTurn = &runtimekernel.TurnSnapshot{
				ID:          "turn-" + tc.name,
				SessionID:   session.ID,
				SessionType: session.Type,
				Mode:        session.Mode,
				Lifecycle:   runtimekernel.TurnLifecycleSuspended,
				ResumeState: runtimekernel.TurnResumeStatePendingApproval,
				Iteration:   1,
				StartedAt:   now,
				UpdatedAt:   now,
				PendingApprovals: []runtimekernel.PendingApproval{{
					ID:        "approval-" + tc.name,
					SessionID: session.ID,
					TurnID:    "turn-" + tc.name,
					Iteration: 1,
					ToolName:  "exec_command",
					CreatedAt: now,
					UpdatedAt: now,
				}},
			}
			session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
			sessions.Update(session)

			runtime := &approvalRuntimeStub{}
			service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
			_, err := service.Decide(context.Background(), ApprovalDecision{ID: "approval-" + tc.name, Decision: tc.input})
			if err != nil {
				t.Fatalf("Decide() error = %v", err)
			}
			if runtime.runReq.TurnID != "" {
				t.Fatalf("RunTurn was called: %+v", runtime.runReq)
			}
			if runtime.resumeReq.SessionID != session.ID || runtime.resumeReq.TurnID != "turn-"+tc.name || runtime.resumeReq.ApprovalID != "approval-"+tc.name {
				t.Fatalf("ResumeTurn request = %+v, want approval target", runtime.resumeReq)
			}
			if runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingApproval || runtime.resumeReq.Decision != tc.wantDecision {
				t.Fatalf("ResumeTurn request = %+v, want pending approval %s", runtime.resumeReq, tc.wantDecision)
			}
		})
	}
}

func TestApprovalService_AcceptSessionPreservesSessionDecision(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-session", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval-session",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-session",
			SessionID: session.ID,
			TurnID:    "turn-approval-session",
			Iteration: 1,
			ToolName:  "exec_command",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	_, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-session",
		Decision: "accept_session",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if runtime.resumeReq.Decision != "approved_for_session" {
		t.Fatalf("ResumeTurn decision = %q, want approved_for_session", runtime.resumeReq.Decision)
	}
}

func TestApprovalService_DecideAsyncReturnsBeforeRuntimeCompletes(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-async", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval-async",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-async",
			SessionID: session.ID,
			TurnID:    "turn-approval-async",
			Iteration: 1,
			ToolName:  "exec_command",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &blockingApprovalRuntimeStub{
		reqCh:     make(chan runtimekernel.ResumeRequest, 1),
		releaseCh: make(chan struct{}),
	}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	asyncService, ok := service.(interface {
		DecideAsync(context.Context, ApprovalDecision) (ActionResult, error)
	})
	if !ok {
		t.Fatal("ApprovalService does not implement DecideAsync")
	}

	started := time.Now()
	result, err := asyncService.DecideAsync(context.Background(), ApprovalDecision{
		ID:       "approval-async",
		Decision: "accept",
	})
	if err != nil {
		t.Fatalf("DecideAsync() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("DecideAsync() took %s, want immediate return before runtime completes", elapsed)
	}
	if result.Status != "accepted" || result.SessionID != session.ID || result.TurnID != "turn-approval-async" {
		t.Fatalf("ActionResult = %+v, want accepted async target", result)
	}

	select {
	case req := <-runtime.reqCh:
		if req.SessionID != session.ID || req.TurnID != "turn-approval-async" || req.ApprovalID != "approval-async" || req.Decision != "approved" {
			t.Fatalf("ResumeTurn request = %+v, want approval async resume request", req)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runtime ResumeTurn was not started asynchronously")
	}
	close(runtime.releaseCh)
}

func TestApprovalService_DecideResumesMatchingPendingEvidence(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-evidence-approval", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-evidence-approval",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-1",
			SessionID:  session.ID,
			TurnID:     "turn-evidence-approval",
			Iteration:  1,
			ToolName:   "exec_command",
			ToolCallID: "call-ifconfig-down",
			Reason:     "needs evidence approval",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
	}
	session.PendingEvidence = append([]runtimekernel.PendingEvidence(nil), session.CurrentTurn.PendingEvidence...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{}
	service := NewApprovalService(runtime, sessions, NewSnapshotBuilder())
	_, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "evidence-1",
		Decision: "accept",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if runtime.resumeReq.SessionID != session.ID || runtime.resumeReq.TurnID != "turn-evidence-approval" {
		t.Fatalf("ResumeTurn target = %+v, want evidence session/turn", runtime.resumeReq)
	}
	if runtime.resumeReq.ApprovalID != "evidence-1" {
		t.Fatalf("ResumeTurn approvalId = %q, want evidence-1", runtime.resumeReq.ApprovalID)
	}
	if runtime.resumeReq.ResumeState != runtimekernel.TurnResumeStatePendingEvidence {
		t.Fatalf("ResumeTurn resumeState = %q, want pending_evidence", runtime.resumeReq.ResumeState)
	}
	if runtime.resumeReq.Decision != "approved" {
		t.Fatalf("ResumeTurn decision = %q, want approved", runtime.resumeReq.Decision)
	}
}

func TestApprovalFallbackRecoveryOnlyDoesNotRunForNormalDenied(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-reject", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.HostID = "host-a"
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval-reject",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-reject",
			SessionID: session.ID,
			TurnID:    "turn-approval-reject",
			Iteration: 1,
			ToolName:  "exec_command",
			HostID:    "host-a",
			Command:   "systemctl restart postgresql",
			Reason:    "restart requires approval",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{runReqCh: make(chan runtimekernel.TurnRequest, 1)}
	service := NewApprovalServiceWithContext(context.Background(), runtime, sessions, NewSnapshotBuilder())
	_, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-reject",
		Decision: "reject",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if runtime.resumeReq.Decision != "denied" {
		t.Fatalf("ResumeTurn decision = %q, want denied", runtime.resumeReq.Decision)
	}

	select {
	case runReq := <-runtime.runReqCh:
		t.Fatalf("RunTurn started unexpectedly for normal denied approval fallback: %+v", runReq)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestApprovalResumeErrorDoesNotStartFallbackRunTurn(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-recovery", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.PendingApprovals = []runtimekernel.PendingApproval{{
		ID:        "approval-recovery",
		SessionID: session.ID,
		TurnID:    "turn-missing",
		Iteration: 1,
		ToolName:  "exec_command",
		HostID:    "host-a",
		Command:   "systemctl restart postgresql",
		Reason:    "restart requires approval",
		CreatedAt: now,
		UpdatedAt: now,
	}}
	sessions.Update(session)

	runtime := &approvalRuntimeStub{
		runReqCh:  make(chan runtimekernel.TurnRequest, 1),
		resumeErr: errors.New(`turn "turn-missing" is not suspended`),
	}
	service := NewApprovalServiceWithContext(context.Background(), runtime, sessions, NewSnapshotBuilder())
	_, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-recovery",
		Decision: "accept",
	})
	if err == nil {
		t.Fatal("Decide() error = nil, want resume error")
	}

	select {
	case runReq := <-runtime.runReqCh:
		t.Fatalf("RunTurn started unexpectedly after ResumeTurn error: %+v", runReq)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestApprovalFallbackDoesNotRunForApprovedDecision(t *testing.T) {
	now := time.Now().UTC()
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-approve-no-fallback", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval-approve-no-fallback",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-approve-no-fallback",
			SessionID: session.ID,
			TurnID:    "turn-approval-approve-no-fallback",
			Iteration: 1,
			ToolName:  "exec_command",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &approvalRuntimeStub{runReqCh: make(chan runtimekernel.TurnRequest, 1)}
	service := NewApprovalServiceWithContext(context.Background(), runtime, sessions, NewSnapshotBuilder())
	_, err := service.Decide(context.Background(), ApprovalDecision{
		ID:       "approval-approve-no-fallback",
		Decision: "accept",
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	select {
	case req := <-runtime.runReqCh:
		t.Fatalf("RunTurn started unexpectedly for approved approval: %+v", req)
	case <-time.After(50 * time.Millisecond):
	}
}
