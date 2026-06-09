package agentmgr

import "testing"

func TestParallelSpawnTraceRequiresSameTurnWorkers(t *testing.T) {
	group := BuildAgentParallelTraceGroup(AgentParallelTraceInput{
		MissionID:         "synthetic-mission",
		ParallelRequested: true,
		RequestedCount:    3,
		SpawnedInTurn:     []string{"synthetic-worker-1"},
	})
	if group.RequestedCount != 3 {
		t.Fatalf("requested count = %d, want 3", group.RequestedCount)
	}
	if !hasSerialReason(group.SerialReasons, "missing_parallel_spawn_batch") {
		t.Fatalf("serial reasons = %#v, want missing_parallel_spawn_batch", group.SerialReasons)
	}
}

func TestAgentBudgetTraceRecordsQueuedWorkers(t *testing.T) {
	group := BuildAgentParallelTraceGroup(AgentParallelTraceInput{
		MissionID:     "synthetic-mission",
		SpawnedInTurn: []string{"synthetic-worker-1", "synthetic-worker-2"},
		Queued:        []string{"synthetic-worker-3"},
	})
	if group.RequestedCount != 3 {
		t.Fatalf("requested count = %d, want spawned + queued", group.RequestedCount)
	}
	if !hasSerialReason(group.SerialReasons, "budget_exceeded") {
		t.Fatalf("serial reasons = %#v, want budget_exceeded", group.SerialReasons)
	}
}

func hasSerialReason(values []AgentSerialReasonTrace, want string) bool {
	for _, value := range values {
		if value.Reason == want {
			return true
		}
	}
	return false
}
