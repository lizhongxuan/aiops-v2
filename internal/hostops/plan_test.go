package hostops

import (
	"context"
	"errors"
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestBuildPlanForMissionCreatesStableHostSteps(t *testing.T) {
	mission := HostOperationMission{
		ID: "mission-1",
		SemanticTask: opssemantic.OpsSemanticTask{
			HostScope: []opssemantic.OpsHostRef{
				{HostID: "host-a", DisplayName: "host-a"},
				{HostID: "host-b", DisplayName: "host-b"},
			},
			ActionType: opssemantic.ActionWrite,
			RiskLevel:  opssemantic.RiskMediumWrite,
			EvidenceRequirements: []opssemantic.EvidenceRequirement{
				{Kind: opssemantic.EvidenceCommandOutput, Description: "command output", Required: true},
			},
		},
	}

	plan, err := BuildPlanForMission(mission)
	if err != nil {
		t.Fatalf("BuildPlanForMission() error = %v", err)
	}
	if plan.Version != 1 {
		t.Fatalf("Version = %d, want 1", plan.Version)
	}
	if plan.Status != PlanStatusWaitingAcceptance {
		t.Fatalf("Status = %q, want %q", plan.Status, PlanStatusWaitingAcceptance)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(plan.Steps))
	}
	for i, step := range plan.Steps {
		if step.Index != i+1 {
			t.Fatalf("step[%d].Index = %d, want %d", i, step.Index, i+1)
		}
		if len(step.HostIDs) != 1 {
			t.Fatalf("step[%d].HostIDs = %#v, want one host", i, step.HostIDs)
		}
		if !step.ApprovalRequired {
			t.Fatalf("step[%d].ApprovalRequired = false, want true", i)
		}
	}
}

func TestOrchestratorCreatePlanWaitsForAcceptanceBeforeSpawningChildren(t *testing.T) {
	store := NewInMemoryMissionStore()
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	mission := HostOperationMission{
		ID:       "mission-1",
		ThreadID: "thread-1",
		Status:   HostMissionStatusPlanning,
		SemanticTask: opssemantic.OpsSemanticTask{
			HostScope: []opssemantic.OpsHostRef{{HostID: "host-a"}},
			RiskLevel: opssemantic.RiskMediumWrite,
		},
		Mentions: []HostMention{{HostID: "host-a", Resolved: true}},
	}
	if err := store.SaveMission(context.Background(), mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}

	planned, err := orchestrator.CreatePlan(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("CreatePlan() error = %v", err)
	}
	if planned.Status != HostMissionStatusWaitingPlanAcceptance {
		t.Fatalf("mission Status = %q, want %q", planned.Status, HostMissionStatusWaitingPlanAcceptance)
	}
	if planned.Plan.ID == "" {
		t.Fatalf("Plan.ID is empty")
	}
	if planned.PlanAccepted {
		t.Fatalf("PlanAccepted = true, want false before acceptance")
	}
	_, err = orchestrator.SpawnChildren(context.Background(), "mission-1", []ChildAgentAssignment{{HostID: "host-a", Task: "inspect assigned host"}})
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("SpawnChildren err = %v, want ErrPlanNotAccepted", err)
	}
	if err := orchestrator.AcceptPlan(context.Background(), "mission-1", planned.Plan.ID); err != nil {
		t.Fatalf("AcceptPlan() error = %v", err)
	}
	accepted, err := store.GetMission(context.Background(), "mission-1")
	if err != nil {
		t.Fatalf("GetMission() error = %v", err)
	}
	if accepted.Plan.Status != PlanStatusAccepted {
		t.Fatalf("Plan.Status = %q, want %q", accepted.Plan.Status, PlanStatusAccepted)
	}
	if accepted.Plan.AcceptedAt == nil {
		t.Fatalf("Plan.AcceptedAt is nil")
	}
}

func TestPlanProgressCountsCompletedSteps(t *testing.T) {
	completed, total := PlanProgress(HostOperationPlan{Steps: []PlanStep{
		{Status: PlanStepStatusCompleted},
		{Status: PlanStepStatusRunning},
		{Status: PlanStepStatusCompleted},
	}})

	if completed != 2 || total != 3 {
		t.Fatalf("PlanProgress() = %d/%d, want 2/3", completed, total)
	}
}

func TestRevisePlanRejectsSilentCompletedStepChange(t *testing.T) {
	mission := HostOperationMission{
		ID:           "mission-1",
		Status:       HostMissionStatusRunning,
		PlanAccepted: true,
		Plan: HostOperationPlan{
			ID:      "plan-1",
			Version: 1,
			Status:  PlanStatusAccepted,
			Steps: []PlanStep{{
				ID:      "step-1",
				Index:   1,
				Title:   "Inspect assigned host",
				Status:  PlanStepStatusCompleted,
				HostIDs: []string{"host-a"},
			}},
		},
	}
	_, err := ReviseMissionPlan(mission, PlanRevisionRequest{
		Reason: "change completed step",
		Steps: []PlanStep{{
			ID:      "step-1",
			Index:   1,
			Title:   "Mutate completed step",
			Status:  PlanStepStatusCompleted,
			HostIDs: []string{"host-a"},
		}},
	})
	if !errors.Is(err, ErrCompletedPlanStepImmutable) {
		t.Fatalf("err = %v, want ErrCompletedPlanStepImmutable", err)
	}
}

func TestRevisePlanRiskIncreaseReturnsToWaitingAcceptance(t *testing.T) {
	mission := HostOperationMission{
		ID:           "mission-1",
		Status:       HostMissionStatusRunning,
		PlanAccepted: true,
		Plan: HostOperationPlan{
			ID:      "plan-1",
			Version: 1,
			Status:  PlanStatusAccepted,
			Steps: []PlanStep{{
				ID:        "step-1",
				Index:     1,
				Title:     "Inspect assigned host",
				Status:    PlanStepStatusPending,
				HostIDs:   []string{"host-a"},
				RiskLevel: opssemantic.RiskReadOnly,
			}},
		},
	}

	revised, err := ReviseMissionPlan(mission, PlanRevisionRequest{
		Reason:          "requires host change",
		AffectedHostIDs: []string{"host-a"},
		Steps: []PlanStep{{
			ID:        "step-1",
			Index:     1,
			Title:     "Change assigned host",
			Status:    PlanStepStatusPending,
			HostIDs:   []string{"host-a"},
			RiskLevel: opssemantic.RiskMediumWrite,
		}},
	})
	if err != nil {
		t.Fatalf("ReviseMissionPlan() error = %v", err)
	}
	if revised.Plan.Version != 2 {
		t.Fatalf("Plan.Version = %d, want 2", revised.Plan.Version)
	}
	if len(revised.Plan.Revisions) != 1 {
		t.Fatalf("len(Revisions) = %d, want 1", len(revised.Plan.Revisions))
	}
	if revised.PlanAccepted {
		t.Fatalf("PlanAccepted = true, want false after risk increase")
	}
	if revised.Status != HostMissionStatusWaitingPlanAcceptance {
		t.Fatalf("Mission.Status = %q, want %q", revised.Status, HostMissionStatusWaitingPlanAcceptance)
	}
}
