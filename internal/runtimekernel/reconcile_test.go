package runtimekernel

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit tests for Reconcile logic.
// Validates Requirement 9.7: reconcile after restart does not restore
// already-failed tasks to running.
// ---------------------------------------------------------------------------

func TestReconcile_NilTaskManager(t *testing.T) {
	bc, _ := NewBudgetController(3)
	_, err := Reconcile(nil, bc)
	if err == nil {
		t.Fatal("expected error for nil TaskManager")
	}
}

func TestReconcile_NilBudgetController(t *testing.T) {
	tm := NewTaskManager()
	_, err := Reconcile(tm, nil)
	if err == nil {
		t.Fatal("expected error for nil BudgetController")
	}
}

func TestReconcile_EmptyTaskManager(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(3)

	summary, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalTasks != 0 {
		t.Fatalf("expected 0 total tasks, got %d", summary.TotalTasks)
	}
	if len(summary.ReconciledTasks) != 0 {
		t.Fatalf("expected 0 reconciled tasks, got %d", len(summary.ReconciledTasks))
	}
	if len(summary.AlreadyTerminal) != 0 {
		t.Fatalf("expected 0 already terminal, got %d", len(summary.AlreadyTerminal))
	}
}

func TestReconcile_MarksNonTerminalAsFailed(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(3)

	// Add tasks in various states
	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", Description: "pending task"})
	_ = tm.AddTask(&WorkspaceTask{ID: "t2", Type: "host_exec", Description: "running task"})
	_ = tm.Transition("t2", TaskStatusRunning, "")

	// Simulate budget state
	_, _ = bc.TryAcquire("t2")

	summary, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both pending and running should be reconciled
	if len(summary.ReconciledTasks) != 2 {
		t.Fatalf("expected 2 reconciled tasks, got %d", len(summary.ReconciledTasks))
	}

	// Verify tasks are now failed
	task1 := tm.GetTask("t1")
	if TaskStatus(task1.Status) != TaskStatusFailed {
		t.Fatalf("expected t1 to be failed, got %q", task1.Status)
	}
	if task1.Error != "reconciled after restart" {
		t.Fatalf("expected reconcile reason, got %q", task1.Error)
	}
	if task1.EndTime == nil {
		t.Fatal("expected EndTime to be set for t1")
	}

	task2 := tm.GetTask("t2")
	if TaskStatus(task2.Status) != TaskStatusFailed {
		t.Fatalf("expected t2 to be failed, got %q", task2.Status)
	}
}

func TestReconcile_DoesNotRestoreFailedToRunning(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(3)

	// Add a task that is already failed
	_ = tm.AddTask(&WorkspaceTask{ID: "t-failed", Type: "host_exec", Description: "already failed"})
	_ = tm.Transition("t-failed", TaskStatusFailed, "original failure reason")

	// Add a completed task
	_ = tm.AddTask(&WorkspaceTask{ID: "t-completed", Type: "host_exec", Description: "completed"})
	_ = tm.Transition("t-completed", TaskStatusRunning, "")
	_ = tm.Transition("t-completed", TaskStatusCompleted, "")

	// Add a killed task
	_ = tm.AddTask(&WorkspaceTask{ID: "t-killed", Type: "host_exec", Description: "killed"})
	_ = tm.Transition("t-killed", TaskStatusRunning, "")
	_ = tm.Transition("t-killed", TaskStatusKilled, "user stopped")

	summary, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No tasks should be reconciled — all are already terminal
	if len(summary.ReconciledTasks) != 0 {
		t.Fatalf("expected 0 reconciled tasks, got %d: %v", len(summary.ReconciledTasks), summary.ReconciledTasks)
	}
	if len(summary.AlreadyTerminal) != 3 {
		t.Fatalf("expected 3 already terminal, got %d", len(summary.AlreadyTerminal))
	}

	// Verify failed task remains failed with original reason
	failedTask := tm.GetTask("t-failed")
	if TaskStatus(failedTask.Status) != TaskStatusFailed {
		t.Fatalf("failed task should remain failed, got %q", failedTask.Status)
	}
	if failedTask.Error != "original failure reason" {
		t.Fatalf("failed task error should be preserved, got %q", failedTask.Error)
	}

	// Verify completed task remains completed
	completedTask := tm.GetTask("t-completed")
	if TaskStatus(completedTask.Status) != TaskStatusCompleted {
		t.Fatalf("completed task should remain completed, got %q", completedTask.Status)
	}

	// Verify killed task remains killed
	killedTask := tm.GetTask("t-killed")
	if TaskStatus(killedTask.Status) != TaskStatusKilled {
		t.Fatalf("killed task should remain killed, got %q", killedTask.Status)
	}
}

