package state

import "fmt"

type TurnLifecycle string

const (
	LifecycleCreated   TurnLifecycle = "created"
	LifecycleRunning   TurnLifecycle = "running"
	LifecycleBlocked   TurnLifecycle = "blocked"
	LifecycleCompleted TurnLifecycle = "completed"
	LifecycleFailed    TurnLifecycle = "failed"
	LifecycleCancelled TurnLifecycle = "cancelled"
)

type TurnTransitionType string

const (
	TransitionTurnStarted           TurnTransitionType = "turn_started"
	TransitionToolInvocationBlocked TurnTransitionType = "tool_invocation_blocked"
	TransitionTurnResumed           TurnTransitionType = "turn_resumed"
	TransitionTurnCompleted         TurnTransitionType = "turn_completed"
	TransitionTurnFailed            TurnTransitionType = "turn_failed"
	TransitionTurnCancelled         TurnTransitionType = "turn_cancelled"
)

type Validator struct{}

func NewValidator() Validator {
	return Validator{}
}

func (Validator) Validate(from TurnLifecycle, typ TurnTransitionType, to TurnLifecycle) error {
	allowed := map[TurnLifecycle]map[TurnTransitionType]TurnLifecycle{
		LifecycleCreated: {
			TransitionTurnStarted: LifecycleRunning,
		},
		LifecycleRunning: {
			TransitionToolInvocationBlocked: LifecycleBlocked,
			TransitionTurnCompleted:         LifecycleCompleted,
			TransitionTurnFailed:            LifecycleFailed,
			TransitionTurnCancelled:         LifecycleCancelled,
		},
		LifecycleBlocked: {
			TransitionTurnResumed:   LifecycleRunning,
			TransitionTurnFailed:    LifecycleFailed,
			TransitionTurnCancelled: LifecycleCancelled,
		},
	}
	next, ok := allowed[from][typ]
	if !ok || next != to {
		return fmt.Errorf("invalid transition: %s --%s--> %s", from, typ, to)
	}
	return nil
}
