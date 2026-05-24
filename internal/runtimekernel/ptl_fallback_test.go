package runtimekernel

import "testing"

func TestPTLFallbackDoesNotRetryOrDropGroups(t *testing.T) {
	groups := []PTLMessageGroup{
		{ID: "g1", Messages: []Message{testToolResultMessage("old", "logs.search", "old", "ref-old")}, HasExternalRef: true},
		{ID: "g2", Messages: []Message{{ID: "user-latest", Role: "user", Content: "latest constraint"}}, Protected: true},
	}
	result := PlanPTLFallback(groups, PTLFallbackOptions{Attempt: 1, MaxAttempts: 3})
	if result.CanRetry {
		t.Fatal("prompt-too-long fallback should not retry")
	}
	if len(result.DroppedGroupIDs) != 0 {
		t.Fatalf("dropped = %#v, want none", result.DroppedGroupIDs)
	}
	if len(result.RetainedGroups) != len(groups) {
		t.Fatalf("retained groups = %d, want %d", len(result.RetainedGroups), len(groups))
	}
}

func TestPTLFallbackGroupsByToolRound(t *testing.T) {
	messages := []Message{
		{ID: "u1", Role: "user", Content: "start"},
		testToolResultMessage("tr-1", "logs.search", "old", "ref-1"),
		{ID: "a1", Role: "assistant", Content: "thinking"},
		testToolResultMessage("tr-2", "metrics.query", "new", "ref-2"),
	}
	groups := GroupMessagesForPTLFallback(messages)
	if len(groups) != 4 {
		t.Fatalf("groups = %d, want 4", len(groups))
	}
	if groups[0].ToolRound != 0 || groups[1].ToolRound != 1 || groups[2].ToolRound != 1 || groups[3].ToolRound != 2 {
		t.Fatalf("tool rounds = %#v", groups)
	}
	if !groups[0].Protected {
		t.Fatal("user group should be protected")
	}
	if !groups[3].Protected {
		t.Fatal("latest group should be protected")
	}
	if !groups[1].HasExternalRef {
		t.Fatal("tool result group should be marked externalized")
	}
}
