package service

import (
	"context"
	"testing"
	"time"

	"runner/server/store/agentstore"
)

func TestSystemServiceMetrics(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	svc := NewSystemService(
		stubRunLister{
			items: []*RunMeta{
				{RunID: "run-1", Status: "success", CreatedAt: now.Add(-1 * time.Hour)},
				{RunID: "run-2", Status: "failed", CreatedAt: now.Add(-2 * time.Hour)},
				{RunID: "run-3", Status: "canceled", CreatedAt: now.Add(-3 * time.Hour)},
				{RunID: "run-4", Status: "queued", CreatedAt: now.Add(-30 * time.Minute)},
				{RunID: "run-5", Status: "interrupted", CreatedAt: now.AddDate(0, 0, -1)},
				{RunID: "run-6", Status: "success", CreatedAt: now.AddDate(0, 0, -6)},
			},
		},
		stubAgentLister{
			items: []*agentstore.AgentRecord{
				{ID: "a1", Status: agentstore.StatusOnline},
				{ID: "a2", Status: agentstore.StatusDegraded},
			},
		},
	)
	svc.now = func() time.Time { return now }

	payload, err := svc.Metrics(context.Background())
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}

	if payload.Summary.TotalRuns != 6 {
		t.Fatalf("expected 6 total runs, got %d", payload.Summary.TotalRuns)
	}
	if payload.Summary.SuccessRuns != 2 || payload.Summary.FailedRuns != 1 || payload.Summary.CanceledRuns != 1 {
		t.Fatalf("unexpected summary counts: %+v", payload.Summary)
	}
	if payload.Summary.QueueDepth != 1 {
		t.Fatalf("expected queue depth 1, got %d", payload.Summary.QueueDepth)
	}
	if payload.Summary.AgentsOnline != 1 || payload.Summary.AgentsTotal != 2 {
		t.Fatalf("unexpected agent summary: %+v", payload.Summary)
	}
	if len(payload.DailyTrend) != 7 {
		t.Fatalf("expected 7 daily trend points, got %d", len(payload.DailyTrend))
	}
	if payload.DailyTrend[0].Success != 1 {
		t.Fatalf("expected oldest bucket success=1, got %+v", payload.DailyTrend[0])
	}
	last := payload.DailyTrend[len(payload.DailyTrend)-1]
	if last.Success != 1 || last.Failed != 1 {
		t.Fatalf("expected latest bucket success=1 failed=1, got %+v", last)
	}
	if payload.StatusDistribution.Interrupted != 1 {
		t.Fatalf("expected interrupted=1, got %+v", payload.StatusDistribution)
	}
	if len(payload.HourlyAPIRequests) != 12 {
		t.Fatalf("expected 12 hourly points, got %d", len(payload.HourlyAPIRequests))
	}
	if payload.HourlyAPIRequests[11] != 1 {
		t.Fatalf("expected latest hourly bucket=1, got %v", payload.HourlyAPIRequests)
	}
	if payload.HourlyAPIRequests[10] != 1 {
		t.Fatalf("expected previous hourly bucket=1, got %v", payload.HourlyAPIRequests)
	}
}
