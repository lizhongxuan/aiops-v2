package specialinputmemory

import (
	"context"
	"testing"
	"time"
)

func TestBuildMemoryReadPlanUsesPreviousGrantForContinuation(t *testing.T) {
	now := time.Unix(800, 0)
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

	plan := BuildMemoryReadPlan(context.Background(), state, MemoryReadPlanInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
	})

	if plan.ActiveExecutionScope == nil {
		t.Fatalf("expected active execution scope")
	}
	if plan.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("active resource = %q, want host-a", plan.ActiveExecutionScope.ResourceID)
	}
	if len(plan.PendingConfirmations) != 0 {
		t.Fatalf("unexpected pending confirmations: %#v", plan.PendingConfirmations)
	}
}

func TestBuildMemoryReadPlanSuspendsGrantWhenRevalidateFails(t *testing.T) {
	now := time.Unix(900, 0)
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

	plan := BuildMemoryReadPlan(context.Background(), state, MemoryReadPlanInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-2",
		Now:       now.Add(time.Minute),
		HostResolver: StaticHostResolver{
			"host-a": {ResourceID: "host-a", Available: false, Reason: "agent_unavailable"},
		},
	})

	if plan.ActiveExecutionScope != nil {
		t.Fatalf("active scope = %#v, want nil when revalidate fails", plan.ActiveExecutionScope)
	}
	if len(plan.SuspendedGrants) != 1 || plan.SuspendedGrants[0].ResourceID != "host-a" {
		t.Fatalf("suspended grants = %#v", plan.SuspendedGrants)
	}
}

func TestBuildMemoryReadPlanRequestsConfirmationForAmbiguousRoleAcrossEnvironments(t *testing.T) {
	now := time.Unix(1000, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{
			{
				Kind: FactKindHost, CanonicalKey: "host:host-a", Display: "host-a",
				ResourceKind: ResourceKindHost, ResourceID: "host-a", RoleKey: "pg_primary",
				EnvironmentKey: "prod", ClusterKey: "orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
			{
				Kind: FactKindHost, CanonicalKey: "host:host-d", Display: "host-d",
				ResourceKind: ResourceKindHost, ResourceID: "host-d", RoleKey: "pg_primary",
				EnvironmentKey: "test", ClusterKey: "orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
		},
	})

	plan := BuildMemoryReadPlan(context.Background(), state, MemoryReadPlanInput{
		SessionID:      "sess-1",
		TaskID:         "task-1",
		TurnID:         "turn-2",
		Now:            now.Add(time.Minute),
		RequestedRole:  "pg_primary",
		EnvironmentKey: "",
		ClusterKey:     "orders",
	})

	if plan.ActiveExecutionScope != nil {
		t.Fatalf("active scope = %#v, want nil for ambiguous env role", plan.ActiveExecutionScope)
	}
	if len(plan.PendingConfirmations) == 0 {
		t.Fatalf("expected pending confirmation for ambiguous env role")
	}
}

func TestBuildMemoryReadPlanResolvesRequestedRoleInEnvironment(t *testing.T) {
	now := time.Unix(1100, 0)
	state, _ := Consolidate(SessionSpecialInputState{}, ConsolidateInput{
		SessionID: "sess-1",
		TaskID:    "task-1",
		TurnID:    "turn-1",
		Now:       now,
		Mentions: []MentionObservation{
			{
				Kind: FactKindHost, CanonicalKey: "host:host-a", Display: "host-a",
				ResourceKind: ResourceKindHost, ResourceID: "host-a", RoleKey: "pg_primary",
				EnvironmentKey: "prod", ClusterKey: "orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
			{
				Kind: FactKindHost, CanonicalKey: "host:host-b", Display: "host-b",
				ResourceKind: ResourceKindHost, ResourceID: "host-b", RoleKey: "pg_standby",
				EnvironmentKey: "prod", ClusterKey: "orders", Source: SourceStructuredSelection, TrustLevel: TrustLevelServerConfirmed,
			},
		},
	})

	plan := BuildMemoryReadPlan(context.Background(), state, MemoryReadPlanInput{
		SessionID:      "sess-1",
		TaskID:         "task-1",
		TurnID:         "turn-2",
		Now:            now.Add(time.Minute),
		RequestedRole:  "pg_standby",
		EnvironmentKey: "prod",
		ClusterKey:     "orders",
	})

	if plan.ActiveExecutionScope == nil || plan.ActiveExecutionScope.ResourceID != "host-b" {
		t.Fatalf("active scope = %#v, want host-b standby", plan.ActiveExecutionScope)
	}
}
