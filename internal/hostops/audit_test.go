package hostops

import (
	"context"
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestOrchestratorWritesMissionPlanAndChildAuditEvents(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	orchestrator := NewOrchestrator(store, transcripts, &fakeChildSpawner{})
	mission := HostOperationMission{
		ID:           "mission-audit",
		ThreadID:     "thread-audit",
		PlanRequired: true,
		PlanAccepted: false,
		SemanticTask: opssemantic.OpsSemanticTask{
			ID:        "task-audit",
			RiskLevel: opssemantic.RiskReadOnly,
			HostScope: []opssemantic.OpsHostRef{{
				HostID: "host-a",
			}},
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

	items, err := transcripts.List(ctx, MissionAuditTranscriptID(planned.ID))
	if err != nil {
		t.Fatalf("List mission transcript error = %v", err)
	}
	for _, want := range []string{"host operation plan created", "host operation plan accepted", "host child agent created"} {
		if !transcriptContainsContent(items, want) {
			t.Fatalf("mission audit items = %#v, missing %q", items, want)
		}
	}
}

func TestCommandApprovalWritesApprovedAndDeniedAuditEvents(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       NewInMemoryCommandApprovalStore(),
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    &fakeHostCommandExecutor{},
	})
	approved, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval approved error = %v", err)
	}
	if _, _, err := controller.Decide(ctx, approved.ID, "approved"); err != nil {
		t.Fatalf("Decide approved error = %v", err)
	}
	denied, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-denied",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval denied error = %v", err)
	}
	if _, _, err := controller.Decide(ctx, denied.ID, "denied"); err != nil {
		t.Fatalf("Decide denied error = %v", err)
	}

	items, err := transcripts.List(ctx, "child-a")
	if err != nil {
		t.Fatalf("List child transcript error = %v", err)
	}
	if !transcriptContainsStatus(items, "approved") || !transcriptContainsStatus(items, "denied") || !transcriptContainsType(items, TranscriptItemToolResult) {
		t.Fatalf("child audit items = %#v, want approved, denied, and tool result events", items)
	}
}

func transcriptContainsContent(items []TranscriptItem, content string) bool {
	for _, item := range items {
		if item.Content == content {
			return true
		}
	}
	return false
}

func transcriptContainsStatus(items []TranscriptItem, status string) bool {
	for _, item := range items {
		if item.Status == status {
			return true
		}
	}
	return false
}

func transcriptContainsType(items []TranscriptItem, itemType TranscriptItemType) bool {
	for _, item := range items {
		if item.Type == itemType {
			return true
		}
	}
	return false
}
