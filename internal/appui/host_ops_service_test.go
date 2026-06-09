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

	mission := hostops.HostOperationMission{
		ID:           "mission-1",
		PlanRequired: true,
		PlanAccepted: false,
		Mentions: []hostops.HostMention{
			{HostID: "host-a", Address: "10.0.0.1", Resolved: true},
		},
	}
	if err := missions.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}

	view, err := service.AcceptPlan(context.Background(), "mission-1", "plan-1")
	if err != nil {
		t.Fatalf("AcceptPlan() error = %v", err)
	}
	if view.ID != "mission-1" || view.Status != string(hostops.HostMissionStatusSpawningChildren) {
		t.Fatalf("AcceptPlan() view = %+v", view)
	}

	children, err := orchestrator.SpawnChildren(context.Background(), "mission-1", []hostops.ChildAgentAssignment{
		{HostID: "host-a", HostAddress: "10.0.0.1", Task: "安装 PostgreSQL"},
	})
	if err != nil {
		t.Fatalf("SpawnChildren() error = %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("len(children) = %d, want 1", len(children))
	}

	childView, err := service.SendChildMessage(context.Background(), children[0].ID, "检查 pg 是否安装")
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

type hostOpsServiceTestSpawner struct{}

func (s *hostOpsServiceTestSpawner) SpawnHostChild(_ context.Context, req hostops.SpawnHostChildRequest) (hostops.HostChildAgent, error) {
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
