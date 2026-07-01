package appui

import "testing"

func TestAgentRunProjectionUsesExistingOpsRunMetadata(t *testing.T) {
	trace := ChatRunTraceView{
		ID:            "opsrun-turn-1",
		SessionID:     "sess-1",
		TurnID:        "turn-1",
		ClientTurnID:  "client-turn-1",
		Source:        "chat",
		Status:        "working",
		Title:         "排查 checkout 服务异常",
		RouteMode:     string(ChatRouteAdvisory),
		TargetSummary: "service:checkout",
		EvidenceCount: 2,
		CurrentStep:   "读取服务指标",
	}

	run := BuildAgentRunViewFromTrace(trace)

	if run == nil {
		t.Fatal("BuildAgentRunViewFromTrace() = nil, want read model")
	}
	if run.ID != trace.ID {
		t.Fatalf("run.ID = %q, want existing ops run id %q", run.ID, trace.ID)
	}
	if run.SessionID != trace.SessionID || run.RootTurnID != trace.TurnID || run.ActiveTurnID != trace.TurnID {
		t.Fatalf("run runtime refs = session:%q root:%q active:%q, want trace refs", run.SessionID, run.RootTurnID, run.ActiveTurnID)
	}
	if run.UserGoal != trace.Title || run.NormalizedGoal != trace.Title {
		t.Fatalf("run goals = %q/%q, want trace title", run.UserGoal, run.NormalizedGoal)
	}
	if run.Status != AgentRunStatusRunning || run.TargetSummary != trace.TargetSummary || run.EvidenceCount != trace.EvidenceCount {
		t.Fatalf("run summary = %#v, want running target/evidence summary", run)
	}
}

func TestAgentRunProjectionBuildsStepsFromProcessBlocks(t *testing.T) {
	state := AiopsTransportState{
		SessionID:     "sess-1",
		CurrentTurnID: "turn-1",
		OpsRun: &AiopsTransportOpsRun{
			ID:            "opsrun-turn-1",
			SessionID:     "sess-1",
			TurnID:        "turn-1",
			Status:        "blocked",
			Title:         "修复 checkout 服务异常",
			CurrentStep:   "等待审批",
			CheckpointID:  "checkpoint-approval-1",
			EvidenceCount: 1,
		},
		Turns: map[string]AiopsTransportTurn{
			"turn-1": {
				ID: "turn-1",
				User: &AiopsTransportMessage{
					ID:   "user-1",
					Text: "修复 checkout 服务异常",
				},
				Process: []AiopsProcessBlock{
					{
						ID:     "reasoning-1",
						Kind:   AiopsTransportProcessKindReasoning,
						Status: AiopsTransportProcessStatusCompleted,
						Text:   "分析现有证据",
					},
					{
						ID:           "search-1",
						Kind:         AiopsTransportProcessKindSearch,
						Status:       AiopsTransportProcessStatusCompleted,
						Text:         "checkout service metrics",
						Source:       "tool_search",
						InputSummary: "checkout service metrics",
					},
					{
						ID:            "tool-1",
						Kind:          AiopsTransportProcessKindTool,
						Status:        AiopsTransportProcessStatusCompleted,
						Text:          "读取 Coroot 指标",
						Source:        "coroot.service_metrics",
						ToolCallID:    "call-coroot-1",
						InputSummary:  "service:checkout",
						OutputPreview: "p95 latency high",
						TargetSummary: "service:checkout",
						EvidenceRefs:  []string{"evidence-coroot-1"},
					},
					{
						ID:         "approval-1",
						Kind:       AiopsTransportProcessKindApproval,
						Status:     AiopsTransportProcessStatusBlocked,
						Text:       "确认是否执行修复命令",
						ApprovalID: "approval-1",
					},
					{
						ID:     "final-1",
						Kind:   AiopsTransportProcessKindAssistant,
						Status: AiopsTransportProcessStatusCompleted,
						Text:   "等待审批后继续",
					},
				},
			},
		},
	}

	run := BuildAgentRunViewFromTransportState(state)

	if run == nil {
		t.Fatal("BuildAgentRunViewFromTransportState() = nil, want read model")
	}
	if got, want := len(run.Steps), 5; got != want {
		t.Fatalf("len(run.Steps) = %d, want %d: %#v", got, want, run.Steps)
	}
	if run.Steps[1].Kind != AgentStepKindToolSearch || run.Steps[1].ToolName != "tool_search" {
		t.Fatalf("search step = %#v, want tool_search step", run.Steps[1])
	}
	if run.Steps[2].Kind != AgentStepKindToolCall ||
		run.Steps[2].ToolName != "coroot.service_metrics" ||
		run.Steps[2].ToolCallID != "call-coroot-1" ||
		run.Steps[2].TargetRefs[0] != "service:checkout" ||
		run.Steps[2].EvidenceRefs[0] != "evidence-coroot-1" {
		t.Fatalf("tool step = %#v, want tool call with target/evidence refs", run.Steps[2])
	}
	if run.Steps[3].Kind != AgentStepKindApproval ||
		run.Steps[3].Status != AgentStepStatusWaitingApproval ||
		run.Steps[3].ApprovalID != "approval-1" ||
		run.Steps[3].CheckpointID != "checkpoint-approval-1" {
		t.Fatalf("approval step = %#v, want waiting approval linked to checkpoint", run.Steps[3])
	}
	if run.Steps[4].Kind != AgentStepKindFinalResponse {
		t.Fatalf("final step = %#v, want final response step", run.Steps[4])
	}
}

