package appui

import (
	"context"
	"strings"

	"aiops-v2/internal/hostops"
)

type defaultHostOpsService struct {
	missions     hostops.MissionStore
	transcripts  hostops.TranscriptStore
	orchestrator *hostops.Orchestrator
}

func NewHostOpsService(missions hostops.MissionStore, transcripts hostops.TranscriptStore, orchestrator *hostops.Orchestrator) HostOpsService {
	return &defaultHostOpsService{
		missions:     missions,
		transcripts:  transcripts,
		orchestrator: orchestrator,
	}
}

func (s *defaultHostOpsService) AcceptPlan(ctx context.Context, missionID, planID string) (HostOperationView, error) {
	missionID = strings.TrimSpace(missionID)
	if err := s.orchestrator.AcceptPlan(ctx, missionID, strings.TrimSpace(planID)); err != nil {
		return HostOperationView{}, err
	}
	return s.operationView(ctx, missionID)
}

func (s *defaultHostOpsService) RevisePlan(ctx context.Context, missionID, instruction string) (HostOperationView, error) {
	missionID = strings.TrimSpace(missionID)
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return HostOperationView{}, err
	}
	mission.PlanAccepted = false
	mission.Status = hostops.HostMissionStatusWaitingPlanAcceptance
	if err := s.missions.SaveMission(ctx, mission); err != nil {
		return HostOperationView{}, err
	}
	return HostOperationView{ID: mission.ID, Status: string(mission.Status)}, nil
}

func (s *defaultHostOpsService) SendChildMessage(ctx context.Context, childAgentID, content string) (HostChildAgentView, error) {
	child, err := s.orchestrator.SendMessage(ctx, childAgentID, content)
	if err != nil {
		return HostChildAgentView{}, err
	}
	return childAgentView(child), nil
}

func (s *defaultHostOpsService) StopChildAgent(ctx context.Context, childAgentID string) (HostChildAgentView, error) {
	child, err := s.orchestrator.Stop(ctx, childAgentID)
	if err != nil {
		return HostChildAgentView{}, err
	}
	return childAgentView(child), nil
}

func (s *defaultHostOpsService) ChildTranscript(ctx context.Context, childAgentID string) (HostChildTranscriptView, error) {
	childAgentID = strings.TrimSpace(childAgentID)
	items, err := s.transcripts.List(ctx, childAgentID)
	if err != nil {
		return HostChildTranscriptView{}, err
	}
	return HostChildTranscriptView{ChildAgentID: childAgentID, Items: items}, nil
}

func (s *defaultHostOpsService) operationView(ctx context.Context, missionID string) (HostOperationView, error) {
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return HostOperationView{}, err
	}
	return HostOperationView{ID: mission.ID, Status: string(mission.Status)}, nil
}

func childAgentView(child hostops.HostChildAgent) HostChildAgentView {
	return HostChildAgentView{ID: child.ID, Status: string(child.Status)}
}
