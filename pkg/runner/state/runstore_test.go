package state

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestInMemoryRunStoreRejectsInvalidBackwardTransition(t *testing.T) {
	store := NewInMemoryRunStore()
	runID := NewRunID()
	now := time.Now().UTC()

	if err := store.CreateRun(context.Background(), RunState{
		RunID:        runID,
		WorkflowName: "wf",
		Status:       RunStatusQueued,
		StartedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	run, err := store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	run.Status = RunStatusRunning
	if err := store.UpdateRun(context.Background(), run); err != nil {
		t.Fatalf("set running: %v", err)
	}

	run, err = store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get running run: %v", err)
	}
	run.Status = RunStatusSuccess
	if err := store.UpdateRun(context.Background(), run); err != nil {
		t.Fatalf("set success: %v", err)
	}

	run, err = store.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get success run: %v", err)
	}
	run.Status = RunStatusRunning
	if err := store.UpdateRun(context.Background(), run); err == nil {
		t.Fatalf("expected transition success->running to fail")
	}
}

func TestFileStorePersistsAndReconcilesInterruptedRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner-state.json")
	store := NewFileStore(path)
	runID := NewRunID()
	now := time.Now().UTC()

	if err := store.CreateRun(context.Background(), RunState{
		RunID:        runID,
		WorkflowName: "wf",
		Status:       RunStatusRunning,
		StartedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	reloaded := NewFileStore(path)
	got, err := reloaded.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get persisted run: %v", err)
	}
	if got.Status != RunStatusRunning {
		t.Fatalf("expected running status, got %q", got.Status)
	}

	updated, err := reloaded.MarkInterruptedRunning(context.Background(), "process restarted")
	if err != nil {
		t.Fatalf("mark interrupted: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 run updated, got %d", updated)
	}

	got, err = reloaded.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("get interrupted run: %v", err)
	}
	if got.Status != RunStatusInterrupted {
		t.Fatalf("expected interrupted status, got %q", got.Status)
	}
	if got.InterruptedReason != "process restarted" {
		t.Fatalf("unexpected interrupted reason %q", got.InterruptedReason)
	}
}

func TestStoreGetRunNotFound(t *testing.T) {
	store := NewInMemoryRunStore()
	_, err := store.GetRun(context.Background(), "run-unknown-001")
	if !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound, got %v", err)
	}
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound to return true")
	}
}
