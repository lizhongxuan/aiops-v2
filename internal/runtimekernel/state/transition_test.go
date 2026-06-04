package state

import "testing"

func TestValidateTurnTransition(t *testing.T) {
	cases := []struct {
		name string
		from TurnLifecycle
		typ  TurnTransitionType
		want TurnLifecycle
		ok   bool
	}{
		{"created starts running", LifecycleCreated, TransitionTurnStarted, LifecycleRunning, true},
		{"running blocks", LifecycleRunning, TransitionToolInvocationBlocked, LifecycleBlocked, true},
		{"blocked resumes", LifecycleBlocked, TransitionTurnResumed, LifecycleRunning, true},
		{"running completes", LifecycleRunning, TransitionTurnCompleted, LifecycleCompleted, true},
		{"running fails", LifecycleRunning, TransitionTurnFailed, LifecycleFailed, true},
		{"running cancels", LifecycleRunning, TransitionTurnCancelled, LifecycleCancelled, true},
		{"blocked fails", LifecycleBlocked, TransitionTurnFailed, LifecycleFailed, true},
		{"blocked cancels", LifecycleBlocked, TransitionTurnCancelled, LifecycleCancelled, true},
		{"completed cannot resume", LifecycleCompleted, TransitionTurnResumed, LifecycleRunning, false},
	}

	validator := NewValidator()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Validate(tc.from, tc.typ, tc.want)
			if tc.ok && err != nil {
				t.Fatalf("expected valid transition, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected invalid transition")
			}
		})
	}
}
