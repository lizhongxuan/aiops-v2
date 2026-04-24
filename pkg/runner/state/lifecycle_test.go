package state

import "testing"

func TestValidateRunTransitionAllowsLifecycle(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{name: "new to queued", from: "", to: RunStatusQueued},
		{name: "queued to running", from: RunStatusQueued, to: RunStatusRunning},
		{name: "queued to failed", from: RunStatusQueued, to: RunStatusFailed},
		{name: "running to success", from: RunStatusRunning, to: RunStatusSuccess},
		{name: "running to failed", from: RunStatusRunning, to: RunStatusFailed},
		{name: "running to canceled", from: RunStatusRunning, to: RunStatusCanceled},
		{name: "running to interrupted", from: RunStatusRunning, to: RunStatusInterrupted},
		{name: "terminal to same terminal", from: RunStatusSuccess, to: RunStatusSuccess},
	}

	for _, tc := range tests {
		if err := ValidateRunTransition(tc.from, tc.to); err != nil {
			t.Fatalf("%s: expected valid transition, got %v", tc.name, err)
		}
	}
}

func TestValidateRunTransitionRejectsBackward(t *testing.T) {
	tests := []struct {
		from string
		to   string
	}{
		{from: RunStatusSuccess, to: RunStatusRunning},
		{from: RunStatusFailed, to: RunStatusRunning},
		{from: RunStatusCanceled, to: RunStatusRunning},
		{from: RunStatusInterrupted, to: RunStatusRunning},
	}

	for _, tc := range tests {
		if err := ValidateRunTransition(tc.from, tc.to); err == nil {
			t.Fatalf("expected transition %q -> %q to fail", tc.from, tc.to)
		}
	}
}
