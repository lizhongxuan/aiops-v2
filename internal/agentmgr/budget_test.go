package agentmgr

import (
	"fmt"
	"sync"
	"testing"
)

func TestNewAgentBudgetController_InvalidBudget(t *testing.T) {
	_, err := NewAgentBudgetController(0)
	if err == nil {
		t.Fatal("expected error for budget 0")
	}
	_, err = NewAgentBudgetController(-1)
	if err == nil {
		t.Fatal("expected error for negative budget")
	}
}

func TestNewAgentBudgetController_Valid(t *testing.T) {
	bc, err := NewAgentBudgetController(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bc.MissionBudget() != 3 {
		t.Fatalf("expected budget 3, got %d", bc.MissionBudget())
	}
}

func TestTryAcquire_BasicFlow(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)
	mission := "m1"

	// First two agents should acquire immediately.
	acquired, err := bc.TryAcquire(mission, "a1")
	if err != nil || !acquired {
		t.Fatalf("expected acquire, got acquired=%v err=%v", acquired, err)
	}
	acquired, err = bc.TryAcquire(mission, "a2")
	if err != nil || !acquired {
		t.Fatalf("expected acquire, got acquired=%v err=%v", acquired, err)
	}

	// Third agent should be queued.
	acquired, err = bc.TryAcquire(mission, "a3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Fatal("expected agent to be queued, but it acquired")
	}

	if bc.RunningCount(mission) != 2 {
		t.Fatalf("expected 2 running, got %d", bc.RunningCount(mission))
	}
	if bc.QueueLen(mission) != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen(mission))
	}
}

func TestTryAcquire_EmptyIDs(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)

	_, err := bc.TryAcquire("", "a1")
	if err == nil {
		t.Fatal("expected error for empty missionID")
	}
	_, err = bc.TryAcquire("m1", "")
	if err == nil {
		t.Fatal("expected error for empty agentID")
	}
}

func TestTryAcquire_DuplicateRunning(t *testing.T) {
	bc, _ := NewAgentBudgetController(5)
	bc.TryAcquire("m1", "a1")

	_, err := bc.TryAcquire("m1", "a1")
	if err == nil {
		t.Fatal("expected error for duplicate running agent")
	}
}

func TestTryAcquire_DuplicateQueued(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)
	bc.TryAcquire("m1", "a1") // running
	bc.TryAcquire("m1", "a2") // queued

	_, err := bc.TryAcquire("m1", "a2")
	if err == nil {
		t.Fatal("expected error for duplicate queued agent")
	}
}

func TestRelease_PromotesFromQueue(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)
	mission := "m1"

	bc.TryAcquire(mission, "a1") // running
	bc.TryAcquire(mission, "a2") // queued
	bc.TryAcquire(mission, "a3") // queued

	promoted, err := bc.Release(mission, "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != "a2" {
		t.Fatalf("expected promoted=a2, got %q", promoted)
	}

	if !bc.IsRunning(mission, "a2") {
		t.Fatal("a2 should be running after promotion")
	}
	if bc.IsRunning(mission, "a1") {
		t.Fatal("a1 should no longer be running")
	}
	if bc.QueueLen(mission) != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen(mission))
	}
}

func TestRelease_NoQueue(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)
	mission := "m1"

	bc.TryAcquire(mission, "a1")

	promoted, err := bc.Release(mission, "a1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != "" {
		t.Fatalf("expected no promotion, got %q", promoted)
	}
	if bc.RunningCount(mission) != 0 {
		t.Fatalf("expected 0 running, got %d", bc.RunningCount(mission))
	}
}

func TestRelease_Errors(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)

	_, err := bc.Release("", "a1")
	if err == nil {
		t.Fatal("expected error for empty missionID")
	}
	_, err = bc.Release("m1", "")
	if err == nil {
		t.Fatal("expected error for empty agentID")
	}
	_, err = bc.Release("m1", "a1")
	if err == nil {
		t.Fatal("expected error for non-existent mission")
	}

	bc.TryAcquire("m1", "a1")
	_, err = bc.Release("m1", "a99")
	if err == nil {
		t.Fatal("expected error for non-running agent")
	}
}

func TestRemove_RunningAgent(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)
	mission := "m1"

	bc.TryAcquire(mission, "a1") // running
	bc.TryAcquire(mission, "a2") // queued

	found, promoted := bc.Remove(mission, "a1")
	if !found {
		t.Fatal("expected agent to be found")
	}
	if promoted != "a2" {
		t.Fatalf("expected promoted=a2, got %q", promoted)
	}
	if !bc.IsRunning(mission, "a2") {
		t.Fatal("a2 should be running after promotion")
	}
}

func TestRemove_QueuedAgent(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)
	mission := "m1"

	bc.TryAcquire(mission, "a1") // running
	bc.TryAcquire(mission, "a2") // queued
	bc.TryAcquire(mission, "a3") // queued

	found, promoted := bc.Remove(mission, "a2")
	if !found {
		t.Fatal("expected agent to be found")
	}
	if promoted != "" {
		t.Fatalf("expected no promotion when removing queued agent, got %q", promoted)
	}
	if bc.QueueLen(mission) != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen(mission))
	}
	if bc.IsQueued(mission, "a2") {
		t.Fatal("a2 should no longer be queued")
	}
}

