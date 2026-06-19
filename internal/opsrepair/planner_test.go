package opsrepair

import (
	"context"
	"testing"

	"aiops-v2/internal/opsmanual"
)

func TestPlanStatefulRepairRequiresReadonlyEvidenceBeforeMutatingSteps(t *testing.T) {
	frame := opsmanual.OperationFrame{
		Target: opsmanual.OperationTarget{Type: "postgresql", Name: "pg-cluster"},
		Roles: []opsmanual.OperationResourceRole{
			{ID: "host-a", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-a"},
			{ID: "host-b", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-b"},
		},
		RiskPreference:       opsmanual.OperationRiskPreference{DataLossAcceptable: true, StillRequiresApproval: true},
		EvidenceRequirements: []string{"cluster_role", "member_health", "replication_status"},
	}
	plan, err := PlanStatefulRepair(context.Background(), PlanRequest{Frame: frame})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatalf("plan must require approval: %#v", plan)
	}
	if len(plan.Options) == 0 {
		t.Fatalf("expected repair options: %#v", plan)
	}
	for _, option := range plan.Options {
		if len(option.Steps) == 0 || option.Steps[0].Phase != PhasePreflight || !option.Steps[0].ReadOnly {
			t.Fatalf("first step must be readonly preflight: %#v", option.Steps)
		}
	}
}
