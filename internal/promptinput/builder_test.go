package promptinput

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptcompiler"
)

func TestBuilderDropsPriorTurnToolNoiseButKeepsCurrentTurnToolContext(t *testing.T) {
	builder := Builder{}
	result, err := builder.Build(BuildRequest{
		History: []Message{
			{Role: "user", Content: "查看今天A股情况"},
			{
				Role:    "assistant",
				Content: "旧轮次工具前说明，不应进入下一轮模型输入",
				ToolCalls: []ToolCall{{
					ID:   "old-search",
					Name: "web_search",
				}},
			},
			{
				Role:    "tool",
				Content: `old-result: 2026-04-24 A股 上证指数 深证成指 创业板指`,
				ToolResult: &ToolResult{
					ToolCallID: "old-search",
					Content:    `old-result: 2026-04-24 A股 上证指数 深证成指 创业板指`,
				},
			},
			{Role: "assistant", Content: "上一轮最终回答摘要可以保留"},
			{Role: "user", Content: "最近A股什么板块行情比较好"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{{
					ID:   "current-command",
					Name: "exec_command",
				}},
			},
			{
				Role:    "tool",
				Content: `current-result: 板块排行数据`,
				ToolResult: &ToolResult{
					ToolCallID: "current-command",
					Content:    `current-result: 板块排行数据`,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	joined := joinedModelInputItemContent(result.Items)
	if strings.Contains(joined, "old-result") || strings.Contains(joined, "旧轮次工具前说明") {
		t.Fatalf("model input leaked prior turn tool noise:\n%s", joined)
	}
	if !strings.Contains(joined, "上一轮最终回答摘要可以保留") {
		t.Fatalf("model input should keep prior final assistant answer:\n%s", joined)
	}
	if !strings.Contains(joined, "current-result: 板块排行数据") {
		t.Fatalf("model input should keep current turn tool result:\n%s", joined)
	}
	if !traceHas(result.Trace, "conversation", "tool_result", "current-command") {
		t.Fatalf("trace missing current tool result item: %#v", result.Trace)
	}
}

func TestBuilderTraceIncludesPromptFragmentsAndProtocolState(t *testing.T) {
	builder := Builder{}
	result, err := builder.Build(BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System:    promptcompiler.SystemPrompt{Content: "system layer"},
			Developer: promptcompiler.DeveloperInstructions{Content: "developer layer"},
			Tools:     promptcompiler.ToolPromptSet{Content: "tool index"},
			Policy:    promptcompiler.RuntimePolicyPrompt{Content: "policy layer"},
			Dynamic: promptcompiler.DynamicPromptDelta{
				Content: "dynamic task state",
				ProtocolState: promptcompiler.ProtocolPromptState{
					Items: []promptcompiler.ProtocolPromptItem{
						{Kind: "plan", ID: "inspect", Status: "in_progress", Text: "Inspect host symptoms"},
						{Kind: "approval", ID: "approval-1", Status: "pending", Text: "restart requires approval"},
						{Kind: "evidence", ID: "evidence-1", Status: "pending", Text: "need logs"},
					},
				},
			},
		},
		State: agentstate.AgentState{SessionID: "sess-1", TurnID: "turn-1"},
		History: []Message{
			{Role: "user", Content: "triage incident"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(result.Items) < 5 {
		t.Fatalf("items len = %d, want prompt layers plus history", len(result.Items))
	}
	if result.Items[0].ProviderRole != ProviderRoleSystem {
		t.Fatalf("first provider role = %q, want system", result.Items[0].ProviderRole)
	}
	for _, want := range []struct {
		source string
		role   string
		id     string
	}{
		{"stable_prompt", "system", ""},
		{"stable_prompt", "developer", ""},
		{"stable_prompt", "tool_index", ""},
		{"dynamic_prompt", "runtime_policy", ""},
		{"protocol_state", "plan", "inspect"},
		{"protocol_state", "approval", "approval-1"},
		{"protocol_state", "evidence", "evidence-1"},
		{"conversation", "user", ""},
	} {
		if !traceHas(result.Trace, want.source, want.role, want.id) {
			t.Fatalf("trace missing source=%s role=%s id=%s: %#v", want.source, want.role, want.id, result.Trace)
		}
	}
}

func TestBuilderIncludesDynamicPromptContentInModelMessages(t *testing.T) {
	result, err := Builder{}.Build(BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System:    promptcompiler.SystemPrompt{Content: "system layer"},
			Developer: promptcompiler.DeveloperInstructions{Content: "developer layer"},
			Tools:     promptcompiler.ToolPromptSet{Content: "tool index"},
			Policy:    promptcompiler.RuntimePolicyPrompt{Content: "policy layer"},
			Dynamic: promptcompiler.DynamicPromptDelta{
				Content: "dynamic retry guard\n\npolicy layer",
				Policy:  promptcompiler.RuntimePolicyPrompt{Content: "policy layer"},
			},
		},
		History: []Message{{Role: "user", Content: "continue"}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	joined := joinedModelInputItemContent(result.Items)
	if !strings.Contains(joined, "dynamic retry guard") {
		t.Fatalf("model messages missing dynamic prompt content:\n%s", joined)
	}
}

func joinedModelInputItemContent(items []ModelInputItem) string {
	var joined strings.Builder
	for _, item := range items {
		joined.WriteString(item.Content)
		joined.WriteString("\n")
	}
	return joined.String()
}

func traceHas(trace PromptInputTrace, source, role, id string) bool {
	for _, item := range trace.Items {
		if item.Source != source || item.SemanticRole != role {
			continue
		}
		if strings.TrimSpace(id) == "" || item.ID == id {
			return true
		}
	}
	return false
}

func TestModelInputToolCallRoundTrip(t *testing.T) {
	args := json.RawMessage(`{"path":"/tmp"}`)
	items, err := MessagesToModelInputItems([]Message{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: args,
		}},
	}})
	if err != nil {
		t.Fatalf("MessagesToModelInputItems() error = %v", err)
	}
	if len(items) != 1 || len(items[0].ToolCalls) != 1 || items[0].ToolCalls[0].Name != "read_file" || string(items[0].ToolCalls[0].Arguments) != string(args) {
		t.Fatalf("model input tool calls = %#v, want read_file with args", items)
	}
}

func TestBuilderInjectsLimitedMemoryWithTraceBeforeCurrentEvidence(t *testing.T) {
	result, err := Builder{}.Build(BuildRequest{
		MaxMemories: 1,
		Memories: []MemoryItem{
			{ID: "mem-1", Scope: "project", Text: "historical redis runbook"},
			{ID: "mem-2", Scope: "project", Text: "older redis note"},
		},
		History: []Message{
			{Role: "user", Content: "check redis"},
			{Role: "tool", Content: "current redis evidence", ToolResult: &ToolResult{ToolCallID: "call-1", Content: "current redis evidence"}},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	joined := joinedModelInputItemContent(result.Items)
	if !strings.Contains(joined, "historical redis runbook") || strings.Contains(joined, "older redis note") {
		t.Fatalf("memory injection did not respect limit:\n%s", joined)
	}
	if strings.Index(joined, "historical redis runbook") > strings.Index(joined, "current redis evidence") {
		t.Fatalf("memory should be injected before current evidence:\n%s", joined)
	}
	if !traceHas(result.Trace, "memory", "memory", "mem-1") {
		t.Fatalf("trace missing memory item: %#v", result.Trace)
	}
}

func TestPromptInputOpsManualContextDoesNotGrowLinearlyAcrossTurns(t *testing.T) {
	var history []Message
	for i := 0; i < 100; i++ {
		history = append(history,
			Message{Role: "user", Content: "排查 Redis"},
			Message{Role: "assistant", ToolCalls: []ToolCall{{ID: fmt.Sprintf("search-%03d", i), Name: "search_ops_manuals"}}},
			Message{Role: "tool", Content: strings.Repeat("ops_manual_search_result artifact ref ", 20), ToolResult: &ToolResult{ToolCallID: fmt.Sprintf("search-%03d", i), Content: strings.Repeat("ops_manual_search_result artifact ref ", 20)}},
			Message{Role: "assistant", ToolCalls: []ToolCall{{ID: fmt.Sprintf("params-%03d", i), Name: "resolve_ops_manual_params"}}},
			Message{Role: "tool", Content: strings.Repeat("ops_manual_param_resolution artifact ref ", 20), ToolResult: &ToolResult{ToolCallID: fmt.Sprintf("params-%03d", i), Content: strings.Repeat("ops_manual_param_resolution artifact ref ", 20)}},
			Message{Role: "assistant", Content: fmt.Sprintf("第 %d 轮结果摘要", i)},
		)
	}
	history = append(history, Message{Role: "user", Content: "继续检查 Redis"})

	result, err := Builder{}.Build(BuildRequest{History: history})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	joined := joinedModelInputItemContent(result.Items)
	if strings.Contains(joined, "ops_manual_search_result artifact ref") || strings.Contains(joined, "ops_manual_param_resolution artifact ref") {
		t.Fatalf("model input leaked historical ops manual artifacts:\n%s", joined)
	}
	if len(joined) > 6000 {
		t.Fatalf("model input size = %d, want bounded non-linear growth", len(joined))
	}
}
