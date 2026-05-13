package appui

import (
	"encoding/json"
	"reflect"
	"testing"
)

func testAgentEvent(kind AgentEventKind, phase AgentEventPhase, status AgentEventStatus, seq int64, payload any) AgentEvent {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	return AgentEvent{
		EventID:    string(kind) + "-" + string(phase) + "-" + string(status),
		Seq:        seq,
		SessionID:  "session-1",
		ThreadID:   "thread-1",
		TurnID:     "turn-1",
		AgentID:    "agent-main",
		Kind:       kind,
		Phase:      phase,
		Status:     status,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  "2026-04-24T00:00:00Z",
		Payload:    raw,
	}
}

func TestAgentEventProjector_TurnRequestedStartedSetsWorking(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseRequested, AgentEventStatusQueued, 1, TurnPayload{Prompt: "hello"}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 2, TurnPayload{Title: "Working"}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if proj.Status != "working" {
		t.Fatalf("Status = %q, want working", proj.Status)
	}
	if !proj.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("ActiveTurns = %+v, want turn-1 active", proj.RuntimeLiveness.ActiveTurns)
	}
	if proj.CurrentTurnID != "turn-1" || proj.LastSeq != 2 {
		t.Fatalf("projection turn/seq = %q/%d, want turn-1/2", proj.CurrentTurnID, proj.LastSeq)
	}
	if len(proj.Timeline) != 1 {
		t.Fatalf("len(Timeline) = %d, want 1 turn row", len(proj.Timeline))
	}
	if got := proj.Timeline[0].Title; got != "hello" {
		t.Fatalf("Timeline[0].Title = %q, want requested prompt", got)
	}
	if got := proj.Timeline[0].Summary; got != "Working" {
		t.Fatalf("Timeline[0].Summary = %q, want latest turn summary", got)
	}
}

