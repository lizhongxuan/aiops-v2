package hostops

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrMissionNotFound    = errors.New("host operation mission not found")
	ErrChildAgentNotFound = errors.New("host child agent not found")
)

type MissionStore interface {
	SaveMission(ctx context.Context, mission HostOperationMission) error
	GetMission(ctx context.Context, missionID string) (HostOperationMission, error)
	ListThreadMissions(ctx context.Context, threadID string) ([]HostOperationMission, error)
	SaveChildAgent(ctx context.Context, child HostChildAgent) error
	GetChildAgent(ctx context.Context, childAgentID string) (HostChildAgent, error)
	ListChildAgents(ctx context.Context, missionID string) ([]HostChildAgent, error)
}

type InMemoryMissionStore struct {
	mu       sync.RWMutex
	missions map[string]HostOperationMission
	children map[string]HostChildAgent
}

func NewInMemoryMissionStore() *InMemoryMissionStore {
	return &InMemoryMissionStore{
		missions: map[string]HostOperationMission{},
		children: map[string]HostChildAgent{},
	}
}

func (s *InMemoryMissionStore) SaveMission(_ context.Context, mission HostOperationMission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if mission.CreatedAt.IsZero() {
		mission.CreatedAt = now
	}
	mission.UpdatedAt = now
	s.missions[mission.ID] = cloneMission(mission)
	return nil
}

func (s *InMemoryMissionStore) GetMission(_ context.Context, missionID string) (HostOperationMission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mission, ok := s.missions[missionID]
	if !ok {
		return HostOperationMission{}, ErrMissionNotFound
	}
	return cloneMission(mission), nil
}

func (s *InMemoryMissionStore) ListThreadMissions(_ context.Context, threadID string) ([]HostOperationMission, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]HostOperationMission, 0)
	for _, mission := range s.missions {
		if mission.ThreadID == threadID {
			result = append(result, cloneMission(mission))
		}
	}
	return result, nil
}

func (s *InMemoryMissionStore) SaveChildAgent(_ context.Context, child HostChildAgent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if child.StartedAt.IsZero() {
		child.StartedAt = now
	}
	child.UpdatedAt = now
	s.children[child.ID] = cloneChildAgent(child)
	if mission, ok := s.missions[child.MissionID]; ok && !stringSliceContains(mission.ChildAgentIDs, child.ID) {
		mission.ChildAgentIDs = append(mission.ChildAgentIDs, child.ID)
		mission.UpdatedAt = now
		s.missions[child.MissionID] = cloneMission(mission)
	}
	return nil
}

func (s *InMemoryMissionStore) GetChildAgent(_ context.Context, childAgentID string) (HostChildAgent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	child, ok := s.children[childAgentID]
	if !ok {
		return HostChildAgent{}, ErrChildAgentNotFound
	}
	return cloneChildAgent(child), nil
}

func (s *InMemoryMissionStore) ListChildAgents(_ context.Context, missionID string) ([]HostChildAgent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]HostChildAgent, 0)
	for _, child := range s.children {
		if child.MissionID == missionID {
			result = append(result, cloneChildAgent(child))
		}
	}
	return result, nil
}

func cloneMission(mission HostOperationMission) HostOperationMission {
	mission.Mentions = append([]HostMention(nil), mission.Mentions...)
	mission.ChildAgentIDs = append([]string(nil), mission.ChildAgentIDs...)
	return mission
}

func cloneChildAgent(child HostChildAgent) HostChildAgent {
	child.PlanStepIDs = append([]string(nil), child.PlanStepIDs...)
	if child.CompletedAt != nil {
		completedAt := *child.CompletedAt
		child.CompletedAt = &completedAt
	}
	return child
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
