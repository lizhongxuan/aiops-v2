package service

import (
	"context"

	"runner/server/store/agentstore"
)

type runMetaLister interface {
	List(ctx context.Context, filter RunFilter) ([]*RunMeta, error)
}

type agentRecordLister interface {
	List(ctx context.Context, filter AgentFilter) ([]*agentstore.AgentRecord, error)
}

type DashboardStats struct {
	TotalRuns    int     `json:"total_runs"`
	SuccessRate  float64 `json:"success_rate"`
	AgentsOnline int     `json:"agents_online"`
	AgentsTotal  int     `json:"agents_total"`
	QueueDepth   int     `json:"queue_depth"`
}

type DashboardService struct {
	runs   runMetaLister
	agents agentRecordLister
}

func NewDashboardService(runs runMetaLister, agents agentRecordLister) *DashboardService {
	return &DashboardService{
		runs:   runs,
		agents: agents,
	}
}

func (s *DashboardService) Stats(ctx context.Context) (DashboardStats, error) {
	runs, err := s.listRuns(ctx)
	if err != nil {
		return DashboardStats{}, err
	}
	agents, err := s.listAgents(ctx)
	if err != nil {
		return DashboardStats{}, err
	}

	successCount := 0
	queuedCount := 0
	for _, run := range runs {
		switch run.Status {
		case "success":
			successCount++
		case "queued":
			queuedCount++
		}
	}

	onlineCount := 0
	for _, agent := range agents {
		if agent != nil && agent.Status == agentstore.StatusOnline {
			onlineCount++
		}
	}

	successRate := 0.0
	if len(runs) > 0 {
		successRate = float64(successCount) * 100 / float64(len(runs))
	}

	return DashboardStats{
		TotalRuns:    len(runs),
		SuccessRate:  successRate,
		AgentsOnline: onlineCount,
		AgentsTotal:  len(agents),
		QueueDepth:   queuedCount,
	}, nil
}

func (s *DashboardService) listRuns(ctx context.Context) ([]*RunMeta, error) {
	if s == nil || s.runs == nil {
		return nil, nil
	}
	return s.runs.List(ctx, RunFilter{})
}

func (s *DashboardService) listAgents(ctx context.Context) ([]*agentstore.AgentRecord, error) {
	if s == nil || s.agents == nil {
		return nil, nil
	}
	return s.agents.List(ctx, AgentFilter{})
}
