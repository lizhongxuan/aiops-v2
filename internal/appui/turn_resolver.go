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
		if approval, ok := findApprovalInSession(session, "", target); ok {
			return session, approval, nil
		}
	}
	return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval %q not found", target)
}

func findApprovalTargetScoped(sessions SessionSource, sessionID, turnID, approvalID string) (*runtimekernel.SessionState, runtimekernel.PendingApproval, error) {
	if sessions == nil {
		return nil, runtimekernel.PendingApproval{}, fmt.Errorf("session source is not configured")
	}
	target := strings.TrimSpace(approvalID)
	if target == "" {
		return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval id is required")
	}
	trimmedSessionID := strings.TrimSpace(sessionID)
	trimmedTurnID := strings.TrimSpace(turnID)
	if trimmedSessionID == "" && trimmedTurnID == "" {
		return findApprovalTarget(sessions, target)
	}
	candidates := sortSessionsByActivity(sessions.List())
	if trimmedSessionID != "" {
		session := sessions.Get(trimmedSessionID)
		if session == nil {
			return nil, runtimekernel.PendingApproval{}, fmt.Errorf("session %q not found", trimmedSessionID)
		}
		candidates = []*runtimekernel.SessionState{session}
	}
	for _, session := range candidates {
		if approval, ok := findApprovalInSession(session, trimmedTurnID, target); ok {
			return session, approval, nil
		}
	}
	if trimmedTurnID != "" {
		return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval %q not found for turn %q", target, trimmedTurnID)
	}
	return nil, runtimekernel.PendingApproval{}, fmt.Errorf("approval %q not found", target)
}

func findApprovalInSession(session *runtimekernel.SessionState, turnID, approvalID string) (runtimekernel.PendingApproval, bool) {
	if session == nil {
		return runtimekernel.PendingApproval{}, false
	}
	target := strings.TrimSpace(approvalID)
	targetTurnID := strings.TrimSpace(turnID)
	matchesTurn := func(value string) bool {
		return targetTurnID == "" || strings.TrimSpace(value) == targetTurnID
	}
	for _, approval := range session.PendingApprovals {
		if approval.ID == target && matchesTurn(approval.TurnID) {
			return approval, true
		}
	}
	for _, evidence := range session.PendingEvidence {
		if evidence.ID == target && matchesTurn(evidence.TurnID) {
			return pendingEvidenceAsApproval(evidence), true
		}
	}
	if session.CurrentTurn == nil || !matchesTurn(session.CurrentTurn.ID) {
		return runtimekernel.PendingApproval{}, false
	}
	for _, approval := range session.CurrentTurn.PendingApprovals {
		if approval.ID == target && matchesTurn(approval.TurnID) {
			return approval, true
		}
	}
	for _, evidence := range session.CurrentTurn.PendingEvidence {
		if evidence.ID == target && matchesTurn(evidence.TurnID) {
			return pendingEvidenceAsApproval(evidence), true
		}
	}
	return runtimekernel.PendingApproval{}, false
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
