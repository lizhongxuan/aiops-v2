package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"runner/state"
)

func TestRunStatusHandlerNotFound(t *testing.T) {
	store := state.NewInMemoryRunStore()
	handler := runStatusHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/run-status?run_id=run-missing-0001", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRunStatusHandlerSuccess(t *testing.T) {
	store := state.NewInMemoryRunStore()
	runID := "run-webui-status-0001"
	now := time.Now().UTC()
	if err := store.CreateRun(context.Background(), state.RunState{
		RunID:        runID,
		WorkflowName: "wf",
		Status:       state.RunStatusSuccess,
		StartedAt:    now,
		FinishedAt:   now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	handler := runStatusHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/run-status?run_id="+runID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var run state.RunState
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if run.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, run.RunID)
	}
	if run.Status != state.RunStatusSuccess {
		t.Fatalf("expected success, got %q", run.Status)
	}
}
