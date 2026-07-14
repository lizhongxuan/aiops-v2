package specialinputmemory

import "testing"

func TestProjectTransportContextIncludesActiveGrantAndPendingConfirmation(t *testing.T) {
	plan := MemoryReadPlan{
		SchemaVersion: SchemaVersion,
		TurnID:        "turn-ctx",
		VisibleFacts: []MentionFact{{
			ID:           "fact-host-a",
			Kind:         FactKindHost,
			ResourceKind: ResourceKindHost,
			ResourceID:   "host-a",
			CanonicalKey: "host:host-a",
			Display:      "host-a",
			Status:       FactStatusActive,
			TrustLevel:   TrustLevelServerConfirmed,
		}},
		CandidateFacts: []MentionFact{{
			ID:           "fact-raw",
			Kind:         FactKindHost,
			ResourceKind: ResourceKindHost,
			ResourceID:   "1.1.1.1",
			CanonicalKey: "host:1.1.1.1",
			Display:      "1.1.1.1",
			Status:       FactStatusActive,
			TrustLevel:   TrustLevelRawTyped,
		}},
		ActiveExecutionScope: &ExecutionScopeGrant{
			ID:             "grant-host-a",
			ResourceKind:   ResourceKindHost,
			ResourceID:     "host-a",
			CanonicalKey:   "host:host-a",
			Display:        "host-a",
			Status:         GrantStatusActive,
			AllowedActions: []string{ActionInspect, ActionRead, ActionExecLowRisk},
		},
		CandidateRoleBindings: []MentionRoleBinding{{
			ID:           "role-primary",
			RoleKey:      "pg_primary",
			RuntimeName:  "pg主节点",
			ResourceID:   "host-a",
			ResourceKind: ResourceKindHost,
			BindingHash:  "role-hash",
			Status:       RoleBindingStatusActive,
		}},
		PendingConfirmations: []PendingConfirmation{{
			ID:           "pending-role",
			Kind:         "role_binding",
			Reason:       "role_binding_ambiguous",
			RoleKey:      "pg_primary",
			CandidateIDs: []string{"role-a", "role-b"},
		}},
	}

	projected := ProjectTransportContext(plan)

	if projected == nil {
		t.Fatal("ProjectTransportContext() = nil, want context")
	}
	if projected.SchemaVersion != SchemaVersion || projected.TurnID != "turn-ctx" {
		t.Fatalf("projected = %#v, want schema and turn id", projected)
	}
	if projected.ActiveGrant == nil || projected.ActiveGrant.ResourceID != "host-a" {
		t.Fatalf("ActiveGrant = %#v, want host-a", projected.ActiveGrant)
	}
	if len(projected.VisibleFacts) != 1 || projected.VisibleFacts[0].ID != "fact-host-a" {
		t.Fatalf("VisibleFacts = %#v, want fact-host-a", projected.VisibleFacts)
	}
	if len(projected.CandidateFacts) != 1 || projected.CandidateFacts[0].TrustLevel != TrustLevelRawTyped {
		t.Fatalf("CandidateFacts = %#v, want raw typed candidate", projected.CandidateFacts)
	}
	if len(projected.RoleBindings) != 1 || projected.RoleBindings[0].BindingHash != "role-hash" {
		t.Fatalf("RoleBindings = %#v, want role hash", projected.RoleBindings)
	}
	if len(projected.PendingConfirmations) != 1 || projected.PendingConfirmations[0].Reason != "role_binding_ambiguous" {
		t.Fatalf("PendingConfirmations = %#v, want ambiguity", projected.PendingConfirmations)
	}
}

func TestProjectTransportContextReturnsNilForEmptyPlan(t *testing.T) {
	if projected := ProjectTransportContext(MemoryReadPlan{SchemaVersion: SchemaVersion}); projected != nil {
		t.Fatalf("ProjectTransportContext(empty) = %#v, want nil", projected)
	}
}
