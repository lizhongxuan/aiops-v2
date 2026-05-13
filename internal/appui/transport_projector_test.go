package appui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestTransportProjectorProjectsStructuredTurnItems(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")

	planData := json.RawMessage(`{
		"title":"排查计划",
		"steps":[
			{"id":"inspect","text":"Inspect payment-api logs","status":"completed"},
			{"id":"rollback","text":"Prepare rollback command","status":"running"}
		]
	}`)
	searchData := json.RawMessage(`{
		"toolCallId":"search-1",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"payment-api 5xx",
		"outputSummary":"found 2 results",
		"outputPreview":{"results":[
			{"title":"Error budget burn","url":"https://example.com/burn","snippet":"budget burn detected"},
			{"title":"Prometheus panel","url":"https://example.com/prom","snippet":"5xx raised"}
		]}
	}`)
	commandData := json.RawMessage(`{
		"toolCallId":"cmd-1",
		"toolName":"exec_command",
		"displayKind":"command",
		"inputSummary":"kubectl rollout undo deployment/payment-api -n prod",
		"outputSummary":"rollout undo started",
		"exitCode":0,
		"durationMs":2300
	}`)
	evidenceData := json.RawMessage(`{
		"id":"metric-1",
		"kind":"metric",
		"title":"5xx rate",
		"summary":"payment-api 5xx increased",
		"source":"prometheus",
		"confidence":"high",
		"window":"15m",
		"rawRef":"promql:5xx"
	}`)
	approvalData := json.RawMessage(`{
		"approvalId":"approval-1",
		"approvalType":"command",
		"command":"kubectl rollout undo deployment/payment-api -n prod",
		"reason":"high risk action"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(5 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "排查 payment-api 5xx"}, CreatedAt: now},
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "analyzing rollout telemetry"}, CreatedAt: now.Add(500 * time.Millisecond)},
			{ID: "plan-1", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "排查计划", Data: planData}, CreatedAt: now.Add(time.Second)},
			{ID: "search-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "payment-api 5xx", Data: searchData}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "cmd-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "rollback command", Data: commandData}, CreatedAt: now.Add(3 * time.Second)},
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "payment-api 5xx increased", Data: evidenceData}, CreatedAt: now.Add(4 * time.Second)},
			{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "需要审批", Data: approvalData}, CreatedAt: now.Add(5 * time.Second)},
			{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "等待审批完成后执行回滚"}, CreatedAt: now.Add(6 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.CurrentTurnID != "turn-1" {
		t.Fatalf("CurrentTurnID = %q, want turn-1", projected.CurrentTurnID)
	}
	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusBlocked)
	}
	if len(projected.TurnOrder) != 1 || projected.TurnOrder[0] != "turn-1" {
		t.Fatalf("TurnOrder = %#v, want [turn-1]", projected.TurnOrder)
	}

	transportTurn := projected.Turns["turn-1"]
	if transportTurn.User == nil || transportTurn.User.Text != "排查 payment-api 5xx" {
		t.Fatalf("turn.User = %+v, want projected user message", transportTurn.User)
	}
	if transportTurn.Status != AiopsTransportTurnStatusBlocked {
		t.Fatalf("turn.Status = %q, want %q", transportTurn.Status, AiopsTransportTurnStatusBlocked)
	}
	if transportTurn.Final == nil || transportTurn.Final.Text != "等待审批完成后执行回滚" {
		t.Fatalf("turn.Final = %+v, want final text", transportTurn.Final)
	}
	if len(transportTurn.Process) != 7 {
		t.Fatalf("len(turn.Process) = %d, want 7", len(transportTurn.Process))
	}

	reasoningBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindReasoning)
	if reasoningBlock.Text != "analyzing rollout telemetry" {
		t.Fatalf("reasoning block = %+v, want model call summary", reasoningBlock)
	}

	planBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindPlan)
	if len(planBlock.Steps) != 2 || planBlock.Steps[1].Status != "running" {
		t.Fatalf("plan steps = %+v, want preserved plan steps", planBlock.Steps)
	}

	searchBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindSearch)
	if len(searchBlock.Queries) != 1 || searchBlock.Queries[0] != "payment-api 5xx" {
		t.Fatalf("search queries = %#v, want input summary", searchBlock.Queries)
	}
	if len(searchBlock.Results) != 2 || searchBlock.Results[0].Title != "Error budget burn" {
		t.Fatalf("search results = %#v, want decoded results", searchBlock.Results)
	}

	commandBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "kubectl rollout undo deployment/payment-api -n prod" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.Command == "exec_command" {
		t.Fatal("command block should not expose tool name as user-visible command")
	}
	if commandBlock.OutputPreview != "" {
		t.Fatalf("command block output = %q, want no preview without explicit outputPreview", commandBlock.OutputPreview)
	}
	if commandBlock.ExitCode == nil || *commandBlock.ExitCode != 0 {
		t.Fatalf("command exit code = %#v, want 0", commandBlock.ExitCode)
	}
	if commandBlock.DurationMs != 2300 {
		t.Fatalf("command duration = %d, want 2300", commandBlock.DurationMs)
	}

	evidenceBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindEvidence)
	if evidenceBlock.Source != "prometheus" || evidenceBlock.Confidence != "high" || evidenceBlock.Window != "15m" || evidenceBlock.RawRef != "promql:5xx" {
		t.Fatalf("evidence block = %+v, want source/confidence/window/rawRef", evidenceBlock)
	}

	approvalBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindApproval)
	if approvalBlock.ApprovalID != "approval-1" || approvalBlock.Status != AiopsTransportProcessStatusBlocked {
		t.Fatalf("approval block = %+v, want blocked approval", approvalBlock)
	}
	assistantBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindAssistant)
	if assistantBlock.Text != "等待审批完成后执行回滚" || assistantBlock.DisplayKind != "assistant.final" {
		t.Fatalf("assistant final block = %+v, want inline final answer block", assistantBlock)
	}
	if _, ok := projected.PendingApprovals["approval-1"]; !ok {
		t.Fatalf("PendingApprovals = %#v, want approval-1", projected.PendingApprovals)
	}
}

func TestTransportProjectorPreservesCommandWhenToolResultOnlyHasOutput(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 38, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolCallData := json.RawMessage(`{
		"id":"call-uptime",
		"name":"exec_command",
		"arguments":{"command":"uptime"}
	}`)
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-uptime",
		"toolName":"exec_command"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-command-output",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "exec_command", Data: toolCallData}, CreatedAt: now},
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "22:38 up 22 days, 8:23, 1 user", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-command-output"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "uptime" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.OutputPreview != "" {
		t.Fatalf("command block output = %q, want no preview when tool result only has summary", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorBackfillsCommandPreviewFromSnapshotToolResult(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 39, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	fullOutput := "PID PPID %MEM RSS STAT COMM\n1 0 0.1 1024 S launchd\n2 1 1.3 204800 S Google Chrome Helper"
	toolCallData := json.RawMessage(`{
		"id":"call-ps",
		"name":"exec_command",
		"arguments":{"command":"ps","args":["-axo","pid,ppid,%mem,rss,state,comm"]}
	}`)
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-ps",
		"toolName":"exec_command",
		"outputSummary":"PID PPID %MEM RSS STAT COMM"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-command-preview",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTime(now.Add(2 * time.Second)),
		Iterations: []runtimekernel.IterationState{
			{
				ID:          "iter-1",
				SessionID:   "session-1",
				TurnID:      "turn-command-preview",
				Iteration:   0,
				Lifecycle:   runtimekernel.TurnLifecycleCompleted,
				ResumeState: runtimekernel.TurnResumeStateNone,
				ToolResults: []runtimekernel.ToolResult{{ToolCallID: "call-ps", Content: fullOutput}},
				StartedAt:   now,
				UpdatedAt:   now.Add(2 * time.Second),
			},
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "exec_command", Data: toolCallData}, CreatedAt: now},
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "PID PPID %MEM RSS STAT COMM", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-command-preview"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "ps -axo pid,ppid,%mem,rss,state,comm" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if !strings.Contains(commandBlock.OutputPreview, "launchd") || !strings.Contains(commandBlock.OutputPreview, "Google Chrome Helper") {
		t.Fatalf("command block output preview = %q, want full multi-line preview", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorKeepsCommandTitleSeparateFromResultOnlyOutput(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 40, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-cpu-brand",
		"toolName":"exec_command",
		"inputSummary":"sysctl -n machdep.cpu.brand_string",
		"outputSummary":"Apple M5",
		"outputPreview":"Apple M5"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-result-only-command",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "Apple M5", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-result-only-command"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "sysctl -n machdep.cpu.brand_string" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.Text == "Apple M5" {
		t.Fatalf("command block text = %q, should not use stdout as title", commandBlock.Text)
	}
	if commandBlock.OutputPreview != "Apple M5" {
		t.Fatalf("command block output = %q, want stdout in output preview", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorProjectsSnapshotPendingApprovalWithoutApprovalItem(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 42, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-pending-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看系统版本"}, CreatedAt: now},
		},
		PendingApprovals: []runtimekernel.PendingApproval{
			{
				ID:         "approval-inline-1",
				SessionID:  "session-1",
				TurnID:     "turn-pending-approval",
				ToolName:   "exec_command",
				ToolCallID: "call-sw-vers",
				Command:    "sw_vers",
				Reason:     "需要确认后执行命令",
				Status:     "pending",
				CreatedAt:  now.Add(time.Second),
				UpdatedAt:  now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("projected.Status = %q, want blocked", projected.Status)
	}
	if _, ok := projected.PendingApprovals["approval-inline-1"]; !ok {
		t.Fatalf("PendingApprovals = %#v, want snapshot approval", projected.PendingApprovals)
	}
	approval := projected.PendingApprovals["approval-inline-1"]
	if approval.Type != "command" || approval.Command != "sw_vers" || approval.Reason != "需要确认后执行命令" {
		t.Fatalf("approval = %+v, want command and reason from snapshot", approval)
	}
	block := findTransportProcessBlock(t, projected.Turns["turn-pending-approval"].Process, AiopsTransportProcessKindApproval)
	if block.ApprovalID != "approval-inline-1" || block.Command != "sw_vers" || block.Status != AiopsTransportProcessStatusBlocked {
		t.Fatalf("approval block = %+v, want inline blocked approval block", block)
	}
}

func TestTransportProjectorProjectsSnapshotPendingEvidenceAsInlineApproval(t *testing.T) {
	now := time.Date(2026, 5, 8, 1, 12, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolCallData := json.RawMessage(`{
		"toolCallId":"call-ifconfig-down",
		"toolName":"exec_command",
		"inputSummary":"ifconfig en0 down"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-pending-evidence",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "运行 ifconfig en0 down"}, CreatedAt: now},
			{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "evidence required", Data: toolCallData}, CreatedAt: now.Add(time.Second)},
		},
		PendingEvidence: []runtimekernel.PendingEvidence{
			{
				ID:         "evidence-inline-1",
				SessionID:  "session-1",
				TurnID:     "turn-pending-evidence",
				ToolName:   "exec_command",
				ToolCallID: "call-ifconfig-down",
				Reason:     "需要确认后执行命令",
				Status:     "pending",
				CreatedAt:  now.Add(time.Second),
				UpdatedAt:  now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("projected.Status = %q, want blocked", projected.Status)
	}
	approval := projected.PendingApprovals["evidence-inline-1"]
	if approval.ID == "" || approval.Type != "command" || approval.Command != "ifconfig en0 down" {
		t.Fatalf("evidence approval = %+v, want command approval projection", approval)
	}
	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-pending-evidence"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.ApprovalID != "evidence-inline-1" {
		t.Fatalf("command block approvalId = %q, want evidence-inline-1", commandBlock.ApprovalID)
	}
}