func TestRemove_NotFound(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)

	found, _ := bc.Remove("m1", "a1")
	if found {
		t.Fatal("expected not found for non-existent mission")
	}

	bc.TryAcquire("m1", "a1")
	found, _ = bc.Remove("m1", "a99")
	if found {
		t.Fatal("expected not found for non-existent agent")
	}
}

func TestPerMissionIsolation(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)

	// Mission 1 fills its budget.
	acquired, _ := bc.TryAcquire("m1", "a1")
	if !acquired {
		t.Fatal("m1/a1 should acquire")
	}

	// Mission 2 should have its own independent budget.
	acquired, _ = bc.TryAcquire("m2", "b1")
	if !acquired {
		t.Fatal("m2/b1 should acquire (independent budget)")
	}

	// Mission 1 budget is full.
	acquired, _ = bc.TryAcquire("m1", "a2")
	if acquired {
		t.Fatal("m1/a2 should be queued (budget full)")
	}

	// Mission 2 budget is full.
	acquired, _ = bc.TryAcquire("m2", "b2")
	if acquired {
		t.Fatal("m2/b2 should be queued (budget full)")
	}

	if bc.RunningCount("m1") != 1 {
		t.Fatalf("m1 running: expected 1, got %d", bc.RunningCount("m1"))
	}
	if bc.RunningCount("m2") != 1 {
		t.Fatalf("m2 running: expected 1, got %d", bc.RunningCount("m2"))
	}
}

func TestResetMission(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)

	bc.TryAcquire("m1", "a1")
	bc.TryAcquire("m1", "a2")
	bc.TryAcquire("m1", "a3") // queued

	bc.ResetMission("m1")

	if bc.RunningCount("m1") != 0 {
		t.Fatalf("expected 0 running after reset, got %d", bc.RunningCount("m1"))
	}
	if bc.QueueLen("m1") != 0 {
		t.Fatalf("expected 0 queued after reset, got %d", bc.QueueLen("m1"))
	}
}

func TestReset(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)

	bc.TryAcquire("m1", "a1")
	bc.TryAcquire("m2", "b1")

	bc.Reset()

	if bc.RunningCount("m1") != 0 {
		t.Fatalf("expected 0 running for m1 after full reset")
	}
	if bc.RunningCount("m2") != 0 {
		t.Fatalf("expected 0 running for m2 after full reset")
	}
}

func TestRunningAgents_QueuedAgents(t *testing.T) {
	bc, _ := NewAgentBudgetController(2)
	mission := "m1"

	bc.TryAcquire(mission, "a1")
	bc.TryAcquire(mission, "a2")
	bc.TryAcquire(mission, "a3") // queued
	bc.TryAcquire(mission, "a4") // queued

	running := bc.RunningAgents(mission)
	if len(running) != 2 {
		t.Fatalf("expected 2 running agents, got %d", len(running))
	}

	queued := bc.QueuedAgents(mission)
	if len(queued) != 2 {
		t.Fatalf("expected 2 queued agents, got %d", len(queued))
	}
	// Queue order should be preserved.
	if queued[0] != "a3" || queued[1] != "a4" {
		t.Fatalf("expected queue order [a3, a4], got %v", queued)
	}
}

func TestConcurrentAccess(t *testing.T) {
	bc, _ := NewAgentBudgetController(5)
	mission := "m1"

	var wg sync.WaitGroup
	const numAgents = 20

	// Concurrently try to acquire budget.
	for i := 0; i < numAgents; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			agentID := fmt.Sprintf("agent-%d", id)
			bc.TryAcquire(mission, agentID)
		}(i)
	}
	wg.Wait()

	running := bc.RunningCount(mission)
	queued := bc.QueueLen(mission)

	if running != 5 {
		t.Fatalf("expected 5 running, got %d", running)
	}
	if queued != 15 {
		t.Fatalf("expected 15 queued, got %d", queued)
	}

	// Concurrently release all running agents.
	runningAgents := bc.RunningAgents(mission)
	for _, agentID := range runningAgents {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			bc.Release(mission, id)
		}(agentID)
	}
	wg.Wait()

	// After releasing 5, 5 more should be promoted from queue.
	if bc.RunningCount(mission) != 5 {
		t.Fatalf("expected 5 running after release, got %d", bc.RunningCount(mission))
	}
	if bc.QueueLen(mission) != 10 {
		t.Fatalf("expected 10 queued after release, got %d", bc.QueueLen(mission))
	}
}

func TestQueueFIFOOrder(t *testing.T) {
	bc, _ := NewAgentBudgetController(1)
	mission := "m1"

	bc.TryAcquire(mission, "a1") // running
	bc.TryAcquire(mission, "a2") // queued
	bc.TryAcquire(mission, "a3") // queued
	bc.TryAcquire(mission, "a4") // queued

	// Release a1 → a2 promoted.
	promoted, _ := bc.Release(mission, "a1")
	if promoted != "a2" {
		t.Fatalf("expected a2 promoted, got %q", promoted)
	}

	// Release a2 → a3 promoted.
	promoted, _ = bc.Release(mission, "a2")
	if promoted != "a3" {
		t.Fatalf("expected a3 promoted, got %q", promoted)
	}

	// Release a3 → a4 promoted.
	promoted, _ = bc.Release(mission, "a3")
	if promoted != "a4" {
		t.Fatalf("expected a4 promoted, got %q", promoted)
	}

	// Release a4 → no more in queue.
	promoted, _ = bc.Release(mission, "a4")
	if promoted != "" {
		t.Fatalf("expected no promotion, got %q", promoted)
	}
}
