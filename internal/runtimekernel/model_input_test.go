package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestBuildModelInputDropsPriorTurnToolNoiseButKeepsCurrentTurnToolContext(t *testing.T) {
	history := []Message{
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
	}

	input, err := buildModelInput(history, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("buildModelInput() error = %v", err)
	}
	var joined strings.Builder
	for _, msg := range input {
		joined.WriteString(msg.Content)
		joined.WriteString("\n")
	}
	got := joined.String()

	if strings.Contains(got, "old-result") || strings.Contains(got, "旧轮次工具前说明") {
		t.Fatalf("model input leaked prior turn tool noise:\n%s", got)
	}
	if !strings.Contains(got, "上一轮最终回答摘要可以保留") {
		t.Fatalf("model input should keep prior final assistant answer:\n%s", got)
	}
	if !strings.Contains(got, "current-result: 板块排行数据") {
		t.Fatalf("model input should keep current turn tool result:\n%s", got)
	}
}
