package opssemantic

import "testing"

func TestOpsSemanticTaskTypesCarryGenericOperationalMeaning(t *testing.T) {
	task := OpsSemanticTask{
		ID:        "task-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		UserGoal:  "inspect host health",
		Intent: OpsIntent{
			Category: "inspect",
			Goal:     "inspect host health",
		},
		Targets: []OpsTarget{{
			Kind:   "host",
			Name:   "host-a",
			Source: SourceHostMention,
		}},
		HostScope: []OpsHostRef{{
			HostID:      "host-a",
			DisplayName: "host-a",
			Source:      SourceHostMention,
		}},
		ActionType:   ActionReadOnly,
		RiskLevel:    RiskReadOnly,
		PlanRequired: true,
		MissingSlots: []MissingSlot{{Name: SlotTargetHost}},
		EvidenceRequirements: []EvidenceRequirement{{
			Kind:        EvidenceCommandOutput,
			Description: "command output",
			Required:    true,
		}},
		ExecutionPolicy: ExecutionPolicy{
			AllowParallel:    true,
			RequiresApproval: false,
		},
	}

	if task.ActionType != ActionReadOnly {
		t.Fatalf("ActionType = %q, want %q", task.ActionType, ActionReadOnly)
	}
	if task.RiskLevel != RiskReadOnly {
		t.Fatalf("RiskLevel = %q, want %q", task.RiskLevel, RiskReadOnly)
	}
	if task.MissingSlots[0].Name != SlotTargetHost {
		t.Fatalf("missing slot = %q, want %q", task.MissingSlots[0].Name, SlotTargetHost)
	}
}