func TestTransportProjectorPrunesStalePendingApprovalsForTurn(t *testing.T) {
	now := time.Date(2026, 5, 8, 2, 10, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	state.PendingApprovals["stale-approval"] = AiopsTransportApproval{
		ID:     "stale-approval",
		TurnID: "turn-stale-approval",
		Type:   "command",
		Status: string(AiopsTransportProcessStatusBlocked),
	}
	state.RuntimeLiveness.PendingApprovals["stale-approval"] = true
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-stale-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(time.Second)),
		FinalOutput: "approval no longer pending",
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if _, ok := projected.PendingApprovals["stale-approval"]; ok {
		t.Fatalf("stale approval was not pruned: %#v", projected.PendingApprovals)
	}
	if projected.RuntimeLiveness.PendingApprovals["stale-approval"] {
		t.Fatalf("stale approval liveness was not pruned: %#v", projected.RuntimeLiveness.PendingApprovals)
	}
}

func TestTransportProjectorIgnoresSnapshotPendingGatesForTerminalTurn(t *testing.T) {
	now := time.Date(2026, 5, 8, 3, 10, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-denied-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleFailed,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		Error:       "approval denied",
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-stale",
			TurnID:    "turn-denied-approval",
			Command:   "ifconfig en0 down",
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}},
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-stale",
			TurnID:     "turn-denied-approval",
			ToolCallID: "tool-call-1",
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "approval-item-stale",
				Type:      agentstate.TurnItemTypeApproval,
				Status:    agentstate.ItemStatusBlocked,
				Payload:   agentstate.PayloadEnvelope{Summary: "等待审批"},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if len(projected.PendingApprovals) != 0 {
		t.Fatalf("PendingApprovals = %#v, want terminal turn pending gates ignored", projected.PendingApprovals)
	}
	if len(projected.RuntimeLiveness.PendingApprovals) != 0 {
		t.Fatalf("RuntimeLiveness.PendingApprovals = %#v, want cleared", projected.RuntimeLiveness.PendingApprovals)
	}
	for _, block := range projected.Turns["turn-denied-approval"].Process {
		if block.Kind == AiopsTransportProcessKindApproval && block.Status == AiopsTransportProcessStatusBlocked {
			t.Fatalf("terminal turn kept blocked approval block: %+v", block)
		}
	}
}

