package service

import (
	"context"
	"path/filepath"
	"strings"
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

func TestBuildRunDetailSynthesizesStepsFromGraphState(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	run := state.RunState{
		RunID:        "run-graph-only",
		WorkflowName: "graph-only",
		Status:       state.RunStatusRunning,
		Graph: &state.GraphRunState{
			Nodes: map[string]state.NodeState{
				"start": {ID: "start", Name: "Start", Type: "start", Status: state.RunStatusSuccess},
				"restore": {
					ID:        "restore",
					Name:      "restore-db",
					Type:      "action",
					Status:    state.RunStatusRunning,
					StartedAt: now,
				},
			},
		},
	}

	detail := buildRunDetail(synthesizeMetaFromRun(run), &run)
	if got, want := len(detail.Steps), 1; got != want {
		t.Fatalf("detail steps = %d, want %d: %+v", got, want, detail.Steps)
	}
	if detail.Steps[0].Name != "restore-db" || detail.Steps[0].Status != state.RunStatusRunning {
		t.Fatalf("unexpected synthesized step: %+v", detail.Steps[0])
	}
	if detail.Graph == nil || detail.Graph.Nodes["restore"].Name != "restore-db" {
		t.Fatalf("detail should still include graph state: %+v", detail.Graph)
	}
}

func TestRunEventRecorderPublishesApprovalEvents(t *testing.T) {
	var published []events.Event
	recorder := &runEventRecorder{
		runID:           "run-approval",
		workflow:        "approval-demo",
		metrics:         metrics.NewCollector(),
		nodeStartedAt:   map[string]time.Time{},
		approvalNodeIDs: map[string]struct{}{"approve": {}},
		publish: func(event events.Event) {
			published = append(published, event)
		},
	}

	recorder.GraphNodeStart("approve")
	recorder.GraphApprovalWaiting("approve")
	recorder.GraphNodeFinish("approve", state.RunStatusSuccess, "approved by sre")
	recorder.GraphApprovalResolved("approve", state.RunStatusSuccess, "approved by sre")

	var types []string
	for _, event := range published {
		types = append(types, event.Type)
	}
	if strings.Join(types, ",") != "node_started,approval_waiting,node_finished,approval_resolved" {
		t.Fatalf("approval event sequence = %v", types)
	}
	if published[1].Status != "waiting" || published[1].Output["node_id"] != "approve" {
		t.Fatalf("approval_waiting event mismatch: %+v", published[1])
	}
	if published[3].Status != state.RunStatusSuccess || published[3].Message != "approved by sre" {
		t.Fatalf("approval_resolved event mismatch: %+v", published[3])
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
