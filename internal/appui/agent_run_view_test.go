package appui

import (
	"testing"
	"time"
)

func TestAgentRunViewReadModelCarriesExistingRuntimeReferences(t *testing.T) {
	run := AgentRunView{
		ID:             "opsrun-turn-1",
		SessionID:      "sess-1",
		RootTurnID:     "turn-root",
		ActiveTurnID:   "turn-1",
		UserGoal:       "检查 checkout 服务异常",
		NormalizedGoal: "service checkout rca",
		RouteMode:      string(ChatRouteAdvisory),
		Profile:        "chat_advisory",
		Status:         AgentRunStatusRunning,
		TargetSummary:  "service:checkout",
		CurrentStep:    "采集证据",
		CurrentStepID:  "step-tool-1",
		CheckpointID:   "checkpoint-1",
		EvidenceCount:  2,
		StartedAt:      time.Unix(1, 0).UTC(),
		UpdatedAt:      time.Unix(2, 0).UTC(),
		Steps: []AgentStepView{{
			ID:            "step-tool-1",
			RunID:         "opsrun-turn-1",
			TurnID:        "turn-1",
			Iteration:     1,
			Kind:          AgentStepKindToolCall,
			Status:        AgentStepStatusCompleted,
			Title:         "读取指标",
			InputSummary:  "query service metrics",
			OutputSummary: "latency high",
			ToolName:      "coroot.service_metrics",
			ToolCallID:    "tool-call-1",
			ApprovalID:    "approval-1",
			CheckpointID:  "checkpoint-1",
			TargetRefs:    []string{"service:checkout"},
			EvidenceRefs:  []string{"evidence-1"},
			Error:         "",
			StartedAt:     time.Unix(1, 0).UTC(),
			CompletedAt:   time.Unix(2, 0).UTC(),
		}},
	}

	if run.ID == "" || run.Steps[0].RunID != run.ID || run.Steps[0].CheckpointID != run.CheckpointID {
		t.Fatalf("AgentRunView lost existing runtime references: %#v", run)
	}
	if run.Steps[0].Kind != AgentStepKindToolCall || run.Steps[0].Status != AgentStepStatusCompleted {
		t.Fatalf("AgentStepView kind/status = %q/%q", run.Steps[0].Kind, run.Steps[0].Status)
	}
}

func TestAgentRunViewDefinesRequiredStepKindsAndStatuses(t *testing.T) {
	kinds := []AgentStepKind{
		AgentStepKindReasoning,
		AgentStepKindToolSearch,
		AgentStepKindToolCall,
		AgentStepKindApproval,
		AgentStepKindMCPHealth,
		AgentStepKindEvidence,
		AgentStepKindCheckpoint,
		AgentStepKindFinalResponse,
		AgentStepKindError,
	}
	for _, kind := range kinds {
		if kind == "" {
			t.Fatalf("empty AgentStepKind in %#v", kinds)
		}
	}

	statuses := []AgentStepStatus{
		AgentStepStatusPending,
		AgentStepStatusRunning,
		AgentStepStatusWaitingApproval,
		AgentStepStatusSkipped,
		AgentStepStatusCompleted,
		AgentStepStatusFailed,
		AgentStepStatusCancelled,
	}
	for _, status := range statuses {
		if status == "" {
			t.Fatalf("empty AgentStepStatus in %#v", statuses)
		}
	}
}