func TestAgentEventProjector_TerminalTurnEventUpdatesExistingClientTurnRow(t *testing.T) {
	projector := NewAgentEventProjector()
	now := "2026-04-24T00:00:00Z"
	requestedPayload, _ := json.Marshal(TurnPayload{
		Prompt:          "刷新后不应出现第二条用户行",
		ClientMessageID: "client-msg-1",
		ClientTurnID:    "client-turn-1",
	})
	completedPayload, _ := json.Marshal(TurnPayload{Summary: "已完成"})

	proj, err := projector.Replay("session-1", []AgentEvent{
		{
			EventID:      "turn-1:requested",
			Seq:          1,
			SessionID:    "session-1",
			TurnID:       "turn-1",
			ClientTurnID: "client-turn-1",
			Kind:         AgentEventTurn,
			Phase:        AgentEventPhaseRequested,
			Status:       AgentEventStatusQueued,
			Visibility:   AgentEventVisibilityPrimary,
			Source:       AgentEventSourceUI,
			CreatedAt:    now,
			Payload:      requestedPayload,
		},
		{
			EventID:      "turn-1:completed",
			Seq:          2,
			SessionID:    "session-1",
			TurnID:       "turn-1",
			ClientTurnID: "client-turn-1",
			Kind:         AgentEventTurn,
			Phase:        AgentEventPhaseCompleted,
			Status:       AgentEventStatusCompleted,
			Visibility:   AgentEventVisibilityPrimary,
			Source:       AgentEventSourceSystem,
			CreatedAt:    now,
			Payload:      completedPayload,
		},
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(proj.Timeline) != 1 {
		t.Fatalf("len(Timeline) = %d, want one logical turn row: %+v", len(proj.Timeline), proj.Timeline)
	}
	if got := proj.Timeline[0].ID; got != "client-msg-1" {
		t.Fatalf("Timeline[0].ID = %q, want existing client message row", got)
	}
	if got := proj.Timeline[0].Title; got != "刷新后不应出现第二条用户行" {
		t.Fatalf("Timeline[0].Title = %q, want original prompt", got)
	}
	if got := proj.Timeline[0].Status; got != AgentEventStatusCompleted {
		t.Fatalf("Timeline[0].Status = %q, want completed", got)
	}
	if got := proj.Timeline[0].Summary; got != "已完成" {
		t.Fatalf("Timeline[0].Summary = %q, want terminal summary", got)
	}
}

func TestAgentEventProjector_AgentCompletionRemovesActiveAgentButKeepsAgentRow(t *testing.T) {
	projector := NewAgentEventProjector()
	started := testAgentEvent(AgentEventAgent, AgentEventPhaseStarted, AgentEventStatusRunning, 1, AgentPayload{
		Handle:     "main",
		Name:       "Main Agent",
		Role:       "primary",
		LastAction: "reading files",
		Stats: AgentStats{
			CommandsRun: 2,
			FilesRead:   3,
			ToolsCalled: 4,
		},
	})
	completed := testAgentEvent(AgentEventAgent, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, AgentPayload{
		Handle:      "main",
		Name:        "Main Agent",
		LastSummary: "done",
	})

	proj, err := projector.Replay("session-1", []AgentEvent{started, completed})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if proj.RuntimeLiveness.ActiveAgents["agent-main"] {
		t.Fatalf("agent-main should not remain active: %+v", proj.RuntimeLiveness.ActiveAgents)
	}
	if len(proj.Agents) != 1 {
		t.Fatalf("Agents length = %d, want 1", len(proj.Agents))
	}
	if proj.Agents[0].Status != "completed" || proj.Agents[0].LastSummary != "done" {
		t.Fatalf("agent row = %+v, want completed summary", proj.Agents[0])
	}
	if proj.Agents[0].Stats.CommandsRun != 2 || proj.Agents[0].Stats.FilesRead != 3 || proj.Agents[0].Stats.ToolsCalled != 4 {
		t.Fatalf("agent stats = %+v, want latest non-zero stats from payload", proj.Agents[0].Stats)
	}
}

func TestAgentEventProjector_ToolProgressUpdatesOneTimelineRow(t *testing.T) {
	projector := NewAgentEventProjector()
	started := testAgentEvent(AgentEventTool, AgentEventPhaseStarted, AgentEventStatusRunning, 1, ToolPayload{
		ToolCallID:   "tool-1",
		ToolName:     "web_search",
		DisplayName:  "搜索网页",
		InputSummary: "A股",
	})
	updated := testAgentEvent(AgentEventTool, AgentEventPhaseUpdated, AgentEventStatusRunning, 2, ToolPayload{
		ToolCallID:    "tool-1",
		ToolName:      "web_search",
		OutputSummary: "找到 3 条",
	})
	completed := testAgentEvent(AgentEventTool, AgentEventPhaseCompleted, AgentEventStatusCompleted, 3, ToolPayload{
		ToolCallID:    "tool-1",
		ToolName:      "web_search",
		OutputSummary: "已搜索 A股",
	})

	proj, err := projector.Replay("session-1", []AgentEvent{started, updated, completed})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if proj.RuntimeLiveness.ActiveCommandStreams["tool-1"] {
		t.Fatalf("tool-1 should not remain active: %+v", proj.RuntimeLiveness.ActiveCommandStreams)
	}
	if len(proj.Timeline) != 1 {
		t.Fatalf("Timeline length = %d, want 1 row", len(proj.Timeline))
	}
	if proj.Timeline[0].Status != "completed" || proj.Timeline[0].Summary != "已搜索 A股" {
		t.Fatalf("tool timeline row = %+v, want completed summary", proj.Timeline[0])
	}
	if len(proj.ProcessGroups["turn-1"]) != 1 {
		t.Fatalf("ProcessGroups[turn-1] length = %d, want 1", len(proj.ProcessGroups["turn-1"]))
	}
}

func TestAgentEventProjector_ReplayMatchesRealtimeApplyForStructuredTurn(t *testing.T) {
	projector := NewAgentEventProjector()
	events := []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Prompt: "排查 payment-api"}),
		testAgentEvent(AgentEventPlan, AgentEventPhaseUpdated, AgentEventStatusRunning, 2, PlanPayload{
			Title: "排查计划",
			Steps: []PlanStep{{ID: "inspect", Text: "Inspect payment-api", Status: "running"}},
		}),
		testAgentEvent(AgentEventTool, AgentEventPhaseCompleted, AgentEventStatusCompleted, 3, ToolPayload{
			ToolCallID:    "search-1",
			ToolName:      "web_search",
			DisplayKind:   "browser.search",
			InputSummary:  "payment-api 5xx",
			OutputSummary: "found metrics",
		}),
		testAgentEvent(AgentEventEvidence, AgentEventPhaseCompleted, AgentEventStatusCompleted, 4, EvidencePayload{
			ID:      "metric-1",
			Kind:    "metric",
			Title:   "5xx rate",
			Summary: "payment-api 5xx increased",
			RawRef:  "promql:5xx",
		}),
		testAgentEvent(AgentEventApproval, AgentEventPhaseRequested, AgentEventStatusBlocked, 5, ApprovalPayload{
			ApprovalID:   "approval-1",
			ApprovalType: "command",
			Command:      "kubectl rollout undo deployment/payment-api -n prod",
			Reason:       "5xx rose after deploy",
			Risk:         "high",
		}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseCompleted, AgentEventStatusCompleted, 6, AssistantPayload{
			Channel: "final",
			Text:    "Final answer",
		}),
	}

	replay, err := projector.Replay("session-1", events)
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	realtime := AgentEventProjection{}
	for _, event := range events {
		next, err := projector.Apply(realtime, event)
		if err != nil {
			t.Fatalf("Apply(%s) error = %v", event.EventID, err)
		}
		realtime = next
	}

	if !reflect.DeepEqual(realtime.ProcessGroups, replay.ProcessGroups) {
		t.Fatalf("ProcessGroups realtime=%#v replay=%#v", realtime.ProcessGroups, replay.ProcessGroups)
	}
	if !reflect.DeepEqual(realtime.Approvals, replay.Approvals) {
		t.Fatalf("Approvals realtime=%#v replay=%#v", realtime.Approvals, replay.Approvals)
	}
	if !reflect.DeepEqual(realtime.FinalMessages, replay.FinalMessages) {
		t.Fatalf("FinalMessages realtime=%#v replay=%#v", realtime.FinalMessages, replay.FinalMessages)
	}
}

