package appui

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

type defaultApprovalService struct {
	runtime     RuntimeGateway
	sessions    SessionSource
	builder     *SnapshotBuilder
	baseContext context.Context
}

func NewApprovalService(runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder) ApprovalService {
	return NewApprovalServiceWithContext(context.Background(), runtime, sessions, builder)
}

func NewApprovalServiceWithContext(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder) ApprovalService {
	return &defaultApprovalService{
		runtime:     runtime,
		sessions:    sessions,
		builder:     builder,
		baseContext: normalizeBaseContext(baseContext),
	}
}

func (s *defaultApprovalService) List(context.Context) ([]ApprovalView, error) {
	if s.sessions == nil {
		return []ApprovalView{}, nil
	}
	approvals := make([]ApprovalView, 0)
	seen := map[string]struct{}{}
	for _, session := range sortSessionsByActivity(s.sessions.List()) {
		for _, approval := range buildApprovals(session.PendingApprovals) {
			if _, ok := seen[approval.ID]; ok {
				continue
			}
			seen[approval.ID] = struct{}{}
			approvals = append(approvals, approval)
		}
		if session.CurrentTurn != nil {
			for _, approval := range buildApprovals(session.CurrentTurn.PendingApprovals) {
				if _, ok := seen[approval.ID]; ok {
					continue
				}
				seen[approval.ID] = struct{}{}
				approvals = append(approvals, approval)
			}
		}
	}
	return approvals, nil
}

func (s *defaultApprovalService) Decide(ctx context.Context, decision ApprovalDecision) (ActionResult, error) {
	_, req, err := s.approvalResumeRequest(decision)
	if err != nil {
		return ActionResult{}, err
	}
	if s.runtime == nil {
		return ActionResult{}, fmt.Errorf("runtime is not configured")
	}
	result, err := s.runtime.ResumeTurn(ctx, req)
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{
		Status:    result.Status,
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
	}, nil
}

func (s *defaultApprovalService) DecideAsync(_ context.Context, decision ApprovalDecision) (ActionResult, error) {
	session, req, err := s.approvalResumeRequest(decision)
	if err != nil {
		return ActionResult{}, err
	}
	if s.runtime == nil {
		return ActionResult{}, fmt.Errorf("runtime is not configured")
	}
	go s.resumeApprovalDecision(req)
	return ActionResult{
		Status:    "accepted",
		SessionID: session.ID,
		TurnID:    req.TurnID,
	}, nil
}

func (s *defaultApprovalService) resumeApprovalDecision(req runtimekernel.ResumeRequest) {
	if s == nil || s.runtime == nil {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	defer func() {
		_ = recover()
	}()
	_, _ = s.runtime.ResumeTurn(ctx, req)
}

func (s *defaultApprovalService) approvalResumeRequest(decision ApprovalDecision) (*runtimekernel.SessionState, runtimekernel.ResumeRequest, error) {
	session, approval, err := findApprovalTarget(s.sessions, decision.ID)
	if err != nil {
		return nil, runtimekernel.ResumeRequest{}, err
	}
	normalizedDecision, err := normalizeApprovalDecision(decision.Decision)
	if err != nil {
		return nil, runtimekernel.ResumeRequest{}, err
	}
	resumeState := runtimekernel.TurnResumeStatePendingApproval
	if strings.EqualFold(strings.TrimSpace(approval.Source), "pending_evidence") {
		resumeState = runtimekernel.TurnResumeStatePendingEvidence
	}
	return session, runtimekernel.ResumeRequest{
		SessionID:    session.ID,
		TurnID:       firstNonEmpty(strings.TrimSpace(approval.TurnID), currentTurnID(session)),
		ApprovalID:   approval.ID,
		CheckpointID: currentCheckpointID(session),
		ResumeState:  resumeState,
		Decision:     normalizedDecision,
	}, nil
}

func normalizeApprovalDecision(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accept", "accept_session", "approve", "approved", "allow", "yes":
		return "approved", nil
	case "reject", "rejected", "deny", "denied", "no":
		return "denied", nil
	default:
		return "", fmt.Errorf("invalid approval decision %q", value)
	}
}

func currentTurnID(session *runtimekernel.SessionState) string {
	if session == nil || session.CurrentTurn == nil {
		return ""
	}
	return session.CurrentTurn.ID
}

func currentCheckpointID(session *runtimekernel.SessionState) string {
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.LatestCheckpoint == nil {
		return ""
	}
	return session.CurrentTurn.LatestCheckpoint.ID
}
