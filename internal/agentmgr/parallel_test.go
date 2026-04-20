package agentmgr

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock AgentRunner for parallel tests.
// ---------------------------------------------------------------------------

// parallelMockRunner simulates agent execution with configurable delays and
// failure behavior for testing parallel execution.
type parallelMockRunner struct {
	mu       sync.Mutex
	delay    time.Duration
	failIDs  map[string]bool // agentIDs that should fail
	callLog  []string        // ordered log of executed agentIDs
	maxConc  int32           // max observed concurrency
	curConc  int32           // current concurrency
}

func newParallelMockRunner(delay time.Duration) *parallelMockRunner {
	return &parallelMockRunner{
		delay:   delay,
		failIDs: make(map[string]bool),
	}
}

func (r *parallelMockRunner) Run(ctx context.Context, config *AgentConfig) (string, error) {
	cur := atomic.AddInt32(&r.curConc, 1)
	defer atomic.AddInt32(&r.curConc, -1)

	// Track max concurrency.
	for {
		old := atomic.LoadInt32(&r.maxConc)
		if cur <= old || atomic.CompareAndSwapInt32(&r.maxConc, old, cur) {
			break
		}
	}

	// Record call.
	r.mu.Lock()
	r.callLog = append(r.callLog, config.HostID)
	shouldFail := r.failIDs[config.HostID]
	r.mu.Unlock()

	// Simulate work.
	select {
	case <-time.After(r.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}

	if shouldFail {
		return "", fmt.Errorf("simulated failure for host %s", config.HostID)
	}
	return fmt.Sprintf("completed work on %s", config.HostID), nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestParallelAgent_NewParallelAgent_Validation(t *testing.T) {
	budget, _ := NewAgentBudgetController(5)
	runner := newParallelMockRunner(0)
	mgr := NewAgentManager(nil, runner, nil)

	// Nil manager.
	_, err := NewParallelAgent(nil, budget)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}

	// Nil budget.
	_, err = NewParallelAgent(mgr, nil)
	if err == nil {
		t.Fatal("expected error for nil budget")
	}

	// Valid.
	pa, err := NewParallelAgent(mgr, budget)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pa == nil {
		t.Fatal("expected non-nil ParallelAgent")
	}
}

func TestParallelAgent_Execute_EmptyWorkers(t *testing.T) {
	budget, _ := NewAgentBudgetController(5)
	runner := newParallelMockRunner(0)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	result, err := pa.Execute(context.Background(), "mission-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result.Results))
	}
}

func TestParallelAgent_Execute_Validation(t *testing.T) {
	budget, _ := NewAgentBudgetController(5)
	runner := newParallelMockRunner(0)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	// Empty missionID.
	_, err := pa.Execute(context.Background(), "", []ParallelWorkerRequest{
		{AgentID: "a1", HostID: "h1", Config: &AgentConfig{}},
	})
	if err == nil {
		t.Fatal("expected error for empty missionID")
	}

	// Empty AgentID.
	_, err = pa.Execute(context.Background(), "m1", []ParallelWorkerRequest{
		{AgentID: "", HostID: "h1", Config: &AgentConfig{}},
	})
	if err == nil {
		t.Fatal("expected error for empty AgentID")
	}

	// Nil Config.
	_, err = pa.Execute(context.Background(), "m1", []ParallelWorkerRequest{
		{AgentID: "a1", HostID: "h1", Config: nil},
	})
	if err == nil {
		t.Fatal("expected error for nil Config")
	}

	// Duplicate AgentID.
	_, err = pa.Execute(context.Background(), "m1", []ParallelWorkerRequest{
		{AgentID: "a1", HostID: "h1", Config: &AgentConfig{}},
		{AgentID: "a1", HostID: "h2", Config: &AgentConfig{}},
	})
	if err == nil {
		t.Fatal("expected error for duplicate AgentID")
	}
}

func TestParallelAgent_Execute_AllSucceed(t *testing.T) {
	budget, _ := NewAgentBudgetController(10)
	runner := newParallelMockRunner(10 * time.Millisecond)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}},
		{AgentID: "w3", HostID: "host-c", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-c"}},
	}

	result, err := pa.Execute(context.Background(), "mission-1", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result.Results))
	}
	if len(result.Succeeded) != 3 {
		t.Fatalf("expected 3 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Fatalf("expected 0 failed, got %d", len(result.Failed))
	}

	// Verify all results are completed.
	for _, w := range workers {
		r, ok := result.Results[w.AgentID]
		if !ok {
			t.Fatalf("missing result for %s", w.AgentID)
		}
		if r.Status != AgentStatusCompleted {
			t.Fatalf("expected completed for %s, got %s", w.AgentID, r.Status)
		}
		if r.Output == "" {
			t.Fatalf("expected non-empty output for %s", w.AgentID)
		}
	}

	// Verify parallel execution happened.
	if atomic.LoadInt32(&runner.maxConc) < 2 {
		t.Log("warning: max concurrency was less than 2, parallel execution may not have been observed")
	}
}

