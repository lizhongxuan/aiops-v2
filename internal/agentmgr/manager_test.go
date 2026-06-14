package agentmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/agentruntime"
)

// ---------------------------------------------------------------------------
// Mock AgentRunner for testing
// ---------------------------------------------------------------------------

// testRunner is a mock AgentRunner that returns configurable results.
type testRunner struct {
	mu      sync.Mutex
	output  string
	err     error
	delay   time.Duration
	called  int
	configs []agentruntime.Config
}

func (r *testRunner) Run(ctx context.Context, config agentruntime.Config) (string, error) {
	r.mu.Lock()
	r.called++
	r.configs = append(r.configs, config)
	delay := r.delay
	output := r.output
	err := r.err
	r.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return output, err
}

// ---------------------------------------------------------------------------
// Helper to create a test AgentManager
// ---------------------------------------------------------------------------

func newTestManager(runner AgentRunner) *AgentManager {
	return NewAgentManager(nil, runner, nil)
}

func validSpawnRequest(id string) SpawnRequest {
	return SpawnRequest{
		ID:        id,
		Kind:      AgentKindWorker,
		MissionID: "mission-1",
		SessionID: "session-1",
		HostID:    "host-1",
		Task:      "check disk",
	}
}

// ---------------------------------------------------------------------------
// Spawn Tests
// ---------------------------------------------------------------------------