func TestTransportProjectorProjectsFailedTurnState(t *testing.T) {
	now := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-failed",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleFailed,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		Error:       "command failed: exit status 1",
		AgentItems: []agentstate.TurnItem{
			{ID: "err-1", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: "command failed: exit status 1"}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusFailed {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusFailed)
	}
	if projected.LastError != "command failed: exit status 1" {
		t.Fatalf("LastError = %q, want runtime error", projected.LastError)
	}
	if projected.Turns["turn-failed"].Status != AiopsTransportTurnStatusFailed {
		t.Fatalf("turn status = %q, want failed", projected.Turns["turn-failed"].Status)
	}
}

func TestTransportProjectorProjectsCanceledTurnState(t *testing.T) {
	now := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-canceled",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCanceled,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusCanceled {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusCanceled)
	}
	if projected.Turns["turn-canceled"].Status != AiopsTransportTurnStatusCanceled {
		t.Fatalf("turn status = %q, want canceled", projected.Turns["turn-canceled"].Status)
	}
}

func TestTransportProjectorUsesFinalOutputWhenFinalItemIsMissing(t *testing.T) {
	now := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-output",
		SessionID:   "session-1",
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(2 * time.Second)),
		FinalOutput: "这是来自 runtime snapshot 的最终回答",
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "ping"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-final-output"]
	if projected.Status != AiopsTransportStatusIdle || transportTurn.Status != AiopsTransportTurnStatusCompleted {
		t.Fatalf("projected status = %q turn=%q, want idle/completed", projected.Status, transportTurn.Status)
	}
	if transportTurn.Final == nil || transportTurn.Final.Text != "这是来自 runtime snapshot 的最终回答" {
		t.Fatalf("turn.Final = %+v, want FinalOutput fallback", transportTurn.Final)
	}
	if projected.RuntimeLiveness.ActiveTurns["turn-final-output"] {
		t.Fatalf("ActiveTurns = %#v, want terminal turn inactive", projected.RuntimeLiveness.ActiveTurns)
	}
}