func TestAgentEventProjector_ReasoningDeltaCreatesThinkingTimelineRow(t *testing.T) {
	projector := NewAgentEventProjector()
	reasoning := testAgentEvent(AgentEventReasoning, AgentEventPhaseDelta, AgentEventStatusRunning, 1, ReasoningPayload{
		ItemID:       "reasoning-1",
		SummaryIndex: 0,
		Delta:        "我会先确认项目结构和事件流。",
		Summary:      "我会先确认项目结构和事件流。",
		Foldable:     true,
		AutoCollapse: false,
	})

	proj, err := projector.Replay("session-1", []AgentEvent{reasoning})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(proj.Timeline) != 1 {
		t.Fatalf("Timeline length = %d, want 1 row", len(proj.Timeline))
	}
	row := proj.Timeline[0]
	if row.Kind != AgentEventReasoning {
		t.Fatalf("row.Kind = %q, want reasoning", row.Kind)
	}
	if row.ID != "reasoning-1" {
		t.Fatalf("row.ID = %q, want reasoning-1", row.ID)
	}
	if row.Title != "正在思考" {
		t.Fatalf("row.Title = %q, want 正在思考", row.Title)
	}
	if row.Summary != "我会先确认项目结构和事件流。" {
		t.Fatalf("row.Summary = %q", row.Summary)
	}
	if row.Status != AgentEventStatusRunning {
		t.Fatalf("row.Status = %q, want running", row.Status)
	}
	if !row.Foldable || row.AutoCollapse || row.Collapsed {
		t.Fatalf("row fold state = foldable:%v auto:%v collapsed:%v, want running expanded", row.Foldable, row.AutoCollapse, row.Collapsed)
	}
	if len(proj.ProcessGroups["turn-1"]) != 1 {
		t.Fatalf("ProcessGroups[turn-1] length = %d, want 1", len(proj.ProcessGroups["turn-1"]))
	}
}