func TestParallelAgent_Execute_PartialFailure(t *testing.T) {
	budget, _ := NewAgentBudgetController(10)
	runner := newParallelMockRunner(10 * time.Millisecond)
	runner.failIDs["host-b"] = true
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}},
		{AgentID: "w3", HostID: "host-c", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-c"}},
	}

	result, err := pa.Execute(context.Background(), "mission-2", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// w1 and w3 should succeed, w2 should fail.
	if len(result.Succeeded) != 2 {
		t.Fatalf("expected 2 succeeded, got %d: %v", len(result.Succeeded), result.Succeeded)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("expected 1 failed, got %d: %v", len(result.Failed), result.Failed)
	}

	// Verify the failed worker.
	r := result.Results["w2"]
	if r == nil {
		t.Fatal("missing result for w2")
	}
	if r.Status != AgentStatusFailed {
		t.Fatalf("expected failed for w2, got %s", r.Status)
	}
	if r.Error == "" {
		t.Fatal("expected non-empty error for w2")
	}
}

func TestParallelAgent_Execute_BudgetRespected(t *testing.T) {
	// Budget of 2 means only 2 workers can run concurrently.
	budget, _ := NewAgentBudgetController(2)
	runner := newParallelMockRunner(50 * time.Millisecond)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}},
		{AgentID: "w3", HostID: "host-c", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-c"}},
		{AgentID: "w4", HostID: "host-d", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-d"}},
	}

	result, err := pa.Execute(context.Background(), "mission-3", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All should eventually complete.
	if len(result.Succeeded) != 4 {
		t.Fatalf("expected 4 succeeded, got %d (failed: %v)", len(result.Succeeded), result.Failed)
	}

	// Max concurrency should not exceed budget of 2.
	maxConc := atomic.LoadInt32(&runner.maxConc)
	if maxConc > 2 {
		t.Fatalf("max concurrency %d exceeded budget of 2", maxConc)
	}
}

func TestParallelAgent_Execute_ContextCancellation(t *testing.T) {
	budget, _ := NewAgentBudgetController(1) // Only 1 slot — forces queuing.
	runner := newParallelMockRunner(200 * time.Millisecond)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}},
	}

	result, err := pa.Execute(ctx, "mission-4", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At least one should fail due to context cancellation.
	if len(result.Failed) == 0 {
		t.Fatal("expected at least one failure due to context cancellation")
	}
}

func TestParallelAgent_Execute_SingleWorker(t *testing.T) {
	budget, _ := NewAgentBudgetController(5)
	runner := newParallelMockRunner(5 * time.Millisecond)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	workers := []ParallelWorkerRequest{
		{AgentID: "solo", HostID: "host-x", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-x"}},
	}

	result, err := pa.Execute(context.Background(), "mission-5", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Succeeded) != 1 {
		t.Fatalf("expected 1 succeeded, got %d", len(result.Succeeded))
	}
	if result.Results["solo"].Status != AgentStatusCompleted {
		t.Fatalf("expected completed, got %s", result.Results["solo"].Status)
	}
}

func TestParallelAgent_Execute_AllFail(t *testing.T) {
	budget, _ := NewAgentBudgetController(5)
	runner := newParallelMockRunner(5 * time.Millisecond)
	runner.failIDs["host-a"] = true
	runner.failIDs["host-b"] = true
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)

	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}},
	}

	result, err := pa.Execute(context.Background(), "mission-6", workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Succeeded) != 0 {
		t.Fatalf("expected 0 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 2 {
		t.Fatalf("expected 2 failed, got %d", len(result.Failed))
	}
}