func TestTransportProjectorUsesStreamingFinalOutputOverRunningItemSummary(t *testing.T) {
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-streaming-final",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		FinalOutput: "第一段第二段完整流式输出",
		AgentItems: []agentstate.TurnItem{
			{ID: "final-running", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "第一段"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	final := projected.Turns["turn-streaming-final"].Final
	if final == nil || final.Text != "第一段第二段完整流式输出" {
		t.Fatalf("turn.Final = %+v, want full streaming FinalOutput", final)
	}
	if final.Status != AiopsTransportFinalStatusRunning {
		t.Fatalf("final status = %q, want running", final.Status)
	}
	assistantBlock := findTransportProcessBlock(t, projected.Turns["turn-streaming-final"].Process, AiopsTransportProcessKindAssistant)
	if assistantBlock.Text != "第一段第二段完整流式输出" {
		t.Fatalf("assistant final block text = %q, want full streaming FinalOutput", assistantBlock.Text)
	}
}

func TestTransportProjectorReordersProcessFromLatestAgentItems(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	searchData := json.RawMessage(`{
		"toolCallId":"search-1",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"BTC current price USD 24h change"
	}`)
	firstSnapshot := &runtimekernel.TurnSnapshot{
		ID:        "turn-process-order",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "search-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "BTC current price USD 24h change", Data: searchData}, CreatedAt: now.Add(time.Second)},
		},
	}
	projected, err := projector.ProjectTurnSnapshot(state, firstSnapshot)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot(first) error = %v", err)
	}
	if len(projected.Turns["turn-process-order"].Process) != 1 || projected.Turns["turn-process-order"].Process[0].Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("first process = %#v, want only search", projected.Turns["turn-process-order"].Process)
	}

	secondSnapshot := *firstSnapshot
	secondSnapshot.UpdatedAt = now.Add(2 * time.Second)
	secondSnapshot.AgentItems = []agentstate.TurnItem{
		{ID: "final-prelude", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "我将先用实时网页搜索获取当前BTC价格、24小时涨跌与主要来源报价，并据此给你一个简明行情摘要。"}, CreatedAt: now.Add(500 * time.Millisecond)},
		{ID: "search-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "BTC current price USD 24h change", Data: searchData}, CreatedAt: now.Add(time.Second)},
	}
	projected, err = projector.ProjectTurnSnapshot(projected, &secondSnapshot)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot(second) error = %v", err)
	}

	process := projected.Turns["turn-process-order"].Process
	if len(process) != 2 {
		t.Fatalf("len(process) = %d, want assistant and search: %#v", len(process), process)
	}
	if process[0].Kind != AiopsTransportProcessKindAssistant || process[0].Text == "" {
		t.Fatalf("process[0] = %+v, want assistant prelude", process[0])
	}
	if process[1].Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("process[1] = %+v, want search after assistant", process[1])
	}
}

