package runtimekernel

import (
	"fmt"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for BudgetController
// ---------------------------------------------------------------------------

func TestNewBudgetController_Valid(t *testing.T) {
	bc, err := NewBudgetController(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bc.MaxBudget() != 3 {
		t.Fatalf("expected maxBudget=3, got %d", bc.MaxBudget())
	}
	if bc.RunningCount() != 0 {
		t.Fatalf("expected 0 running, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 0 {
		t.Fatalf("expected 0 queued, got %d", bc.QueueLen())
	}
}

func TestNewBudgetController_InvalidBudget(t *testing.T) {
	_, err := NewBudgetController(0)
	if err == nil {
		t.Fatal("expected error for maxBudget=0")
	}
	_, err = NewBudgetController(-1)
	if err == nil {
		t.Fatal("expected error for maxBudget=-1")
	}
}

func TestTryAcquire_WithinBudget(t *testing.T) {
	bc, _ := NewBudgetController(2)

	acquired, err := bc.TryAcquire("task-1")
	if err != nil || !acquired {
		t.Fatalf("expected acquired=true, err=nil; got acquired=%v, err=%v", acquired, err)
	}

	acquired, err = bc.TryAcquire("task-2")
	if err != nil || !acquired {
		t.Fatalf("expected acquired=true, err=nil; got acquired=%v, err=%v", acquired, err)
	}

	if bc.RunningCount() != 2 {
		t.Fatalf("expected 2 running, got %d", bc.RunningCount())
	}
}

func TestTryAcquire_BudgetExhausted_Enqueues(t *testing.T) {
	bc, _ := NewBudgetController(1)

	acquired, _ := bc.TryAcquire("task-1")
	if !acquired {
		t.Fatal("expected task-1 to acquire budget")
	}

	acquired, err := bc.TryAcquire("task-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acquired {
		t.Fatal("expected task-2 to be queued, not acquired")
	}

	if bc.RunningCount() != 1 {
		t.Fatalf("expected 1 running, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen())
	}
	if !bc.IsQueued("task-2") {
		t.Fatal("expected task-2 to be queued")
	}
}

func TestTryAcquire_EmptyTaskID(t *testing.T) {
	bc, _ := NewBudgetController(2)
	_, err := bc.TryAcquire("")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestTryAcquire_DuplicateRunning(t *testing.T) {
	bc, _ := NewBudgetController(2)
	bc.TryAcquire("task-1")

	_, err := bc.TryAcquire("task-1")
	if err == nil {
		t.Fatal("expected error for duplicate running task")
	}
}

func TestTryAcquire_DuplicateQueued(t *testing.T) {
	bc, _ := NewBudgetController(1)
	bc.TryAcquire("task-1") // running
	bc.TryAcquire("task-2") // queued

	_, err := bc.TryAcquire("task-2")
	if err == nil {
		t.Fatal("expected error for duplicate queued task")
	}
}

func TestRelease_TriggersBackfill(t *testing.T) {
	bc, _ := NewBudgetController(1)
	bc.TryAcquire("task-1")
	bc.TryAcquire("task-2") // queued
	bc.TryAcquire("task-3") // queued

	promoted, err := bc.Release("task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != "task-2" {
		t.Fatalf("expected promoted=task-2, got %q", promoted)
	}

	if bc.RunningCount() != 1 {
		t.Fatalf("expected 1 running, got %d", bc.RunningCount())
	}
	if !bc.IsRunning("task-2") {
		t.Fatal("expected task-2 to be running after backfill")
	}
	if bc.QueueLen() != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen())
	}
}

func TestRelease_NoBackfillWhenQueueEmpty(t *testing.T) {
	bc, _ := NewBudgetController(2)
	bc.TryAcquire("task-1")

	promoted, err := bc.Release("task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != "" {
		t.Fatalf("expected no promotion, got %q", promoted)
	}
	if bc.RunningCount() != 0 {
		t.Fatalf("expected 0 running, got %d", bc.RunningCount())
	}
}

func TestRelease_NotRunning(t *testing.T) {
	bc, _ := NewBudgetController(2)
	_, err := bc.Release("nonexistent")
	if err == nil {
		t.Fatal("expected error for releasing non-running task")
	}
}

func TestRelease_EmptyTaskID(t *testing.T) {
	bc, _ := NewBudgetController(2)
	_, err := bc.Release("")
	if err == nil {
		t.Fatal("expected error for empty taskID")
	}
}

func TestRemove_RunningTask_TriggersBackfill(t *testing.T) {
	bc, _ := NewBudgetController(1)
	bc.TryAcquire("task-1")
	bc.TryAcquire("task-2") // queued

	found, promoted := bc.Remove("task-1")
	if !found {
		t.Fatal("expected task-1 to be found")
	}
	if promoted != "task-2" {
		t.Fatalf("expected promoted=task-2, got %q", promoted)
	}
	if bc.IsRunning("task-1") {
		t.Fatal("task-1 should no longer be running")
	}
	if !bc.IsRunning("task-2") {
		t.Fatal("task-2 should be running after backfill")
	}
}

func TestRemove_QueuedTask(t *testing.T) {
	bc, _ := NewBudgetController(1)
	bc.TryAcquire("task-1")
	bc.TryAcquire("task-2") // queued
	bc.TryAcquire("task-3") // queued

	found, promoted := bc.Remove("task-2")
	if !found {
		t.Fatal("expected task-2 to be found")
	}
	if promoted != "" {
		t.Fatalf("expected no promotion when removing from queue, got %q", promoted)
	}
	if bc.QueueLen() != 1 {
		t.Fatalf("expected 1 queued, got %d", bc.QueueLen())
	}
	// task-3 should still be queued
	if !bc.IsQueued("task-3") {
		t.Fatal("expected task-3 to still be queued")
	}
}

func TestRemove_NotFound(t *testing.T) {
	bc, _ := NewBudgetController(2)
	found, _ := bc.Remove("nonexistent")
	if found {
		t.Fatal("expected not found for nonexistent task")
	}
}

func TestQueueOrder_FIFO(t *testing.T) {
	bc, _ := NewBudgetController(1)
	bc.TryAcquire("task-1") // running

	// Enqueue in order
	bc.TryAcquire("task-2")
	bc.TryAcquire("task-3")
	bc.TryAcquire("task-4")

	queued := bc.QueuedTasks()
	expected := []string{"task-2", "task-3", "task-4"}
	if len(queued) != len(expected) {
		t.Fatalf("expected %d queued, got %d", len(expected), len(queued))
	}
	for i, id := range expected {
		if queued[i] != id {
			t.Fatalf("queue[%d]: expected %q, got %q", i, id, queued[i])
		}
	}

	// Release should promote in FIFO order
	promoted, _ := bc.Release("task-1")
	if promoted != "task-2" {
		t.Fatalf("expected task-2 promoted first, got %q", promoted)
	}
	promoted, _ = bc.Release("task-2")
	if promoted != "task-3" {
		t.Fatalf("expected task-3 promoted second, got %q", promoted)
	}
	promoted, _ = bc.Release("task-3")
	if promoted != "task-4" {
		t.Fatalf("expected task-4 promoted third, got %q", promoted)
	}
}

func TestRunningTasks_ReturnsCopy(t *testing.T) {
	bc, _ := NewBudgetController(3)
	bc.TryAcquire("task-1")
	bc.TryAcquire("task-2")

	tasks := bc.RunningTasks()
	if len(tasks) != 2 {
		t.Fatalf("expected 2 running tasks, got %d", len(tasks))
	}
}

func TestConcurrency_ThreadSafety(t *testing.T) {
	bc, _ := NewBudgetController(5)
	var wg sync.WaitGroup

	// Spawn 20 goroutines trying to acquire
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := fmt.Sprintf("task-%d", id)
			bc.TryAcquire(taskID)
		}(i)
	}
	wg.Wait()

	// Should have exactly 5 running and 15 queued
	if bc.RunningCount() != 5 {
		t.Fatalf("expected 5 running, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 15 {
		t.Fatalf("expected 15 queued, got %d", bc.QueueLen())
	}

	// Release all running tasks concurrently
	running := bc.RunningTasks()
	for _, id := range running {
		wg.Add(1)
		go func(taskID string) {
			defer wg.Done()
			bc.Release(taskID)
		}(id)
	}
	wg.Wait()

	// After releasing 5, 5 should be promoted from queue
	if bc.RunningCount() != 5 {
		t.Fatalf("expected 5 running after backfill, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 10 {
		t.Fatalf("expected 10 queued after backfill, got %d", bc.QueueLen())
	}
}

func TestBudgetInvariant_NeverExceedsMax(t *testing.T) {
	bc, _ := NewBudgetController(3)

	// Fill up budget
	for i := 0; i < 10; i++ {
		bc.TryAcquire(fmt.Sprintf("task-%d", i))
	}

	// Running should never exceed max
	if bc.RunningCount() > 3 {
		t.Fatalf("running count %d exceeds max budget 3", bc.RunningCount())
	}

	// Release and check invariant holds through backfill
	for i := 0; i < 10; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		if bc.IsRunning(taskID) {
			bc.Release(taskID)
			if bc.RunningCount() > 3 {
				t.Fatalf("running count %d exceeds max budget 3 after release", bc.RunningCount())
			}
		}
	}
}
