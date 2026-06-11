package hostops

import (
	"context"
	"testing"

	"aiops-v2/internal/observability"
	"aiops-v2/internal/opssemantic"
)

func TestHostOpsRecordsPlanAgentApprovalAndCommandMetrics(t *testing.T) {
	observability.ResetOpsMetricsForTest()
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	orchestrator := NewOrchestrator(store, transcripts, &fakeChildSpawner{})
	mission := HostOperationMission{
		ID:           "mission-metrics",
		PlanRequired: true,
		PlanAccepted: false,
		SemanticTask: opssemantic.OpsSemanticTask{
			ID:        "task-metrics",
			RiskLevel: opssemantic.RiskReadOnly,
			HostScope: []opssemantic.OpsHostRef{{HostID: "host-a"}},
		},
		Mentions: []HostMention{{HostID: "host-a", Resolved: true}},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatalf("SaveMission error = %v", err)
	}
	planned, err := orchestrator.CreatePlan(ctx, mission.ID)
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	if err := orchestrator.AcceptPlan(ctx, planned.ID, planned.Plan.ID); err != nil {
		t.Fatalf("AcceptPlan error = %v", err)
	}
	if _, err := orchestrator.SpawnChildren(ctx, planned.ID, []ChildAgentAssignment{{HostID: "host-a", Task: "run assigned host operation"}}); err != nil {
		t.Fatalf("SpawnChildren error = %v", err)
	}
	missions, transcripts := hostCommandApprovalFixture(t)
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       NewInMemoryCommandApprovalStore(),
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    &fakeHostCommandExecutor{},
	})
	approval, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval error = %v", err)
	}
	if _, _, err := controller.Decide(ctx, approval.ID, "approved"); err != nil {
		t.Fatalf("Decide error = %v", err)
	}

	snapshot := observability.OpsMetricsSnapshot()
	for _, metric := range []string{
		observability.OpsMetricPlanGeneration,
		observability.OpsMetricPlanAcceptance,
		observability.OpsMetricHostAgentCreation,
		observability.OpsMetricCommandApproval,
		observability.OpsMetricCommandExecution,
	} {
		if snapshot[metric].Success == 0 {
			t.Fatalf("metrics = %#v, want success counter for %s", snapshot, metric)
		}
	}
}