func TestSpawn_Success(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	inst, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instance")
	}
	if inst.ID != "agent-1" {
		t.Errorf("expected ID agent-1, got %s", inst.ID)
	}
	if inst.Kind != AgentKindWorker {
		t.Errorf("expected kind worker, got %s", inst.Kind)
	}
	if inst.Status != AgentStatusIdle {
		t.Errorf("expected status idle, got %s", inst.Status)
	}
	if inst.MissionID != "mission-1" {
		t.Errorf("expected missionID mission-1, got %s", inst.MissionID)
	}
	if inst.HostID != "host-1" {
		t.Errorf("expected hostID host-1, got %s", inst.HostID)
	}
	if inst.Task != "check disk" {
		t.Errorf("expected task 'check disk', got %s", inst.Task)
	}
	if inst.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestSpawn_EmptyID(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	req := validSpawnRequest("")
	_, err := mgr.Spawn(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestSpawn_InvalidKind(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	req := validSpawnRequest("agent-1")
	req.Kind = "invalid"
	_, err := mgr.Spawn(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestSpawn_EmptyMissionID(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	req := validSpawnRequest("agent-1")
	req.MissionID = ""
	_, err := mgr.Spawn(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty missionID")
	}
}

func TestSpawn_EmptySessionID(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	req := validSpawnRequest("agent-1")
	req.SessionID = ""
	_, err := mgr.Spawn(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty sessionID")
	}
}

func TestSpawn_DuplicateID(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestSpawn_PlannerKind(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	req := validSpawnRequest("planner-1")
	req.Kind = AgentKindPlanner
	req.HostID = "" // planners don't bind to hosts

	inst, err := mgr.Spawn(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Kind != AgentKindPlanner {
		t.Errorf("expected kind planner, got %s", inst.Kind)
	}
}

// ---------------------------------------------------------------------------
// RunAgent Tests
// ---------------------------------------------------------------------------

func TestRunAgent_Success(t *testing.T) {
	runner := &testRunner{output: "disk usage: 42%"}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	config := &AgentConfig{Kind: AgentKindWorker, MaxIterations: 10}
	result, err := mgr.RunAgent(context.Background(), "agent-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != AgentStatusCompleted {
		t.Errorf("expected status completed, got %s", result.Status)
	}
	if result.Output != "disk usage: 42%" {
		t.Errorf("expected output 'disk usage: 42%%', got %s", result.Output)
	}
	if result.AgentID != "agent-1" {
		t.Errorf("expected agentID agent-1, got %s", result.AgentID)
	}
	if result.HostID != "host-1" {
		t.Errorf("expected hostID host-1, got %s", result.HostID)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}

	// Verify instance state updated
	inst := mgr.GetInstance("agent-1")
	if inst.Status != AgentStatusCompleted {
		t.Errorf("expected instance status completed, got %s", inst.Status)
	}
	if inst.Output != "disk usage: 42%" {
		t.Errorf("expected instance output, got %s", inst.Output)
	}
}

func TestRunAgentReturnsErrorWhenRunnerMissing(t *testing.T) {
	mgr := newTestManager(nil)
	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	_, err = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})
	if err == nil || !strings.Contains(err.Error(), "agent runner is required") {
		t.Fatalf("RunAgent() error = %v, want missing runner error", err)
	}
}

func TestRunAgent_Failure(t *testing.T) {
	runner := &testRunner{output: "partial output", err: errors.New("host offline")}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	config := &AgentConfig{Kind: AgentKindWorker}
	result, err := mgr.RunAgent(context.Background(), "agent-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != AgentStatusFailed {
		t.Errorf("expected status failed, got %s", result.Status)
	}
	if result.Error != "host offline" {
		t.Errorf("expected error 'host offline', got %s", result.Error)
	}
	if result.Output != "partial output" {
		t.Errorf("expected output 'partial output', got %s", result.Output)
	}

	// Verify instance state
	inst := mgr.GetInstance("agent-1")
	if inst.Status != AgentStatusFailed {
		t.Errorf("expected instance status failed, got %s", inst.Status)
	}
	if inst.Error != "host offline" {
		t.Errorf("expected instance error, got %s", inst.Error)
	}
}

func TestRunAgent_NotFound(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	_, err := mgr.RunAgent(context.Background(), "nonexistent", &AgentConfig{})
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestRunAgent_NotIdle(t *testing.T) {
	runner := &testRunner{output: "ok"}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	// Run it once to completion
	config := &AgentConfig{Kind: AgentKindWorker}
	_, err = mgr.RunAgent(context.Background(), "agent-1", config)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}

	// Try to run again — should fail because it's completed
	_, err = mgr.RunAgent(context.Background(), "agent-1", config)
	if err == nil {
		t.Fatal("expected error for non-idle agent")
	}
}

func TestRunAgent_ContextCancelled(t *testing.T) {
	runner := &testRunner{delay: 500 * time.Millisecond}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	config := &AgentConfig{Kind: AgentKindWorker}
	result, err := mgr.RunAgent(ctx, "agent-1", config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be failed due to context cancellation
	if result.Status != AgentStatusFailed {
		t.Errorf("expected status failed, got %s", result.Status)
	}
}

// ---------------------------------------------------------------------------
// KillAgent Tests
// ---------------------------------------------------------------------------

func TestKillAgent_Idle(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	err = mgr.KillAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	inst := mgr.GetInstance("agent-1")
	if inst.Status != AgentStatusKilled {
		t.Errorf("expected status killed, got %s", inst.Status)
	}
}

func TestKillAgent_NotFound(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	err := mgr.KillAgent(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestKillAgent_AlreadyTerminal(t *testing.T) {
	runner := &testRunner{output: "done"}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	// Complete the agent
	_, err = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Try to kill — should fail
	err = mgr.KillAgent(context.Background(), "agent-1")
	if err == nil {
		t.Fatal("expected error for terminal agent")
	}
}

func TestKillAgent_DuringExecution(t *testing.T) {
	runner := &testRunner{delay: 200 * time.Millisecond, output: "partial"}
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	// Start running in background
	var result *AgentResult
	var runErr error
	done := make(chan struct{})
	go func() {
		result, runErr = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})
		close(done)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Kill while running
	err = mgr.KillAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("kill error: %v", err)
	}

	<-done

	if runErr != nil {
		t.Fatalf("run error: %v", runErr)
	}
	// The result should reflect killed status
	if result.Status != AgentStatusKilled {
		t.Errorf("expected status killed, got %s", result.Status)
	}
}

func TestKillAgentCancelsActiveExecutionContext(t *testing.T) {
	runner := newCancelAwareRunner()
	mgr := newTestManager(runner)

	_, err := mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))
	if err != nil {
		t.Fatalf("spawn error: %v", err)
	}

	var result *AgentResult
	var runErr error
	done := make(chan struct{})
	go func() {
		result, runErr = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})
		close(done)
	}()

	runner.waitStarted(t)
	if err := mgr.KillAgent(context.Background(), "agent-1"); err != nil {
		runner.release()
		t.Fatalf("KillAgent() error = %v", err)
	}

	select {
	case <-runner.cancelled:
	case <-time.After(time.Second):
		runner.release()
		<-done
		t.Fatal("runner context was not cancelled")
	}
	<-done

	if runErr != nil {
		t.Fatalf("RunAgent() error = %v", runErr)
	}
	if result == nil || result.Status != AgentStatusKilled {
		t.Fatalf("result = %#v, want killed", result)
	}
}

// ---------------------------------------------------------------------------
// CollectResults Tests
// ---------------------------------------------------------------------------

func TestCollectResults_Empty(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	results := mgr.CollectResults("mission-1")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCollectResults_OnlyTerminal(t *testing.T) {
	runner := &testRunner{output: "done"}
	mgr := newTestManager(runner)

	// Spawn and complete one agent
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "agent-1", Kind: AgentKindWorker, MissionID: "mission-1",
		SessionID: "s1", HostID: "host-1", Task: "task1",
	})
	_, _ = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})

	// Spawn another but leave idle
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "agent-2", Kind: AgentKindWorker, MissionID: "mission-1",
		SessionID: "s1", HostID: "host-2", Task: "task2",
	})

	// Spawn and kill a third
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "agent-3", Kind: AgentKindWorker, MissionID: "mission-1",
		SessionID: "s1", HostID: "host-3", Task: "task3",
	})
	_ = mgr.KillAgent(context.Background(), "agent-3")

	results := mgr.CollectResults("mission-1")
	// Should include agent-1 (completed) and agent-3 (killed), not agent-2 (idle)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	resultMap := make(map[string]AgentResult)
	for _, r := range results {
		resultMap[r.AgentID] = r
	}

	if r, ok := resultMap["agent-1"]; !ok {
		t.Error("expected agent-1 in results")
	} else if r.Status != AgentStatusCompleted {
		t.Errorf("expected agent-1 completed, got %s", r.Status)
	}

	if r, ok := resultMap["agent-3"]; !ok {
		t.Error("expected agent-3 in results")
	} else if r.Status != AgentStatusKilled {
		t.Errorf("expected agent-3 killed, got %s", r.Status)
	}
}