func TestReconcile_ClearsBudgetControllerState(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(2)

	// Add tasks and acquire budget
	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", Description: "running"})
	_ = tm.Transition("t1", TaskStatusRunning, "")
	_, _ = bc.TryAcquire("t1")

	_ = tm.AddTask(&WorkspaceTask{ID: "t2", Type: "host_exec", Description: "running"})
	_ = tm.Transition("t2", TaskStatusRunning, "")
	_, _ = bc.TryAcquire("t2")

	// Queue a task (budget exhausted)
	_ = tm.AddTask(&WorkspaceTask{ID: "t3", Type: "host_exec", Description: "queued"})
	_, _ = bc.TryAcquire("t3")

	// Verify pre-reconcile state
	if bc.RunningCount() != 2 {
		t.Fatalf("expected 2 running before reconcile, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 1 {
		t.Fatalf("expected 1 queued before reconcile, got %d", bc.QueueLen())
	}

	_, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Budget controller should be cleared
	if bc.RunningCount() != 0 {
		t.Fatalf("expected 0 running after reconcile, got %d", bc.RunningCount())
	}
	if bc.QueueLen() != 0 {
		t.Fatalf("expected 0 queued after reconcile, got %d", bc.QueueLen())
	}
}

func TestReconcile_MixedStates(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(5)

	// Mix of terminal and non-terminal tasks
	_ = tm.AddTask(&WorkspaceTask{ID: "pending1", Type: "host_exec", Description: "pending"})
	_ = tm.AddTask(&WorkspaceTask{ID: "pending2", Type: "host_exec", Description: "pending"})

	_ = tm.AddTask(&WorkspaceTask{ID: "running1", Type: "host_exec", Description: "running"})
	_ = tm.Transition("running1", TaskStatusRunning, "")
	_, _ = bc.TryAcquire("running1")

	_ = tm.AddTask(&WorkspaceTask{ID: "failed1", Type: "host_exec", Description: "failed"})
	_ = tm.Transition("failed1", TaskStatusFailed, "some error")

	_ = tm.AddTask(&WorkspaceTask{ID: "completed1", Type: "host_exec", Description: "completed"})
	_ = tm.Transition("completed1", TaskStatusRunning, "")
	_ = tm.Transition("completed1", TaskStatusCompleted, "")

	summary, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.TotalTasks != 5 {
		t.Fatalf("expected 5 total tasks, got %d", summary.TotalTasks)
	}
	if len(summary.ReconciledTasks) != 3 {
		t.Fatalf("expected 3 reconciled (2 pending + 1 running), got %d: %v",
			len(summary.ReconciledTasks), summary.ReconciledTasks)
	}
	if len(summary.AlreadyTerminal) != 2 {
		t.Fatalf("expected 2 already terminal (1 failed + 1 completed), got %d",
			len(summary.AlreadyTerminal))
	}

	// Verify all non-terminal are now failed
	for _, id := range []string{"pending1", "pending2", "running1"} {
		task := tm.GetTask(id)
		if TaskStatus(task.Status) != TaskStatusFailed {
			t.Fatalf("task %q should be failed after reconcile, got %q", id, task.Status)
		}
	}

	// Verify terminal tasks are untouched
	if TaskStatus(tm.GetTask("failed1").Status) != TaskStatusFailed {
		t.Fatal("failed1 should remain failed")
	}
	if tm.GetTask("failed1").Error != "some error" {
		t.Fatal("failed1 error should be preserved")
	}
	if TaskStatus(tm.GetTask("completed1").Status) != TaskStatusCompleted {
		t.Fatal("completed1 should remain completed")
	}

	// Budget should be cleared
	if bc.RunningCount() != 0 {
		t.Fatalf("budget running should be 0, got %d", bc.RunningCount())
	}
}

func TestReconcile_EndTimeSetForReconciledTasks(t *testing.T) {
	tm := NewTaskManager()
	bc, _ := NewBudgetController(3)

	before := time.Now()

	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", Description: "pending"})

	_, err := Reconcile(tm, bc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task := tm.GetTask("t1")
	if task.EndTime == nil {
		t.Fatal("EndTime should be set after reconcile")
	}
	if task.EndTime.Before(before) {
		t.Fatal("EndTime should be after test start time")
	}
}
