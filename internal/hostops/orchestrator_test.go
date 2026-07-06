package hostops

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/opssemantic"
	"aiops-v2/internal/resourcebinding"
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
		{HostID: "host-a", Role: "host_child", Task: "prepare host a"},
		{HostID: "host-b", Role: "host_child", Task: "prepare host b"},
		{HostID: "host-c", Role: "host_child", Task: "verify host c"},
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

func TestOrchestratorSpawnsOneChildAgentPerMissionHost(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID:           "mission-multi-host",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		Status:       HostMissionStatusSpawningChildren,
		PlanRequired: true,
		PlanAccepted: true,
		Mentions: []HostMention{
			{HostID: "host-a", DisplayName: "主机A", Resolved: true},
			{HostID: "host-b", DisplayName: "主机B", Resolved: true},
		},
		Plan: HostOperationPlan{
			ID:     "plan-1",
			Status: PlanStatusAccepted,
			Steps:  []PlanStep{{ID: "step-1", HostIDs: []string{"host-a", "host-b"}, ActionType: "read", RiskLevel: "low"}},
		},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatal(err)
	}
	children, err := orchestrator.SpawnChildren(ctx, mission.ID, []ChildAgentAssignment{
		{HostID: "host-a", HostDisplayName: "主机A", Task: "检查主机A", PlanStepID: "step-1"},
		{HostID: "host-b", HostDisplayName: "主机B", Task: "检查主机B", PlanStepID: "step-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("children = %d, want 2: %#v", len(children), children)
	}
	if children[0].HostID == children[1].HostID || children[0].ID == children[1].ID {
		t.Fatalf("child agents must be unique per host: %#v", children)
	}
}

func TestOrchestratorPropagatesRoleBindingToSpawnRequestAndChild(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID:           "mission-role",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		Status:       HostMissionStatusSpawningChildren,
		PlanRequired: true,
		PlanAccepted: true,
		Mentions: []HostMention{
			{HostID: "host-a", DisplayName: "主机A", Resolved: true},
		},
		Plan: HostOperationPlan{
			ID:     "plan-role",
			Status: PlanStatusAccepted,
			Steps:  []PlanStep{{ID: "step-primary", HostIDs: []string{"host-a"}, ActionType: "read", RiskLevel: "low"}},
		},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatal(err)
	}
	children, err := orchestrator.SpawnChildren(ctx, mission.ID, []ChildAgentAssignment{{
		HostID:          "host-a",
		Task:            "检查主节点状态",
		PlanStepID:      "step-primary",
		BoundRole:       "pg_primary",
		RoleBindingHash: "role-hash-a",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1", len(children))
	}
	if spawner.lastReq.BoundRole != "pg_primary" || spawner.lastReq.RoleBindingHash != "role-hash-a" {
		t.Fatalf("spawn request = %+v, want bound role/hash", spawner.lastReq)
	}
	if children[0].BoundRole != "pg_primary" || children[0].RoleBindingHash != "role-hash-a" {
		t.Fatalf("child = %+v, want bound role/hash", children[0])
	}
}

func TestOrchestratorResolvesAssignmentRoleToUniqueMissionHost(t *testing.T) {
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	primary := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
		Role:        resourcebinding.RolePGPrimary,
	})
	standby := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-b"},
		Role:        resourcebinding.RolePGStandby,
	})
	mission := HostOperationMission{
		ID:                           "mission-role-resolve",
		ThreadID:                     "thread-1",
		UserTurnID:                   "turn-1",
		PlanRequired:                 true,
		PlanAccepted:                 true,
		RoleBindings:                 []resourcebinding.ResourceRoleBinding{primary, standby},
		RoleConflicts:                nil,
		RoleBindingAssignmentEnabled: true,
		ManagerAgentID:               "manager-a",
		Mentions: []HostMention{
			{HostID: "host-a", Address: "1.1.1.1", DisplayName: "hostA", Resolved: true},
			{HostID: "host-b", Address: "1.1.1.2", DisplayName: "hostB", Resolved: true},
		},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}

	children, err := orchestrator.SpawnChildren(context.Background(), mission.ID, []ChildAgentAssignment{{
		BoundRole: "主节点",
		Task:      "检查主节点复制状态",
	}})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 1 || children[0].HostID != "host-a" || children[0].BoundRole != resourcebinding.RolePGPrimary {
		t.Fatalf("children = %#v, want unique primary host-a", children)
	}
	if spawner.lastReq.HostID != "host-a" || spawner.lastReq.RoleBindingHash != primary.TraceHash {
		t.Fatalf("spawn request = %#v, want resolved host-a role hash", spawner.lastReq)
	}
}