func TestCollectResults_FiltersByMission(t *testing.T) {
	runner := &testRunner{output: "done"}
	mgr := newTestManager(runner)

	// Agent in mission-1
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "agent-1", Kind: AgentKindWorker, MissionID: "mission-1",
		SessionID: "s1", HostID: "host-1", Task: "task1",
	})
	_, _ = mgr.RunAgent(context.Background(), "agent-1", &AgentConfig{Kind: AgentKindWorker})

	// Agent in mission-2
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "agent-2", Kind: AgentKindWorker, MissionID: "mission-2",
		SessionID: "s2", HostID: "host-2", Task: "task2",
	})
	_, _ = mgr.RunAgent(context.Background(), "agent-2", &AgentConfig{Kind: AgentKindWorker})

	results := mgr.CollectResults("mission-1")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", results[0].AgentID)
	}
}

// ---------------------------------------------------------------------------
// GetInstance / ListInstances / RunningCount Tests
// ---------------------------------------------------------------------------

func TestGetInstance(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	_, _ = mgr.Spawn(context.Background(), validSpawnRequest("agent-1"))

	inst := mgr.GetInstance("agent-1")
	if inst == nil {
		t.Fatal("expected non-nil instance")
	}
	if inst.ID != "agent-1" {
		t.Errorf("expected ID agent-1, got %s", inst.ID)
	}

	// Non-existent
	inst = mgr.GetInstance("nonexistent")
	if inst != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

func TestListInstances(t *testing.T) {
	mgr := newTestManager(&testRunner{})

	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "a1", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1", HostID: "h1",
	})
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "a2", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1", HostID: "h2",
	})
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "a3", Kind: AgentKindWorker, MissionID: "m2", SessionID: "s2", HostID: "h3",
	})

	list := mgr.ListInstances("m1")
	if len(list) != 2 {
		t.Fatalf("expected 2 instances for m1, got %d", len(list))
	}
}

