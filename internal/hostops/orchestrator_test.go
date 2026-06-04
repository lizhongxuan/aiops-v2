package hostops

import (
	"context"
	"errors"
	"testing"
)

func TestOrchestratorSpawnsOneChildPerMentionedHost(t *testing.T) {
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID: "mission-1", ThreadID: "thread-1", UserTurnID: "turn-1",
		PlanRequired: true, PlanAccepted: true,
		Mentions: []HostMention{
			{HostID: "host-a", Address: "1.1.1.1", Resolved: true},
			{HostID: "host-b", Address: "1.1.1.2", Resolved: true},
			{HostID: "host-c", Address: "1.1.1.3", Resolved: true},
		},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{
		{HostID: "host-a", Role: "pg primary candidate", Task: "prepare pg primary"},
		{HostID: "host-b", Role: "pg standby candidate", Task: "prepare pg standby"},
		{HostID: "host-c", Role: "pg_mon", Task: "prepare monitor"},
	})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("len(children) = %d, want 3", len(children))
	}
	if spawner.spawnCount != 3 {
		t.Fatalf("spawnCount = %d, want 3", spawner.spawnCount)
	}
}

func TestOrchestratorRejectsSpawnBeforePlanAccepted(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	_ = store.SaveMission(context.Background(), HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false})
	_, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "install pg"}})
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("err = %v, want ErrPlanNotAccepted", err)
	}
}

func TestOrchestratorRejectsHostOutsideMissionMentions(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	_ = store.SaveMission(context.Background(), HostOperationMission{
		ID:           "mission-1",
		PlanAccepted: true,
		Mentions:     []HostMention{{HostID: "host-a", Address: "1.1.1.1", Resolved: true}},
	})
	_, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-b", Task: "install pg"}})
	if !errors.Is(err, ErrHostOutsideMission) {
		t.Fatalf("err = %v, want ErrHostOutsideMission", err)
	}
}

func TestOrchestratorDuplicateHostReturnsExistingChild(t *testing.T) {
	store := NewInMemoryMissionStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), spawner)
	_ = store.SaveMission(context.Background(), HostOperationMission{
		ID:           "mission-1",
		PlanAccepted: true,
		Mentions:     []HostMention{{HostID: "host-a", Address: "1.1.1.1", Resolved: true}},
	})

	first, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect pg"}})
	if err != nil {
		t.Fatalf("first SpawnChildren() error = %v", err)
	}
	second, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect pg again"}})
	if err != nil {
		t.Fatalf("second SpawnChildren() error = %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("children = %+v / %+v, want same existing child", first, second)
	}
	if spawner.spawnCount != 1 {
		t.Fatalf("spawnCount = %d, want 1", spawner.spawnCount)
	}
}

type fakeChildSpawner struct {
	spawnCount int
}

func (s *fakeChildSpawner) SpawnHostChild(_ context.Context, req SpawnHostChildRequest) (HostChildAgent, error) {
	s.spawnCount++
	return HostChildAgent{
		ID:               req.ChildAgentID,
		MissionID:        req.MissionID,
		ParentAgentID:    req.ParentAgentID,
		SessionID:        req.SessionID,
		HostID:           req.HostID,
		HostAddress:      req.HostAddress,
		Role:             req.Role,
		Task:             req.Task,
		Status:           HostChildAgentStatusRunning,
		LastInputPreview: req.Task,
	}, nil
}

func (s *fakeChildSpawner) SendMessage(_ context.Context, childAgentID, content string) (HostChildAgent, error) {
	return HostChildAgent{ID: childAgentID, Status: HostChildAgentStatusRunning, LastInputPreview: content}, nil
}

func (s *fakeChildSpawner) Stop(_ context.Context, childAgentID string) (HostChildAgent, error) {
	return HostChildAgent{ID: childAgentID, Status: HostChildAgentStatusCancelled}, nil
}
