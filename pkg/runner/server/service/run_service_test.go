package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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
    action: script.shell
    args:
      script: "echo hello"
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

func TestRunServiceCarriesOpsManualWorkflowMetadata(t *testing.T) {
	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(`
version: "1"
name: manual-meta-demo
steps:
  - name: hello
    action: script.shell
    args:
      script: "echo hello"
`)
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "manual-meta-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, wfSvc, nil, state.NewFileStore(filepath.Join(t.TempDir(), "run-state.json")), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	resp, err := runSvc.Submit(context.Background(), &RunRequest{
		WorkflowName:         "manual-meta-demo",
		ManualID:             "manual-redis-restart",
		WorkflowID:           "manual-meta-demo",
		WorkflowVersion:      "v3",
		WorkflowDigest:       DigestWorkflowContent(wfYAML),
		PreflightStatus:      "passed",
		PreflightEvidenceRef: "preflight:manual-meta-demo:ok",
		Metadata: map[string]any{
			"decision_state": "direct",
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail.ManualID != "manual-redis-restart" || detail.WorkflowID != "manual-meta-demo" || detail.WorkflowVersion != "v3" || detail.WorkflowDigest == "" {
		t.Fatalf("ops manual workflow metadata not carried: %+v", detail.RunMeta)
	}
	if detail.Metadata["decision_state"] != "direct" {
		t.Fatalf("request metadata not carried: %+v", detail.Metadata)
	}
}

func TestRunServiceBlocksWorkflowDigestMismatch(t *testing.T) {
	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(testWorkflowYAML("digest-run-demo", "echo initial"))
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "digest-run-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, wfSvc, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	_, err := runSvc.Submit(context.Background(), &RunRequest{
		WorkflowName:   "digest-run-demo",
		ManualID:       "manual-digest-demo",
		WorkflowDigest: DigestWorkflowContent([]byte(testWorkflowYAML("digest-run-demo", "echo stale"))),
	})
	if !errors.Is(err, ErrWorkflowDigestMismatch) {
		t.Fatalf("submit digest mismatch error = %v, want ErrWorkflowDigestMismatch", err)
	}
}

func TestRunServiceBlocksOpsManualWithoutPassedPreflight(t *testing.T) {
	workflowYAML := []byte(successWorkflowYAML("preflight-guard-demo", "echo guarded"))
	tests := []struct {
		name            string
		preflightStatus string
		wantErr         bool
	}{
		{name: "missing preflight", wantErr: true},
		{name: "failed preflight", preflightStatus: "failed", wantErr: true},
		{name: "passed preflight", preflightStatus: "passed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runSvc := NewRunService(RunServiceConfig{
				MaxConcurrentRuns: 1,
				MaxOutputBytes:    65536,
			}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
			defer runSvc.Close()

			resp, err := runSvc.Submit(context.Background(), &RunRequest{
				WorkflowYAML:         string(workflowYAML),
				ManualID:             "manual-preflight-guard",
				WorkflowDigest:       DigestWorkflowContent(workflowYAML),
				PreflightStatus:      tt.preflightStatus,
				PreflightEvidenceRef: "preflight:guard:ok",
			})
			if tt.wantErr {
				if !errors.Is(err, ErrOpsManualPreflightRequired) {
					t.Fatalf("submit error = %v, want ErrOpsManualPreflightRequired", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("submit should pass with preflight: %v", err)
			}
			if resp.Status != state.RunStatusQueued {
				t.Fatalf("response status = %s, want queued", resp.Status)
			}
		})
	}
}

func TestRunServiceRecordsDigestMismatchForOpsManualSink(t *testing.T) {
	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(testWorkflowYAML("digest-record-demo", "echo current"))
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "digest-record-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	store := NewInMemoryRunRecordStore()
	sink := &capturingOpsManualRunRecordSink{}
	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns:      1,
		MaxOutputBytes:         65536,
		MetaStore:              store,
		OpsManualRunRecordSink: sink,
	}, wfSvc, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	staleDigest := DigestWorkflowContent([]byte(testWorkflowYAML("digest-record-demo", "echo stale")))
	_, err := runSvc.Submit(context.Background(), &RunRequest{
		WorkflowName:    "digest-record-demo",
		ManualID:        "manual-digest-record",
		WorkflowID:      "workflow-digest-record",
		WorkflowVersion: "v7",
		WorkflowDigest:  staleDigest,
		TriggeredBy:     "sre",
		IdempotencyKey:  "digest-record-key",
		Metadata:        map[string]any{"decision_state": "approved"},
	})
	if !errors.Is(err, ErrWorkflowDigestMismatch) {
		t.Fatalf("submit digest mismatch error = %v, want ErrWorkflowDigestMismatch", err)
	}

	items, err := store.List(context.Background(), RunFilter{})
	if err != nil {
		t.Fatalf("list run records: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("digest mismatch should persist one failed run meta, got %d: %+v", len(items), items)
	}
	meta := items[0]
	if meta.Status != state.RunStatusFailed || meta.ManualID != "manual-digest-record" || meta.WorkflowID != "workflow-digest-record" || meta.WorkflowVersion != "v7" || meta.WorkflowDigest != staleDigest {
		t.Fatalf("digest mismatch failed meta mismatch: %+v", meta)
	}
	if meta.Metadata["error_code"] != WorkflowErrorCodeDigestMismatch || meta.Metadata["decision_state"] != "approved" {
		t.Fatalf("digest mismatch metadata mismatch: %+v", meta.Metadata)
	}
	record := sink.waitForRecord(t, 3*time.Second)
	if record.RunID != meta.RunID || record.Status != state.RunStatusFailed || record.ErrorCode != WorkflowErrorCodeDigestMismatch {
		t.Fatalf("digest mismatch sink record mismatch: %+v meta=%+v", record, meta)
	}
	if record.ManualID != "manual-digest-record" || record.WorkflowID != "workflow-digest-record" || record.WorkflowVersion != "v7" || record.WorkflowDigest != staleDigest {
		t.Fatalf("digest mismatch sink metadata mismatch: %+v", record)
	}
}

