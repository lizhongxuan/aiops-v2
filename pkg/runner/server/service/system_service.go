package service

import (
	"context"
	"time"

	"runner/server/store/agentstore"
)

type SystemMetricsSummary struct {
	TotalRuns    int `json:"total_runs"`
	SuccessRuns  int `json:"success_runs"`
	FailedRuns   int `json:"failed_runs"`
	CanceledRuns int `json:"canceled_runs"`
	QueueDepth   int `json:"queue_depth"`
	AgentsOnline int `json:"agents_online"`
	AgentsTotal  int `json:"agents_total"`
}

type DailyTrendPoint struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

type StatusDistribution struct {
	Success     int `json:"success"`
	Failed      int `json:"failed"`
	Canceled    int `json:"canceled"`
	Interrupted int `json:"interrupted"`
}

type SystemMetrics struct {
	Summary            SystemMetricsSummary `json:"summary"`
	DailyTrend         []DailyTrendPoint    `json:"daily_trend"`
	StatusDistribution StatusDistribution   `json:"status_distribution"`
	HourlyAPIRequests  []int                `json:"hourly_api_requests"`
}

type SystemService struct {
	runs   runMetaLister
	agents agentRecordLister
	now    func() time.Time
}

func NewSystemService(runs runMetaLister, agents agentRecordLister) *SystemService {
	return &SystemService{
		runs:   runs,
		agents: agents,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *SystemService) Metrics(ctx context.Context) (SystemMetrics, error) {
	now := s.currentTime()
	runs, err := s.listRuns(ctx)
	if err != nil {
		return SystemMetrics{}, err
	}
	agents, err := s.listAgents(ctx)
	if err != nil {
		return SystemMetrics{}, err
	}

	summary := SystemMetricsSummary{
		TotalRuns: len(runs),
	}
	statusDistribution := StatusDistribution{}

	for _, run := range runs {
		switch run.Status {
		case "success":
			summary.SuccessRuns++
			statusDistribution.Success++
		case "failed":
			summary.FailedRuns++
			statusDistribution.Failed++
		case "canceled":
			summary.CanceledRuns++
			statusDistribution.Canceled++
		case "interrupted":
			statusDistribution.Interrupted++
		case "queued":
			summary.QueueDepth++
		}
	}

	for _, agent := range agents {
		if agent != nil && agent.Status == agentstore.StatusOnline {
			summary.AgentsOnline++
		}
	}
	summary.AgentsTotal = len(agents)

	trendMap := make(map[string]*DailyTrendPoint, 7)
	dailyTrend := make([]DailyTrendPoint, 0, 7)
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		key := day.Format("2006-01-02")
		point := DailyTrendPoint{
			Date:    key,
			Success: 0,
			Failed:  0,
		}
		dailyTrend = append(dailyTrend, point)
		trendMap[key] = &dailyTrend[len(dailyTrend)-1]
	}

	hourlyRequests := make([]int, 12)
	for _, run := range runs {
		timestamp := runEventTime(*run)
		if timestamp.IsZero() {
			continue
		}

		dayKey := timestamp.UTC().Format("2006-01-02")
		if point := trendMap[dayKey]; point != nil {
			switch run.Status {
			case "success":
				point.Success++
			case "failed":
				point.Failed++
			}
		}

		diff := now.Sub(timestamp.UTC())
		if diff < 0 {
			continue
		}
		hour := int(diff / time.Hour)
		if hour >= 0 && hour < len(hourlyRequests) {
			hourlyRequests[len(hourlyRequests)-1-hour]++
		}
	}

	return SystemMetrics{
		Summary:            summary,
		DailyTrend:         dailyTrend,
		StatusDistribution: statusDistribution,
		HourlyAPIRequests:  hourlyRequests,
	}, nil
}

func (s *SystemService) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func runEventTime(run RunMeta) time.Time {
	if !run.CreatedAt.IsZero() {
		return run.CreatedAt
	}
	if !run.StartedAt.IsZero() {
		return run.StartedAt
	}
	if !run.FinishedAt.IsZero() {
		return run.FinishedAt
	}
	return time.Time{}
}

func (s *SystemService) listRuns(ctx context.Context) ([]*RunMeta, error) {
	if s == nil || s.runs == nil {
		return nil, nil
	}
	return s.runs.List(ctx, RunFilter{})
}

func (s *SystemService) listAgents(ctx context.Context) ([]*agentstore.AgentRecord, error) {
	if s == nil || s.agents == nil {
		return nil, nil
	}
	return s.agents.List(ctx, AgentFilter{})
}
