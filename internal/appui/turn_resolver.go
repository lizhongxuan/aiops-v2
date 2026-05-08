package appui

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

func sortSessionsByActivity(sessions []*runtimekernel.SessionState) []*runtimekernel.SessionState {
	cloned := append([]*runtimekernel.SessionState(nil), sessions...)
	sort.SliceStable(cloned, func(i, j int) bool {
		return cloned[i].UpdatedAt.After(cloned[j].UpdatedAt)
	})
	return cloned
}

func resolveTurnTarget(sessions SessionSource, sessionID, turnID string) (*runtimekernel.SessionState, *runtimekernel.TurnSnapshot, error) {
	if sessions == nil {
		return nil, nil, fmt.Errorf("session source is not configured")
	}
	candidates := sortSessionsByActivity(sessions.List())
	if trimmed := strings.TrimSpace(sessionID); trimmed != "" {
		session := sessions.Get(trimmed)
		if session == nil {
			return nil, nil, fmt.Errorf("session %q not found", trimmed)
		}
		candidates = []*runtimekernel.SessionState{session}
	}
	for _, session := range candidates {
		turn := session.CurrentTurn
		if turn == nil {
			continue
		}
		if trimmed := strings.TrimSpace(turnID); trimmed != "" && turn.ID != trimmed {
			continue
		}
		if turn.Lifecycle.IsTerminal() && strings.TrimSpace(turnID) == "" {
			continue
		}
		return session, turn, nil
	}
	if strings.TrimSpace(turnID) != "" {
		return nil, nil, fmt.Errorf("turn %q not found", turnID)
	}
	return nil, nil, fmt.Errorf("no active turn found")
}

func findApprovalTarget(sessions SessionSource, approvalID string) (*runtimekernel.SessionState, runtimekernel.PendingApproval, error) {
	if sessions == nil {
		return nil, runtimekernel.PendingApproval{}, fmt.Errorf("session source is not configured")
	}
	target := strings.TrimSpace(approvalID)
	if target == "" {
		return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval id is required")
	}
	for _, session := range sortSessionsByActivity(sessions.List()) {
		for _, approval := range session.PendingApprovals {
			if approval.ID == target {
				return session, approval, nil
			}
		}
		for _, evidence := range session.PendingEvidence {
			if evidence.ID == target {
				return session, pendingEvidenceAsApproval(evidence), nil
			}
		}
		if session.CurrentTurn == nil {
			continue
		}
		for _, approval := range session.CurrentTurn.PendingApprovals {
			if approval.ID == target {
				return session, approval, nil
			}
		}
		for _, evidence := range session.CurrentTurn.PendingEvidence {
			if evidence.ID == target {
				return session, pendingEvidenceAsApproval(evidence), nil
			}
		}
	}
	return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval %q not found", target)
}

func pendingEvidenceAsApproval(evidence runtimekernel.PendingEvidence) runtimekernel.PendingApproval {
	return runtimekernel.PendingApproval{
		ID:         evidence.ID,
		SessionID:  evidence.SessionID,
		TurnID:     evidence.TurnID,
		Iteration:  evidence.Iteration,
		ToolName:   firstNonEmpty(strings.TrimSpace(evidence.ToolName), "tool"),
		ToolCallID: evidence.ToolCallID,
		Reason:     evidence.Reason,
		Source:     "pending_evidence",
		Status:     evidence.Status,
		CreatedAt:  evidence.CreatedAt,
		UpdatedAt:  evidence.UpdatedAt,
	}
}

func resolveChoiceTarget(sessions SessionSource, requestID string) (*runtimekernel.SessionState, *runtimekernel.TurnSnapshot, string, error) {
	target := strings.TrimSpace(requestID)
	session, turn, err := resolveTurnTarget(sessions, "", "")
	if target == "" {
		if err != nil {
			return nil, nil, "", err
		}
		checkpointID := ""
		if turn.LatestCheckpoint != nil {
			checkpointID = turn.LatestCheckpoint.ID
		}
		return session, turn, checkpointID, nil
	}
	for _, candidate := range sortSessionsByActivity(sessions.List()) {
		turn := candidate.CurrentTurn
		if turn == nil {
			continue
		}
		if turn.ID == target {
			return candidate, turn, target, nil
		}
		if turn.LatestCheckpoint != nil && turn.LatestCheckpoint.ID == target {
			return candidate, turn, target, nil
		}
		for _, iteration := range turn.Iterations {
			if iteration.Checkpoint != nil && iteration.Checkpoint.ID == target {
				return candidate, turn, target, nil
			}
		}
	}
	if err == nil && session != nil && turn != nil {
		return session, turn, target, nil
	}
	return nil, nil, "", fmt.Errorf("choice request %q not found", target)
}
