package agentmgr

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/agentruntime"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/opssemantic"
)

func TestKernelAdapterSpawnHostChildRegistersBoundWorker(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	manager := NewAgentManager(factory, &testRunner{}, nil)
	adapter := NewKernelAdapter(manager, factory)

	child, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID:         "child-1",
		MissionID:            "mission-1",
		ParentAgentID:        "manager-1",
		SessionID:            "session-child-1",
		HostID:               "host-a",
		Task:                 "inspect assigned host readiness",
		PlanStepID:           "step-1",
		RiskLevel:            opssemantic.RiskReadOnly,
		EvidenceRequirements: []string{"command_result"},
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	if child.ID != "child-1" || child.HostID != "host-a" || child.MissionID != "mission-1" {
		t.Fatalf("child = %+v, want registered host-a child", child)
	}
	inst := manager.GetInstance("child-1")
	if inst == nil {
		t.Fatal("child instance was not registered")
	}
	if inst.Kind != AgentKindWorker || inst.HostID != "host-a" || inst.ParentID != "manager-1" {
		t.Fatalf("instance = %+v, want host-bound worker child", inst)
	}
	if inst.AssignmentSummary == "" || inst.EvidenceRequirement.MinEvidenceRefs != 1 || !containsString(inst.EvidenceRequirement.RequiredKinds, "command_result") {
		t.Fatalf("instance assignment = %q / %#v, want HostSubTask assignment metadata", inst.AssignmentSummary, inst.EvidenceRequirement)
	}
	if len(child.PlanStepIDs) != 1 || child.PlanStepIDs[0] != "step-1" {
		t.Fatalf("child plan steps = %#v, want step-1", child.PlanStepIDs)
	}
}

func TestKernelAdapterSpawnHostChildRunsBoundWorker(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	runner := newRecordingAgentRunner("pg installed")
	manager := NewAgentManager(factory, runner, nil)
	adapter := NewKernelAdapter(manager, factory)

	child, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID:  "child-run-1",
		MissionID:     "mission-1",
		ParentAgentID: "manager-1",
		SessionID:     "session-child-run-1",
		HostID:        "host-a",
		Task:          "install pg",
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	if child.Status != hostops.HostChildAgentStatusRunning {
		t.Fatalf("child.Status = %q, want running", child.Status)
	}
	config := runner.waitForConfig(t)
	if config.RuntimeHostID() != "host-a" {
		t.Fatalf("runner host = %q, want host-a", config.RuntimeHostID())
	}
	if config.RuntimeInput() != "install pg" {
		t.Fatalf("runner input = %q, want install pg", config.RuntimeInput())
	}
	if config.RuntimeSessionID() != "session-child-run-1" {
		t.Fatalf("runner session = %q, want session-child-run-1", config.RuntimeSessionID())
	}
}

func TestKernelAdapterSendMessageRunsChildFollowupTurn(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	runner := newRecordingAgentRunner("ok")
	manager := NewAgentManager(factory, runner, nil)
	adapter := NewKernelAdapter(manager, factory)

	_, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID:  "child-followup-1",
		MissionID:     "mission-1",
		ParentAgentID: "manager-1",
		SessionID:     "session-child-followup-1",
		HostID:        "host-a",
		Task:          "install pg",
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	_ = runner.waitForConfig(t)

	child, err := adapter.SendMessage(context.Background(), "child-followup-1", "check replication")
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if child.Status != hostops.HostChildAgentStatusRunning {
		t.Fatalf("child.Status = %q, want running", child.Status)
	}
	config := runner.waitForConfig(t)
	if config.RuntimeHostID() != "host-a" {
		t.Fatalf("follow-up host = %q, want host-a", config.RuntimeHostID())
	}
	if config.RuntimeInput() != "check replication" {
		t.Fatalf("follow-up input = %q, want check replication", config.RuntimeInput())
	}
	if config.RuntimeSessionID() != "session-child-followup-1" {
		t.Fatalf("follow-up session = %q, want session-child-followup-1", config.RuntimeSessionID())
	}
}

func TestKernelAdapterSpawnHostChildDoesNotBlockForRunner(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	runner := newBlockingAgentRunner()
	manager := NewAgentManager(factory, runner, nil)
	adapter := NewKernelAdapter(manager, factory)

	started := time.Now()
	child, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID: "child-slow-1",
		MissionID:    "mission-1",
		SessionID:    "session-child-slow-1",
		HostID:       "host-a",
		Task:         "install pg slowly",
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("SpawnHostChild() blocked for %s, want async return", elapsed)
	}
	if child.Status != hostops.HostChildAgentStatusRunning {
		t.Fatalf("child.Status = %q, want running", child.Status)
	}
	runner.waitStarted(t)
	runner.release("done")
}

func TestKernelAdapterRecordsHostChildResultToSinks(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	runner := newRecordingAgentRunner("pg installed")
	manager := NewAgentManager(factory, runner, nil)
	store := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	adapter := NewKernelAdapter(manager, factory).WithHostOpsSinks(store, transcripts)

	_, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID: "child-sink-1",
		MissionID:    "mission-1",
		SessionID:    "session-child-sink-1",
		HostID:       "host-a",
		Task:         "install pg",
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	_ = runner.waitForConfig(t)

	waitForCondition(t, func() bool {
		child, err := store.GetChildAgent(context.Background(), "child-sink-1")
		return err == nil && child.Status == hostops.HostChildAgentStatusCompleted && child.LastOutputPreview == "pg installed"
	})
	items, err := transcripts.List(context.Background(), "child-sink-1")
	if err != nil {
		t.Fatalf("List transcript error = %v", err)
	}
	if len(items) != 1 || items[0].Type != hostops.TranscriptItemAssistantMessage || items[0].Content != "pg installed" {
		t.Fatalf("transcript items = %+v, want assistant pg installed", items)
	}
}

type recordingAgentRunner struct {
	output  string
	configs chan agentruntime.Config
}

func newRecordingAgentRunner(output string) *recordingAgentRunner {
	return &recordingAgentRunner{output: output, configs: make(chan agentruntime.Config, 8)}
}

func (r *recordingAgentRunner) Run(_ context.Context, config agentruntime.Config) (string, error) {
	r.configs <- config
	return r.output, nil
}

func (r *recordingAgentRunner) waitForConfig(t *testing.T) agentruntime.Config {
	t.Helper()
	select {
	case config := <-r.configs:
		return config
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner config")
		return nil
	}
}

type blockingAgentRunner struct {
	started  chan struct{}
	releaseC chan string
}

func newBlockingAgentRunner() *blockingAgentRunner {
	return &blockingAgentRunner{
		started:  make(chan struct{}),
		releaseC: make(chan string),
	}
}

func (r *blockingAgentRunner) Run(_ context.Context, _ agentruntime.Config) (string, error) {
	close(r.started)
	return <-r.releaseC, nil
}

func (r *blockingAgentRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking runner start")
	}
}

func (r *blockingAgentRunner) release(output string) {
	r.releaseC <- output
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
