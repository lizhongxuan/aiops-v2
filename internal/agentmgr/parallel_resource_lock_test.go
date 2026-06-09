package agentmgr

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelAgentResourceLockRejectsConflictingWriteWorkers(t *testing.T) {
	budget, _ := NewAgentBudgetController(2)
	runner := newParallelMockRunner(80 * time.Millisecond)
	mgr := NewAgentManager(nil, runner, nil)
	pa, _ := NewParallelAgent(mgr, budget)
	pa = pa.WithResourceLockManager(NewResourceLockManager())

	lock := ResourceLockKey{
		ResourceType:  "file",
		ResourceID:    "config://service-a",
		OperationKind: "write",
	}
	workers := []ParallelWorkerRequest{
		{AgentID: "w1", HostID: "host-a", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-a"}, ResourceLocks: []ResourceLockKey{lock}},
		{AgentID: "w2", HostID: "host-b", Config: &AgentConfig{Kind: AgentKindWorker, HostID: "host-b"}, ResourceLocks: []ResourceLockKey{lock}},
	}

	result, err := pa.Execute(context.Background(), "mission-lock", workers)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Succeeded) != 1 || len(result.Failed) != 1 {
		t.Fatalf("succeeded=%v failed=%v, want one success and one lock failure", result.Succeeded, result.Failed)
	}
	if len(runner.callLog) != 1 {
		t.Fatalf("runner call log = %v, want exactly one worker to execute", runner.callLog)
	}
	failed := result.Results[result.Failed[0]]
	if failed == nil || failed.Status != AgentStatusFailed || !strings.Contains(failed.Error, "resource_lock_conflict") {
		t.Fatalf("failed result = %#v, want resource_lock_conflict", failed)
	}
	if len(result.ResourceLocks[result.Failed[0]]) != 1 || result.ResourceLocks[result.Failed[0]][0].Acquired {
		t.Fatalf("failed lock trace = %#v, want denied resource lock result", result.ResourceLocks[result.Failed[0]])
	}
	if atomic.LoadInt32(&runner.maxConc) != 1 {
		t.Fatalf("max runner concurrency = %d, want 1 because the conflicting worker never ran", runner.maxConc)
	}
}
