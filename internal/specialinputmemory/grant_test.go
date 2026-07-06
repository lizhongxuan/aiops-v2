package specialinputmemory

import (
	"testing"
	"time"
)

func TestExecutionScopeGrantActionClassesAndExpiry(t *testing.T) {
	now := time.Unix(1200, 0)
	grant := NewExecutionScopeGrant(MentionFact{
		ID:           "fact-host-a",
		Kind:         FactKindHost,
		CanonicalKey: "host:host-a",
		ResourceKind: ResourceKindHost,
		ResourceID:   "host-a",
		TrustLevel:   TrustLevelServerConfirmed,
		Status:       FactStatusActive,
	}, GrantInput{TurnID: "turn-1", Now: now})

	if !grant.Allows(ActionInspect) || !grant.Allows(ActionRead) || !grant.Allows(ActionExecLowRisk) {
		t.Fatalf("grant allowed actions = %#v", grant.AllowedActions)
	}
	if grant.Allows(ActionMutate) || grant.Allows(ActionDestructive) {
		t.Fatalf("host grant should not allow mutation/destructive actions: %#v", grant.AllowedActions)
	}
	if grant.Expired(now.Add(30 * time.Minute)) {
		t.Fatalf("grant expired too early by time")
	}
	if !grant.Expired(now.Add(30*time.Minute + time.Second)) {
		t.Fatalf("grant did not expire after host TTL")
	}
}

func TestGrantUseUpdatesLastUsedTurnAndWeight(t *testing.T) {
	now := time.Unix(1300, 0)
	grant := ExecutionScopeGrant{
		ID:             "grant-1",
		Status:         GrantStatusActive,
		AllowedActions: []string{ActionInspect},
		Weight:         1,
	}
	next := grant.MarkUsed("turn-2", now)
	if next.LastUsedTurnID != "turn-2" {
		t.Fatalf("last used turn = %q, want turn-2", next.LastUsedTurnID)
	}
	if next.Weight <= grant.Weight {
		t.Fatalf("weight did not increase: before=%v after=%v", grant.Weight, next.Weight)
	}
}
