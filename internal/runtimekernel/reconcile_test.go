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

type testSessionRepository struct {
	sessions map[string]*SessionState
}

func newTestSessionRepository() *testSessionRepository {
	return &testSessionRepository{sessions: make(map[string]*SessionState)}
}

func (r *testSessionRepository) GetSession(id string) (*SessionState, error) {
	sess, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	cp := *sess
	return &cp, nil
}

func (r *testSessionRepository) SaveSession(session *SessionState) error {
	if session == nil {
		return nil
	}
	cp := *session
	r.sessions[cp.ID] = &cp
	return nil
}

func (r *testSessionRepository) ListSessions() ([]*SessionState, error) {
	result := make([]*SessionState, 0, len(r.sessions))
	for _, sess := range r.sessions {
		cp := *sess
		result = append(result, &cp)
	}
	return result, nil
}

func (r *testSessionRepository) DeleteSession(id string) error {
	delete(r.sessions, id)
	return nil
}

func TestSessionManager_RepositoryBackedLifecycle(t *testing.T) {
	repo := newTestSessionRepository()
	sm := NewSessionManager(repo)

	session := sm.GetOrCreate("", SessionTypeHost, ModeChat)
	if session == nil {
		t.Fatal("GetOrCreate returned nil session")
	}
	if session.ID == "" {
		t.Fatal("new session should have generated ID")
	}
	if _, ok := repo.sessions[session.ID]; !ok {
		t.Fatal("expected session to be saved to repository on create")
	}

	session.HostID = "host-1"
	sm.Update(session)
	if got := repo.sessions[session.ID]; got == nil || got.HostID != "host-1" {
		t.Fatalf("repository was not updated, got %#v", got)
	}

	sm2 := NewSessionManager(repo)
	loaded := sm2.Get(session.ID)
	if loaded == nil {
		t.Fatal("expected session to be loaded from repository")
	}
	if loaded.HostID != "host-1" {
		t.Fatalf("loaded session host mismatch: got %q", loaded.HostID)
	}

	sm2.Delete(session.ID)
	if _, ok := repo.sessions[session.ID]; ok {
		t.Fatal("expected session to be deleted from repository")
	}
}

func TestSessionManager_GetLatestReturnsMostRecentlyUpdatedSession(t *testing.T) {
	repo := newTestSessionRepository()
	now := time.Now()
	repo.sessions["sess-old"] = &SessionState{
		ID:        "sess-old",
		Type:      SessionTypeHost,
		Mode:      ModeChat,
		UpdatedAt: now.Add(-2 * time.Hour),
	}
	repo.sessions["sess-new"] = &SessionState{
		ID:        "sess-new",
		Type:      SessionTypeWorkspace,
		Mode:      ModePlan,
		UpdatedAt: now,
	}

	sm := NewSessionManager(repo)
	latest := sm.GetLatest()
	if latest == nil {
		t.Fatal("expected latest session")
	}
	if latest.ID != "sess-new" {
		t.Fatalf("latest session = %q, want sess-new", latest.ID)
	}

	workspaceLatest := sm.GetLatestByType(SessionTypeWorkspace)
	if workspaceLatest == nil || workspaceLatest.ID != "sess-new" {
		t.Fatalf("latest workspace session = %#v", workspaceLatest)
	}
}

