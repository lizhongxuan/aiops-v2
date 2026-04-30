package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/agentui"
	"aiops-v2/internal/planning"
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
	turnItems := mockTurnItems(c, toolCalls, now)
	answer := mockAnswer(c)
	events := []agentui.AgentEvent{
		mockEvent(c, 1, agentui.AgentEventTurn, agentui.AgentEventPhaseStarted, agentui.AgentEventStatusRunning, now, "eval case started"),
		mockEvent(c, 2, agentui.AgentEventAssistant, agentui.AgentEventPhaseCompleted, agentui.AgentEventStatusCompleted, now, "mock answer completed"),
	}
	for i, call := range toolCalls {
		events = append(events, mockToolEvent(c, int64(i+3), call, now))
	}
	return RunOutput{Answer: answer, Events: events, ToolCalls: toolCalls, TurnItems: turnItems}, nil
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
	case "approval-blocked":
		return "审批阻断：高风险动作必须先进入 approval blocked 状态，审批通过前未执行工具，也不能发出 tool completed；关键检查点在 internal/runtimekernel/dispatch.go。验证方式：go test ./internal/runtimekernel -run TestApproval。"
	case "tool-failure-fallback":
		return "工具失败 fallback：先用 run_command 复现失败，再按 FailurePolicy 判断是否回灌模型、终止 turn 或改用 fallback，不能盲目重试。关键文件是 internal/runtimekernel/dispatch.go。验证方式：go test ./internal/runtimekernel -run TestToolFailure。"
	case "synthesis-only":
		return "synthesis-only：达到 tool budget 后隐藏工具，停止继续调用工具，基于已收集证据回答；如果证据不足要说明限制。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_SwitchesToSynthesisOnly。"
	case "simple-chat-no-plan":
		return "简单问答：直接回答即可，不强制 plan，也不使用工具；plan 只应用在复杂任务、工具任务或多步任务。相关规则在 internal/promptcompiler/developer_rules.go。验证方式：go test ./internal/promptcompiler。"
	case "simple-no-plan":
		return "简单问答：直接回答即可，不强制 plan，也不使用工具；结构化计划只应用在复杂任务。相关规则在 internal/promptcompiler/developer_rules.go。验证方式：go test ./internal/promptcompiler。"
	case "plan-required":
		return "复杂任务必须生成结构化 plan，并至少包含 in_progress 步骤，后续根据 tool_result 更新状态。关键文件是 internal/planning/tool.go。验证方式：go test ./internal/planning ./internal/eval。"
	case "approval-denied":
		return "审批拒绝：用户 denied 后必须停止高风险工具执行，记录 approval denied 和 blocked/failed 状态，不产生 completed tool_result。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestApproval。"
	case "high-risk-blocked":
		return "高风险阻断：high risk tool 在 approval 前必须 blocked，不执行工具，不发 tool.completed。关键文件是 internal/runtimekernel/dispatch.go。验证方式：go test ./internal/runtimekernel -run TestToolDispatcher_HighRiskMetadataRequiresApproval。"
	case "context-compaction-goal":
		return "上下文压缩目标：超过预算时保留任务目标、最新用户约束、关键 tool_result 摘要和 external reference，避免把完整大输出塞回 prompt。关键文件是 internal/runtimekernel/context.go。验证方式：go test ./internal/runtimekernel -run TestContext。"
	case "prompt-trace-diff":
		return "Prompt trace diff：相邻模型调用应生成 PromptInputTrace 和 input.diff.md，能看到新增 tool_result 或 plan delta，且 diff 不泄漏 raw secret。关键文件是 internal/promptinput/diff.go。验证方式：go test ./internal/promptinput ./internal/modeltrace。"
	case "memory-hit":
		return "memory hit：命中历史 memory 时只注入最相关摘要，并在 PromptInputTrace 标记 source=memory；仍以当前工具证据优先。关键文件是 internal/memory/store.go。验证方式：go test ./internal/memory ./internal/promptinput。"
	case "memory-miss":
		return "memory miss：没有命中 memory 时保持普通回答路径，不额外注入 stale 内容，也不影响 answer/tool/plan 评分。关键文件是 internal/memory/store.go。验证方式：go test ./internal/memory。"
	case "stale-memory-ignored":
		return "stale memory ignored：过期 memory 必须被过滤，不能覆盖当前证据；当前证据来自 tool_result 或用户输入。关键文件是 internal/memory/store.go。验证方式：go test ./internal/memory -run TestJSONStoreSearchFiltersStaleAndLimitsResults。"
	case "loop-max-iterations":
		return "最大循环保护：模型持续请求工具时必须在 iteration limit 停止，写入 failed error TurnItem，避免无上限重复执行。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_MaxIterationsWritesFailedAgentError。"
	case "tool-state-after-call":
		return "工具状态更新：每次工具请求先记录 tool_call，materialize 后记录 tool_result，最终无工具调用时记录 final_answer。关键文件是 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_WritesAgentItemsForToolTurn。"
	case "high-risk-approval-required":
		return "高风险审批：审批前 tool_call 必须是 blocked，工具未执行，也不能写 completed tool_result；通过 approval 后才能继续。关键文件是 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_ApprovalBlockedAgentItemsDoNotCompleteTool。"
	case "tool-failure-no-blind-retry":
		return "工具失败策略：按 FailurePolicy 处理失败，记录 failed tool_result，不能盲目重试；可回灌模型生成 final_answer 或终止 turn。关键文件是 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry。"
	case "finish-criteria-required":
		return "完成条件：存在 pending approval/evidence/tool 或 plan in_progress 时不能标记 completed，必须保留 blocked/failed/error 状态。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_MaxIterationsWritesFailedAgentError。"
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
	case "tool-failure-fallback":
		return []ToolCall{{ID: "mock-call-1", Name: "run_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/runtimekernel -run TestToolFailure"}`)}}
	case "tool-state-after-call":
		return []ToolCall{{ID: "mock-call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"internal/runtimekernel/agent_items.go"}`)}}
	case "tool-failure-no-blind-retry":
		return []ToolCall{{ID: "mock-call-1", Name: "run_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/runtimekernel -run TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry"}`)}}
	default:
		return nil
	}
}

func mockTurnItems(c Case, toolCalls []ToolCall, now time.Time) []agentstate.TurnItem {
	switch c.ID {
	case "loop-max-iterations", "finish-criteria-required":
		items := []agentstate.TurnItem{
			mockTurnItem(c.ID+"-user", agentstate.TurnItemTypeUserMessage, agentstate.ItemStatusCompleted, "mock user input", now),
			mockTurnItem(c.ID+"-model-0", agentstate.TurnItemTypeModelCall, agentstate.ItemStatusCompleted, "mock model call", now),
		}
		if c.Expected.MustHavePlan || len(c.Expected.ExpectedPlanStatuses) > 0 {
			items = append(items, mockPlanItem(c, now))
		}
		items = append(items, mockTurnItem(c.ID+"-error", agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, "mock failed completion gate", now))
		return items
	case "high-risk-approval-required", "high-risk-blocked":
		return []agentstate.TurnItem{
			mockTurnItem(c.ID+"-user", agentstate.TurnItemTypeUserMessage, agentstate.ItemStatusCompleted, "mock user input", now),
			mockTurnItem(c.ID+"-model-0", agentstate.TurnItemTypeModelCall, agentstate.ItemStatusCompleted, "mock model call", now),
			mockTurnItem(c.ID+"-tool-call-blocked", agentstate.TurnItemTypeToolCall, agentstate.ItemStatusBlocked, "approval required", now),
		}
	case "approval-denied":
		return []agentstate.TurnItem{
			mockTurnItem(c.ID+"-user", agentstate.TurnItemTypeUserMessage, agentstate.ItemStatusCompleted, "mock user input", now),
			mockTurnItem(c.ID+"-model-0", agentstate.TurnItemTypeModelCall, agentstate.ItemStatusCompleted, "mock model call", now),
			mockTurnItem(c.ID+"-approval", agentstate.TurnItemTypeApproval, agentstate.ItemStatusFailed, "approval denied", now),
			mockTurnItem(c.ID+"-tool-call-blocked", agentstate.TurnItemTypeToolCall, agentstate.ItemStatusBlocked, "approval denied", now),
		}
	}
	items := []agentstate.TurnItem{
		mockTurnItem(c.ID+"-user", agentstate.TurnItemTypeUserMessage, agentstate.ItemStatusCompleted, "mock user input", now),
		mockTurnItem(c.ID+"-model-0", agentstate.TurnItemTypeModelCall, agentstate.ItemStatusCompleted, "mock model call", now),
	}
	for _, call := range toolCalls {
		items = append(items,
			mockTurnItem(c.ID+"-tool-call-"+call.ID, agentstate.TurnItemTypeToolCall, agentstate.ItemStatusCompleted, call.Name, now),
			mockTurnItem(c.ID+"-tool-result-"+call.ID, agentstate.TurnItemTypeToolResult, agentstate.ItemStatusCompleted, call.Name+" result", now),
		)
	}
	if c.Expected.MustHavePlan || len(c.Expected.ExpectedPlanStatuses) > 0 {
		items = append(items, mockPlanItem(c, now))
	}
	for _, approval := range c.Expected.ExpectedApprovals {
		items = append(items, mockTurnItem(c.ID+"-approval-"+sanitizePathComponent(approval), agentstate.TurnItemTypeApproval, agentstate.ItemStatusPending, approval, now))
	}
	for _, evidence := range c.Expected.ExpectedEvidence {
		items = append(items, mockTurnItem(c.ID+"-evidence-"+sanitizePathComponent(evidence), agentstate.TurnItemTypeEvidence, agentstate.ItemStatusCompleted, evidence, now))
	}
	items = append(items, mockTurnItem(c.ID+"-final", agentstate.TurnItemTypeFinalAnswer, agentstate.ItemStatusCompleted, "mock final answer", now))
	return items
}

func mockTurnItem(id string, typ agentstate.TurnItemType, status agentstate.ItemStatus, summary string, ts time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:        id,
		Type:      typ,
		Status:    status,
		Payload:   agentstate.PayloadEnvelope{Summary: summary},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func mockPlanItem(c Case, ts time.Time) agentstate.TurnItem {
	statuses := append([]string(nil), c.Expected.ExpectedPlanStatuses...)
	if len(statuses) == 0 {
		statuses = []string{string(planning.StepStatusCompleted)}
	}
	steps := make([]planning.PlanStep, 0, len(statuses))
	for i, rawStatus := range statuses {
		status := planning.StepStatus(strings.TrimSpace(rawStatus))
		if !status.IsValid() {
			status = planning.StepStatusPending
		}
		steps = append(steps, planning.PlanStep{
			ID:     fmt.Sprintf("step-%d", i+1),
			Text:   fmt.Sprintf("mock plan step %d", i+1),
			Status: status,
		})
	}
	plan := planning.PlanState{Status: planning.PlanStatusActive, Steps: steps}
	data, _ := json.Marshal(plan)
	return agentstate.TurnItem{
		ID:        c.ID + "-plan",
		Type:      agentstate.TurnItemTypePlan,
		Status:    agentstate.ItemStatusCompleted,
		Payload:   agentstate.PayloadEnvelope{Summary: planning.CompactSummary(plan), Data: data},
		CreatedAt: ts,
		UpdatedAt: ts,
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
