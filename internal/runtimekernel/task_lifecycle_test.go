package runtimekernel

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit tests for TaskStatus state machine
// ---------------------------------------------------------------------------

func TestTaskStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   TaskStatus
		terminal bool
	}{
		{TaskStatusPending, false},
		{TaskStatusRunning, false},
		{TaskStatusCompleted, true},
		{TaskStatusFailed, true},
		{TaskStatusKilled, true},
	}
	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.terminal {
			t.Errorf("TaskStatus(%q).IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
		}
	}
}

func TestTaskStatus_IsValid(t *testing.T) {
	for _, s := range AllTaskStatuses() {
		if !s.IsValid() {
			t.Errorf("AllTaskStatuses() contains invalid status %q", s)
		}
	}
	if TaskStatus("bogus").IsValid() {
		t.Error("bogus status should not be valid")
	}
}

func TestIsValidTransition(t *testing.T) {
	// Valid transitions
	validCases := []struct{ from, to TaskStatus }{
		{TaskStatusPending, TaskStatusRunning},
		{TaskStatusPending, TaskStatusFailed},
		{TaskStatusPending, TaskStatusKilled},
		{TaskStatusRunning, TaskStatusCompleted},
		{TaskStatusRunning, TaskStatusFailed},
		{TaskStatusRunning, TaskStatusKilled},
	}
	for _, tc := range validCases {
		if !IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected valid transition %s → %s", tc.from, tc.to)
		}
	}

	// Invalid transitions
	invalidCases := []struct{ from, to TaskStatus }{
		{TaskStatusPending, TaskStatusCompleted},  // can't complete without running
		{TaskStatusCompleted, TaskStatusRunning},   // terminal → non-terminal
		{TaskStatusFailed, TaskStatusRunning},      // terminal → non-terminal
		{TaskStatusKilled, TaskStatusRunning},      // terminal → non-terminal
		{TaskStatusCompleted, TaskStatusFailed},    // terminal → terminal
		{TaskStatusRunning, TaskStatusPending},     // can't go back to pending
	}
	for _, tc := range invalidCases {
		if IsValidTransition(tc.from, tc.to) {
			t.Errorf("expected invalid transition %s → %s", tc.from, tc.to)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit tests for TaskManager
// ---------------------------------------------------------------------------

func TestTaskManager_AddTask(t *testing.T) {
	tm := NewTaskManager()

	task := &WorkspaceTask{
		ID:          "task-1",
		Type:        "host_exec",
		Description: "check disk",
		HostIDs:     []string{"host-a"},
		StartTime:   time.Now(),
	}

	if err := tm.AddTask(task); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	// Task should be in pending state
	got := tm.GetTask("task-1")
	if got == nil {
		t.Fatal("GetTask returned nil")
	}
	if got.Status != string(TaskStatusPending) {
		t.Errorf("expected status pending, got %q", got.Status)
	}

	// Duplicate add should fail
	if err := tm.AddTask(task); err == nil {
		t.Error("expected error on duplicate add")
	}

	// Nil task should fail
	if err := tm.AddTask(nil); err == nil {
		t.Error("expected error on nil task")
	}

	// Empty ID should fail
	if err := tm.AddTask(&WorkspaceTask{}); err == nil {
		t.Error("expected error on empty ID")
	}
}

func TestTaskManager_Transition(t *testing.T) {
	tm := NewTaskManager()
	task := &WorkspaceTask{
		ID:        "task-1",
		Type:      "host_exec",
		HostIDs:   []string{"host-a"},
		StartTime: time.Now(),
	}
	_ = tm.AddTask(task)

	// pending → running
	if err := tm.Transition("task-1", TaskStatusRunning, ""); err != nil {
		t.Fatalf("transition pending→running failed: %v", err)
	}
	if tm.GetTask("task-1").Status != string(TaskStatusRunning) {
		t.Error("expected running status")
	}

	// running → completed
	if err := tm.Transition("task-1", TaskStatusCompleted, ""); err != nil {
		t.Fatalf("transition running→completed failed: %v", err)
	}
	got := tm.GetTask("task-1")
	if got.Status != string(TaskStatusCompleted) {
		t.Error("expected completed status")
	}
	if got.EndTime == nil {
		t.Error("expected EndTime to be set for terminal state")
	}

	// completed → running (invalid)
	if err := tm.Transition("task-1", TaskStatusRunning, ""); err == nil {
		t.Error("expected error on invalid transition from terminal state")
	}

	// non-existent task
	if err := tm.Transition("no-such-task", TaskStatusRunning, ""); err == nil {
		t.Error("expected error for non-existent task")
	}
}

func TestTaskManager_TransitionSetsError(t *testing.T) {
	tm := NewTaskManager()
	task := &WorkspaceTask{
		ID:        "task-1",
		Type:      "host_exec",
		StartTime: time.Now(),
	}
	_ = tm.AddTask(task)
	_ = tm.Transition("task-1", TaskStatusRunning, "")

	// running → failed with reason
	if err := tm.Transition("task-1", TaskStatusFailed, "timeout"); err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	got := tm.GetTask("task-1")
	if got.Error != "timeout" {
		t.Errorf("expected error 'timeout', got %q", got.Error)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for Host Agent offline detection
// ---------------------------------------------------------------------------

func TestTaskManager_SetHostOffline(t *testing.T) {
	tm := NewTaskManager()
	tm.SetHostOnline("host-a")
	tm.SetHostOnline("host-b")

	// Add tasks assigned to host-a
	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", HostIDs: []string{"host-a"}, StartTime: time.Now()})
	_ = tm.AddTask(&WorkspaceTask{ID: "t2", Type: "host_exec", HostIDs: []string{"host-a"}, StartTime: time.Now()})
	_ = tm.AddTask(&WorkspaceTask{ID: "t3", Type: "host_exec", HostIDs: []string{"host-b"}, StartTime: time.Now()})

	// Move t1 to running
	_ = tm.Transition("t1", TaskStatusRunning, "")

	// Host-a goes offline
	failed := tm.SetHostOffline("host-a")

	// t1 (running) and t2 (pending) should be failed
	if len(failed) != 2 {
		t.Fatalf("expected 2 failed tasks, got %d: %v", len(failed), failed)
	}

	// t3 (host-b) should be unaffected
	t3 := tm.GetTask("t3")
	if t3.Status != string(TaskStatusPending) {
		t.Errorf("t3 should still be pending, got %q", t3.Status)
	}

	// Verify error messages
	t1 := tm.GetTask("t1")
	if t1.Error == "" {
		t.Error("t1 should have error message about host offline")
	}

	// Host status should be offline
	if tm.IsHostOnline("host-a") {
		t.Error("host-a should be offline")
	}
	if !tm.IsHostOnline("host-b") {
		t.Error("host-b should still be online")
	}
}

func TestTaskManager_SetHostOffline_TerminalTasksUnaffected(t *testing.T) {
	tm := NewTaskManager()
	tm.SetHostOnline("host-a")

	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", HostIDs: []string{"host-a"}, StartTime: time.Now()})
	_ = tm.Transition("t1", TaskStatusRunning, "")
	_ = tm.Transition("t1", TaskStatusCompleted, "")

	// Host goes offline — completed task should not be affected
	failed := tm.SetHostOffline("host-a")
	if len(failed) != 0 {
		t.Errorf("expected 0 failed tasks (terminal unaffected), got %d", len(failed))
	}
	if tm.GetTask("t1").Status != string(TaskStatusCompleted) {
		t.Error("completed task should remain completed")
	}
}

// ---------------------------------------------------------------------------
// Unit tests for Mission stop convergence
// ---------------------------------------------------------------------------

func TestTaskManager_StopMission(t *testing.T) {
	tm := NewTaskManager()

	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", StartTime: time.Now()})
	_ = tm.AddTask(&WorkspaceTask{ID: "t2", Type: "multi_host", StartTime: time.Now()})
	_ = tm.AddTask(&WorkspaceTask{ID: "t3", Type: "plan", StartTime: time.Now()})

	// Move t2 to running, t3 to completed
	_ = tm.Transition("t2", TaskStatusRunning, "")
	_ = tm.Transition("t3", TaskStatusRunning, "")
	_ = tm.Transition("t3", TaskStatusCompleted, "")

	// Stop mission
	killed := tm.StopMission("user requested stop")

	// t1 (pending) and t2 (running) should be killed; t3 (completed) unaffected
	if len(killed) != 2 {
		t.Fatalf("expected 2 killed tasks, got %d: %v", len(killed), killed)
	}

	t1 := tm.GetTask("t1")
	if t1.Status != string(TaskStatusKilled) {
		t.Errorf("t1 should be killed, got %q", t1.Status)
	}
	if t1.Error != "user requested stop" {
		t.Errorf("t1 error should be 'user requested stop', got %q", t1.Error)
	}
	if t1.EndTime == nil {
		t.Error("t1 should have EndTime set")
	}

	t3 := tm.GetTask("t3")
	if t3.Status != string(TaskStatusCompleted) {
		t.Errorf("t3 should remain completed, got %q", t3.Status)
	}
}

func TestTaskManager_StopMission_DefaultReason(t *testing.T) {
	tm := NewTaskManager()
	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", StartTime: time.Now()})

	tm.StopMission("")

	t1 := tm.GetTask("t1")
	if t1.Error != "mission stopped" {
		t.Errorf("expected default reason 'mission stopped', got %q", t1.Error)
	}
}

func TestTaskManager_ListByStatus(t *testing.T) {
	tm := NewTaskManager()
	_ = tm.AddTask(&WorkspaceTask{ID: "t1", Type: "host_exec", StartTime: time.Now()})
	_ = tm.AddTask(&WorkspaceTask{ID: "t2", Type: "host_exec", StartTime: time.Now()})
	_ = tm.Transition("t2", TaskStatusRunning, "")

	pending := tm.ListByStatus(TaskStatusPending)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(pending))
	}

	running := tm.ListByStatus(TaskStatusRunning)
	if len(running) != 1 {
		t.Errorf("expected 1 running task, got %d", len(running))
	}
}