func TestTransportProjectorDedupesProviderNativeWebSearchBlocks(t *testing.T) {
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	searchCallData := json.RawMessage(`{
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"2026-05-07 A股 行情"
	}`)
	searchResultData := json.RawMessage(`{
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"2026-05-07 A股 行情",
		"outputSummary":"{\"content\":\"provider-native web_search completed for query \\\"2026-05-07 A股 行情\\\"; provider returned no textual summary and public fallback found no relevant result.\"}"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-search",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "search-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "2026-05-07 A股 行情", Data: searchCallData}, CreatedAt: now},
			{ID: "search-result-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "2026-05-07 A股 行情", Data: searchResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	process := projected.Turns["turn-search"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want 1 deduped search block: %#v", len(process), process)
	}
	block := process[0]
	if block.DisplayKind != "web_search" {
		t.Fatalf("DisplayKind = %q, want web_search", block.DisplayKind)
	}
	if block.Text != "2026-05-07 A股 行情" || block.OutputPreview != "2026-05-07 A股 行情" {
		t.Fatalf("block text/output = %q/%q, want cleaned query", block.Text, block.OutputPreview)
	}
	if block.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("Status = %q, want completed", block.Status)
	}
}

func TestTransportProjectorExtractsRuntimeToolCallQueryAndMergesSearchResult(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 31, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	query := "2026-05-07 BTC price now 24h change market cap official exchange or market data"
	searchCallData := json.RawMessage(`{
		"id":"call-search-1",
		"name":"web_search",
		"arguments":{"query":"` + query + `"}
	}`)
	searchResultSummary := `{"content":"provider-native web_search completed for query \"` + query + `\"; provider returned no textual summary and public fallback found no relevant result. Do not repeat this exact query; refine with more specific entities, dates, or authoritative domains, or answer with explicit limitations if evidence is sufficient."}`
	searchResultData := json.RawMessage(`{
		"toolCallId":"call-search-1",
		"toolName":"web_search"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-runtime-search",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "turn-runtime-search-tool-call",
				Type:      agentstate.TurnItemTypeToolCall,
				Status:    agentstate.ItemStatusRunning,
				Payload:   agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "web_search", Data: searchCallData},
				CreatedAt: now,
			},
			{
				ID:        "turn-runtime-search-tool-result",
				Type:      agentstate.TurnItemTypeToolResult,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Kind: "browser.search", Summary: searchResultSummary, Data: searchResultData},
				CreatedAt: now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	process := projected.Turns["turn-runtime-search"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want 1 merged search block: %#v", len(process), process)
	}
	block := process[0]
	if block.Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("Kind = %q, want search", block.Kind)
	}
	if block.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("Status = %q, want completed", block.Status)
	}
	if block.InputSummary != query {
		t.Fatalf("InputSummary = %q, want %q", block.InputSummary, query)
	}
	if len(block.Queries) != 1 || block.Queries[0] != query {
		t.Fatalf("Queries = %#v, want [%q]", block.Queries, query)
	}
	if block.Text != query {
		t.Fatalf("Text = %q, want %q", block.Text, query)
	}
}