func TestAgentRunProjectionDoesNotRequireSeparateRuntimeID(t *testing.T) {
	state := AiopsTransportState{
		CurrentTurnID: "turn-existing",
		OpsRun: &AiopsTransportOpsRun{
			ID:     "opsrun-existing",
			TurnID: "turn-existing",
			Status: "completed",
			Title:  "一次普通运维问答",
			AgentRun: &AgentRunView{
				ID: "should-be-overwritten-from-existing-opsrun",
			},
		},
		Turns: map[string]AiopsTransportTurn{
			"turn-existing": {
				ID: "turn-existing",
				User: &AiopsTransportMessage{
					Text: "一次普通运维问答",
				},
			},
		},
	}

	run := BuildAgentRunViewFromTransportState(state)

	if run == nil {
		t.Fatal("BuildAgentRunViewFromTransportState() = nil, want read model")
	}
	if run.ID != "opsrun-existing" {
		t.Fatalf("run.ID = %q, want existing ops run id", run.ID)
	}
	if run.RootTurnID != "turn-existing" || run.ActiveTurnID != "turn-existing" {
		t.Fatalf("run turn refs = %q/%q, want existing turn id", run.RootTurnID, run.ActiveTurnID)
	}
}

func TestAgentRunProjectionMarksWebLearnSkippedAndContinues(t *testing.T) {
	state := AiopsTransportState{
		CurrentTurnID: "turn-weblearn",
		OpsRun: &AiopsTransportOpsRun{
			ID:     "opsrun-weblearn",
			TurnID: "turn-weblearn",
			Status: "completed",
			Title:  "排查未知中间件异常",
		},
		Turns: map[string]AiopsTransportTurn{
			"turn-weblearn": {
				ID: "turn-weblearn",
				User: &AiopsTransportMessage{
					Text: "排查未知中间件异常",
				},
				Process: []AiopsProcessBlock{
					{
						ID:            "weblearn-1",
						Kind:          AiopsTransportProcessKindSearch,
						Status:        AiopsTransportProcessStatusSkipped,
						Text:          "WebLearn skipped",
						Source:        "weblearn",
						OutputPreview: "network_unavailable; continue normal analysis",
					},
					{
						ID:            "final-1",
						Kind:          AiopsTransportProcessKindAssistant,
						Status:        AiopsTransportProcessStatusCompleted,
						Text:          "继续基于本地证据分析。",
						OutputPreview: "继续基于本地证据分析。",
					},
				},
			},
		},
	}

	run := BuildAgentRunViewFromTransportState(state)

	if run == nil {
		t.Fatal("BuildAgentRunViewFromTransportState() = nil, want read model")
	}
	if len(run.Steps) != 2 {
		t.Fatalf("steps = %#v, want WebLearn step and final response", run.Steps)
	}
	if run.Steps[0].Kind != AgentStepKindToolSearch ||
		run.Steps[0].Status != AgentStepStatusSkipped ||
		run.Steps[0].ToolName != "weblearn" {
		t.Fatalf("weblearn step = %#v, want skipped tool search step", run.Steps[0])
	}
	if run.Steps[1].Kind != AgentStepKindFinalResponse || run.Steps[1].Status != AgentStepStatusCompleted {
		t.Fatalf("final step = %#v, want completed continuation", run.Steps[1])
	}
}
