package runtimekernel

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/taskdepth"
)

func TestUXProgressTraceIncludesApprovalChildAgentAndBlocker(t *testing.T) {
	snapshot := syntheticRuntimeKernelSnapshot("synthetic-turn-ux")
	snapshot.TaskDepth = taskdepth.Profile{Level: taskdepth.LevelMultiAgent}
	snapshot.PendingApprovals = []PendingApproval{{
		ID:        "synthetic-approval-1",
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.ID,
		ToolName:  "synthetic.mutate",
		Status:    "pending",
	}}
	snapshot.PendingEvidence = []PendingEvidence{{
		ID:        "synthetic-evidence-blocker-1",
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.ID,
		Status:    "blocked",
		Reason:    "synthetic_missing_required_evidence",
	}}
	snapshot.AgentItems = []agentstate.TurnItem{
		syntheticPlanItem(t, snapshot.ID, planning.PlanState{
			Status: planning.PlanStatusActive,
			Steps: []planning.PlanStep{
				{ID: "synthetic-step-1", Text: "synthetic current step", Status: planning.StepStatusInProgress, AgentID: "synthetic-child-agent-1", EvidenceRefs: []string{"synthetic-evidence-1"}},
				{ID: "synthetic-step-2", Text: "synthetic next step", Status: planning.StepStatusPending, AgentID: "synthetic-child-agent-2"},
			},
		}),
	}

	trace := BuildUXProgressTrace(snapshot)

	if trace.TurnID != "synthetic-turn-ux" {
		t.Fatalf("TurnID = %q, want synthetic-turn-ux", trace.TurnID)
	}
	if trace.TaskDepth != "multi_agent" {
		t.Fatalf("TaskDepth = %q, want multi_agent", trace.TaskDepth)
	}
	if trace.Phase != "blocked" {
		t.Fatalf("Phase = %q, want blocked because blockers outrank approval", trace.Phase)
	}
	if trace.CurrentStepID != "synthetic-step-1" {
		t.Fatalf("CurrentStepID = %q, want synthetic-step-1", trace.CurrentStepID)
	}
	if !runtimeKernelTestContains(trace.PendingApprovals, "synthetic-approval-1") {
		t.Fatalf("PendingApprovals = %#v, want synthetic-approval-1", trace.PendingApprovals)
	}
	if !runtimeKernelTestContains(trace.ChildAgents, "synthetic-child-agent-1") || !runtimeKernelTestContains(trace.ChildAgents, "synthetic-child-agent-2") {
		t.Fatalf("ChildAgents = %#v, want both synthetic child agents", trace.ChildAgents)
	}
	if !runtimeKernelTestContains(trace.Blockers, "synthetic-evidence-blocker-1") {
		t.Fatalf("Blockers = %#v, want synthetic-evidence-blocker-1", trace.Blockers)
	}
	if !runtimeKernelTestContains(trace.EvidenceRefs, "synthetic-evidence-1") {
		t.Fatalf("EvidenceRefs = %#v, want synthetic-evidence-1", trace.EvidenceRefs)
	}
}

func syntheticRuntimeKernelSnapshot(turnID string) *TurnSnapshot {
	return &TurnSnapshot{
		ID:          turnID,
		SessionID:   "synthetic-session-1",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Metadata:    map[string]string{},
	}
}

func syntheticPlanItem(t *testing.T, turnID string, plan planning.PlanState) agentstate.TurnItem {
	t.Helper()
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal synthetic plan: %v", err)
	}
	return agentstate.TurnItem{
		ID:     turnID + "-synthetic-plan",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Summary: "synthetic plan",
			Data:    raw,
		},
	}
}

func runtimeKernelTestContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
