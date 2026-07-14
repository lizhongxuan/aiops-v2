package specialinputmemory

import (
	"testing"
	"time"
)

func TestConsolidateStructuredHostMentionCreatesGrant(t *testing.T) {
	now := time.Unix(300, 0)
	state, events := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{{
			Kind:         FactKindHost,
			CanonicalKey: "host:host-a",
			Display:      "host-a",
			ResourceKind: ResourceKindHost,
			ResourceID:   "host-a",
			Source:       SourceStructuredSelection,
			TrustLevel:   TrustLevelServerConfirmed,
		}},
	})

	if len(state.Facts) != 1 {
		t.Fatalf("facts len = %d, want 1", len(state.Facts))
	}
	if len(state.Grants) != 1 {
		t.Fatalf("grants len = %d, want 1", len(state.Grants))
	}
	grant := state.Grants[0]
	if grant.CanonicalKey != "host:host-a" || grant.Status != GrantStatusActive {
		t.Fatalf("grant = %#v", grant)
	}
	if !grant.Allows(ActionInspect) || !grant.Allows(ActionRead) || !grant.Allows(ActionExecLowRisk) {
		t.Fatalf("host grant allowed actions = %#v, want inspect/read/exec_low_risk", grant.AllowedActions)
	}
	if grant.Allows(ActionMutate) {
		t.Fatalf("host grant should not allow mutate by default")
	}
	if len(events) == 0 {
		t.Fatalf("expected memory events")
	}
}

func TestConsolidateExplicitHostSupersedesPreviousGrant(t *testing.T) {
	now := time.Unix(400, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{{
			Kind: FactKindHost, CanonicalKey: "host:host-a", Display: "host-a",
			ResourceKind: ResourceKindHost, ResourceID: "host-a", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
		}},
	})
	state, _ = Consolidate(state, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
		Mentions: []MentionObservation{{
			Kind: FactKindHost, CanonicalKey: "host:host-b", Display: "host-b",
			ResourceKind: ResourceKindHost, ResourceID: "host-b", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
		}},
	})

	active := ActiveGrants(state.Grants)
	if len(active) != 1 || active[0].CanonicalKey != "host:host-b" {
		t.Fatalf("active grants = %#v, want only host-b", active)
	}
	if state.Grants[0].CanonicalKey == "host:host-a" && state.Grants[0].Status == GrantStatusActive {
		t.Fatalf("old host-a grant remained active: %#v", state.Grants[0])
	}
}

func TestConsolidateRawTypedHostDoesNotCreateExecutableGrant(t *testing.T) {
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       time.Unix(500, 0),
		Mentions: []MentionObservation{{
			Kind:         FactKindHost,
			CanonicalKey: "host:addr:1.1.1.1",
			Display:      "1.1.1.1",
			ResourceKind: ResourceKindHost,
			ResourceID:   "1.1.1.1",
			Source:       SourceTypedFallback,
			TrustLevel:   TrustLevelRawTyped,
		}},
	})

	if len(state.Facts) != 1 {
		t.Fatalf("facts len = %d, want 1", len(state.Facts))
	}
	if len(state.Grants) != 0 {
		t.Fatalf("raw typed mention created grants: %#v", state.Grants)
	}
}

func TestConsolidateCorrectionRevokesOldGrantAndWritesTombstone(t *testing.T) {
	now := time.Unix(600, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{{
			Kind: FactKindHost, CanonicalKey: "host:host-b", Display: "host-b",
			ResourceKind: ResourceKindHost, ResourceID: "host-b", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
		}},
	})
	state, _ = Consolidate(state, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
		Intent:    UserSpecialInputIntent{Kind: IntentCorrection, TargetKind: FactKindHost},
		Mentions: []MentionObservation{{
			Kind: FactKindHost, CanonicalKey: "host:host-c", Display: "host-c",
			ResourceKind: ResourceKindHost, ResourceID: "host-c", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
		}},
	})

	active := ActiveGrants(state.Grants)
	if len(active) != 1 || active[0].CanonicalKey != "host:host-c" {
		t.Fatalf("active grants = %#v, want only host-c", active)
	}
	if len(state.Tombstones) == 0 {
		t.Fatalf("expected tombstone for revoked host-b")
	}
}

