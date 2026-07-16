package appui

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestProcessGroupClassificationUsesTypedDisplayKind(t *testing.T) {
	for _, tc := range []struct {
		name        string
		envelope    string
		displayKind string
		want        AiopsTransportProcessKind
	}{
		{name: "web lookup", envelope: "tool", displayKind: "browser.search", want: AiopsTransportProcessKindSearch},
		{name: "command", envelope: "tool", displayKind: "terminal.command", want: AiopsTransportProcessKindCommand},
		{name: "file", envelope: "tool", displayKind: "file.read", want: AiopsTransportProcessKindFile},
		{name: "mcp", envelope: "tool", displayKind: "mcp.action", want: AiopsTransportProcessKindMCP},
		{name: "subagent", envelope: "tool", displayKind: "hostops.wait_host_agents", want: AiopsTransportProcessKindSubagent},
		{name: "tool name shaped display remains generic", envelope: "tool", displayKind: "skill_search", want: AiopsTransportProcessKindTool},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectTransportToolBlockKind(tc.envelope, tc.displayKind); got != tc.want {
				t.Fatalf("detectTransportToolBlockKind(%q, %q) = %q, want %q", tc.envelope, tc.displayKind, got, tc.want)
			}
		})
	}
}

func TestProcessGroupCommentaryBindsMultipleTypedToolCalls(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-commentary-tool-group",
		SessionID: "session-commentary-tool-group",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(3 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "commentary", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "采集两类只读证据。", Data: json.RawMessage(`{"phase":"commentary","streamState":"complete","commentarySource":"runtime_tool_intent","toolCallIds":["call-file","call-mcp"]}`)}},
			{ID: "file", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "read config", Data: json.RawMessage(`{"toolCallId":"call-file","displayKind":"file.read","inputSummary":"/etc/service.conf"}`)}},
			{ID: "mcp", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "read resource", Data: json.RawMessage(`{"toolCallId":"call-mcp","displayKind":"mcp.action","inputSummary":"ops://service"}`)}},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-commentary-tool-group"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	process := projected.Turns[turn.ID].Process
	if len(process) != 3 {
		t.Fatalf("process = %#v, want commentary and two tools", process)
	}
	groupID := process[0].FoldGroupID
	if groupID == "" || process[0].FoldGroupKind != "tool" {
		t.Fatalf("commentary group = %q/%q", process[0].FoldGroupKind, groupID)
	}
	for _, block := range process[1:] {
		if block.FoldGroupID != groupID || block.FoldGroupKind != "tool" {
			t.Fatalf("tool %q group = %q/%q, want commentary group", block.ToolCallID, block.FoldGroupKind, block.FoldGroupID)
		}
	}
	if process[1].ToolCallID != "call-file" || process[2].ToolCallID != "call-mcp" {
		t.Fatalf("tool order = %q, %q", process[1].ToolCallID, process[2].ToolCallID)
	}
}
