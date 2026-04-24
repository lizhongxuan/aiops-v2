package service

import (
	"context"
	"testing"

	"runner/server/store/agentstore"
)

type stubRunLister struct {
	items []*RunMeta
	err   error
}

func (s stubRunLister) List(_ context.Context, _ RunFilter) ([]*RunMeta, error) {
	return s.items, s.err
}

type stubAgentLister struct {
	items []*agentstore.AgentRecord
	err   error
}

func (s stubAgentLister) List(_ context.Context, _ AgentFilter) ([]*agentstore.AgentRecord, error) {
	return s.items, s.err
}

func TestDashboardServiceStats(t *testing.T) {
	svc := NewDashboardService(
		stubRunLister{
			items: []*RunMeta{
				{RunID: "run-1", Status: "success"},
				{RunID: "run-2", Status: "failed"},
				{RunID: "run-3", Status: "queued"},
				{RunID: "run-4", Status: "success"},
			},
		},
		stubAgentLister{
			items: []*agentstore.AgentRecord{
				{ID: "a1", Status: agentstore.StatusOnline},
				{ID: "a2", Status: agentstore.StatusOffline},
				{ID: "a3", Status: agentstore.StatusOnline},
			},
		},
	)

	stats, err := svc.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.TotalRuns != 4 {
		t.Fatalf("expected 4 total runs, got %d", stats.TotalRuns)
	}
	if stats.QueueDepth != 1 {
		t.Fatalf("expected queue depth 1, got %d", stats.QueueDepth)
	}
	if stats.AgentsOnline != 2 || stats.AgentsTotal != 3 {
		t.Fatalf("unexpected agent counts: %+v", stats)
	}
	if stats.SuccessRate != 50 {
		t.Fatalf("expected success rate 50, got %v", stats.SuccessRate)
	}
}
