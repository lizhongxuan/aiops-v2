package runtimekernel

import "testing"

func TestCheckpointIdentityIsDeterministicButFactSensitive(t *testing.T) {
	first := newCheckpointMetadata("session-1", "turn-1", 2, 3, "tool_result", TurnLifecycleRunning, TurnResumeStateNone)
	second := newCheckpointMetadata("session-1", "turn-1", 2, 3, "tool_result", TurnLifecycleRunning, TurnResumeStateNone)
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("same checkpoint facts produced IDs %q and %q", first.ID, second.ID)
	}
	changed := newCheckpointMetadata("session-1", "turn-1", 2, 4, "tool_result", TurnLifecycleRunning, TurnResumeStateNone)
	if changed.ID == first.ID {
		t.Fatalf("checkpoint sequence change reused ID %q", first.ID)
	}
}
