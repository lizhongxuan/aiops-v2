package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
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

func TestBuildModelInputCompactsCorootChartReportsForModelContext(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "检查 checkout 网络异常"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{{
				ID:   "call-coroot",
				Name: "coroot.service_metrics",
			}},
		},
		{
			Role: "tool",
			Content: `{
				"schemaVersion":"aiops.coroot/v1",
				"tool":"coroot.service_metrics",
				"status":"ok",
				"project":"prod",
				"service":"prod:default:Deployment:checkout",
				"metrics":[
					{"name":"cpu","status":"ok","unit":"cores","chartTitle":"CPU usage","values":[[1710000000000,0.4],[1710000030000,0.6]],"series":[{"name":"checkout-1","values":[[1710000000000,0.4],[1710000030000,0.6]]}]}
				],
				"chartReports":[
					{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,1]}]}}]}
				],
				"rawRef":{"uri":"http://coroot/api/project/prod/app/checkout","digest":"sha256:abc","bytes":1024}
			}`,
			ToolResult: &ToolResult{
				ToolCallID: "call-coroot",
				Content: `{
					"schemaVersion":"aiops.coroot/v1",
					"tool":"coroot.service_metrics",
					"status":"ok",
					"project":"prod",
					"service":"prod:default:Deployment:checkout",
					"metrics":[
						{"name":"cpu","status":"ok","unit":"cores","chartTitle":"CPU usage","values":[[1710000000000,0.4],[1710000030000,0.6]],"series":[{"name":"checkout-1","values":[[1710000000000,0.4],[1710000030000,0.6]]}]}
					],
					"chartReports":[
						{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,1]}]}}]}
					],
					"rawRef":{"uri":"http://coroot/api/project/prod/app/checkout","digest":"sha256:abc","bytes":1024}
				}`,
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

	for _, want := range []string{"chartSummary", "prod:default:Deployment:checkout", "Net", "warning", "pointCount"} {
		if !strings.Contains(got, want) {
			t.Fatalf("model input missing %q in compact Coroot summary:\n%s", want, got)
		}
	}
	for _, leaked := range []string{`"chartReports"`, `"metrics"`, `"series"`, `"data"`, `"values"`, "1710000000000"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("model input leaked raw Coroot chart payload marker %q:\n%s", leaked, got)
		}
	}
}

func TestBuildPromptInputReturnsSemanticTrace(t *testing.T) {
	result, err := buildPromptInput([]Message{{Role: "user", Content: "triage"}}, promptcompiler.CompiledPrompt{
		System: promptcompiler.SystemPrompt{Content: "system layer"},
	})
	if err != nil {
		t.Fatalf("buildPromptInput() error = %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected provider messages")
	}
	if !runtimeTraceHas(result.Trace, "stable_prompt", "system") || !runtimeTraceHas(result.Trace, "conversation", "user") {
		t.Fatalf("semantic trace missing prompt or user item: %#v", result.Trace)
	}
}

func runtimeTraceHas(trace promptinput.PromptInputTrace, source, role string) bool {
	for _, item := range trace.Items {
		if item.Source == source && item.SemanticRole == role {
			return true
		}
	}
	return false
}
