package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestCommitAssistantOutputWritesModelPreludeCommentaryWithToolIDs(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "sess-commit", ID: "turn-commit"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-0",
		UserInput:     "查看 CPU",
		AssistantText: "我会先执行只读命令获取 CPU 状态。",
		ToolCalls: []ToolCall{{
			ID:        "call-cpu",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"top -l 1"}`),
		}},
		Duration: 1200 * time.Millisecond,
	})

	if !result.Committed || result.Phase != AssistantMessagePhaseCommentary {
		t.Fatalf("result = %+v, want committed commentary", result)
	}
	if result.CommentarySource != "model_prelude" || result.SuppressedRawDraft {
		t.Fatalf("result = %+v, want model_prelude without suppression", result)
	}
	item := singleAssistantCommitTestItem(t, snapshot)
	if item.Payload.Summary != "我会先执行只读命令获取 CPU 状态。" {
		t.Fatalf("summary = %q", item.Payload.Summary)
	}
	payload := assistantCommitTestPayload(t, item)
	if payload["phase"] != "commentary" || payload["commentarySource"] != "model_prelude" {
		t.Fatalf("payload = %#v, want commentary model_prelude", payload)
	}
	ids := payload["toolCallIds"].([]any)
	if len(ids) != 1 || ids[0] != "call-cpu" {
		t.Fatalf("toolCallIds = %#v, want call-cpu", payload["toolCallIds"])
	}
}

func TestCommitAssistantOutputWritesRuntimeToolIntentWhenPreludeEmpty(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "sess-empty", ID: "turn-empty"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:    snapshot.ID,
		Iteration: 0,
		MessageID: "msg-empty",
		UserInput: "查看 CPU",
		ToolCalls: []ToolCall{{
			ID:        "call-top",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"top -l 1 | head"}`),
		}},
	})

	if !result.Committed || result.CommentarySource != "runtime_tool_intent" {
		t.Fatalf("result = %+v, want runtime_tool_intent commentary", result)
	}
	item := singleAssistantCommitTestItem(t, snapshot)
	if !strings.Contains(item.Payload.Summary, "执行只读命令") {
		t.Fatalf("summary = %q, want deterministic read-only command commentary", item.Payload.Summary)
	}
	payload := assistantCommitTestPayload(t, item)
	if payload["phase"] != "commentary" || payload["commentarySource"] != "runtime_tool_intent" {
		t.Fatalf("payload = %#v, want runtime_tool_intent commentary", payload)
	}
}

func TestCommitAssistantOutputReplacesLongFinalLikeToolPrelude(t *testing.T) {
	longDraft := strings.Repeat("根因：CPU 飙高可能来自未验证进程。证据：还没有读取。下一步：", 8)
	snapshot := &TurnSnapshot{SessionID: "sess-long", ID: "turn-long"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-long",
		UserInput:     "查看 CPU",
		AssistantText: longDraft,
		ToolCalls: []ToolCall{{
			ID:        "call-uptime",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"uptime"}`),
		}},
	})

	if !result.Committed || !result.SuppressedRawDraft || result.CommentarySource != "runtime_tool_intent" {
		t.Fatalf("result = %+v, want suppressed runtime_tool_intent", result)
	}
	item := singleAssistantCommitTestItem(t, snapshot)
	if item.Payload.Summary == longDraft {
		t.Fatalf("long draft was exposed as process commentary")
	}
	for _, forbidden := range []string{"根因", "证据：", "机制链路"} {
		if strings.Contains(item.Payload.Summary, forbidden) {
			t.Fatalf("summary = %q contains final-like marker %q", item.Payload.Summary, forbidden)
		}
	}
}

func TestCommitAssistantOutputReturnsUnclassifiedCandidateWithoutCommitting(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "sess-final", ID: "turn-final"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-final",
		AssistantText: "最终回答。",
		Duration:      500 * time.Millisecond,
	})

	if result.Committed || result.Phase != AssistantMessagePhaseUnclassified || result.Text != "最终回答。" {
		t.Fatalf("result = %+v, want uncommitted unclassified candidate", result)
	}
	if len(snapshot.AgentItems) != 0 {
		t.Fatalf("agent items = %#v, want no final write before final gates", snapshot.AgentItems)
	}
}

func singleAssistantCommitTestItem(t *testing.T, snapshot *TurnSnapshot) agentstate.TurnItem {
	t.Helper()
	if len(snapshot.AgentItems) != 1 {
		t.Fatalf("agent items = %#v, want one assistant item", snapshot.AgentItems)
	}
	item := snapshot.AgentItems[0]
	if item.Type != agentstate.TurnItemTypeAssistantMessage || item.Status != agentstate.ItemStatusCompleted {
		t.Fatalf("item = %#v, want completed assistant_message", item)
	}
	return item
}

func assistantCommitTestPayload(t *testing.T, item agentstate.TurnItem) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}