func TestRunningCount(t *testing.T) {
	runner := &testRunner{delay: 200 * time.Millisecond, output: "ok"}
	mgr := newTestManager(runner)

	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "a1", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1", HostID: "h1",
	})
	_, _ = mgr.Spawn(context.Background(), SpawnRequest{
		ID: "a2", Kind: AgentKindWorker, MissionID: "m1", SessionID: "s1", HostID: "h2",
	})

	// Start both running
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func() {
		mgr.RunAgent(context.Background(), "a1", &AgentConfig{Kind: AgentKindWorker})
		close(done1)
	}()
	go func() {
		mgr.RunAgent(context.Background(), "a2", &AgentConfig{Kind: AgentKindWorker})
		close(done2)
	}()

	// Give them time to start
	time.Sleep(50 * time.Millisecond)

	count := mgr.RunningCount("m1")
	if count != 2 {
		t.Errorf("expected 2 running, got %d", count)
	}

	<-done1
	<-done2

	count = mgr.RunningCount("m1")
	if count != 0 {
		t.Errorf("expected 0 running after completion, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Concurrency Safety Test
// ---------------------------------------------------------------------------

func TestConcurrentSpawnAndKill(t *testing.T) {
	mgr := newTestManager(&testRunner{output: "ok"})

	var wg sync.WaitGroup
	// Spawn 50 agents concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("agent-%d", idx)
			mgr.Spawn(context.Background(), SpawnRequest{
				ID: id, Kind: AgentKindWorker, MissionID: "m1",
				SessionID: "s1", HostID: "h1", Task: "task",
			})
		}(i)
	}
	wg.Wait()

	// Kill half concurrently
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("agent-%d", idx)
			mgr.KillAgent(context.Background(), id)
		}(i)
	}
	wg.Wait()

	// Verify state consistency
	killed := 0
	idle := 0
	for i := 0; i < 50; i++ {
		inst := mgr.GetInstance(fmt.Sprintf("agent-%d", i))
		if inst == nil {
			t.Errorf("agent-%d not found", i)
			continue
		}
		switch inst.Status {
		case AgentStatusKilled:
			killed++
		case AgentStatusIdle:
			idle++
		default:
			t.Errorf("unexpected status %s for agent-%d", inst.Status, i)
		}
	}
	if killed != 25 {
		t.Errorf("expected 25 killed, got %d", killed)
	}
	if idle != 25 {
		t.Errorf("expected 25 idle, got %d", idle)
	}
}

type cancelAwareRunner struct {
	started     chan struct{}
	cancelled   chan struct{}
	releaseCh   chan struct{}
	startOnce   sync.Once
	cancelOnce  sync.Once
	releaseOnce sync.Once
}

func newCancelAwareRunner() *cancelAwareRunner {
	return &cancelAwareRunner{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (r *cancelAwareRunner) Run(ctx context.Context, _ agentruntime.Config) (string, error) {
	r.startOnce.Do(func() { close(r.started) })
	select {
	case <-ctx.Done():
		r.cancelOnce.Do(func() { close(r.cancelled) })
		return "", ctx.Err()
	case <-r.releaseCh:
		return "released", nil
	}
}

func (r *cancelAwareRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner start")
	}
}

func (r *cancelAwareRunner) release() {
	r.releaseOnce.Do(func() { close(r.releaseCh) })
}