func TestOrchestratorRoleBindingAssignmentFlagOffKeepsHostIDRequired(t *testing.T) {
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	orchestrator := NewOrchestrator(store, transcripts, &fakeChildSpawner{})
	mission := HostOperationMission{
		ID:           "mission-role-flag-off",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		PlanRequired: true,
		PlanAccepted: true,
		RoleBindings: []resourcebinding.ResourceRoleBinding{
			resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
				ResourceRef: resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-a"},
				Role:        resourcebinding.RolePGPrimary,
			}),
		},
		Mentions: []HostMention{{HostID: "host-a", Resolved: true}},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}

	_, err := orchestrator.SpawnChildren(context.Background(), mission.ID, []ChildAgentAssignment{{
		BoundRole: "主节点",
		Task:      "检查主节点复制状态",
	}})
	if err == nil || !strings.Contains(err.Error(), "hostId is required") {
		t.Fatalf("SpawnChildren() error = %v, want hostId required with flag off", err)
	}
}

func TestOrchestratorDeduplicatesRepeatedHostAssignments(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, transcripts, spawner)
	mission := HostOperationMission{
		ID:           "mission-dedupe",
		ThreadID:     "thread-1",
		UserTurnID:   "turn-1",
		Status:       HostMissionStatusSpawningChildren,
		PlanRequired: true,
		PlanAccepted: true,
		Mentions:     []HostMention{{HostID: "host-a", DisplayName: "主机A", Resolved: true}},
		Plan: HostOperationPlan{
			ID:     "plan-1",
			Status: PlanStatusAccepted,
			Steps:  []PlanStep{{ID: "step-1", HostIDs: []string{"host-a"}, ActionType: "read", RiskLevel: "low"}},
		},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatal(err)
	}
	children, err := orchestrator.SpawnChildren(ctx, mission.ID, []ChildAgentAssignment{
		{HostID: "host-a", Task: "检查主机A", PlanStepID: "step-1"},
		{HostID: "host-a", Task: "再次检查主机A", PlanStepID: "step-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1 after dedupe: %#v", len(children), children)
	}
	if spawner.spawnCount != 1 {
		t.Fatalf("spawnCount = %d, want 1", spawner.spawnCount)
	}
}

func TestOrchestratorRejectsSpawnBeforePlanAccepted(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	_ = store.SaveMission(context.Background(), HostOperationMission{ID: "mission-1", PlanRequired: true, PlanAccepted: false})
	_, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "run host preparation"}})
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
	_, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-b", Task: "run host preparation"}})
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

	first, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect host state"}})
	if err != nil {
		t.Fatalf("first SpawnChildren() error = %v", err)
	}
	second, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect host state again"}})
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