func ptrTransportProjectorTime(value time.Time) *time.Time {
	return &value
}

func findTransportProcessBlock(t *testing.T, blocks []AiopsProcessBlock, kind AiopsTransportProcessKind) AiopsProcessBlock {
	t.Helper()
	for _, block := range blocks {
		if block.Kind == kind {
			return block
		}
	}
	t.Fatalf("missing process block kind %q in %+v", kind, blocks)
	return AiopsProcessBlock{}
}

func TestIsHTMLContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"DOCTYPE uppercase", "<!DOCTYPE html>", true},
		{"html lowercase tag", "<html><body></body></html>", true},
		{"HTML uppercase tag", "<HTML><BODY></BODY></HTML>", true},
		{"leading whitespace with DOCTYPE", "  \t\n<!DOCTYPE html>", true},
		{"leading whitespace with html tag", "   <html>", true},
		{"leading whitespace with HTML tag", "\n\n  <HTML>", true},
		{"plain text", "hello world", false},
		{"empty string", "", false},
		{"partial match", "<ht", false},
		{"json content", `{"key":"value"}`, false},
		{"markdown content", "# Title\n\nSome text", false},
		{"xml but not html", "<?xml version=\"1.0\"?>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHTMLContent(tt.input)
			if got != tt.want {
				t.Errorf("isHTMLContent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeOutputPreview(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"short plain text", "hello world", "hello world"},
		{"plain text under 500 runes", "short content", "short content"},
		{"plain text exactly 500 runes", string(make([]rune, 500)), string(make([]rune, 500))},
		{"plain text over 500 runes truncated", func() string {
			r := make([]rune, 510)
			for i := range r {
				r[i] = 'a'
			}
			return string(r)
		}(), func() string {
			r := make([]rune, 500)
			for i := range r {
				r[i] = 'a'
			}
			return string(r) + "…"
		}()},
		{"HTML content stripped and under 200 runes", "<!DOCTYPE html><html><body><p>Hello</p></body></html>", "Hello"},
		{"HTML content stripped and over 200 runes truncated", func() string {
			// Build HTML with >200 runes of text content
			r := make([]rune, 250)
			for i := range r {
				r[i] = '中'
			}
			return "<!DOCTYPE html><html><body><p>" + string(r) + "</p></body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = '中'
			}
			return string(r) + "…"
		}()},
		{"HTML content stripped exactly 200 runes", func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = 'x'
			}
			return "<html><body>" + string(r) + "</body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = 'x'
			}
			return string(r)
		}()},
		{"multi-byte rune-aware truncation for non-HTML", func() string {
			r := make([]rune, 510)
			for i := range r {
				r[i] = '日'
			}
			return string(r)
		}(), func() string {
			r := make([]rune, 500)
			for i := range r {
				r[i] = '日'
			}
			return string(r) + "…"
		}()},
		{"multi-byte rune-aware truncation for HTML", func() string {
			r := make([]rune, 210)
			for i := range r {
				r[i] = '本'
			}
			return "<html><body>" + string(r) + "</body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = '本'
			}
			return string(r) + "…"
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOutputPreview(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeOutputPreview() got len=%d, want len=%d", len([]rune(got)), len([]rune(tt.want)))
				if len(got) < 100 && len(tt.want) < 100 {
					t.Errorf("  got=%q, want=%q", got, tt.want)
				}
			}
		})
	}
}

