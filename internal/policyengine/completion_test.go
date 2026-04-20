package policyengine

import (
	"context"
	"testing"
)

func TestDefaultCompletionEvaluator_PendingApprovals(t *testing.T) {
	eval := &DefaultCompletionEvaluator{}
	state := TurnState{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		PendingApprovals: []string{"approval-1", "approval-2"},
	}

	decision := eval.CheckCompletion(context.Background(), state)

	if decision.Action != PolicyActionDeny {
		t.Errorf("expected Deny, got %s", decision.Action)
	}
	if decision.Reason != "pending approvals" {
		t.Errorf("expected reason 'pending approvals', got %q", decision.Reason)
	}
}

func TestDefaultCompletionEvaluator_PendingEvidence(t *testing.T) {
	eval := &DefaultCompletionEvaluator{}
	state := TurnState{
		SessionID:       "sess-1",
		TurnID:          "turn-1",
		PendingEvidence: []string{"evidence-1"},
	}

	decision := eval.CheckCompletion(context.Background(), state)

	if decision.Action != PolicyActionNeedEvidence {
		t.Errorf("expected NeedEvidence, got %s", decision.Action)
	}
	if decision.Reason != "pending evidence" {
		t.Errorf("expected reason 'pending evidence', got %q", decision.Reason)
	}
}

func TestDefaultCompletionEvaluator_AllComplete(t *testing.T) {
	eval := &DefaultCompletionEvaluator{}
	state := TurnState{
		SessionID:     "sess-1",
		TurnID:        "turn-1",
		ToolCallCount: 5,
		Completed:     true,
	}

	decision := eval.CheckCompletion(context.Background(), state)

	if decision.Action != PolicyActionAllow {
		t.Errorf("expected Allow, got %s", decision.Action)
	}
}

func TestDefaultCompletionEvaluator_ApprovalsBeforeEvidence(t *testing.T) {
	// When both pending approvals and evidence exist, approvals take priority.
	eval := &DefaultCompletionEvaluator{}
	state := TurnState{
		SessionID:        "sess-1",
		TurnID:           "turn-1",
		PendingApprovals: []string{"approval-1"},
		PendingEvidence:  []string{"evidence-1"},
	}

	decision := eval.CheckCompletion(context.Background(), state)

	if decision.Action != PolicyActionDeny {
		t.Errorf("expected Deny (approvals checked first), got %s", decision.Action)
	}
	if decision.Reason != "pending approvals" {
		t.Errorf("expected reason 'pending approvals', got %q", decision.Reason)
	}
}

func TestDefaultCompletionEvaluator_EmptyState(t *testing.T) {
	// A zero-value TurnState with no pending items should allow completion.
	eval := &DefaultCompletionEvaluator{}
	state := TurnState{}

	decision := eval.CheckCompletion(context.Background(), state)

	if decision.Action != PolicyActionAllow {
		t.Errorf("expected Allow for empty state, got %s", decision.Action)
	}
}
