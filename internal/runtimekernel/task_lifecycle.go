package runtimekernel

import (
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// TaskStatus — typed status for WorkspaceTask state machine.
// Valid transitions: pending → running → completed/failed/killed
// ---------------------------------------------------------------------------

// TaskStatus represents the status of a WorkspaceTask.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusKilled    TaskStatus = "killed"
)

// AllTaskStatuses returns all valid task statuses.
func AllTaskStatuses() []TaskStatus {
	return []TaskStatus{
		TaskStatusPending,
		TaskStatusRunning,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusKilled,
	}
}

// IsTerminal reports whether the status is a terminal state (no further transitions).
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusKilled:
		return true
	default:
		return false
	}
}

// IsValid reports whether the status is one of the canonical values.
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusRunning, TaskStatusCompleted, TaskStatusFailed, TaskStatusKilled:
		return true
	default:
		return false
	}
}

// ValidTransitions returns the set of valid next statuses from the given status.
func ValidTransitions(from TaskStatus) []TaskStatus {
	switch from {
	case TaskStatusPending:
		return []TaskStatus{TaskStatusRunning, TaskStatusFailed, TaskStatusKilled}
	case TaskStatusRunning:
		return []TaskStatus{TaskStatusCompleted, TaskStatusFailed, TaskStatusKilled}
	default:
		// Terminal states have no valid transitions
		return nil
	}
}

// IsValidTransition checks whether transitioning from → to is allowed.
func IsValidTransition(from, to TaskStatus) bool {
	for _, valid := range ValidTransitions(from) {
		if valid == to {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// TaskManager — manages WorkspaceTask lifecycle, host offline detection,
// and mission stop convergence.
// ---------------------------------------------------------------------------

// TaskManager manages the lifecycle of WorkspaceTask instances.
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*WorkspaceTask // taskID → task
	hosts map[string]bool           // hostID → online status
}

// NewTaskManager creates a new TaskManager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*WorkspaceTask),
		hosts: make(map[string]bool),
	}
}

// AddTask registers a new task in pending state.
func (tm *TaskManager) AddTask(task *WorkspaceTask) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.tasks[task.ID]; exists {
		return fmt.Errorf("task %q already exists", task.ID)
	}

	// Ensure task starts in pending state
	task.Status = string(TaskStatusPending)
	tm.tasks[task.ID] = task
	return nil
}

// Transition attempts to move a task from its current status to the target status.
// Returns an error if the transition is invalid.
func (tm *TaskManager) Transition(taskID string, to TaskStatus, reason string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task, ok := tm.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	from := TaskStatus(task.Status)
	if !IsValidTransition(from, to) {
		return fmt.Errorf("invalid transition: %s → %s for task %q", from, to, taskID)
	}

	task.Status = string(to)

	// Set end time for terminal states
	if to.IsTerminal() {
		now := time.Now()
		task.EndTime = &now
	}

	// Set error message for failed/killed states
	if (to == TaskStatusFailed || to == TaskStatusKilled) && reason != "" {
		task.Error = reason
	}

	return nil
}

// GetTask returns a task by ID, or nil if not found.
func (tm *TaskManager) GetTask(taskID string) *WorkspaceTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tasks[taskID]
}

// ListTasks returns all tasks.
func (tm *TaskManager) ListTasks() []*WorkspaceTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]*WorkspaceTask, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		result = append(result, t)
	}
	return result
}

// ListByStatus returns tasks filtered by status.
func (tm *TaskManager) ListByStatus(status TaskStatus) []*WorkspaceTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var result []*WorkspaceTask
	for _, t := range tm.tasks {
		if TaskStatus(t.Status) == status {
			result = append(result, t)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Host Agent offline detection — marks tasks as failed when host goes offline.
// Requirement 9.4: WHEN Host Agent 离线时, mark corresponding tasks as failed.
// ---------------------------------------------------------------------------

// SetHostOnline marks a host as online.
func (tm *TaskManager) SetHostOnline(hostID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.hosts[hostID] = true
}

// SetHostOffline marks a host as offline and fails all non-terminal tasks
// assigned to that host.
// Returns the list of task IDs that were marked as failed.
func (tm *TaskManager) SetHostOffline(hostID string) []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.hosts[hostID] = false

	var failedIDs []string
	for _, task := range tm.tasks {
		from := TaskStatus(task.Status)
		if from.IsTerminal() {
			continue
		}
		// Check if this task is assigned to the offline host
		if taskUsesHost(task, hostID) {
			if IsValidTransition(from, TaskStatusFailed) {
				task.Status = string(TaskStatusFailed)
				task.Error = fmt.Sprintf("host %s went offline", hostID)
				now := time.Now()
				task.EndTime = &now
				failedIDs = append(failedIDs, task.ID)
			}
		}
	}
	return failedIDs
}

// IsHostOnline reports whether a host is currently online.
func (tm *TaskManager) IsHostOnline(hostID string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	online, exists := tm.hosts[hostID]
	return exists && online
}

// ---------------------------------------------------------------------------
// Mission stop convergence — converges all incomplete tasks to terminal state.
// Requirement 9.5: WHEN workspace mission is stopped, converge all incomplete tasks.
// ---------------------------------------------------------------------------

// StopMission marks all non-terminal tasks as killed with the given reason.
// Returns the list of task IDs that were converged.
func (tm *TaskManager) StopMission(reason string) []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if reason == "" {
		reason = "mission stopped"
	}

	var killedIDs []string
	for _, task := range tm.tasks {
		from := TaskStatus(task.Status)
		if from.IsTerminal() {
			continue
		}
		if IsValidTransition(from, TaskStatusKilled) {
			task.Status = string(TaskStatusKilled)
			task.Error = reason
			now := time.Now()
			task.EndTime = &now
			killedIDs = append(killedIDs, task.ID)
		}
	}
	return killedIDs
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// taskUsesHost checks if a task is assigned to the given host.
func taskUsesHost(task *WorkspaceTask, hostID string) bool {
	for _, h := range task.HostIDs {
		if h == hostID {
			return true
		}
	}
	return false
}
