package appui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestTransportProjectorProjectsStructuredTurnItemsAsV2Blocks(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	commandData := json.RawMessage(`{
		"toolCallId":"cmd-1",
		"toolName":"exec_command",
		"displayKind":"command",
		"inputSummary":"npm --prefix web run build",
		"outputSummary":"built in 1.78s\n",
		"exitCode":0,
		"durationMs":1780
	}`)
	searchData := json.RawMessage(`{
		"toolCallId":"search-1",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"assistant-ui transport",
		"outputPreview":{"results":[
			{"title":"AssistantTransport docs","url":"https://example.com/docs","snippet":"runtime transport"}
		]}
	}`)
	approvalData := json.RawMessage(`{
		"approvalId":"approval-1",
		"approvalType":"command",
		"command":"npm --prefix web run build",
		"reason":"需要确认构建命令"
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
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "检查前端构建"}, CreatedAt: now},
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "我先检查项目结构。"}, CreatedAt: now.Add(time.Second)},
			{ID: "search-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "assistant-ui transport", Data: searchData}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "cmd-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Data: commandData}, CreatedAt: now.Add(3 * time.Second)},
			{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "等待审批", Data: approvalData}, CreatedAt: now.Add(4 * time.Second)},
			{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "等待审批后继续。"}, CreatedAt: now.Add(5 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.SchemaVersion != "aiops.transport.v2" {
		t.Fatalf("SchemaVersion = %q, want v2", projected.SchemaVersion)
	}
	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("Status = %q, want blocked", projected.Status)
	}
	transportTurn := projected.Turns["turn-1"]
	if transportTurn.User == nil || transportTurn.User.Text != "检查前端构建" {
		t.Fatalf("user = %+v, want projected user", transportTurn.User)
	}
	if len(transportTurn.BlockOrder) == 0 {
		t.Fatalf("BlockOrder empty: %+v", transportTurn)
	}
	if findTextBlock(transportTurn, "我先检查项目结构。").ID == "" {
		t.Fatalf("missing model text block: %+v", transportTurn.BlocksByID)
	}
	if findTextBlock(transportTurn, "等待审批后继续。").ID == "" {
		t.Fatalf("missing final text block: %+v", transportTurn.BlocksByID)
	}
	command := findToolBlock(transportTurn, AiopsTranscriptToolKindCommand)
	if command.Tool == nil || command.Tool.Command != "npm --prefix web run build" || command.Tool.Output.Text != "built in 1.78s" {
		if command.Tool != nil {
			t.Fatalf("command tool = %+v", *command.Tool)
		}
		t.Fatalf("command block = %+v", command)
	}
	search := findToolBlock(transportTurn, AiopsTranscriptToolKindSearch)
	if search.Tool == nil || !strings.Contains(search.Tool.Output.Text, "AssistantTransport docs") {
		t.Fatalf("search block = %+v", search)
	}
	approval := findApprovalBlock(transportTurn, "approval-1")
	if approval.Approval == nil || approval.Approval.Command != "npm --prefix web run build" {
		t.Fatalf("approval block = %+v", approval)
	}
	if _, ok := projected.PendingApprovals["approval-1"]; !ok {
		t.Fatalf("PendingApprovals = %+v, want approval-1", projected.PendingApprovals)
	}
}

func TestTransportProjectorProjectsFinalOutputFallbackAsTextBlock(t *testing.T) {
	now := time.Date(2026, 5, 8, 11, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-output",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTransportTestTime(now.Add(time.Second)),
		FinalOutput: "这是来自 runtime snapshot 的最终回答",
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-1", "thread-1"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	block := findTextBlock(projected.Turns["turn-final-output"], "这是来自 runtime snapshot 的最终回答")
	if block.Text == nil || block.Text.Status != AiopsTranscriptTextStatusCompleted {
		t.Fatalf("final output block = %+v", block)
	}
}

func TestTransportProjectorProjectsSnapshotPendingApprovalAsTimelineBlock(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-pending-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-inline-1",
			SessionID: "session-1",
			TurnID:    "turn-pending-approval",
			Iteration: 1,
			ToolName:  "exec_command",
			Command:   "sw_vers",
			Reason:    "需要确认后执行命令",
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-1", "thread-1"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	block := findApprovalBlock(projected.Turns["turn-pending-approval"], "approval-inline-1")
	if block.Approval == nil || block.Approval.Command != "sw_vers" || block.Approval.Status != string(AiopsTransportProcessStatusBlocked) {
		t.Fatalf("approval block = %+v", block)
	}
	if !projected.RuntimeLiveness.PendingApprovals["approval-inline-1"] {
		t.Fatalf("pending approval liveness missing: %+v", projected.RuntimeLiveness.PendingApprovals)
	}
}

func TestTransportProjectorAggregatesAdjacentSuccessfulShortTools(t *testing.T) {
	turn := AiopsTransportTurn{
		ID:         "turn-1",
		BlockOrder: []string{"text-1", "cmd-1", "cmd-2", "text-2"},
		BlocksByID: map[string]AiopsTranscriptBlock{
			"text-1": textTestBlock("text-1", "before"),
			"cmd-1":  commandTestBlock("cmd-1", "rg foo", AiopsTransportProcessStatusCompleted),
			"cmd-2":  commandTestBlock("cmd-2", "rg bar", AiopsTransportProcessStatusCompleted),
			"text-2": textTestBlock("text-2", "after"),
		},
	}

	got := aggregateTranscriptBlocks(turn)
	if len(got.BlockOrder) != 3 {
		t.Fatalf("BlockOrder = %+v, want text aggregate text", got.BlockOrder)
	}
	aggregate := got.BlocksByID[got.BlockOrder[1]]
	if aggregate.Type != AiopsTranscriptBlockTypeAggregate {
		t.Fatalf("middle block = %+v, want aggregate", aggregate)
	}
	if aggregate.Aggregate.Summary != "已运行 2 条命令" {
		t.Fatalf("summary = %q", aggregate.Aggregate.Summary)
	}
	if !reflect.DeepEqual(aggregate.Aggregate.ChildBlockIDs, []string{"cmd-1", "cmd-2"}) {
		t.Fatalf("children = %+v", aggregate.Aggregate.ChildBlockIDs)
	}
}

func TestTransportProjectorDoesNotAggregateAcrossTextOrFailedTool(t *testing.T) {
	turn := AiopsTransportTurn{
		ID:         "turn-1",
		BlockOrder: []string{"cmd-1", "text-1", "cmd-2", "cmd-3"},
		BlocksByID: map[string]AiopsTranscriptBlock{
			"cmd-1":  commandTestBlock("cmd-1", "rg foo", AiopsTransportProcessStatusCompleted),
			"text-1": textTestBlock("text-1", "middle"),
			"cmd-2":  commandTestBlock("cmd-2", "go test ./...", AiopsTransportProcessStatusFailed),
			"cmd-3":  commandTestBlock("cmd-3", "pwd", AiopsTransportProcessStatusCompleted),
		},
	}

	got := aggregateTranscriptBlocks(turn)
	for _, id := range got.BlockOrder {
		if got.BlocksByID[id].Type == AiopsTranscriptBlockTypeAggregate {
			t.Fatalf("unexpected aggregate across text or failed tool: %+v", got)
		}
	}
}

func TestTransportProjectorSanitizesHTMLToolOutput(t *testing.T) {
	input := "<!DOCTYPE html><html><body><h1>Hello</h1><script>bad()</script></body></html>"
	got := sanitizeOutputPreview(input)
	if strings.Contains(got, "<html") || strings.Contains(got, "<h1") {
		t.Fatalf("sanitizeOutputPreview() = %q, want stripped html", got)
	}
}

func findTextBlock(turn AiopsTransportTurn, text string) AiopsTranscriptBlock {
	for _, id := range turn.BlockOrder {
		block := turn.BlocksByID[id]
		if block.Type == AiopsTranscriptBlockTypeText && block.Text != nil && block.Text.Text == text {
			return block
		}
	}
	return AiopsTranscriptBlock{}
}

func findToolBlock(turn AiopsTransportTurn, kind AiopsTranscriptToolKind) AiopsTranscriptBlock {
	for _, block := range turn.BlocksByID {
		if block.Type == AiopsTranscriptBlockTypeTool && block.Tool != nil && block.Tool.ToolKind == kind {
			return block
		}
	}
	return AiopsTranscriptBlock{}
}

func findApprovalBlock(turn AiopsTransportTurn, approvalID string) AiopsTranscriptBlock {
	for _, block := range turn.BlocksByID {
		if block.Type == AiopsTranscriptBlockTypeApproval && block.Approval != nil && block.Approval.ApprovalID == approvalID {
			return block
		}
	}
	return AiopsTranscriptBlock{}
}

func textTestBlock(id, text string) AiopsTranscriptBlock {
	return AiopsTranscriptBlock{
		ID:   id,
		Type: AiopsTranscriptBlockTypeText,
		Text: &AiopsTextBlock{Role: "assistant", Text: text, Status: AiopsTranscriptTextStatusCompleted},
	}
}

func commandTestBlock(id, command string, status AiopsTransportProcessStatus) AiopsTranscriptBlock {
	return AiopsTranscriptBlock{
		ID:   id,
		Type: AiopsTranscriptBlockTypeTool,
		Tool: &AiopsToolBlock{
			ToolKind: AiopsTranscriptToolKindCommand,
			Command:  command,
			Summary:  command,
			Status:   status,
			Output:   AiopsToolOutput{},
		},
	}
}

func ptrTransportTestTime(value time.Time) *time.Time {
	return &value
}
