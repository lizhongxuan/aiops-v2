package hostops

import (
	"errors"
)

var ErrPlanNotAccepted = errors.New("host operation plan has not been accepted")

type OperationRisk string

const (
	OperationRiskReadOnly OperationRisk = "read_only"
	OperationRiskMutating OperationRisk = "mutating"
)

func EnforcePlanGate(mission HostOperationMission, risk OperationRisk) error {
	if risk == OperationRiskReadOnly {
		return nil
	}
	if mission.PlanRequired && !mission.PlanAccepted {
		return ErrPlanNotAccepted
	}
	return nil
}
