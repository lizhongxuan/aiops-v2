package hostops

import (
	"context"
	"testing"
)

func TestMissionStoreCreatesMissionAndChildAgents(t *testing.T) {
	store := NewInMemoryMissionStore()
	mission := HostOperationMission{
		ID:           "mission-1",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		Status:       HostMissionStatusPlanning,
		PlanRequired: true,
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	child := HostChildAgent{
		ID:          "agent-1",
		MissionID:   "mission-1",
		HostID:      "host-a",
		HostAddress: "1.1.1.1",
		Status:      HostChildAgentStatusPlanned,
	}
	if err := store.SaveChildAgent(context.Background(), child); err != nil {
		t.Fatalf("SaveChildAgent() error = %v", err)
	}
	children, err := store.ListChildAgents(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("ListChildAgents() error = %v", err)
	}
	if len(children) != 1 || children[0].HostID != "host-a" {
		t.Fatalf("children = %#v, want host-a", children)
	}
}

func TestMissionStoreCopiesSavedValues(t *testing.T) {
	store := NewInMemoryMissionStore()
	mission := HostOperationMission{
		ID:            "mission-1",
		ThreadID:      "thread-1",
		ChildAgentIDs: []string{"agent-1"},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	mission.ChildAgentIDs[0] = "mutated"
	stored, err := store.GetMission(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("GetMission() error = %v", err)
	}
	if stored.ChildAgentIDs[0] != "agent-1" {
		t.Fatalf("stored.ChildAgentIDs = %#v, want copy", stored.ChildAgentIDs)
	}
}
