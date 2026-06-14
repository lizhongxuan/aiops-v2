package hostops

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/opssemantic"
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
	mu                sync.RWMutex
	missions          map[string]HostOperationMission
	children          map[string]HostChildAgent
	subtaskDecisions  map[string]HostSubTaskScheduleDecision
	activeWriteByHost map[string]string
}

func NewInMemoryMissionStore() *InMemoryMissionStore {
	return &InMemoryMissionStore{
		missions:          map[string]HostOperationMission{},
		children:          map[string]HostChildAgent{},
		subtaskDecisions:  map[string]HostSubTaskScheduleDecision{},
		activeWriteByHost: map[string]string{},
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

func (s *InMemoryMissionStore) SaveHostSubTaskScheduleDecision(_ context.Context, decision HostSubTaskScheduleDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subtaskDecisions == nil {
		s.subtaskDecisions = map[string]HostSubTaskScheduleDecision{}
	}
	if s.activeWriteByHost == nil {
		s.activeWriteByHost = map[string]string{}
	}
	s.subtaskDecisions[decision.SubTaskID] = decision
	key := scheduleHostKey(decision.MissionID, decision.HostID)
	if decision.Status == HostSubTaskStatusRunning {
		s.activeWriteByHost[key] = decision.SubTaskID
	}
	if decision.Status == HostSubTaskStatusCancelled && decision.ActiveSubTaskID != "" && s.activeWriteByHost[key] == decision.ActiveSubTaskID {
		delete(s.activeWriteByHost, key)
	}
	return nil
}

func (s *InMemoryMissionStore) ActiveHostSubTaskID(_ context.Context, missionID, hostID string) (string, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active, ok := s.activeWriteByHost[scheduleHostKey(missionID, hostID)]
	return active, ok, nil
}

func (s *InMemoryMissionStore) ListHostSubTaskScheduleDecisions(_ context.Context, missionID string) ([]HostSubTaskScheduleDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HostSubTaskScheduleDecision, 0)
	for _, decision := range s.subtaskDecisions {
		if decision.MissionID == missionID {
			out = append(out, decision)
		}
	}
	return out, nil
}

func scheduleHostKey(missionID, hostID string) string {
	return strings.TrimSpace(missionID) + "\x00" + strings.TrimSpace(hostID)
}

func cloneMission(mission HostOperationMission) HostOperationMission {
	mission.SemanticTask = cloneSemanticTask(mission.SemanticTask)
	mission.Plan = clonePlan(mission.Plan)
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

func cloneSemanticTask(task opssemantic.OpsSemanticTask) opssemantic.OpsSemanticTask {
	task.Targets = append(task.Targets[:0:0], task.Targets...)
	task.HostScope = append(task.HostScope[:0:0], task.HostScope...)
	task.MissingSlots = append(task.MissingSlots[:0:0], task.MissingSlots...)
	task.EvidenceRequirements = append(task.EvidenceRequirements[:0:0], task.EvidenceRequirements...)
	return task
}

func clonePlan(plan HostOperationPlan) HostOperationPlan {
	plan.Steps = append([]PlanStep(nil), plan.Steps...)
	for i := range plan.Steps {
		plan.Steps[i] = clonePlanStep(plan.Steps[i])
	}
	plan.Revisions = append([]PlanRevision(nil), plan.Revisions...)
	for i := range plan.Revisions {
		plan.Revisions[i].AffectedHostIDs = append([]string(nil), plan.Revisions[i].AffectedHostIDs...)
		plan.Revisions[i].Changes = append([]string(nil), plan.Revisions[i].Changes...)
	}
	if plan.AcceptedAt != nil {
		acceptedAt := *plan.AcceptedAt
		plan.AcceptedAt = &acceptedAt
	}
	return plan
}

func clonePlanStep(step PlanStep) PlanStep {
	step.HostIDs = append([]string(nil), step.HostIDs...)
	step.ChildAgentIDs = append([]string(nil), step.ChildAgentIDs...)
	step.EvidenceRequired = append([]string(nil), step.EvidenceRequired...)
	if step.StartedAt != nil {
		startedAt := *step.StartedAt
		step.StartedAt = &startedAt
	}
	if step.CompletedAt != nil {
		completedAt := *step.CompletedAt
		step.CompletedAt = &completedAt
	}
	return step
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
