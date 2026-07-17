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
		Duration:     1200 * time.Millisecond,
		FinishReason: "tool_calls",
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
		FinishReason: "tool_calls",
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

func TestCommitAssistantOutputReplacesMachineIncompleteToolPrelude(t *testing.T) {
	draft := "我先运行检查：\n```sh\nstatus --json"
	snapshot := &TurnSnapshot{SessionID: "sess-incomplete", ID: "turn-incomplete"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-incomplete",
		UserInput:     "查看 CPU",
		AssistantText: draft,
		ToolCalls: []ToolCall{{
			ID:        "call-uptime",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"uptime"}`),
		}},
		FinishReason: "tool_calls",
	})

	if !result.Committed || !result.SuppressedRawDraft || result.CommentarySource != "runtime_tool_intent" {
		t.Fatalf("result = %+v, want suppressed runtime_tool_intent", result)
	}
	item := singleAssistantCommitTestItem(t, snapshot)
	if item.Payload.Summary == draft {
		t.Fatalf("machine-incomplete draft was exposed as process commentary")
	}
	for _, forbidden := range []string{"```", "status --json"} {
		if strings.Contains(item.Payload.Summary, forbidden) {
			t.Fatalf("summary = %q contains incomplete markup %q", item.Payload.Summary, forbidden)
		}
	}
}

func TestCommitAssistantOutputReplacesStructurallyOversizedToolPreludeWithoutVocabularyChecks(t *testing.T) {
	draft := strings.Repeat("This is a generic explanatory sentence with no domain-specific boundary vocabulary. ", 4)
	snapshot := &TurnSnapshot{SessionID: "sess-oversized", ID: "turn-oversized"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-oversized",
		UserInput:     "检查状态",
		AssistantText: draft,
		ToolCalls: []ToolCall{{
			ID:        "call-status",
			Name:      "exec_command",
			Arguments: json.RawMessage(`{"command":"status"}`),
		}},
		FinishReason: "tool_calls",
	})
	if !result.Committed || !result.SuppressedRawDraft || result.CommentarySource != assistantCommentarySourceRuntimeToolIntent {
		t.Fatalf("result = %+v, want structural-size fallback commentary", result)
	}
}

func TestAssistantMessageBoundaryTypedToolCallsClassifyCommentaryAcrossDomains(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		toolName string
		args     string
	}{
		{name: "consultation", text: "我先核对 API 文档；根因、证据和下一步这些词不会改变消息相位。" + strings.Repeat("这段说明只界定待调用工具的范围，不代表最终回答。", 4), toolName: "lookup_docs", args: `{}`},
		{name: "file", text: "我先读取文件，再说明配置结论。", toolName: "read_file", args: `{"path":"README.md"}`},
		{name: "host", text: "我先检查主机状态，再整理影响面。", toolName: "exec_command", args: `{"command":"uptime"}`},
		{name: "web", text: "I will search the official site, then summarize the evidence and next step.", toolName: "web_search", args: `{"query":"official guide"}`},
		{name: "database", text: "我先执行只读查询，确认数据库连接状态。", toolName: "execute_readonly_query", args: `{"query":"select 1"}`},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := &TurnSnapshot{SessionID: "sess-domain", ID: "turn-domain-" + tt.name}
			result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
				TurnID:        snapshot.ID,
				Iteration:     i,
				MessageID:     "msg-domain-" + tt.name,
				AssistantText: tt.text,
				ToolCalls: []ToolCall{{
					ID:        "call-domain-" + tt.name,
					Name:      tt.toolName,
					Arguments: json.RawMessage(tt.args),
				}},
				FinishReason: "tool_calls",
			})
			if !result.Committed || result.Phase != AssistantMessagePhaseCommentary || result.CommentarySource != assistantCommentarySourceModelPrelude {
				t.Fatalf("result = %+v, want typed commentary", result)
			}
			if result.Text != tt.text {
				t.Fatalf("commentary text = %q, want wording preserved %q", result.Text, tt.text)
			}
		})
	}
}

func TestAssistantMessageBoundaryToolFreeTextRemainsUnclassifiedAcrossDomains(t *testing.T) {
	texts := []string{
		"这里是通用咨询的根因、证据和下一步。",
		"文件已读取完成。",
		"主机状态正常。",
		"Web search evidence is summarized here.",
		"数据库连接正常。",
	}
	for i, text := range texts {
		snapshot := &TurnSnapshot{SessionID: "sess-untyped", ID: "turn-untyped-" + string(rune('a'+i))}
		result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
			TurnID:        snapshot.ID,
			Iteration:     i,
			MessageID:     "msg-untyped",
			AssistantText: text,
			FinishReason:  "stop",
		})
		if result.Committed || result.Phase != AssistantMessagePhaseUnclassified || result.Text != text {
			t.Fatalf("text %q result = %+v, want unclassified candidate", text, result)
		}
		if len(snapshot.AgentItems) != 0 {
			t.Fatalf("text %q agent items = %#v, want no commentary commit", text, snapshot.AgentItems)
		}
	}
}

