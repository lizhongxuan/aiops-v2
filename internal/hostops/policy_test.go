package hostops

import (
	"errors"
	"testing"
)

func TestPlanGateBlocksMutatingOperationBeforePlanAccepted(t *testing.T) {
	mission := HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false}
	err := EnforcePlanGate(mission, OperationRiskMutating)
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
}

func TestPlanGateAllowsReadOnlyPrecheckBeforePlanAccepted(t *testing.T) {
	mission := HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false}
	if err := EnforcePlanGate(mission, OperationRiskReadOnly); err != nil {
		t.Fatalf("EnforcePlanGate(readonly) error = %v", err)
	}
}

func TestPlanGateAllowsMutatingAfterPlanAccepted(t *testing.T) {
	mission := HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: true}
	if err := EnforcePlanGate(mission, OperationRiskMutating); err != nil {
		t.Fatalf("EnforcePlanGate(mutating accepted) error = %v", err)
	}
}
