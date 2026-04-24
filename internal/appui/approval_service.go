package appui

import (
	"context"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

type defaultApprovalService struct {
	runtime  RuntimeGateway
	sessions SessionSource
	builder  *SnapshotBuilder
}

func NewApprovalService(runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder) ApprovalService {
	return &defaultApprovalService{
		runtime:  runtime,
		sessions: sessions,
		builder:  builder,
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
	session, approval, err := findApprovalTarget(s.sessions, decision.ID)
	if err != nil {
		return ActionResult{}, err
	}
	result, err := s.runtime.ResumeTurn(ctx, runtimekernel.ResumeRequest{
		SessionID:   session.ID,
		TurnID:      firstNonEmpty(strings.TrimSpace(approval.TurnID), currentTurnID(session)),
		ApprovalID:  approval.ID,
		CheckpointID: currentCheckpointID(session),
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Decision:    normalizeApprovalDecision(decision.Decision),
	})
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{
		Status:    result.Status,
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
	}, nil
}

func normalizeApprovalDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accept", "accept_session", "approve", "approved", "allow", "yes":
		return "approved"
	default:
		return "denied"
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