func TestConsolidateConfirmPromotesSingleRawTypedCandidate(t *testing.T) {
	now := time.Unix(650, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{{
			Kind:         FactKindHost,
			CanonicalKey: "host:addr:1.1.1.1",
			Display:      "1.1.1.1",
			ResourceKind: ResourceKindHost,
			ResourceID:   "1.1.1.1",
			Source:       SourceTypedFallback,
			TrustLevel:   TrustLevelRawTyped,
		}},
	})
	state, events := Consolidate(state, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
		Intent:    UserSpecialInputIntent{Kind: IntentConfirm, TargetKind: FactKindHost},
	})

	active := ActiveGrants(state.Grants)
	if len(active) != 1 || active[0].CanonicalKey != "host:addr:1.1.1.1" {
		t.Fatalf("active grants = %#v, want confirmed raw candidate", active)
	}
	if state.Facts[0].TrustLevel != TrustLevelServerConfirmed || state.Facts[0].Source != SourceUserConfirmation {
		t.Fatalf("confirmed fact = %#v, want user confirmation", state.Facts[0])
	}
	if !eventTypeObserved(events, "fact_confirmed") {
		t.Fatalf("events = %#v, want fact_confirmed", events)
	}
}

func TestConsolidateConfirmRejectsAmbiguousRawTypedCandidates(t *testing.T) {
	now := time.Unix(660, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{
			{Kind: FactKindHost, CanonicalKey: "host:addr:1.1.1.1", Display: "1.1.1.1", ResourceKind: ResourceKindHost, ResourceID: "1.1.1.1", Source: SourceTypedFallback, TrustLevel: TrustLevelRawTyped},
			{Kind: FactKindHost, CanonicalKey: "host:addr:2.2.2.2", Display: "2.2.2.2", ResourceKind: ResourceKindHost, ResourceID: "2.2.2.2", Source: SourceTypedFallback, TrustLevel: TrustLevelRawTyped},
		},
	})
	state, events := Consolidate(state, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
		Intent:    UserSpecialInputIntent{Kind: IntentConfirm, TargetKind: FactKindHost},
	})

	if len(ActiveGrants(state.Grants)) != 0 {
		t.Fatalf("ambiguous confirmation created grants: %#v", state.Grants)
	}
	if !eventTypeObserved(events, "confirm_rejected") {
		t.Fatalf("events = %#v, want confirm_rejected", events)
	}
}

func TestConsolidateRoleBindingKeepsEnvironmentAndCluster(t *testing.T) {
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       time.Unix(700, 0),
		Mentions: []MentionObservation{
			{
				Kind: FactKindHost, CanonicalKey: "host:host-a", Display: "host-a",
				ResourceKind: ResourceKindHost, ResourceID: "host-a", RoleKey: "pg_primary",
				EnvironmentKey: "prod", ClusterKey: "pg-orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
			{
				Kind: FactKindHost, CanonicalKey: "host:host-b", Display: "host-b",
				ResourceKind: ResourceKindHost, ResourceID: "host-b", RoleKey: "pg_standby",
				EnvironmentKey: "prod", ClusterKey: "pg-orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
			{
				Kind: FactKindHost, CanonicalKey: "host:host-c", Display: "host-c",
				ResourceKind: ResourceKindHost, ResourceID: "host-c", RoleKey: "monitor", RuntimeName: "pg_mon",
				EnvironmentKey: "prod", ClusterKey: "pg-orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
		},
	})

	if len(state.RoleBindings) != 3 {
		t.Fatalf("role bindings len = %d, want 3: %#v", len(state.RoleBindings), state.RoleBindings)
	}
	for _, binding := range state.RoleBindings {
		if binding.EnvironmentKey != "prod" || binding.ClusterKey != "pg-orders" {
			t.Fatalf("binding lost env/cluster: %#v", binding)
		}
		if binding.BindingHash == "" {
			t.Fatalf("binding missing hash: %#v", binding)
		}
	}
}

func eventTypeObserved(events []SpecialInputMemoryEvent, typ string) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}