func TestCommentaryFinishReasonSuppressesIncompleteModelPrelude(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "sess-length", ID: "turn-length"}
	result := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
		TurnID:        snapshot.ID,
		Iteration:     0,
		MessageID:     "msg-length",
		UserInput:     "检查文件",
		AssistantText: "我先读取目标文件并核对配置。",
		ToolCalls: []ToolCall{{
			ID:        "call-file",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"config.yaml"}`),
		}},
		FinishReason: "length",
	})
	if !result.Committed || !result.SuppressedRawDraft || result.CommentarySource != assistantCommentarySourceRuntimeToolIntent {
		t.Fatalf("result = %+v, want finish-reason fallback commentary", result)
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

func TestCommitFinalAssistantOutputUsesStructuredBoundaryAcrossDomains(t *testing.T) {
	texts := []string{
		"接口的零值可直接使用。",
		"文件已保存并通过校验。",
		"主机当前运行正常。",
		"官方网页确认该参数可用。",
		"数据库只读查询已完成。",
	}
	for i, answer := range texts {
		contract := BuildTerminalFinalContract(answer, FinalContractStatusPartial, nil)
		snapshot := &TurnSnapshot{SessionID: "sess-final-domain", ID: "turn-final-domain-" + string(rune('a'+i))}
		result := commitFinalAssistantOutput(snapshot, assistantOutputCommitInput{
			TurnID:           snapshot.ID,
			Iteration:        i,
			MessageID:        "msg-final-domain",
			AssistantText:    answer,
			FinishReason:     "stop",
			EvidenceBoundary: "limited",
			BoundaryAction:   FinalMessageBoundaryConstrain,
			FinalContract:    &contract,
		})
		if !result.Committed || result.Phase != AssistantMessagePhaseFinalAnswer || result.Text != answer {
			t.Fatalf("answer %q result = %+v, want structured final commit", answer, result)
		}
		if len(snapshot.AgentItems) != 2 {
			t.Fatalf("answer %q items = %#v, want assistant message and final response", answer, snapshot.AgentItems)
		}
	}
}

func TestCommitFinalAssistantOutputRejectsMissingOrIncompleteMachineBoundary(t *testing.T) {
	answer := "完整回答。"
	contract := BuildTerminalFinalContract(answer, FinalContractStatusPartial, nil)
	tests := []struct {
		name  string
		input assistantOutputCommitInput
	}{
		{name: "missing structured boundary", input: assistantOutputCommitInput{AssistantText: answer, FinishReason: "stop"}},
		{name: "non terminal finish reason", input: assistantOutputCommitInput{AssistantText: answer, FinishReason: "length", FinalContract: &contract}},
		{name: "typed tool call", input: assistantOutputCommitInput{AssistantText: answer, FinishReason: "stop", FinalContract: &contract, ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file"}}}},
		{name: "malformed typed tool call", input: assistantOutputCommitInput{AssistantText: answer, FinishReason: "stop", FinalContract: &contract, ToolCalls: []ToolCall{{ID: "call-empty-name"}}}},
		{name: "raw tool markup", input: assistantOutputCommitInput{AssistantText: `<tool_calls><invoke name="read_file"></invoke></tool_calls>`, FinishReason: "stop", FinalContract: &contract}},
		{name: "unclosed delimiter", input: assistantOutputCommitInput{AssistantText: "回答（尚未闭合", FinishReason: "stop", FinalContract: &contract}},
		{name: "unclosed code fence", input: assistantOutputCommitInput{AssistantText: "```text\n尚未闭合", FinishReason: "stop", FinalContract: &contract}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := &TurnSnapshot{SessionID: "sess-final-reject", ID: "turn-final-reject"}
			tt.input.TurnID = snapshot.ID
			result := commitFinalAssistantOutput(snapshot, tt.input)
			if result.Committed || result.Phase != AssistantMessagePhaseUnclassified {
				t.Fatalf("result = %+v, want unclassified rejection", result)
			}
			if len(snapshot.AgentItems) != 0 {
				t.Fatalf("agent items = %#v, want no final commit", snapshot.AgentItems)
			}
		})
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