func TestAgentEventProjector_ReasoningCompletedCollapsesSummary(t *testing.T) {
	projector := NewAgentEventProjector()
	delta := testAgentEvent(AgentEventReasoning, AgentEventPhaseDelta, AgentEventStatusRunning, 1, ReasoningPayload{
		ItemID:       "reasoning-1",
		SummaryIndex: 0,
		Delta:        "我会先确认项目结构和事件流。",
		Summary:      "我会先确认项目结构和事件流。",
		Foldable:     true,
	})
	completed := testAgentEvent(AgentEventReasoning, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, ReasoningPayload{
		ItemID:       "reasoning-1",
		SummaryIndex: 0,
		Summary:      "已确认需要检查项目结构和事件流实现。",
		Foldable:     true,
		AutoCollapse: true,
	})

	proj, err := projector.Replay("session-1", []AgentEvent{delta, completed})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(proj.Timeline) != 1 {
		t.Fatalf("Timeline length = %d, want 1 row", len(proj.Timeline))
	}
	row := proj.Timeline[0]
	if row.Title != "思考摘要" {
		t.Fatalf("row.Title = %q, want 思考摘要", row.Title)
	}
	if row.Summary != "已确认需要检查项目结构和事件流实现。" {
		t.Fatalf("row.Summary = %q", row.Summary)
	}
	if row.Status != AgentEventStatusCompleted {
		t.Fatalf("row.Status = %q, want completed", row.Status)
	}
	if !row.Foldable || !row.AutoCollapse || !row.Collapsed {
		t.Fatalf("row fold state = foldable:%v auto:%v collapsed:%v, want completed collapsed", row.Foldable, row.AutoCollapse, row.Collapsed)
	}
	if len(proj.ProcessGroups["turn-1"]) != 1 {
		t.Fatalf("ProcessGroups[turn-1] length = %d, want 1", len(proj.ProcessGroups["turn-1"]))
	}
}

func TestAgentEventProjector_PlanAllowsOnlyOneRunningStep(t *testing.T) {
	projector := NewAgentEventProjector()
	plan := testAgentEvent(AgentEventPlan, AgentEventPhaseUpdated, AgentEventStatusRunning, 1, PlanPayload{
		Title: "修复计划",
		Steps: []PlanStep{
			{ID: "step-1", Text: "收集 nginx 日志证据", Status: "in_progress"},
			{ID: "step-2", Text: "调整 upstream 超时配置", Status: "running"},
			{ID: "step-3", Text: "验证 service-a 恢复", Status: "pending"},
		},
	})

	proj, err := projector.Replay("session-1", []AgentEvent{plan})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(proj.Timeline) != 1 {
		t.Fatalf("Timeline length = %d, want 1 plan row", len(proj.Timeline))
	}
	row := proj.Timeline[0]
	running := 0
	for _, step := range row.Steps {
		if step.Status == "running" || step.Status == "in_progress" {
			running++
		}
	}
	if running != 1 {
		t.Fatalf("running plan steps = %d, want exactly 1: %+v", running, row.Steps)
	}
	if row.Steps[0].Status != "running" || row.Steps[1].Status != "pending" {
		t.Fatalf("plan steps = %+v, want first running and second downgraded to pending", row.Steps)
	}
	if row.Summary != "收集 nginx 日志证据" {
		t.Fatalf("row.Summary = %q, want first running step text", row.Summary)
	}
}

func TestAgentEventProjector_FailedExecCommandKeepsCommandAndError(t *testing.T) {
	projector := NewAgentEventProjector()
	failed := testAgentEvent(AgentEventTool, AgentEventPhaseFailed, AgentEventStatusFailed, 1, ToolPayload{
		ToolCallID:   "exec-1",
		ToolName:     "exec_command",
		DisplayName:  "exec_command",
		InputSummary: "date -d tomorrow",
		Error:        "command failed: exit status 1",
	})

	proj, err := projector.Replay("session-1", []AgentEvent{failed})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(proj.Timeline) != 1 {
		t.Fatalf("Timeline length = %d, want 1", len(proj.Timeline))
	}
	got := proj.Timeline[0].Summary
	if got != "date -d tomorrow: command failed: exit status 1" {
		t.Fatalf("failed exec summary = %q, want command plus error", got)
	}
}