func TestOrchestratorBindsChildToPlanStepSubTaskFields(t *testing.T) {
	store := NewInMemoryMissionStore()
	spawner := &fakeChildSpawner{}
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), spawner)
	_ = store.SaveMission(context.Background(), HostOperationMission{
		ID:           "mission-1",
		PlanAccepted: true,
		Mentions:     []HostMention{{HostID: "host-a", Resolved: true}},
		Plan: HostOperationPlan{ID: "plan-1", Steps: []PlanStep{{
			ID:               "step-1",
			Index:            1,
			Title:            "Run assigned host operation",
			Status:           PlanStepStatusPending,
			HostIDs:          []string{"host-a"},
			RiskLevel:        opssemantic.RiskMediumWrite,
			EvidenceRequired: []string{"command_result"},
		}}},
	})

	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "run assigned host operation"}})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 1 || len(children[0].PlanStepIDs) != 1 || children[0].PlanStepIDs[0] != "step-1" {
		t.Fatalf("children = %#v, want child bound to step-1", children)
	}
	if spawner.lastReq.PlanStepID != "step-1" || spawner.lastReq.RiskLevel != opssemantic.RiskMediumWrite || len(spawner.lastReq.EvidenceRequirements) != 1 {
		t.Fatalf("spawn request = %#v, want subtask step/risk/evidence", spawner.lastReq)
	}
	mission, err := store.GetMission(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("GetMission() error = %v", err)
	}
	if len(mission.Plan.Steps) != 1 || !stringSliceContains(mission.Plan.Steps[0].ChildAgentIDs, children[0].ID) || mission.Plan.Steps[0].Status != PlanStepStatusRunning {
		t.Fatalf("mission plan = %#v, want child agent attached to running step", mission.Plan)
	}
}

func TestOrchestratorDoesNotOverwriteCompletedChildWithSpawnReturn(t *testing.T) {
	store := NewInMemoryMissionStore()
	spawner := &persistingChildSpawner{store: store}
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), spawner)
	_ = store.SaveMission(context.Background(), HostOperationMission{
		ID:           "mission-1",
		PlanAccepted: true,
		Mentions:     []HostMention{{HostID: "host-a", Resolved: true}},
	})

	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect host state"}})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}
	if children[0].Status != HostChildAgentStatusCompleted {
		t.Fatalf("children[0].Status = %q, want completed", children[0].Status)
	}
	stored, err := store.GetChildAgent(context.Background(), children[0].ID)
	if err != nil {
		t.Fatalf("GetChildAgent() error = %v", err)
	}
	if stored.Status != HostChildAgentStatusCompleted || stored.LastOutputPreview != "done" {
		t.Fatalf("stored child = %+v, want completed result preserved", stored)
	}
}

type fakeChildSpawner struct {
	spawnCount int
	lastReq    SpawnHostChildRequest
}

func (s *fakeChildSpawner) SpawnHostChild(_ context.Context, req SpawnHostChildRequest) (HostChildAgent, error) {
	s.spawnCount++
	s.lastReq = req
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
		PlanStepIDs:      []string{req.PlanStepID},
	}, nil
}

func (s *fakeChildSpawner) SendMessage(_ context.Context, childAgentID, content string) (HostChildAgent, error) {
	return HostChildAgent{ID: childAgentID, Status: HostChildAgentStatusRunning, LastInputPreview: content}, nil
}

func (s *fakeChildSpawner) Stop(_ context.Context, childAgentID string) (HostChildAgent, error) {
	return HostChildAgent{ID: childAgentID, Status: HostChildAgentStatusCancelled}, nil
}

type persistingChildSpawner struct {
	store *InMemoryMissionStore
}

func (s *persistingChildSpawner) SpawnHostChild(ctx context.Context, req SpawnHostChildRequest) (HostChildAgent, error) {
	completedAt := time.Now().UTC()
	_ = s.store.SaveChildAgent(ctx, HostChildAgent{
		ID:                req.ChildAgentID,
		MissionID:         req.MissionID,
		ParentAgentID:     req.ParentAgentID,
		SessionID:         req.SessionID,
		HostID:            req.HostID,
		Task:              req.Task,
		Status:            HostChildAgentStatusCompleted,
		LastInputPreview:  req.Task,
		LastOutputPreview: "done",
		CompletedAt:       &completedAt,
	})
	return HostChildAgent{
		ID:               req.ChildAgentID,
		MissionID:        req.MissionID,
		ParentAgentID:    req.ParentAgentID,
		SessionID:        req.SessionID,
		HostID:           req.HostID,
		Task:             req.Task,
		Status:           HostChildAgentStatusRunning,
		LastInputPreview: req.Task,
	}, nil
}

func (s *persistingChildSpawner) SendMessage(context.Context, string, string) (HostChildAgent, error) {
	return HostChildAgent{}, nil
}

func (s *persistingChildSpawner) Stop(context.Context, string) (HostChildAgent, error) {
	return HostChildAgent{}, nil
}
