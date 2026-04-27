package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentui"
)

// MockAgent is a deterministic local agent for smoke tests and CI.
type MockAgent struct{}

// Run returns canned but realistic answers without calling any online service.
func (MockAgent) Run(ctx context.Context, c Case) (RunOutput, error) {
	select {
	case <-ctx.Done():
		return RunOutput{}, ctx.Err()
	default:
	}
	now := time.Now().UTC()
	toolCalls := mockToolCalls(c)
	answer := mockAnswer(c)
	events := []agentui.AgentEvent{
		mockEvent(c, 1, agentui.AgentEventTurn, agentui.AgentEventPhaseStarted, agentui.AgentEventStatusRunning, now, "eval case started"),
		mockEvent(c, 2, agentui.AgentEventAssistant, agentui.AgentEventPhaseCompleted, agentui.AgentEventStatusCompleted, now, "mock answer completed"),
	}
	for i, call := range toolCalls {
		events = append(events, mockToolEvent(c, int64(i+3), call, now))
	}
	return RunOutput{Answer: answer, Events: events, ToolCalls: toolCalls}, nil
}

func mockAnswer(c Case) string {
	switch c.ID {
	case "design-basic":
		return "方案设计：使用 internal/eval 拆成 CaseLoader、Runner、Scorer、BaselineComparator 四个小模块，输入来自 testdata/eval_cases，输出 report.json。验证方式：go test ./internal/eval，并用 cmd/agent-eval -agent mock 跑 smoke。"
	case "code-analysis":
		return "代码分析结论：RuntimeKernel 在 internal/runtimekernel/eino_kernel.go 负责 turn 主链，AgentEvent 合约在 internal/agentui/agent_event.go。风险点是绕开事件投影会让 trace 缺失。验证方式：go test ./internal/runtimekernel ./internal/eval。"
	case "debug-basic":
		return "Debug 排错：先复现失败，再看 internal/runtimekernel/dispatch.go 的 ToolDispatcher 是否产出 tool result 和 blocked 状态；根因应落到具体输入、策略或执行错误。验证方式：go test ./internal/runtimekernel -run TestToolDispatcher。"
	case "doc-generation":
		return "文档生成：在 README.md 或 docs/eval.md 记录 case JSON、runner 参数、baseline 流程和 mock agent smoke 命令。验证方式：go test ./internal/eval && go run ./cmd/agent-eval -agent mock -cases testdata/eval_cases -out .data/eval-smoke。"
	case "multi-turn-context":
		return "多轮上下文：回答必须引用上一轮约束、当前输入和最终决策，避免丢失用户指定的 baseline vs current。相关文件是 internal/eval/runner.go。验证方式：go test ./internal/eval -run TestRunner。"
	case "tool-calling":
		return "工具调用：需要先用 read_file 查看 testdata/eval_cases，再用 run_command 执行 go test ./internal/eval；toolCalls 会写入 tool_calls.json。验证方式：检查 report.json 中 expectedToolCalls 全部命中。"
	default:
		category := strings.TrimSpace(c.Category)
		if category == "" {
			category = "通用"
		}
		return fmt.Sprintf("%s：这是 mock agent 的本地回答，包含具体执行路径 internal/eval/runner.go 和验证方式：go test ./internal/eval。", category)
	}
}

func mockToolCalls(c Case) []ToolCall {
	switch c.ID {
	case "code-analysis":
		return []ToolCall{{ID: "mock-call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"internal/runtimekernel/eino_kernel.go"}`)}}
	case "debug-basic":
		return []ToolCall{{ID: "mock-call-1", Name: "run_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/runtimekernel -run TestToolDispatcher"}`)}}
	case "tool-calling":
		return []ToolCall{
			{ID: "mock-call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"testdata/eval_cases/tool-calling.json"}`)},
			{ID: "mock-call-2", Name: "run_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/eval"}`)},
		}
	default:
		return nil
	}
}

func mockEvent(c Case, seq int64, kind agentui.AgentEventKind, phase agentui.AgentEventPhase, status agentui.AgentEventStatus, ts time.Time, summary string) agentui.AgentEvent {
	payload, _ := json.Marshal(agentui.SystemPayload{Title: c.ID, Summary: summary, Stage: "eval"})
	return agentui.AgentEvent{
		EventID:    fmt.Sprintf("%s-%d", c.ID, seq),
		Seq:        seq,
		SessionID:  "eval-" + c.ID,
		TurnID:     "turn-" + c.ID,
		Kind:       kind,
		Phase:      phase,
		Status:     status,
		Visibility: agentui.AgentEventVisibilityDebug,
		Source:     agentui.AgentEventSourceSystem,
		CreatedAt:  ts.Format(time.RFC3339Nano),
		Payload:    payload,
	}
}

func mockToolEvent(c Case, seq int64, call ToolCall, ts time.Time) agentui.AgentEvent {
	payload, _ := json.Marshal(agentui.ToolPayload{
		ToolCallID:   call.ID,
		ToolName:     call.Name,
		InputPreview: call.Arguments,
	})
	return agentui.AgentEvent{
		EventID:    fmt.Sprintf("%s-tool-%d", c.ID, seq),
		Seq:        seq,
		SessionID:  "eval-" + c.ID,
		TurnID:     "turn-" + c.ID,
		Kind:       agentui.AgentEventTool,
		Phase:      agentui.AgentEventPhaseCompleted,
		Status:     agentui.AgentEventStatusCompleted,
		Visibility: agentui.AgentEventVisibilityDebug,
		Source:     agentui.AgentEventSourceTool,
		CreatedAt:  ts.Format(time.RFC3339Nano),
		Payload:    payload,
	}
}