func TestTransportProjectorSanitizesHTMLInToolOutput(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-html", "thread-html")

	rawHTML := `<!DOCTYPE html><html><body><h1>Title</h1><p>Some content here</p></body></html>`
	toolResultData := json.RawMessage(`{
		"toolCallId":"tool-html-1",
		"toolName":"fetch_page",
		"displayKind":"tool",
		"inputSummary":"https://example.com",
		"outputSummary":"` + rawHTML + `",
		"outputPreview":"` + rawHTML + `"
	}`)

	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-html",
		SessionID: "session-html",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-html",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "tool",
					Summary: rawHTML,
					Data:    toolResultData,
				},
				CreatedAt: now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-html"]
	if len(transportTurn.Process) == 0 {
		t.Fatal("expected at least one process block")
	}

	block := transportTurn.Process[0]

	// OutputPreview must not contain raw HTML tags
	if strings.Contains(block.OutputPreview, "<") || strings.Contains(block.OutputPreview, ">") {
		t.Errorf("OutputPreview contains raw HTML tags: %q", block.OutputPreview)
	}
	// Text must not contain raw HTML tags
	if strings.Contains(block.Text, "<") || strings.Contains(block.Text, ">") {
		t.Errorf("Text contains raw HTML tags: %q", block.Text)
	}

	// Verify the text content is preserved (stripped of tags)
	if !strings.Contains(block.OutputPreview, "Title") || !strings.Contains(block.OutputPreview, "Some content here") {
		t.Errorf("OutputPreview should contain stripped text content, got: %q", block.OutputPreview)
	}
	if !strings.Contains(block.Text, "Title") || !strings.Contains(block.Text, "Some content here") {
		t.Errorf("Text should contain stripped text content, got: %q", block.Text)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"plain text no tags", "hello world", "hello world"},
		{"simple paragraph", "<p>hello</p>", "hello"},
		{"nested tags", "<div><p>hello</p></div>", "hello"},
		{"multiple elements", "<h1>Title</h1><p>Body text</p>", "Title Body text"},
		{"self-closing tags", "before<br/>after", "before after"},
		{"attributes in tags", `<a href="https://example.com">link</a>`, "link"},
		{"full HTML document", "<!DOCTYPE html><html><head><title>Test</title></head><body><p>Content</p></body></html>", "Test Content"},
		{"whitespace between tags", "<p>  hello  </p>  <p>  world  </p>", "hello world"},
		{"newlines and tabs", "<div>\n\t<span>text</span>\n</div>", "text"},
		{"only tags no content", "<br/><hr/><img src='x'/>", ""},
		{"mixed content and tags", "Start <b>bold</b> middle <i>italic</i> end", "Start bold middle italic end"},
		{"tag with multiline attributes", "<div\n  class=\"foo\"\n  id=\"bar\">content</div>", "content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
