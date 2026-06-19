package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/hostops"
)

func TestServicesExposeConfiguredHostOpsService(t *testing.T) {
	service := &fakeHostOpsService{}
	services := NewServices(runtimeStub{}, nil, WithHostOpsService(service))

	if got := services.HostOpsService(); got != service {
		t.Fatalf("HostOpsService() = %T, want configured service", got)
	}
}

func TestHostOpsServiceWrapsOrchestratorAndTranscriptStore(t *testing.T) {
	missions := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	spawner := &hostOpsServiceTestSpawner{}
	orchestrator := hostops.NewOrchestrator(missions, transcripts, spawner)
	service := NewHostOpsService(missions, transcripts, orchestrator)

	created, err := service.CreateMission(context.Background(), HostMissionCreateCommand{
		ID:      "mission-1",
		Goal:    "在两台主机上执行通用运维变更并回传证据",
		HostIDs: []string{"host-a", "host-b"},
	})
	if err != nil {
		t.Fatalf("CreateMission() error = %v", err)
	}
	if created.ID != "mission-1" || !created.PlanRequired || created.Status != string(hostops.HostMissionStatusWaitingPlanAcceptance) {
		t.Fatalf("CreateMission() view = %+v", created)
	}
	if len(created.MentionedHosts) != 2 {
		t.Fatalf("len(created.MentionedHosts) = %d, want 2", len(created.MentionedHosts))
	}
	if created.Plan == nil || created.Plan.TotalCount != 2 || len(created.Plan.Steps) != 2 {
		t.Fatalf("CreateMission() plan = %+v, want two host steps", created.Plan)
	}

	mission, err := missions.GetMission(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("GetMission() error = %v", err)
	}
	mission.Mentions = []hostops.HostMention{
		{HostID: "host-a", Address: "10.0.0.1", Resolved: true},
		{HostID: "host-b", Address: "10.0.0.2", Resolved: true},
	}
	if err := missions.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	got, err := service.GetMission(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("GetMission() error = %v", err)
	}
	if got.ID != "mission-1" || len(got.MentionedHosts) != 2 {
		t.Fatalf("GetMission() view = %+v", got)
	}

	view, err := service.AcceptPlan(context.Background(), "mission-1", created.Plan.ID)
	if err != nil {
		t.Fatalf("AcceptPlan() error = %v", err)
	}
	if view.ID != "mission-1" || view.Status != string(hostops.HostMissionStatusSpawningChildren) {
		t.Fatalf("AcceptPlan() view = %+v", view)
	}

	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []hostops.ChildAgentAssignment{
		{HostID: "host-a", HostAddress: "10.0.0.1", Task: "执行主机侧通用检查并回传证据"},
	})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}

	childView, err := service.SendChildMessage(context.Background(), children[0].ID, "继续收集主机侧证据")
	if err != nil {
		t.Fatalf("SendChildMessage() error = %v", err)
	}
	if childView.ID != children[0].ID || childView.Status != string(hostops.HostChildAgentStatusWaiting) {
		t.Fatalf("SendChildMessage() view = %+v", childView)
	}

	transcript, err := service.ChildTranscript(context.Background(), children[0].ID)
	if err != nil {
		t.Fatalf("ChildTranscript() error = %v", err)
	}
	if transcript.ChildAgentID != children[0].ID {
		t.Fatalf("ChildTranscript().ChildAgentID = %q, want %q", transcript.ChildAgentID, children[0].ID)
	}
	if len(transcript.Items) < 2 {
		t.Fatalf("len(transcript.Items) = %d, want at least 2", len(transcript.Items))
	}
}

