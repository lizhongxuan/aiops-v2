package hostops

import (
	"errors"
	"fmt"
	"strings"
)

var ErrPlanNotAccepted = errors.New("host operation plan has not been accepted")
var ErrCrossHostDenied = errors.New("host child agent cannot operate on another host")
var ErrManagerDirectHostDenied = errors.New("manager agent cannot execute host commands directly")

type OperationRisk string

const (
	OperationRiskReadOnly OperationRisk = "read_only"
	OperationRiskMutating OperationRisk = "mutating"
)

type AgentKind string

const (
	AgentKindManager   AgentKind = "manager"
	AgentKindHostChild AgentKind = "host_child"
)

type ToolContext struct {
	AgentKind   AgentKind
	BoundHostID string
}

func EnforcePlanGate(mission HostOperationMission, risk OperationRisk) error {
	if risk == OperationRiskReadOnly {
		return nil
	}
	if mission.PlanRequired && !mission.PlanAccepted {
		return ErrPlanNotAccepted
	}
	return nil
}

func EnforceHostBinding(ctx ToolContext, requestedHostID string) error {
	requestedHostID = strings.TrimSpace(requestedHostID)
	boundHostID := strings.TrimSpace(ctx.BoundHostID)
	switch ctx.AgentKind {
	case AgentKindHostChild:
		if boundHostID == "" {
			return fmt.Errorf("%w: bound host is empty", ErrCrossHostDenied)
		}
		if requestedHostID == "" {
			return nil
		}
		if !strings.EqualFold(requestedHostID, boundHostID) {
			return fmt.Errorf("%w: requested=%s bound=%s", ErrCrossHostDenied, requestedHostID, boundHostID)
		}
		return nil
	case AgentKindManager:
		if requestedHostID != "" || boundHostID != "" {
			return ErrManagerDirectHostDenied
		}
		return nil
	default:
		if requestedHostID != "" && boundHostID != "" && !strings.EqualFold(requestedHostID, boundHostID) {
			return fmt.Errorf("%w: requested=%s bound=%s", ErrCrossHostDenied, requestedHostID, boundHostID)
		}
		return nil
	}
}