func TestAgentEventProjector_LateOldTurnToolDoesNotReplaceCurrentTurn(t *testing.T) {
	projector := NewAgentEventProjector()
	turn1Started := testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Title: "第一轮"})
	turn1Completed := testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, TurnPayload{Summary: "done"})
	turn2Started := testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 3, TurnPayload{Title: "第二轮"})
	turn2Started.EventID = "turn-2-started"
	turn2Started.TurnID = "turn-2"
	lateOldTool := testAgentEvent(AgentEventTool, AgentEventPhaseCompleted, AgentEventStatusCompleted, 4, ToolPayload{
		ToolCallID:    "old-search",
		ToolName:      "web_search",
		OutputSummary: "旧轮次搜索结果",
	})
	lateOldTool.EventID = "late-old-tool"
	lateOldTool.TurnID = "turn-1"

	proj, err := projector.Replay("session-1", []AgentEvent{turn1Started, turn1Completed, turn2Started, lateOldTool})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if got := proj.CurrentTurnID; got != "turn-2" {
		t.Fatalf("CurrentTurnID = %q, want turn-2 after late old tool event", got)
	}
	if !proj.RuntimeLiveness.ActiveTurns["turn-2"] {
		t.Fatalf("turn-2 should stay active: %+v", proj.RuntimeLiveness.ActiveTurns)
	}
}

func TestAgentEventProjector_ApprovalBlocksThenResolves(t *testing.T) {
	projector := NewAgentEventProjector()
	requested := testAgentEvent(AgentEventApproval, AgentEventPhaseRequested, AgentEventStatusBlocked, 1, ApprovalPayload{
		ApprovalID:   "approval-1",
		ApprovalType: "command",
		Title:        "运行命令",
		Command:      "free -h",
		Reason:       "需要确认",
	})
	resolved := testAgentEvent(AgentEventApproval, AgentEventPhaseResolved, AgentEventStatusCompleted, 2, ApprovalPayload{
		ApprovalID: "approval-1",
		Decision:   "approved",
	})

	proj, err := projector.Apply(AgentEventProjection{}, requested)
	if err != nil {
		t.Fatalf("Apply(requested) error = %v", err)
	}
	if proj.Status != "blocked" || !proj.RuntimeLiveness.PendingApprovals["approval-1"] {
		t.Fatalf("requested projection = %+v, want blocked pending approval", proj)
	}
	if proj.Approvals[0].Command != "free -h" {
		t.Fatalf("approval command = %q, want real command", proj.Approvals[0].Command)
	}

	proj, err = projector.Apply(proj, resolved)
	if err != nil {
		t.Fatalf("Apply(resolved) error = %v", err)
	}
	if proj.Status != "idle" || proj.RuntimeLiveness.PendingApprovals["approval-1"] {
		t.Fatalf("resolved projection = %+v, want idle without pending approval", proj)
	}
}

func TestAgentEventProjector_TurnFailureClearsBlockedApproval(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Title: "Working"}),
		testAgentEvent(AgentEventApproval, AgentEventPhaseRequested, AgentEventStatusBlocked, 2, ApprovalPayload{
			ApprovalID:   "approval-1",
			ApprovalType: "command",
			Title:        "exec_command",
			Command:      "bash -lc free -h",
			Reason:       "requires approval",
		}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseFailed, AgentEventStatusFailed, 3, TurnPayload{Error: "command failed"}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if proj.Status != "failed" {
		t.Fatalf("Status = %q, want failed after terminal turn failure", proj.Status)
	}
	if len(proj.RuntimeLiveness.PendingApprovals) != 0 || len(proj.RuntimeLiveness.PendingUserInputs) != 0 {
		t.Fatalf("pending approvals after failure = %+v / %+v, want cleared", proj.RuntimeLiveness.PendingApprovals, proj.RuntimeLiveness.PendingUserInputs)
	}
	if len(proj.Approvals) != 1 || proj.Approvals[0].Status != AgentEventStatusFailed {
		t.Fatalf("Approvals = %+v, want stale blocked approval marked failed", proj.Approvals)
	}
}