func TestRestoreRuntimeState_LoadsLatestSessionAndReconcilesTasks(t *testing.T) {
	sessionRepo := newTestSessionRepository()
	now := time.Now()
	sessionRepo.sessions["sess-old"] = &SessionState{
		ID:        "sess-old",
		Type:      SessionTypeHost,
		Mode:      ModeChat,
		UpdatedAt: now.Add(-time.Hour),
	}
	sessionRepo.sessions["sess-new"] = &SessionState{
		ID:               "sess-new",
		Type:             SessionTypeWorkspace,
		Mode:             ModePlan,
		UpdatedAt:        now,
		PendingApprovals: []PendingApproval{{ID: "approval-1", SessionID: "sess-new", TurnID: "turn-1", Iteration: 1, ToolName: "read_file", CreatedAt: now, UpdatedAt: now}},
	}

	taskRepo := newTestTaskRepository()
	taskRepo.tasks["task-pending"] = &WorkspaceTask{
		ID:          "task-pending",
		SessionID:   "sess-new",
		TurnID:      "turn-1",
		Type:        "host_exec",
		Status:      string(TaskStatusPending),
		Description: "pending task",
		StartTime:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	taskRepo.tasks["task-completed"] = &WorkspaceTask{
		ID:          "task-completed",
		SessionID:   "sess-new",
		TurnID:      "turn-1",
		Type:        "host_exec",
		Status:      string(TaskStatusCompleted),
		Description: "done task",
		StartTime:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	sm := NewSessionManager(sessionRepo)
	tm := NewTaskManager(taskRepo)
	bc, _ := NewBudgetController(4)

	state, err := RestoreRuntimeState(sm, tm, bc)
	if err != nil {
		t.Fatalf("RestoreRuntimeState failed: %v", err)
	}
	if state.LatestSession == nil || state.LatestSession.ID != "sess-new" {
		t.Fatalf("latest session = %#v", state.LatestSession)
	}
	if state.ReconcileSummary == nil {
		t.Fatal("expected reconcile summary")
	}
	if len(state.ReconcileSummary.ReconciledTasks) != 1 || state.ReconcileSummary.ReconciledTasks[0] != "task-pending" {
		t.Fatalf("reconciled tasks = %v", state.ReconcileSummary.ReconciledTasks)
	}
	if got := tm.GetTask("task-pending"); got == nil || got.Status != string(TaskStatusFailed) {
		t.Fatalf("pending task after reconcile = %#v", got)
	}
	if got := tm.GetTask("task-completed"); got == nil || got.Status != string(TaskStatusCompleted) {
		t.Fatalf("completed task after reconcile = %#v", got)
	}
}

func TestValidateTurnRecoveryPreconditions(t *testing.T) {
	now := time.Now()
	snapshot := &TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "sess-1",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleSuspended,
		ResumeState: TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &CheckpointMetadata{
			ID:          "chk-1",
			SessionID:   "sess-1",
			TurnID:      "turn-1",
			Iteration:   1,
			Sequence:    1,
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []PendingApproval{{ID: "approval-1", SessionID: "sess-1", TurnID: "turn-1", Iteration: 1, ToolName: "write_file", CreatedAt: now, UpdatedAt: now}},
	}

	state := InspectTurnRecovery(snapshot)
	if !state.Resumable {
		t.Fatal("expected suspended snapshot to be resumable")
	}
	if len(state.Reasons) != 0 {
		t.Fatalf("expected no recovery reasons, got %v", state.Reasons)
	}
	if err := ValidateTurnRecoveryPreconditions(snapshot); err != nil {
		t.Fatalf("expected resumable snapshot to validate, got %v", err)
	}
	result, err := RecoverTurnFromSnapshot(snapshot, func() (TurnResult, error) {
		return TurnResult{
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			SessionID:   snapshot.SessionID,
			TurnID:      snapshot.ID,
			Status:      "completed",
		}, nil
	})
	if err != nil {
		t.Fatalf("expected recoverable turn to execute, got %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed result, got %q", result.Status)
	}

	blocked := *snapshot
	blocked.Lifecycle = TurnLifecyclePending
	if err := ValidateTurnRecoveryPreconditions(&blocked); err == nil {
		t.Fatal("pending turn should not pass recovery preconditions")
	}

	missingCheckpoint := *snapshot
	missingCheckpoint.LatestCheckpoint = nil
	missingCheckpoint.PendingApprovals = nil
	missingCheckpoint.PendingEvidence = nil
	missingCheckpoint.Iterations = nil
	if err := ValidateTurnRecoveryPreconditions(&missingCheckpoint); err == nil {
		t.Fatal("snapshot without checkpoint or iteration history should not be resumable")
	}
}

func TestRecoverTurnFromSnapshotMarksRunningMutatingInvocationSideEffectUnknown(t *testing.T) {
	now := time.Now()
	snapshot := &TurnSnapshot{
		ID:          "turn-side-effect",
		SessionID:   "sess-side-effect",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		Lifecycle:   TurnLifecycleSuspended,
		ResumeState: TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &CheckpointMetadata{
			ID:          "chk-side-effect",
			SessionID:   "sess-side-effect",
			TurnID:      "turn-side-effect",
			Iteration:   1,
			Sequence:    1,
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []PendingApproval{{ID: "approval-side-effect", SessionID: "sess-side-effect", TurnID: "turn-side-effect", Iteration: 1, ToolName: "restart_service", CreatedAt: now, UpdatedAt: now}},
		Iterations: []IterationState{{
			ID:          "turn-side-effect-iter-1",
			SessionID:   "sess-side-effect",
			TurnID:      "turn-side-effect",
			Iteration:   1,
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			StartedAt:   now,
			UpdatedAt:   now,
			ToolInvocations: []ToolInvocationState{
				{ID: "mutating", ToolCallID: "call-mutating", ToolName: "restart_service", Status: ToolInvocationRunning, Mutating: true, StartedAt: now, UpdatedAt: now},
				{ID: "readonly", ToolCallID: "call-readonly", ToolName: "read_metrics", Status: ToolInvocationRunning, StartedAt: now, UpdatedAt: now},
			},
		}},
	}

	result, err := RecoverTurnFromSnapshot(snapshot, func() (TurnResult, error) {
		return TurnResult{SessionType: SessionTypeHost, Mode: ModeExecute, SessionID: snapshot.SessionID, TurnID: snapshot.ID, Status: "blocked"}, nil
	})
	if err != nil {
		t.Fatalf("RecoverTurnFromSnapshot error = %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("recover result status = %q, want blocked", result.Status)
	}
	invocations := snapshot.Iterations[0].ToolInvocations
	if invocations[0].Status != ToolInvocationFailed || invocations[0].FailureKind != "side_effect_unknown" || invocations[0].CompletedAt == nil {
		t.Fatalf("mutating invocation = %#v, want failed side_effect_unknown", invocations[0])
	}
	if len(invocations[0].Attempts) == 0 {
		t.Fatalf("mutating invocation attempts = %#v, want manual reconcile attempt", invocations[0].Attempts)
	}
	lastAttempt := invocations[0].Attempts[len(invocations[0].Attempts)-1]
	if lastAttempt.Action != ToolAttemptActionManualReconcile || lastAttempt.Outcome != ToolAttemptOutcomePlanned || lastAttempt.TriggerFailureKind != "side_effect_unknown" {
		t.Fatalf("manual reconcile attempt = %#v, want planned side_effect_unknown", lastAttempt)
	}
	if invocations[1].Status != ToolInvocationRunning || invocations[1].FailureKind != "" {
		t.Fatalf("readonly invocation = %#v, want unchanged running", invocations[1])
	}
}