func TestHostOpsServiceCreateMissionAutoSpawnsSingleHostWhenPlanNotRequired(t *testing.T) {
	missions := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	spawner := &hostOpsServiceTestSpawner{}
	orchestrator := hostops.NewOrchestrator(missions, transcripts, spawner)
	service := NewHostOpsService(missions, transcripts, orchestrator)

	created, err := service.CreateMission(context.Background(), HostMissionCreateCommand{
		ID:      "mission-single-host",
		Goal:    "检查主机内存情况并回传证据",
		HostIDs: []string{"host-a"},
	})
	if err != nil {
		t.Fatalf("CreateMission() error = %v", err)
	}
	if created.PlanRequired {
		t.Fatalf("PlanRequired = true, want false for single host read-only task")
	}
	if created.Status != string(hostops.HostMissionStatusRunning) {
		t.Fatalf("Status = %q, want running after child agent is spawned", created.Status)
	}
	if len(created.ChildAgents) != 1 {
		t.Fatalf("len(ChildAgents) = %d, want 1: %+v", len(created.ChildAgents), created.ChildAgents)
	}
	child := created.ChildAgents[0]
	if child.HostID != "host-a" || child.Task != "检查主机内存情况并回传证据" {
		t.Fatalf("child = %+v, want host-a bound to original goal", child)
	}
	if spawner.spawnCount != 1 {
		t.Fatalf("spawnCount = %d, want 1", spawner.spawnCount)
	}
}

func TestHostOpsServiceCreateMissionAutoSpawnsExplicitReadOnlyKernelInspection(t *testing.T) {
	missions := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	spawner := &hostOpsServiceTestSpawner{}
	orchestrator := hostops.NewOrchestrator(missions, transcripts, spawner)
	service := NewHostOpsService(missions, transcripts, orchestrator)

	created, err := service.CreateMission(context.Background(), HostMissionCreateCommand{
		ID:      "mission-single-host-kernel-info",
		Goal:    "查看@120.77.239.90主机系统版本和内核信息，只读执行uname -a和hostnamectl并总结",
		HostIDs: []string{"120.77.239.90"},
	})
	if err != nil {
		t.Fatalf("CreateMission() error = %v", err)
	}
	if created.PlanRequired {
		t.Fatalf("PlanRequired = true, want false for explicit read-only single-host inspection")
	}
	if created.Status != string(hostops.HostMissionStatusRunning) {
		t.Fatalf("Status = %q, want running after child agent is spawned", created.Status)
	}
	if len(created.ChildAgents) != 1 {
		t.Fatalf("len(ChildAgents) = %d, want 1: %+v", len(created.ChildAgents), created.ChildAgents)
	}
	if spawner.spawnCount != 1 {
		t.Fatalf("spawnCount = %d, want 1", spawner.spawnCount)
	}
}

type hostOpsServiceTestSpawner struct {
	spawnCount int
}

func (s *hostOpsServiceTestSpawner) SpawnHostChild(_ context.Context, req hostops.SpawnHostChildRequest) (hostops.HostChildAgent, error) {
	s.spawnCount++
	return hostops.HostChildAgent{
		ID:          req.ChildAgentID,
		MissionID:   req.MissionID,
		SessionID:   req.SessionID,
		HostID:      req.HostID,
		HostAddress: req.HostAddress,
		Task:        req.Task,
		Status:      hostops.HostChildAgentStatusSpawning,
	}, nil
}

func (s *hostOpsServiceTestSpawner) SendMessage(_ context.Context, childAgentID, content string) (hostops.HostChildAgent, error) {
	return hostops.HostChildAgent{ID: childAgentID, Status: hostops.HostChildAgentStatusWaiting, LastInputPreview: content}, nil
}

func (s *hostOpsServiceTestSpawner) Stop(_ context.Context, childAgentID string) (hostops.HostChildAgent, error) {
	return hostops.HostChildAgent{ID: childAgentID, Status: hostops.HostChildAgentStatusCancelled}, nil
}

type fakeHostOpsService struct{}

func (s *fakeHostOpsService) CreateMission(context.Context, HostMissionCreateCommand) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *fakeHostOpsService) GetMission(context.Context, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *fakeHostOpsService) AcceptPlan(context.Context, string, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *fakeHostOpsService) RevisePlan(context.Context, string, string) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *fakeHostOpsService) SendChildMessage(context.Context, string, string) (HostChildAgentView, error) {
	return HostChildAgentView{}, nil
}

func (s *fakeHostOpsService) StopChildAgent(context.Context, string) (HostChildAgentView, error) {
	return HostChildAgentView{}, nil
}

func (s *fakeHostOpsService) ChildTranscript(context.Context, string) (HostChildTranscriptView, error) {
	return HostChildTranscriptView{}, nil
}
