package runtimekernel

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Reconcile logic — executed after workspace restart.
//
// Requirement 9.7: WHEN workspace 重启后，THE RuntimeKernel SHALL 执行 reconcile
// 逻辑，不将已失败任务误恢复为运行中状态.
//
// Design Property 30: Reconcile 安全性
// 重启后 reconcile 应将所有非终态任务标记为 failed，不将已 failed 任务误恢复为
// running.
// ---------------------------------------------------------------------------

// ReconcileSummary contains the results of a reconcile operation.
type ReconcileSummary struct {
	// ReconciledTasks lists the task IDs that were moved to failed state.
	ReconciledTasks []string
	// AlreadyTerminal lists the task IDs that were already in a terminal state.
	AlreadyTerminal []string
	// TotalTasks is the total number of tasks examined.
	TotalTasks int
}

// Reconcile executes post-restart reconciliation on the TaskManager and BudgetController.
// It marks all non-terminal tasks (pending/running) as failed with a reconcile reason,
// clears the BudgetController state, and returns a summary.
//
// Key invariant: already-failed tasks are NEVER restored to running.
func Reconcile(tm *TaskManager, bc *BudgetController) (*ReconcileSummary, error) {
	if tm == nil {
		return nil, fmt.Errorf("TaskManager is nil")
	}
	if bc == nil {
		return nil, fmt.Errorf("BudgetController is nil")
	}

	// Clear budget controller state first — after restart, no tasks should be
	// considered running or queued in the budget.
	bc.Reset()

	// Reconcile all tasks in the TaskManager.
	tm.mu.Lock()
	defer tm.mu.Unlock()

	summary := &ReconcileSummary{
		TotalTasks: len(tm.tasks),
	}

	now := time.Now()
	reason := "reconciled after restart"

	for _, task := range tm.tasks {
		status := TaskStatus(task.Status)

		if status.IsTerminal() {
			// Already in terminal state (completed/failed/killed) — leave untouched.
			summary.AlreadyTerminal = append(summary.AlreadyTerminal, task.ID)
			continue
		}

		// Non-terminal (pending or running) — mark as failed.
		task.Status = string(TaskStatusFailed)
		task.Error = reason
		task.EndTime = &now
		summary.ReconciledTasks = append(summary.ReconciledTasks, task.ID)
	}

	return summary, nil
}
