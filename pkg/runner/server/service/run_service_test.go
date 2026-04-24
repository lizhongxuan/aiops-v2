package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/store/eventstore"
	"runner/state"
)

func TestRunServiceIdempotencyKey(t *testing.T) {
	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(`
version: "1"
name: idempotency-demo
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: hello
    targets: [local]
    action: cmd.run
    args:
      cmd: "echo hello"
`)
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "idempotency-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, wfSvc, nil, state.NewFileStore(filepath.Join(t.TempDir(), "run-state.json")), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	req := &RunRequest{
		WorkflowName:   "idempotency-demo",
		IdempotencyKey: "same-key",
	}
	first, err := runSvc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := runSvc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if first.RunID == "" || second.RunID == "" {
		t.Fatalf("run id should not be empty")
	}
	if first.RunID != second.RunID {
		t.Fatalf("idempotency mismatch: %s != %s", first.RunID, second.RunID)
	}
}

func TestRunServiceRestoresMetaAndHistoryAfterRestart(t *testing.T) {
	t.Parallel()

	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(`
version: "1"
name: persistence-demo
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: hello
    targets: [local]
    action: cmd.run
    args:
      cmd: "echo persisted"
`)
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "persistence-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	base := t.TempDir()
	runStateFile := filepath.Join(base, "run-state.json")
	newService := func() *RunService {
		return NewRunService(RunServiceConfig{
			MaxConcurrentRuns: 1,
			MaxOutputBytes:    65536,
			MetaStore:         NewFileRunRecordStore(DeriveRunRecordFile(runStateFile)),
			EventStore:        eventstore.NewFileStore(eventstore.DeriveRunEventDir(runStateFile)),
		}, wfSvc, nil, state.NewFileStore(runStateFile), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	}

	runSvc := newService()
	req := &RunRequest{
		WorkflowName:   "persistence-demo",
		IdempotencyKey: "restart-key",
		TriggeredBy:    "tester",
		Vars: map[string]any{
			"operator": "qa",
		},
	}
	first, err := runSvc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitRunTerminal(t, runSvc, first.RunID, 6*time.Second)
	runSvc.Close()

	restarted := newService()
	defer restarted.Close()

	second, err := restarted.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("submit after restart: %v", err)
	}
	if first.RunID != second.RunID {
		t.Fatalf("idempotency mismatch after restart: %s != %s", first.RunID, second.RunID)
	}

	detail, err := restarted.Get(context.Background(), first.RunID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail.TriggeredBy != "tester" {
		t.Fatalf("unexpected triggered_by: %s", detail.TriggeredBy)
	}
	if detail.IdempotencyKey != "restart-key" {
		t.Fatalf("unexpected idempotency key: %s", detail.IdempotencyKey)
	}
	if detail.WorkflowYAML == "" {
		t.Fatal("workflow yaml should persist")
	}
	if detail.Vars["operator"] != "qa" {
		t.Fatalf("vars should persist: %+v", detail.Vars)
	}

	history, err := restarted.History(context.Background(), first.RunID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected persisted history, got %+v", history)
	}
	if history[0].Type != "run_queued" {
		t.Fatalf("expected first history event run_queued, got %s", history[0].Type)
	}
	if history[len(history)-1].Type != "run_finish" {
		t.Fatalf("expected last history event run_finish, got %s", history[len(history)-1].Type)
	}
}

func waitRunTerminal(t *testing.T, svc *RunService, runID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		detail, err := svc.Get(context.Background(), runID)
		if err == nil && detail != nil && state.IsTerminalRunStatus(detail.Status) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish within %s", runID, timeout)
}
