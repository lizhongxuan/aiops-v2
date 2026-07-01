package runtimekernel

import (
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// BudgetController — enforces concurrency budget for workspace tasks.
//
// Requirement 9.6: THE RuntimeKernel SHALL 实现预算和队列机制，只启动预算范围内的
// worker，完成后释放预算并由队列补位.
//
// Design Property 29: 预算/队列并发控制
// For any 任务队列和预算配置，同时处于 running 状态的任务数不应超过预算上限；
// 任务完成后应释放预算并由队列补位.
// ---------------------------------------------------------------------------

// BudgetController manages concurrency budget for workspace tasks.
// It enforces a maximum number of concurrently running tasks and queues
// excess tasks for automatic backfill when running tasks complete.
type BudgetController struct {
	mu        sync.Mutex
	maxBudget int             // maximum concurrent running tasks
	running   map[string]bool // taskID → true for currently running tasks
	queue     []string        // ordered queue of pending task IDs waiting for budget
}

// NewBudgetController creates a BudgetController with the given max concurrency budget.
// maxBudget must be >= 1.
func NewBudgetController(maxBudget int) (*BudgetController, error) {
	if maxBudget < 1 {
		return nil, fmt.Errorf("maxBudget must be >= 1, got %d", maxBudget)
	}
	return &BudgetController{
		maxBudget: maxBudget,
		running:   make(map[string]bool),
		queue:     nil,
	}, nil
}

// TryAcquire attempts to acquire a budget slot for the given task.
// If budget is available, the task is marked as running and returns true.
// If budget is exhausted, the task is enqueued and returns false.
// Returns an error if the taskID is already running or already queued.
func (bc *BudgetController) TryAcquire(taskID string) (acquired bool, err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if taskID == "" {
		return false, fmt.Errorf("taskID is required")
	}

	// Check if already running
	if bc.running[taskID] {
		return false, fmt.Errorf("task %q is already running", taskID)
	}

	// Check if already queued
	for _, qid := range bc.queue {
		if qid == taskID {
			return false, fmt.Errorf("task %q is already queued", taskID)
		}
	}

	// Try to acquire budget
	if len(bc.running) < bc.maxBudget {
		bc.running[taskID] = true
		return true, nil
	}

	// Budget exhausted — enqueue
	bc.queue = append(bc.queue, taskID)
	return false, nil
}

// Release releases the budget slot for the given task and triggers queue backfill.
// Returns the taskID that was promoted from the queue (empty string if none).
func (bc *BudgetController) Release(taskID string) (promoted string, err error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if taskID == "" {
		return "", fmt.Errorf("taskID is required")
	}

	if !bc.running[taskID] {
		return "", fmt.Errorf("task %q is not running", taskID)
	}

	// Release the slot
	delete(bc.running, taskID)

	// Queue backfill: promote the next queued task
	if len(bc.queue) > 0 {
		promoted = bc.queue[0]
		bc.queue = bc.queue[1:]
		bc.running[promoted] = true
	}

	return promoted, nil
}

// RunningCount returns the current number of running tasks.
func (bc *BudgetController) RunningCount() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return len(bc.running)
}

// QueueLen returns the current number of tasks waiting in the queue.
func (bc *BudgetController) QueueLen() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return len(bc.queue)
}

// MaxBudget returns the configured maximum concurrency budget.
func (bc *BudgetController) MaxBudget() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.maxBudget
}

// IsRunning reports whether the given task currently holds a budget slot.
func (bc *BudgetController) IsRunning(taskID string) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.running[taskID]
}

// IsQueued reports whether the given task is currently in the wait queue.
func (bc *BudgetController) IsQueued(taskID string) bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	for _, qid := range bc.queue {
		if qid == taskID {
			return true
		}
	}
	return false
}

// RunningTasks returns a copy of the currently running task IDs.
func (bc *BudgetController) RunningTasks() []string {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	result := make([]string, 0, len(bc.running))
	for id := range bc.running {
		result = append(result, id)
	}
	return result
}

// QueuedTasks returns a copy of the queued task IDs in order.
func (bc *BudgetController) QueuedTasks() []string {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	result := make([]string, len(bc.queue))
	copy(result, bc.queue)
	return result
}

// Reset clears all running tasks and queued tasks from the BudgetController.
// This is used during reconcile after a restart to reset the budget state.
func (bc *BudgetController) Reset() {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.running = make(map[string]bool)
	bc.queue = nil
}

// Remove removes a task from either the running set or the queue.
// This is used when a task is killed or fails before completion.
// Returns true if the task was found and removed, false otherwise.
// If the task was running, triggers queue backfill and returns the promoted taskID.
func (bc *BudgetController) Remove(taskID string) (found bool, promoted string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Check running set
	if bc.running[taskID] {
		delete(bc.running, taskID)
		// Backfill from queue
		if len(bc.queue) > 0 {
			promoted = bc.queue[0]
			bc.queue = bc.queue[1:]
			bc.running[promoted] = true
		}
		return true, promoted
	}

	// Check queue
	for i, qid := range bc.queue {
		if qid == taskID {
			bc.queue = append(bc.queue[:i], bc.queue[i+1:]...)
			return true, ""
		}
	}

	return false, ""
}
