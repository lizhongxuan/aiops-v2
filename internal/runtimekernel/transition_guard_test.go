package runtimekernel

import (
	"testing"

	runtimestate "aiops-v2/internal/runtimekernel/state"
)

func TestValidateTurnLifecycleTransitionMapsRuntimeStates(t *testing.T) {
	cases := []struct {
		name string
		from TurnLifecycleState
		typ  runtimestate.TurnTransitionType
		to   TurnLifecycleState
		ok   bool
	}{
		{"running blocks as suspended", TurnLifecycleRunning, runtimestate.TransitionToolInvocationBlocked, TurnLifecycleSuspended, true},
		{"suspended resumes as running", TurnLifecycleSuspended, runtimestate.TransitionTurnResumed, TurnLifecycleRunning, true},
		{"running completes", TurnLifecycleRunning, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted, true},
		{"completed cannot block", TurnLifecycleCompleted, runtimestate.TransitionToolInvocationBlocked, TurnLifecycleSuspended, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTurnLifecycleTransition(&TurnSnapshot{Lifecycle: tc.from}, tc.typ, tc.to)
			if tc.ok && err != nil {
				t.Fatalf("expected valid transition, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected invalid transition")
			}
		})
	}
}
