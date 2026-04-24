package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"runner/scheduler"
	"runner/state"
	"runner/workflow"
)

type fakeDispatcher struct {
	result scheduler.Result
	err    error
}

func (d fakeDispatcher) Dispatch(ctx context.Context, task scheduler.Task) (scheduler.Result, error) {
	res := d.result
	if strings.TrimSpace(res.TaskID) == "" {
		res.TaskID = task.ID
	}
	if strings.TrimSpace(res.Status) == "" {
		if d.err != nil {
			res.Status = "failed"
			res.Error = d.err.Error()
		} else {
			res.Status = "success"
		}
	}
	return res, d.err
}

type failingNotifier struct{}

func (f failingNotifier) NotifyRunState(ctx context.Context, payload state.RunStateCallback) error {
	_ = ctx
	_ = payload
	return errors.New("callback unavailable")
}

func TestApplyWithRunPersistsLifecycle(t *testing.T) {
	eng := New()
	eng.dispatcher = fakeDispatcher{
		result: scheduler.Result{
			Status: "success",
			Output: map[string]any{"stdout": "ok"},
		},
	}

	store := state.NewInMemoryRunStore()
	runID := "run-apply-success-0001"
	snapshot, err := eng.ApplyWithRun(context.Background(), simpleWorkflow(), RunOptions{
		RunID: runID,
		Store: store,
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if snapshot.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, snapshot.RunID)
	}
	if snapshot.Status != state.RunStatusSuccess {
		t.Fatalf("expected success status, got %q", snapshot.Status)
	}

	persisted, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if persisted.Status != state.RunStatusSuccess {
		t.Fatalf("expected persisted success, got %q", persisted.Status)
	}
	if persisted.StartedAt.IsZero() || persisted.FinishedAt.IsZero() {
		t.Fatalf("expected started_at and finished_at to be set")
	}
	if len(persisted.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(persisted.Steps))
	}
	step := persisted.Steps[0]
	if step.Name != "step-1" {
		t.Fatalf("unexpected step name %q", step.Name)
	}
	if step.Status != state.RunStatusSuccess {
		t.Fatalf("expected step success, got %q", step.Status)
	}
	if _, ok := step.Hosts["local"]; !ok {
		t.Fatalf("expected host result for local")
	}
}

func TestApplyWithRunCallbackFailureDoesNotFlipStatus(t *testing.T) {
	eng := New()
	eng.dispatcher = fakeDispatcher{
		result: scheduler.Result{
			Status: "success",
			Output: map[string]any{"stdout": "done"},
		},
	}

	store := state.NewInMemoryRunStore()
	runID := "run-callback-fail-0001"
	_, err := eng.ApplyWithRun(context.Background(), simpleWorkflow(), RunOptions{
		RunID:       runID,
		Store:       store,
		Notifier:    failingNotifier{},
		NotifyRetry: 0,
		NotifyDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("apply should succeed even when callback fails: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var persisted state.RunState
	for {
		persisted, err = store.GetRun(context.Background(), runID)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if strings.TrimSpace(persisted.LastNotifyError) != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected last_notify_error to be recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if persisted.Status != state.RunStatusSuccess {
		t.Fatalf("expected status to remain success, got %q", persisted.Status)
	}
}

func TestApplyWithStateIncludesExportedArgs(t *testing.T) {
	eng := New()
	eng.dispatcher = fakeDispatcher{
		result: scheduler.Result{
			Status: "success",
			Output: map[string]any{
				"stdout": "done",
				"vars": map[string]any{
					"BACKUP_LABEL": "20260227-104500F",
					"DATA_SIZE":    "4096",
				},
			},
		},
	}

	store := state.NewInMemoryRunStore()
	runID := "run-export-args-0001"
	snapshot, err := eng.ApplyWithState(context.Background(), simpleWorkflow())
	if err != nil {
		t.Fatalf("apply with state failed: %v", err)
	}
	if snapshot.RunID == "" {
		t.Fatalf("expected run_id in snapshot")
	}
	if len(snapshot.Args) != 2 {
		t.Fatalf("expected 2 exported args, got %d", len(snapshot.Args))
	}
	if snapshot.Args["BACKUP_LABEL"] != "20260227-104500F" {
		t.Fatalf("unexpected BACKUP_LABEL: %#v", snapshot.Args["BACKUP_LABEL"])
	}
	if snapshot.Args["DATA_SIZE"] != "4096" {
		t.Fatalf("unexpected DATA_SIZE: %#v", snapshot.Args["DATA_SIZE"])
	}

	// Ensure ApplyWithRun path still persists args when store/run_id are provided.
	eng2 := New()
	eng2.dispatcher = eng.dispatcher
	snapshot2, err := eng2.ApplyWithRun(context.Background(), simpleWorkflow(), RunOptions{
		RunID: runID,
		Store: store,
	})
	if err != nil {
		t.Fatalf("apply with run failed: %v", err)
	}
	if len(snapshot2.Args) != 2 {
		t.Fatalf("expected snapshot args, got %d", len(snapshot2.Args))
	}
	persisted, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if persisted.Args["BACKUP_LABEL"] != "20260227-104500F" {
		t.Fatalf("unexpected persisted BACKUP_LABEL: %#v", persisted.Args["BACKUP_LABEL"])
	}
}

func TestReconcileRunningMarksInterrupted(t *testing.T) {
	store := state.NewInMemoryRunStore()
	now := time.Now().UTC()

	runningID := "run-reconcile-running-0001"
	successID := "run-reconcile-success-0001"
	if err := store.CreateRun(context.Background(), state.RunState{
		RunID:        runningID,
		WorkflowName: "wf",
		Status:       state.RunStatusRunning,
		StartedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create running run: %v", err)
	}
	if err := store.CreateRun(context.Background(), state.RunState{
		RunID:        successID,
		WorkflowName: "wf",
		Status:       state.RunStatusSuccess,
		StartedAt:    now,
		FinishedAt:   now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create success run: %v", err)
	}

	eng := New()
	updated, err := eng.ReconcileRunning(context.Background(), store, "runner restarted")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 run updated, got %d", updated)
	}

	runningRun, err := store.GetRun(context.Background(), runningID)
	if err != nil {
		t.Fatalf("get running run: %v", err)
	}
	if runningRun.Status != state.RunStatusInterrupted {
		t.Fatalf("expected interrupted status, got %q", runningRun.Status)
	}
	if runningRun.InterruptedReason != "runner restarted" {
		t.Fatalf("unexpected interrupted reason %q", runningRun.InterruptedReason)
	}

	successRun, err := store.GetRun(context.Background(), successID)
	if err != nil {
		t.Fatalf("get success run: %v", err)
	}
	if successRun.Status != state.RunStatusSuccess {
		t.Fatalf("expected success status unchanged, got %q", successRun.Status)
	}
}

func simpleWorkflow() workflow.Workflow {
	return workflow.Workflow{
		Version: "v0.1",
		Name:    "wf",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{
				Name:   "step-1",
				Action: "cmd.run",
			},
		},
	}
}