func TestAgentEventProjector_AssistantFinalDeltaAppendsByTurn(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 1, AssistantPayload{Channel: "final", Delta: "第一段"}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 2, AssistantPayload{Channel: "final", Delta: "第二段"}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if got := proj.FinalMessages["turn-1"].Text; got != "第一段第二段" {
		t.Fatalf("FinalMessages[turn-1].Text = %q, want concatenated chunks", got)
	}
}

func TestAgentEventProjector_ToolCallClearsProvisionalAssistantFinal(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Prompt: "查行情"}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 2, AssistantPayload{Channel: "final", Delta: "我将先核实行情。"}),
		testAgentEvent(AgentEventTool, AgentEventPhaseStarted, AgentEventStatusRunning, 3, ToolPayload{
			ToolCallID:   "search-1",
			ToolName:     "web_search",
			DisplayName:  "web_search",
			InputSummary: "BTC price",
		}),
		testAgentEvent(AgentEventTool, AgentEventPhaseCompleted, AgentEventStatusCompleted, 4, ToolPayload{
			ToolCallID:    "search-1",
			ToolName:      "web_search",
			DisplayName:   "web_search",
			OutputSummary: "已搜索 BTC price",
		}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 5, AssistantPayload{Channel: "final", Delta: "最终行情结论。"}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 6, TurnPayload{Summary: "done"}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if got := proj.FinalMessages["turn-1"].Text; got != "最终行情结论。" {
		t.Fatalf("FinalMessages[turn-1].Text = %q, want only post-tool final answer", got)
	}
	group := proj.ProcessGroups["turn-1"]
	if len(group) != 2 {
		t.Fatalf("ProcessGroups[turn-1] length = %d, want assistant prelude and tool: %+v", len(group), group)
	}
	if group[0].Kind != AgentEventAssistant || group[0].DisplayKind != "assistant.process" || group[0].Summary != "我将先核实行情。" {
		t.Fatalf("first process row = %+v, want assistant prelude", group[0])
	}
	if group[1].Kind != AgentEventTool || group[1].ToolCallID != "search-1" {
		t.Fatalf("second process row = %+v, want search tool", group[1])
	}
}

func TestAgentEventProjector_ToolProjectionPreservesInputAndOutputSummary(t *testing.T) {
	projector := NewAgentEventProjector()
	outputPreview := json.RawMessage(`"Filesystem      Size   Used  Avail Capacity Mounted on\n/dev/disk3s1s1   460Gi   12Gi  239Gi     5% /"`)
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Prompt: "查看主机资源"}),
		testAgentEvent(AgentEventTool, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, ToolPayload{
			ToolCallID:    "cmd-1",
			ToolName:      "exec_command",
			DisplayKind:   "host.command",
			InputSummary:  "df -h",
			OutputSummary: "Filesystem Size Used Avail Capacity Mounted on",
			OutputPreview: outputPreview,
		}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(proj.Timeline) < 2 {
		t.Fatalf("Timeline length = %d, want tool row", len(proj.Timeline))
	}
	row := proj.Timeline[1]
	if row.InputSummary != "df -h" {
		t.Fatalf("InputSummary = %q, want command", row.InputSummary)
	}
	if row.OutputSummary != "Filesystem Size Used Avail Capacity Mounted on" {
		t.Fatalf("OutputSummary = %q, want command output summary", row.OutputSummary)
	}
	if string(row.OutputPreview) != string(outputPreview) {
		t.Fatalf("OutputPreview = %s, want %s", row.OutputPreview, outputPreview)
	}
}

func TestAgentEventProjector_AssistantIntentGoesToProcessGroup(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Prompt: "search"}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 2, AssistantPayload{
			Channel: "intent",
			Text:    "我会先说明处理路径，再搜索网页核对信息。",
		}),
		testAgentEvent(AgentEventTool, AgentEventPhaseStarted, AgentEventStatusRunning, 3, ToolPayload{
			ToolCallID:   "tool-1",
			ToolName:     "web_search",
			DisplayName:  "web_search",
			InputSummary: "query",
		}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	group := proj.ProcessGroups["turn-1"]
	if len(group) != 2 {
		t.Fatalf("ProcessGroups[turn-1] length = %d, want intent and tool", len(group))
	}
	if group[0].Kind != AgentEventAssistant || group[0].Summary != "我会先说明处理路径，再搜索网页核对信息。" {
		t.Fatalf("first process group row = %+v, want assistant intent", group[0])
	}
	if _, exists := proj.FinalMessages["turn-1"]; exists {
		t.Fatalf("intent should not be projected as final message: %+v", proj.FinalMessages["turn-1"])
	}
}

