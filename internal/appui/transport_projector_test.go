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

func TestTransportProjectorProjectsToolMockAndEvidenceRefs(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-metrics",
		"toolName":"coroot.service_metrics",
		"displayKind":"coroot.metrics",
		"inputSummary":"redis-local-01 memory",
		"outputSummary":"rss/used_memory ratio is 1.8",
		"mock":true,
		"evidenceRefs":["evidence:redis:rss","evidence:redis:events"]
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-tool-evidence",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "rss/used_memory ratio is 1.8", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	toolBlock := findTransportProcessBlock(t, projected.Turns["turn-tool-evidence"].Process, AiopsTransportProcessKindTool)
	if toolBlock.Source != "coroot.service_metrics" || !strings.Contains(toolBlock.InputSummary, "redis-local-01 memory") {
		t.Fatalf("tool block source/input = %q/%q, want tool name and input summary", toolBlock.Source, toolBlock.InputSummary)
	}
	if !toolBlock.Mock {
		t.Fatalf("tool block Mock = false, want true: %+v", toolBlock)
	}
	if got, want := strings.Join(toolBlock.EvidenceRefs, ","), "evidence:redis:rss,evidence:redis:events"; got != want {
		t.Fatalf("tool block EvidenceRefs = %q, want %q", got, want)
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

func TestTransportProjectorRendersOpsManualPreflightArtifact(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-preflight", "thread-preflight")
	preflightPayload, _ := json.Marshal(map[string]any{
		"status":      "blocked",
		"ready":       false,
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"reason":      "preflight probe permission is missing",
		"next_action": "request_permission",
		"missing_permissions": []string{
			"redis-readonly-probe",
		},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-preflight",
		"toolName":"run_ops_manual_preflight",
		"displayKind":"ops_manual_preflight_result",
		"outputPreview":` + string(preflightPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-preflight",
		SessionID: "session-preflight",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-preflight",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_preflight_result",
					Summary: "blocked",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-preflight"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_preflight_result" {
		t.Fatalf("artifacts = %#v, want one ops_manual_preflight_result", artifacts)
	}
	if artifacts[0].Status != "blocked" || artifacts[0].Severity != "warning" {
		t.Fatalf("artifact = %#v, want blocked warning", artifacts[0])
	}
	if len(artifacts[0].Actions) != 1 || artifacts[0].Actions[0]["id"] != "request_permission" {
		t.Fatalf("actions = %#v, want request_permission", artifacts[0].Actions)
	}
}

func TestTransportProjectorRendersOpsManualPreflightPassedConfirmationAction(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-preflight-passed", "thread-preflight-passed")
	preflightPayload, _ := json.Marshal(map[string]any{
		"status":      "passed",
		"ready":       true,
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"next_action": "confirm_execution",
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-preflight-passed",
		"toolName":"run_ops_manual_preflight",
		"displayKind":"ops_manual_preflight_result",
		"outputPreview":` + string(preflightPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-preflight-passed",
		SessionID: "session-preflight-passed",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-preflight-passed",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_preflight_result",
					Summary: "passed",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-preflight-passed"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_preflight_result" {
		t.Fatalf("artifacts = %#v, want one ops_manual_preflight_result", artifacts)
	}
	if actions := artifacts[0].Actions; len(actions) != 1 || actions[0]["id"] != "confirm_execution" {
		t.Fatalf("preflight passed actions = %#v, want confirm_execution", actions)
	}
}

func TestTransportProjectorRendersOpsManualParamResolutionArtifact(t *testing.T) {
	now := time.Date(2026, 5, 17, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-param", "thread-param")
	payload, _ := json.Marshal(map[string]any{
		"status":      "ambiguous",
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"fields": []map[string]any{{
			"id":         "target_instance",
			"label":      "Redis 实例",
			"type":       "resource_ref",
			"ui_control": "select",
		}},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-param-resolution",
		"toolName":"resolve_ops_manual_params",
		"displayKind":"ops_manual_param_resolution",
		"outputPreview":` + string(payload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-param",
		SessionID: "session-param",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-param",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_param_resolution",
					Summary: "ambiguous",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-param"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_param_resolution" {
		t.Fatalf("artifacts = %#v, want one param resolution artifact", artifacts)
	}
	if artifacts[0].Status != "ambiguous" || artifacts[0].Severity != "warning" {
		t.Fatalf("artifact = %#v, want ambiguous warning", artifacts[0])
	}
	if len(artifacts[0].Actions) != 1 || artifacts[0].Actions[0]["id"] != "fill_params" {
		t.Fatalf("actions = %#v, want fill_params", artifacts[0].Actions)
	}
}

func TestTransportProjectorProjectsRCAReportArtifact(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-rca", "thread-rca")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-artifact-1",
		"toolName":"aiops.ui_artifact_emit",
		"displayKind":"rca_report",
		"outputPreview":{
			"schemaVersion":"aiops.agent_ui_artifact/v1",
			"type":"rca_report",
			"titleZh":"checkout 根因分析",
			"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。",
			"status":"ok",
			"severity":"high",
			"source":"coroot",
			"evidenceRef":"ev-coroot-latency",
			"permissionScope":"read",
			"redactionStatus":"redacted",
			"inlineData":{
				"schemaVersion":"aiops.rca_report/v1",
				"source":"coroot",
				"status":"ok",
				"target":{"service":"checkout"},
				"window":{"timeRange":"30m"},
				"conclusion":{"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。","confidence":0.72},
				"hypotheses":[],
				"sections":[],
				"evidenceRefs":["ev-coroot-latency"],
				"rawRefs":[]
			}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-rca",
		SessionID:   "session-rca",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-rca", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "RCA report", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-rca"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "rca_report" || artifact.Source != "coroot" || artifact.PermissionScope != "read" {
		t.Fatalf("artifact = %+v, want rca_report from coroot", artifact)
	}
	if artifact.InlineData == nil {
		t.Fatal("artifact inline data is nil")
	}
	if artifact.Metadata["evidenceRef"] != "ev-coroot-latency" {
		t.Fatalf("artifact metadata = %#v, want evidenceRef copied into metadata", artifact.Metadata)
	}
}

func TestTransportProjectorProjectsCorootServiceMetricsChartArtifact(t *testing.T) {
	now := time.Date(2026, 5, 19, 9, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-chart", "thread-coroot-chart")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics",
		"toolName":"coroot.service_metrics",
		"displayKind":"coroot",
		"inputSummary":"checkout net",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:smecloud:Deployment:web",
			"metrics":[
				{"name":"cpu","status":"ok","unit":"cores","chartTitle":"CPU usage <selector>, cores","values":[[1710000000000,0.4],[1710000030000,0.6]],"series":[{"name":"web-1","values":[[1710000000000,0.4],[1710000030000,0.6]]}]},
				{"name":"memory","status":"warning","unit":"bytes","chartTitle":"Memory usage <selector>, bytes","values":[[1710000000000,1024],[1710000030000,2048]],"series":[{"name":"web-1","values":[[1710000000000,1024],[1710000030000,2048]]}]}
			],
			"chartReports":[
				{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: web","series":[{"name":"web-1","data":[0.4,0.6]}]}]}}]},
				{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,1]}]}}]}
			],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/web","digest":"sha256:abc","bytes":1024}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-chart",
		SessionID:   "session-coroot-chart",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-coroot-metrics", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "检查 checkout 网络异常"}, CreatedAt: now},
			{ID: "tool-result-coroot-metrics", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-chart"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "coroot_chart" || artifact.Source != "coroot" || artifact.PermissionScope != "read" {
		t.Fatalf("artifact = %+v, want coroot_chart from coroot", artifact)
	}
	card := artifact.InlineData["mcpCard"].(map[string]any)
	visual := card["visual"].(map[string]any)
	series, ok := visual["series"].([]map[string]any)
	if !ok {
		t.Fatalf("series type = %T, want []map[string]any", visual["series"])
	}
	if len(series) != 2 {
		t.Fatalf("series len = %d, want cpu and memory series", len(series))
	}
	chartReports, ok := artifact.InlineData["chartReports"].([]any)
	if !ok {
		t.Fatalf("chartReports type = %T, want []any", artifact.InlineData["chartReports"])
	}
	if len(chartReports) != 2 {
		t.Fatalf("chartReports len = %d, want native CPU and Net reports", len(chartReports))
	}
	if card["visual"].(map[string]any)["kind"] != "coroot_report_charts" {
		t.Fatalf("visual kind = %#v, want coroot_report_charts", card["visual"].(map[string]any)["kind"])
	}
	if artifact.Metadata["service"] != "5hxbfx6p:smecloud:Deployment:web" || artifact.DataRef != "http://coroot/api/project/5hxbfx6p/app/web" {
		t.Fatalf("artifact metadata=%#v dataRef=%q, want service and raw ref", artifact.Metadata, artifact.DataRef)
	}
	placement, ok := artifact.Metadata["placement"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.placement type = %T, want map", artifact.Metadata["placement"])
	}
	if placement["topic"] != "net" || placement["priority"] != "primary" || placement["service"] != "5hxbfx6p:smecloud:Deployment:web" {
		t.Fatalf("metadata.placement = %#v, want root_cause/evidence net primary placement", placement)
	}
	if !transportTestStringListContains(placement["supports"], "root_cause") {
		t.Fatalf("metadata.placement.supports = %#v, want root_cause", placement["supports"])
	}
	if !transportTestStringListContains(placement["preferredAfter"], "root_cause") {
		t.Fatalf("metadata.placement.preferredAfter = %#v, want root_cause", placement["preferredAfter"])
	}
	if !transportTestStringListContains(placement["preferredBefore"], "evidence") {
		t.Fatalf("metadata.placement.preferredBefore = %#v, want evidence", placement["preferredBefore"])
	}
	chartSummary, ok := artifact.Metadata["chartSummary"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.chartSummary type = %T, want map", artifact.Metadata["chartSummary"])
	}
	if chartSummary["service"] != "5hxbfx6p:smecloud:Deployment:web" || chartSummary["defaultReportName"] != "Net" {
		t.Fatalf("metadata.chartSummary = %#v, want service and preferred Net report", chartSummary)
	}
	summaryJSON, err := json.Marshal(chartSummary)
	if err != nil {
		t.Fatalf("marshal metadata.chartSummary: %v", err)
	}
	if strings.Contains(string(summaryJSON), `"data"`) || strings.Contains(string(summaryJSON), `"values"`) {
		t.Fatalf("metadata.chartSummary leaked raw series arrays: %s", summaryJSON)
	}
}

func transportTestStringListContains(value any, want string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func TestTransportProjectorDeduplicatesCorootChartArtifactPerService(t *testing.T) {
	now := time.Date(2026, 5, 20, 2, 50, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-chart-dedupe", "thread-coroot-chart-dedupe")
	firstToolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics-24h",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"{\"service\":\"aiops-host-agent\",\"timeRange\":\"24h\",\"metrics\":[\"cpu\",\"memory\"]}",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
			"metrics":[],
			"chartReports":[{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"CPU usage","series":[{"name":"node-1","data":[0.1,0.2]}]}}]}],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=24h","digest":"sha256:24h","bytes":2048}
		}
	}`)
	secondToolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics-1h",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"{\"service\":\"aiops-host-agent\",\"timeRange\":\"1h\",\"metrics\":[\"cpu\"]}",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
			"metrics":[],
			"chartReports":[{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710003600000,"step":30000},"title":"CPU usage","series":[{"name":"node-1","data":[0.3,0.4]}]}}]}],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=1h","digest":"sha256:1h","bytes":1024}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-chart-dedupe",
		SessionID:   "session-coroot-chart-dedupe",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-coroot-metrics-24h", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics 24h", Data: firstToolResultData}, CreatedAt: now},
			{ID: "tool-result-coroot-metrics-1h", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics 1h", Data: secondToolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-chart-dedupe"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want one deduplicated coroot_chart", len(artifacts))
	}
	if artifacts[0].DataRef != "http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=1h" {
		t.Fatalf("DataRef = %q, want latest service metrics artifact", artifacts[0].DataRef)
	}
}

func TestTransportProjectorProjectsCorootChartArtifactFromRawToolResultContent(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-raw-chart", "thread-coroot-raw-chart")
	rawContent := `{
		"schemaVersion":"aiops.coroot/v1",
		"tool":"coroot.service_metrics",
		"status":"ok",
		"project":"5hxbfx6p",
		"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
		"metrics":[],
		"chartReports":[
			{"name":"Instances","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Instances","series":[{"name":"up","data":[2,2]}]}}]},
			{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: aiops-host-agent","series":[{"name":"aiops-host-agent@node-1","data":[0.0004,0.0006]}]}]}}]}
		],
		"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent","digest":"sha256:abc","bytes":4096}
	}`
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-raw-metrics",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"aiops-host-agent metrics"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-raw-chart",
		SessionID:   "session-coroot-raw-chart",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		Iterations: []runtimekernel.IterationState{{
			ID:          "iter-coroot-raw-chart",
			SessionID:   "session-coroot-raw-chart",
			TurnID:      "turn-coroot-raw-chart",
			Iteration:   0,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			ResumeState: runtimekernel.TurnResumeStateNone,
			ToolResults: []runtimekernel.ToolResult{{ToolCallID: "call-coroot-raw-metrics", Content: rawContent}},
			StartedAt:   now,
			UpdatedAt:   now,
		}},
		AgentItems: []agentstate.TurnItem{
			{ID: "user-coroot-raw-metrics", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看 aiops-host-agent 的 CPU 图表"}, CreatedAt: now},
			{ID: "tool-result-coroot-raw-metrics", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-raw-chart"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1 coroot_chart from raw tool result content", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "coroot_chart" {
		t.Fatalf("artifact.Type = %q, want coroot_chart", artifact.Type)
	}
	chartReports := artifact.InlineData["chartReports"].([]any)
	if len(chartReports) != 2 {
		t.Fatalf("chartReports len = %d, want Instances and CPU reports", len(chartReports))
	}
	if artifact.InlineData["defaultReportName"] != "CPU" {
		t.Fatalf("defaultReportName = %#v, want CPU", artifact.InlineData["defaultReportName"])
	}
	if artifact.DataRef != "http://coroot/api/project/5hxbfx6p/app/aiops-host-agent" {
		t.Fatalf("DataRef = %q, want rawRef URI", artifact.DataRef)
	}
}

func TestTransportProjectorProjectsRCAReportArtifactFromFinalPayload(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-final-rca", "thread-final-rca")
	finalPayload := `{
		"schemaVersion":"aiops.rca_report/v1",
		"status":"partial",
		"source":"coroot",
		"target":{"service":"checkout"},
		"conclusion":{"summaryZh":"checkout 延迟升高与 catalog 依赖相关。","confidence":0.72},
		"evidenceRefs":["ev-coroot-latency"],
		"rawRefs":[{"uri":"coroot://raw/latency","digest":"abc123"}]
	}`
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-rca",
		SessionID:   "session-final-rca",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		FinalOutput: finalPayload,
		StartedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
		AgentItems: []agentstate.TurnItem{
			{ID: "final-rca", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: finalPayload}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-final-rca"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "rca_report" || artifact.Status != "partial" || artifact.Source != "coroot" {
		t.Fatalf("artifact = %+v, want partial coroot rca_report", artifact)
	}
	if artifact.InlineData == nil || artifact.InlineData["schemaVersion"] != "aiops.rca_report/v1" {
		t.Fatalf("artifact inline data = %#v, want rca payload", artifact.InlineData)
	}
	if artifact.SummaryZh != "checkout 延迟升高与 catalog 依赖相关。" {
		t.Fatalf("summaryZh = %q, want conclusion summary", artifact.SummaryZh)
	}
}

func TestTransportProjectorCompactsOpsManualSearchProcessPreview(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-ops-manual-search", "thread-ops-manual-search")
	searchPayload, _ := json.Marshal(map[string]any{
		"decision": "need_info",
		"summary":  "信息不足，不能直接使用工作流。",
		"manuals": []map[string]any{
			{
				"manual": map[string]any{
					"id":    "manual-redis-rca-ssh",
					"title": "Redis SSH 排障运维手册",
				},
				"missing_fields": []string{"environment", "execution_surface", "symptom", "metrics"},
			},
		},
		"operation_frame": map[string]any{
			"evidence": map[string]any{"missing": []string{"environment", "execution_surface", "symptom", "metrics"}},
		},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-search",
		"toolName":"search_ops_manuals",
		"displayKind":"ops_manual_search_result",
		"outputPreview":` + string(searchPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ops-manual-search",
		SessionID: "session-ops-manual-search",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "need_info",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-ops-manual-search"]
	if len(transportTurn.AgentUIArtifacts) != 1 || transportTurn.AgentUIArtifacts[0].Type != "ops_manual_search_result" {
		t.Fatalf("artifacts = %#v, want ops manual search artifact", transportTurn.AgentUIArtifacts)
	}
	if len(transportTurn.Process) != 1 {
		t.Fatalf("process = %#v, want one compact tool block", transportTurn.Process)
	}
	block := transportTurn.Process[0]
	if block.OutputPreview != "" {
		t.Fatalf("output preview = %q, want hidden preview for ops manual search", block.OutputPreview)
	}
	if !strings.Contains(block.Text, "运维手册匹配：手册缺上下文") || strings.Contains(block.Text, "need_info") || strings.Contains(block.Text, "execution_surface") {
		t.Fatalf("block text = %q, want human compact decision without internal missing fields", block.Text)
	}
}

func TestTransportProjectorSkipsOpsManualNoMatchArtifact(t *testing.T) {
	now := time.Date(2026, 5, 19, 18, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-ops-manual-no-match", "thread-ops-manual-no-match")
	searchPayload, _ := json.Marshal(map[string]any{
		"decision": "no_match",
		"summary":  "没有找到适用于 service 的可用运维手册。",
		"manuals":  []map[string]any{},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-search-no-match",
		"toolName":"search_ops_manuals",
		"displayKind":"ops_manual_search_result",
		"outputPreview":` + string(searchPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ops-manual-no-match",
		SessionID: "session-ops-manual-no-match",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search-no-match",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "no_match",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-ops-manual-no-match"]
	if len(transportTurn.AgentUIArtifacts) != 0 {
		t.Fatalf("artifacts = %#v, want no Agent UI artifact for no_match ops manual search", transportTurn.AgentUIArtifacts)
	}
}

func TestOpsManualSearchReferenceOnlySummaryPromotesReadOnlyInvestigation(t *testing.T) {
	if got := opsManualSearchSummaryZh("reference_only"); !strings.Contains(got, "没有可直接运行的 Workflow") || !strings.Contains(got, "继续只读自动化排查") {
		t.Fatalf("summary = %q, want read-only continuation without runnable Workflow", got)
	}
	if actions := opsManualSearchArtifactActions("reference_only"); len(actions) != 0 {
		t.Fatalf("actions = %#v, want no executable or step-by-step action for reference_only search", actions)
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
