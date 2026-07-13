package runtimekernel

import "testing"

func TestPendingApprovalIdentityIsDeterministicAndControlSensitive(t *testing.T) {
	call := ToolCall{ID: "call-1", Name: "restart_service"}
	first := pendingApprovalID("session-1", "turn-1", 2, call, "sha256:args", []string{"host:host-a"})
	second := pendingApprovalID("session-1", "turn-1", 2, call, "sha256:args", []string{"host:host-a"})
	if first == "" || first != second {
		t.Fatalf("same approval facts produced IDs %q and %q", first, second)
	}
	changed := pendingApprovalID("session-1", "turn-1", 2, call, "sha256:changed", []string{"host:host-a"})
	if changed == first {
		t.Fatalf("argument hash change reused approval ID %q", first)
	}
}
