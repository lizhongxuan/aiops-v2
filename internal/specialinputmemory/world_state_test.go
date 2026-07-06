package specialinputmemory

import "testing"

func TestBuildWorldStateSectionProjectsOnlyExecutableReadPlan(t *testing.T) {
	binding := NewMentionRoleBinding(RoleBindingInput{
		ResourceKind:   ResourceKindHost,
		ResourceID:     "host-a",
		Display:        "host-a",
		RoleKey:        "pg_primary",
		RuntimeName:    "primary",
		EnvironmentKey: "prod",
		ClusterKey:     "orders",
		SourceTurnID:   "turn-1",
	})
	plan := MemoryReadPlan{
		SchemaVersion: SchemaVersion,
		TurnID:        "turn-2",
		VisibleFacts: []MentionFact{{
			ID:           "fact-a",
			ResourceKind: ResourceKindHost,
			ResourceID:   "host-a",
			Display:      "host-a",
			TrustLevel:   TrustLevelServerConfirmed,
			Status:       FactStatusActive,
		}},
		CandidateFacts: []MentionFact{{
			ID:           "raw-1",
			ResourceKind: ResourceKindHost,
			ResourceID:   "1.1.1.1",
			TrustLevel:   TrustLevelRawTyped,
			Status:       FactStatusActive,
		}},
		ActiveExecutionScope: &ExecutionScopeGrant{
			ID:             "grant-a",
			FactID:         "fact-a",
			ResourceKind:   ResourceKindHost,
			ResourceID:     "host-a",
			Display:        "host-a",
			AllowedActions: []string{ActionInspect, ActionRead, ActionExecLowRisk},
			Status:         GrantStatusActive,
			TrustLevel:     TrustLevelServerConfirmed,
			ValidationHash: "vh-a",
		},
		CandidateRoleBindings: []MentionRoleBinding{binding},
		PendingConfirmations:  []PendingConfirmation{{ID: "pending-raw", Kind: "target", Reason: "raw_typed_requires_confirmation"}},
		ModelSummary:          "active host host-a from previous confirmed mention",
	}

	section := BuildWorldStateSection(plan)
	if section == nil {
		t.Fatal("BuildWorldStateSection() = nil")
	}
	if section.SchemaVersion != SchemaVersion || section.TurnID != "turn-2" {
		t.Fatalf("section identity = %#v", section)
	}
	if section.ActiveExecutionScope == nil || section.ActiveExecutionScope.ResourceID != "host-a" {
		t.Fatalf("ActiveExecutionScope = %#v, want host-a", section.ActiveExecutionScope)
	}
	if len(section.ActiveRoleBindings) != 1 || section.ActiveRoleBindings[0].BindingHash != binding.BindingHash {
		t.Fatalf("ActiveRoleBindings = %#v, want binding hash", section.ActiveRoleBindings)
	}
	if section.MemorySnapshot == nil || section.MemorySnapshot.CandidateFactCount != 1 || section.MemorySnapshot.PendingConfirmationCount != 1 {
		t.Fatalf("MemorySnapshot = %#v, want candidate/pending counts", section.MemorySnapshot)
	}
	if section.ReadPlan == nil || len(section.ReadPlan.CandidateFactIDs) != 1 || len(section.ReadPlan.AllowedActions) != 3 {
		t.Fatalf("ReadPlan = %#v, want ids/actions", section.ReadPlan)
	}
}

func TestBuildWorldStateSectionSkipsEmptyPlan(t *testing.T) {
	if got := BuildWorldStateSection(MemoryReadPlan{}); got != nil {
		t.Fatalf("BuildWorldStateSection(empty) = %#v, want nil", got)
	}
}

func TestCloneWorldStateSectionDeepCopiesMutableSlices(t *testing.T) {
	section := &SpecialInputWorldStateSection{
		SchemaVersion: SchemaVersion,
		ActiveExecutionScope: &ExecutionScopeGrantTrace{
			ID:             "grant-a",
			AllowedActions: []string{ActionRead},
		},
		Conflicts: []MemoryConflictTrace{{
			ID:          "conflict-1",
			ResourceIDs: []string{"host-a"},
			Reasons:     []string{"duplicate_role"},
		}},
		ReadPlan: &MemoryReadPlanTrace{
			AllowedActions:         []string{ActionRead},
			PendingConfirmationIDs: []string{"pending-1"},
		},
	}
	cloned := CloneWorldStateSection(section)
	cloned.ActiveExecutionScope.AllowedActions[0] = ActionMutate
	cloned.Conflicts[0].ResourceIDs[0] = "host-b"
	cloned.ReadPlan.PendingConfirmationIDs[0] = "pending-2"

	if section.ActiveExecutionScope.AllowedActions[0] != ActionRead ||
		section.Conflicts[0].ResourceIDs[0] != "host-a" ||
		section.ReadPlan.PendingConfirmationIDs[0] != "pending-1" {
		t.Fatalf("CloneWorldStateSection mutated source: %#v", section)
	}
}
