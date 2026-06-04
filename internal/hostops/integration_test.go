package hostops

import (
	"context"
	"errors"
	"testing"
)

func TestMultiHostMissionCreatesThreeChildrenAndBlocksBeforePlanAccepted(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	missionID := createResolvedThreeHostMission(t, store, false)

	_, err := orchestrator.SpawnChildren(context.Background(), missionID, threeHostAssignments())
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
	if err := orchestrator.AcceptPlan(context.Background(), missionID, "plan-1"); err != nil {
		t.Fatalf("AcceptPlan() error = %v", err)
	}
	children, err := orchestrator.SpawnChildren(context.Background(), missionID, threeHostAssignments())
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("len(children) = %d, want 3", len(children))
	}
	seenHosts := map[string]bool{}
	for _, child := range children {
		seenHosts[child.HostID] = true
	}
	for _, hostID := range []string{"host-a", "host-b", "host-c"} {
		if !seenHosts[hostID] {
			t.Fatalf("children hosts = %#v, want %s", seenHosts, hostID)
		}
	}
}

func createResolvedThreeHostMission(t *testing.T, store MissionStore, accepted bool) string {
	t.Helper()
	mission := HostOperationMission{
		ID:           "mission-three-hosts",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		Status:       HostMissionStatusWaitingPlanAcceptance,
		PlanRequired: true,
		PlanAccepted: accepted,
		Mentions: []HostMention{
			{Raw: "@1.1.1.1", HostID: "host-a", Address: "1.1.1.1", DisplayName: "@1.1.1.1", Source: HostMentionSourceInventory, Resolved: true},
			{Raw: "@1.1.1.2", HostID: "host-b", Address: "1.1.1.2", DisplayName: "@1.1.1.2", Source: HostMentionSourceInventory, Resolved: true},
			{Raw: "@1.1.1.3", HostID: "host-c", Address: "1.1.1.3", DisplayName: "@1.1.1.3", Source: HostMentionSourceInventory, Resolved: true},
		},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	return mission.ID
}

func threeHostAssignments() []ChildAgentAssignment {
	return []ChildAgentAssignment{
		{HostID: "host-a", HostAddress: "1.1.1.1", HostDisplayName: "@1.1.1.1", Role: "pg primary", Task: "准备 PostgreSQL 主库"},
		{HostID: "host-b", HostAddress: "1.1.1.2", HostDisplayName: "@1.1.1.2", Role: "pg standby", Task: "准备 PostgreSQL 从库"},
		{HostID: "host-c", HostAddress: "1.1.1.3", HostDisplayName: "@1.1.1.3", Role: "pg_mon", Task: "准备 PostgreSQL 监控节点"},
	}
}
