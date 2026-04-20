package agentmgr

import (
	"testing"
	"time"
)

func TestReconcileAgents_NilManager(t *testing.T) {
	bc, _ := NewAgentBudgetController(3)
	_, err := ReconcileAgents(nil, bc)
	if err == nil {
		t.Fatal("expected error for nil AgentManager")
	}
}

func TestReconcileAgents_NilBudgetController(t *testing.T) {
	mgr := NewAgentManager(nil, nil, nil)
	_, err := ReconcileAgents(mgr, nil)
	if err == nil {
		t.Fatal("expected error for nil AgentBudgetController")
	}
}

func TestReconcileAgents_EmptyManager(t *testing.T) {
	mgr := NewAgentManager(nil, nil, nil)
	bc, _ := NewAgentBudgetController(3)

	summary, err := ReconcileAgents(mgr, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalAgents != 0 {
		t.Errorf("expected 0 total agents, got %d", summary.TotalAgents)
	}
	if len(summary.ReconciledAgents) != 0 {
		t.Errorf("expected 0 reconciled agents, got %d", len(summary.ReconciledAgents))
	}
	if len(summary.AlreadyTerminal) != 0 {
		t.Errorf("expected 0 already terminal agents, got %d", len(summary.AlreadyTerminal))
	}
}

func TestReconcileAgents_MarksNonTerminalAsFailed(t *testing.T) {
	mgr := NewAgentManager(nil, nil, nil)
	bc, _ := NewAgentBudgetController(3)

	now := time.Now()

	// Seed instances with various statuses.
	mgr.instances["agent-idle"] = &AgentInstance{
		ID:        "agent-idle",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mgr.instances["agent-running"] = &AgentInstance{
		ID:        "agent-running",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	mgr.instances["agent-waiting"] = &AgentInstance{
		ID:        "agent-waiting",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusWaiting,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Seed budget controller with some state.
	bc.TryAcquire("mission-1", "agent-running")

	summary, err := ReconcileAgents(mgr, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalAgents != 3 {
		t.Errorf("expected 3 total agents, got %d", summary.TotalAgents)
	}
	if len(summary.ReconciledAgents) != 3 {
		t.Errorf("expected 3 reconciled agents, got %d", len(summary.ReconciledAgents))
	}
	if len(summary.AlreadyTerminal) != 0 {
		t.Errorf("expected 0 already terminal, got %d", len(summary.AlreadyTerminal))
	}

	// Verify all are now failed.
	for _, id := range []string{"agent-idle", "agent-running", "agent-waiting"} {
		inst := mgr.instances[id]
		if inst.Status != AgentStatusFailed {
			t.Errorf("agent %q: expected status failed, got %q", id, inst.Status)
		}
		if inst.Error != "reconciled after restart" {
			t.Errorf("agent %q: expected error 'reconciled after restart', got %q", id, inst.Error)
		}
	}

	// Verify budget controller was reset.
	if bc.RunningCount("mission-1") != 0 {
		t.Errorf("expected budget running count 0 after reset, got %d", bc.RunningCount("mission-1"))
	}
}

func TestReconcileAgents_LeavesTerminalUntouched(t *testing.T) {
	mgr := NewAgentManager(nil, nil, nil)
	bc, _ := NewAgentBudgetController(3)

	now := time.Now()

	mgr.instances["agent-completed"] = &AgentInstance{
		ID:        "agent-completed",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusCompleted,
		Output:    "done",
		CreatedAt: now,
		UpdatedAt: now,
	}
	mgr.instances["agent-failed"] = &AgentInstance{
		ID:        "agent-failed",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusFailed,
		Error:     "original error",
		CreatedAt: now,
		UpdatedAt: now,
	}
	mgr.instances["agent-killed"] = &AgentInstance{
		ID:        "agent-killed",
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		Status:    AgentStatusKilled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	summary, err := ReconcileAgents(mgr, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalAgents != 3 {
		t.Errorf("expected 3 total agents, got %d", summary.TotalAgents)
	}
	if len(summary.ReconciledAgents) != 0 {
		t.Errorf("expected 0 reconciled agents, got %d", len(summary.ReconciledAgents))
	}
	if len(summary.AlreadyTerminal) != 3 {
		t.Errorf("expected 3 already terminal, got %d", len(summary.AlreadyTerminal))
	}

	// Verify statuses unchanged.
	if mgr.instances["agent-completed"].Status != AgentStatusCompleted {
		t.Error("completed agent status was changed")
	}
	if mgr.instances["agent-failed"].Error != "original error" {
		t.Error("failed agent error was overwritten")
	}
	if mgr.instances["agent-killed"].Status != AgentStatusKilled {
		t.Error("killed agent status was changed")
	}
}

func TestReconcileAgents_MixedStatuses(t *testing.T) {
	mgr := NewAgentManager(nil, nil, nil)
	bc, _ := NewAgentBudgetController(5)

	now := time.Now()

	// Mix of terminal and non-terminal agents.
	mgr.instances["a1"] = &AgentInstance{
		ID: "a1", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1",
		Status: AgentStatusRunning, CreatedAt: now, UpdatedAt: now,
	}
	mgr.instances["a2"] = &AgentInstance{
		ID: "a2", Kind: AgentKindPlanner, MissionID: "m1", SessionID: "s1",
		Status: AgentStatusCompleted, Output: "plan done", CreatedAt: now, UpdatedAt: now,
	}
	mgr.instances["a3"] = &AgentInstance{
		ID: "a3", Kind: AgentKindWorker, MissionID: "m2", SessionID: "s2",
		Status: AgentStatusIdle, CreatedAt: now, UpdatedAt: now,
	}
	mgr.instances["a4"] = &AgentInstance{
		ID: "a4", Kind: AgentKindWorker, MissionID: "m2", SessionID: "s2",
		Status: AgentStatusFailed, Error: "host offline", CreatedAt: now, UpdatedAt: now,
	}
	mgr.instances["a5"] = &AgentInstance{
		ID: "a5", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1",
		Status: AgentStatusWaiting, CreatedAt: now, UpdatedAt: now,
	}

	// Add budget state for mission m1.
	bc.TryAcquire("m1", "a1")

	summary, err := ReconcileAgents(mgr, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalAgents != 5 {
		t.Errorf("expected 5 total agents, got %d", summary.TotalAgents)
	}
	if len(summary.ReconciledAgents) != 3 {
		t.Errorf("expected 3 reconciled (a1, a3, a5), got %d", len(summary.ReconciledAgents))
	}
	if len(summary.AlreadyTerminal) != 2 {
		t.Errorf("expected 2 already terminal (a2, a4), got %d", len(summary.AlreadyTerminal))
	}

	// Verify non-terminal agents are now failed.
	for _, id := range []string{"a1", "a3", "a5"} {
		if mgr.instances[id].Status != AgentStatusFailed {
			t.Errorf("agent %q should be failed, got %q", id, mgr.instances[id].Status)
		}
	}

	// Verify terminal agents are untouched.
	if mgr.instances["a2"].Status != AgentStatusCompleted {
		t.Error("a2 should remain completed")
	}
	if mgr.instances["a4"].Error != "host offline" {
		t.Error("a4 error should remain 'host offline'")
	}

	// Verify budget controller fully reset.
	if bc.RunningCount("m1") != 0 {
		t.Errorf("expected m1 running count 0, got %d", bc.RunningCount("m1"))
	}
	if bc.RunningCount("m2") != 0 {
		t.Errorf("expected m2 running count 0, got %d", bc.RunningCount("m2"))
	}
}