func TestAgentEventProjector_TurnCompletedMarksStreamingFinalCompleted(t *testing.T) {
	projector := NewAgentEventProjector()
	proj, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseStarted, AgentEventStatusRunning, 1, TurnPayload{Prompt: "hello"}),
		testAgentEvent(AgentEventAssistant, AgentEventPhaseDelta, AgentEventStatusRunning, 2, AssistantPayload{Channel: "final", Delta: "最终"}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 3, TurnPayload{Summary: "done"}),
	})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	final := proj.FinalMessages["turn-1"]
	if final.Status != AgentEventStatusCompleted {
		t.Fatalf("FinalMessages[turn-1].Status = %q, want completed", final.Status)
	}
}

func TestAgentEventProjector_TurnCompletedWithDiffReviewsOtherwiseIdle(t *testing.T) {
	projector := NewAgentEventProjector()

	withDiff, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventDiff, AgentEventPhaseUpdated, AgentEventStatusCompleted, 1, DiffPayload{
			FilesCount:   1,
			AddedLines:   10,
			RemovedLines: 2,
			Summary:      "修改 1 个文件",
		}),
		testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, TurnPayload{Summary: "done"}),
	})
	if err != nil {
		t.Fatalf("Replay(with diff) error = %v", err)
	}
	if withDiff.Status != "reviewing" {
		t.Fatalf("with diff Status = %q, want reviewing", withDiff.Status)
	}

	withoutDiff, err := projector.Replay("session-1", []AgentEvent{
		testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 1, TurnPayload{Summary: "done"}),
	})
	if err != nil {
		t.Fatalf("Replay(without diff) error = %v", err)
	}
	if withoutDiff.Status != "idle" {
		t.Fatalf("without diff Status = %q, want idle", withoutDiff.Status)
	}
}

func TestAgentEventProjectorApplyDoesNotMutateInputMapFields(t *testing.T) {
	projector := NewAgentEventProjector()
	proj := ensureAgentEventProjection(AgentEventProjection{
		SessionID: "session-1",
		Status:    "working",
	})
	proj.RuntimeLiveness.ActiveTurns["turn-1"] = true
	proj.RuntimeLiveness.ActiveAgents["agent-main"] = true
	proj.FinalMessages["turn-1"] = AssistantFinal{TurnID: "turn-1", Text: "旧消息", Status: AgentEventStatusRunning}
	proj.ProcessGroups["turn-1"] = []TimelineEntry{{
		ID:     "existing",
		Kind:   AgentEventTool,
		TurnID: "turn-1",
		Seq:    1,
	}}

	next, err := projector.Apply(proj, testAgentEvent(AgentEventTurn, AgentEventPhaseCompleted, AgentEventStatusCompleted, 2, TurnPayload{Summary: "done"}))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !proj.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("input ActiveTurns was mutated: %+v", proj.RuntimeLiveness.ActiveTurns)
	}
	if proj.FinalMessages["turn-1"].Status != AgentEventStatusRunning {
		t.Fatalf("input FinalMessages was mutated: %+v", proj.FinalMessages["turn-1"])
	}
	if got := len(proj.ProcessGroups["turn-1"]); got != 1 {
		t.Fatalf("input ProcessGroups length = %d, want unchanged 1", got)
	}
	if next.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("next ActiveTurns still contains completed turn: %+v", next.RuntimeLiveness.ActiveTurns)
	}
	if next.RuntimeLiveness.ActiveAgents["agent-main"] {
		t.Fatalf("next ActiveAgents still contains completed turn agent: %+v", next.RuntimeLiveness.ActiveAgents)
	}
	if next.FinalMessages["turn-1"].Status != AgentEventStatusCompleted {
		t.Fatalf("next FinalMessages status = %q, want completed", next.FinalMessages["turn-1"].Status)
	}
}