func TestRunServiceDigestMismatchDoesNotReserveIdempotencyKey(t *testing.T) {
	workflowDir := t.TempDir()
	wfSvc := NewWorkflowService(workflowDir)
	wfYAML := []byte(testWorkflowYAML("digest-idempotency-demo", "echo current"))
	if err := wfSvc.Create(context.Background(), &WorkflowRecord{
		Name:    "digest-idempotency-demo",
		RawYAML: wfYAML,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns: 1,
		MaxOutputBytes:    65536,
	}, wfSvc, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	req := &RunRequest{
		WorkflowName:   "digest-idempotency-demo",
		WorkflowDigest: DigestWorkflowContent([]byte(testWorkflowYAML("digest-idempotency-demo", "echo stale"))),
		IdempotencyKey: "digest-retry-key",
	}
	if _, err := runSvc.Submit(context.Background(), req); !errors.Is(err, ErrWorkflowDigestMismatch) {
		t.Fatalf("submit digest mismatch error = %v, want ErrWorkflowDigestMismatch", err)
	}

	req.WorkflowDigest = DigestWorkflowContent(wfYAML)
	resp, err := runSvc.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("retry with corrected digest should submit: %v", err)
	}
	if resp.Status != state.RunStatusQueued {
		t.Fatalf("corrected retry response mismatch: %+v", resp)
	}
}

func TestRunServiceCallsOpsManualSinkOnTerminalRun(t *testing.T) {
	sink := &capturingOpsManualRunRecordSink{}
	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns:      1,
		MaxOutputBytes:         65536,
		OpsManualRunRecordSink: sink,
	}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	workflowYAML := []byte(successWorkflowYAML("terminal-sink-demo", "echo terminal"))
	resp, err := runSvc.Submit(context.Background(), &RunRequest{
		WorkflowYAML:         string(workflowYAML),
		ManualID:             "manual-terminal",
		WorkflowID:           "workflow-terminal",
		WorkflowVersion:      "v2",
		WorkflowDigest:       DigestWorkflowContent(workflowYAML),
		PreflightStatus:      "passed",
		PreflightEvidenceRef: "preflight:terminal:ok",
		Metadata:             map[string]any{"decision_state": "direct"},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 6*time.Second)

	record := sink.waitForRecord(t, 3*time.Second)
	if record.RunID != resp.RunID || record.Status != state.RunStatusSuccess {
		t.Fatalf("terminal sink record mismatch: %+v", record)
	}
	if record.ManualID != "manual-terminal" || record.WorkflowID != "workflow-terminal" || record.WorkflowVersion != "v2" || record.WorkflowDigest == "" {
		t.Fatalf("terminal sink workflow metadata mismatch: %+v", record)
	}
	if record.Metadata["decision_state"] != "direct" {
		t.Fatalf("terminal sink request metadata mismatch: %+v", record.Metadata)
	}
	if record.Metadata["preflight_status"] != "passed" || record.Metadata["preflight_evidence_ref"] != "preflight:terminal:ok" {
		t.Fatalf("terminal sink preflight metadata mismatch: %+v", record.Metadata)
	}
}

func TestRunServiceSinkFailureDoesNotBreakRunnerState(t *testing.T) {
	sink := &capturingOpsManualRunRecordSink{err: fmt.Errorf("opsmanual sink unavailable")}
	runSvc := NewRunService(RunServiceConfig{
		MaxConcurrentRuns:      1,
		MaxOutputBytes:         65536,
		OpsManualRunRecordSink: sink,
	}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(8), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()

	workflowYAML := []byte(successWorkflowYAML("sink-failure-demo", "echo ok"))
	resp, err := runSvc.Submit(context.Background(), &RunRequest{
		WorkflowYAML:    string(workflowYAML),
		ManualID:        "manual-sink-failure",
		WorkflowDigest:  DigestWorkflowContent(workflowYAML),
		PreflightStatus: "passed",
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 6*time.Second)
	_ = sink.waitForRecord(t, 3*time.Second)

	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("sink failure should not change run status: %+v", detail.RunMeta)
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
    action: script.shell
    args:
      script: "echo persisted"
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

func successWorkflowYAML(name, cmd string) string {
	return fmt.Sprintf(`
version: "1"
name: %s
inventory:
  hosts:
    local:
      address: 127.0.0.1
steps:
  - name: run
    targets: [local]
    action: script.shell
    args:
      script: %q
`, name, cmd)
}

type capturingOpsManualRunRecordSink struct {
	mu      sync.Mutex
	records []OpsManualRunRecord
	err     error
}

func (s *capturingOpsManualRunRecordSink) RecordRun(ctx context.Context, record OpsManualRunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return s.err
}

func (s *capturingOpsManualRunRecordSink) waitForRecord(t *testing.T, timeout time.Duration) OpsManualRunRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if len(s.records) > 0 {
			record := s.records[0]
			s.mu.Unlock()
			return record
		}
		s.mu.Unlock()
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("ops manual run record sink was not called within %s", timeout)
	return OpsManualRunRecord{}
}
