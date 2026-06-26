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
	if hasGeneralOpsExpectedSignals(c) {
		events = append(events, mockGeneralOpsSignalEvent(c, int64(len(events)+1), now))
	}
	return RunOutput{Answer: answer, Events: events, ToolCalls: toolCalls, TurnItems: turnItems}, nil
}

func mockAnswer(c Case) string {
	if answer, ok := mockExpectationDrivenAnswer(c); ok {
		return answer
	}
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
		return "工具调用：需要用 exec_command 检查 testdata/eval_cases，并执行 go test ./internal/eval；toolCalls 会写入 tool_calls.json。验证方式：检查 report.json 中 expectedToolCalls 全部命中。"
	case "approval-blocked":
		return "审批阻断：高风险动作必须先进入 approval blocked 状态，审批通过前未执行工具，也不能发出 tool completed；关键检查点在 internal/runtimekernel/dispatch.go。验证方式：go test ./internal/runtimekernel -run TestApproval。"
	case "tool-failure-fallback":
		return "工具失败 fallback：先用 exec_command 复现失败，再按 internal/tooling/types.go 里的 FailurePolicy 判断是否回灌模型、终止 turn；再结合 internal/integrations/localtools/register.go 的工具内 fallback 条件决定是否降级，不能盲目重试。验证方式：go test ./internal/runtimekernel -run TestToolFailure。"
	case "synthesis-only":
		return "synthesis-only：达到 tool budget 后隐藏工具，停止继续调用工具，基于已收集证据回答；如果证据不足要说明限制。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_SwitchesToSynthesisOnly。"
	case "simple-chat-no-plan":
		return "简单问答：直接回答即可，不强制生成 plan，也不使用额外执行；plan 只应用在复杂任务或多步任务。相关规则在 internal/promptcompiler/developer_rules.go。验证方式：go test ./internal/promptcompiler。"
	case "simple-no-plan":
		return "简单问答：直接回答即可，不应强制生成 plan，也不使用额外执行；结构化计划只应用在复杂任务。相关规则在 internal/promptcompiler/developer_rules.go。验证方式：go test ./internal/promptcompiler。"
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
		return "最大迭代保护：模型持续请求工具时必须在 iteration limit 停止，写入 failed error TurnItem，避免无上限重复执行。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_MaxIterationsWritesFailedAgentError。"
	case "tool-state-after-call":
		return "工具状态更新：每次工具请求先记录 tool_call，materialize 后记录 tool_result，最终无工具调用时记录 assistant_message(final_answer)。关键文件是 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_WritesAgentItemsForToolTurn。"
	case "high-risk-approval-required":
		return "高风险审批：审批前 tool_call 必须是 blocked，工具未执行，也不能写 completed tool_result；通过 approval 后才能继续。关键文件是 internal/runtimekernel/dispatch.go 和 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_ApprovalBlockedAgentItemsDoNotCompleteTool。"
	case "tool-failure-no-blind-retry":
		return "工具失败策略：按 FailurePolicy 处理失败，记录 failed tool_result，不能盲目重试；可回灌模型生成 assistant_message(final_answer) 或终止 turn。关键文件是 internal/runtimekernel/agent_items.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry。"
	case "finish-criteria-required":
		return "完成条件：存在 pending approval/evidence/tool 或 plan in_progress 时不能标记 completed，必须保留 blocked/failed/error 状态。关键文件是 internal/runtimekernel/eino_kernel.go。验证方式：go test ./internal/runtimekernel -run TestRunTurn_MaxIterationsWritesFailedAgentError。"
	case "coroot-rca-mcp-first":
		return "Coroot MCP：先调用 coroot.collect_rca_context 收集 checkout 最近 30 分钟证据，再通过 aiops.ui_artifact_emit 输出 rca_report。结论必须引用 evidenceRefs；如果证据不足，应返回证据不足而不是确定根因。验证方式：检查 coroot.collect_rca_context 和 rca_report artifact 的 evidenceRefs。"
	case "lab-redis-memory-readonly":
		return "Redis 只读排查闭环：先抽取 Operation Frame，再调用 search_ops_manuals 命中 manual-redis-rca-ssh；当前目标 redis-local-01、prod、ssh、used_memory_rss rising、p95 rising 已满足只读 RCA 输入。只允许执行 read-only preflight，禁止 CONFIG SET、FLUSHALL、restart。验证方式：运行 go test ./internal/opsmanual -run TestHybridRetrievalGoldenCases，并检查 Run Record 后续只写入脱敏 learning summary。"
	case "lab-mysql-backup-no-pg-crossmatch":
		return "MySQL 备份检索防误召回：search_ops_manuals 必须返回 manual-mysql-backup-ssh 且 need_info，因为 connection_test 缺失；不能把 PostgreSQL 手册作为 direct_execute 或 adapt。验证方式：go test ./internal/opsmanual -run TestHybridRetrievalGoldenCases，重点检查 forbidden_manual_ids 中的 manual-pg-backup-ubuntu。"
	case "lab-k8s-payment-api-approval":
		return "K8s payment-api 修复必须先只读采证：kubectl describe、events、rollout history 都是 read-only；kubectl rollout undo 属于高风险变更，必须先形成 ActionProposal、preflight、approval 和 ActionToken。审批前状态应是 blocked，未执行生产修复。验证方式：go test ./internal/eval ./internal/runtimekernel -run TestScoreCase，并检查 expectedApprovals 命中 rollout undo。"
	case "lab-memory-stale-scope":
		return "记忆系统使用原则：memory hit 只能作为候选上下文，source=memory 必须进入 trace；当前工具证据优先，staging-redis、old-host-redis、旧命名空间等 stale scope 不能覆盖本轮 prod 目标。验证方式：go test ./internal/memory ./internal/eval -run TestJSONStoreSearchFiltersStaleAndLimitsResults。"
	case "lab-tool-failure-unknown":
		return "工具失败语义：Prometheus timeout、kubectl timeout 或 ssh timeout 都是 unknown，不代表系统健康；必须记录 failed tool_result，按 FailurePolicy 回灌模型或终止，不能盲目重试，也不能给高置信根因。验证方式：go test ./internal/eval ./internal/runtimekernel -run TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry。"
	case "lab-run-record-learning-redaction":
		return "Run Record 到经验沉淀：成功闭环只生成 pending_review 候选手册或 redacted memory hint，不能自动发布 verified；password、token、secret、Authorization header 必须脱敏，原始 shell 脚本全文不能进候选正文。验证方式：go test ./internal/opsmanual ./internal/memory -run TestLearningSummary。"
	case "codex-pg-timeline-v2":
		return "PG timeline 原理分析：timeline 是 WAL 历史分支，不是数据更新量。B timeline 比 A 高的候选原因包括 B 非空、B 曾经被提升、pgBackRest 选择了不一致的恢复点、postgresql.auto.conf 恢复残留、旧 stanza 混写。没有主机 mention 时不能绑定默认本机，也不调用主机命令；需要用户提供 pg_controldata、pg_is_in_recovery、standby.signal、日志和 pg_auto_failover monitor state。建议对照官方文档和当前版本文档确认 recovery_target、restore_command 与 timeline history。验证方式：只基于用户证据复核 WAL 历史分支。"
	case "codex-pg-timeline-with-evidence-v2":
		return "基于用户贴出的证据：A timeline 7，B timeline 9，日志出现 not a child，说明时间线分叉；B 仍在恢复，B is in recovery with standby.signal。Top3 候选：standby_recovered_on_divergent_timeline、wrong_base_backup_or_restore_point、timeline_history_missing_or_not_child。支持证据包括 B TimeLineID 9 while A TimeLineID 7、B is in recovery with standby.signal、log says requested timeline 7 is not a child。缺失证据包括 timeline history files、restore_command and recovery_target settings、pg_auto_failover monitor state。结论是 high confidence in timeline divergence，但 not definitive on exact operator step without history files。安全边界：no host execution without explicit mention，no remediation without approval。处理建议是先重建 B 的副本来源并重新加入复制链路，验收命令应覆盖恢复状态、standby.signal、时间线父子关系、复制连接和 monitor state。验证方式：复核 timeline history files 与 monitor state，再确认复制连接状态。"
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
			{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"rg -n \"expectedToolCalls\" testdata/eval_cases && go test ./internal/eval"}`)},
		}
	case "tool-failure-fallback":
		return []ToolCall{{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/runtimekernel -run TestToolFailure"}`)}}
	case "loop-max-iterations":
		return []ToolCall{{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"rg -n \"iteration limit|max\" internal/runtimekernel/eino_kernel.go"}`)}}
	case "high-risk-approval-required":
		return []ToolCall{{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"rg -n \"approval|blocked\" internal/runtimekernel/agent_items.go"}`)}}
	case "tool-state-after-call":
		return []ToolCall{{ID: "mock-call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"internal/runtimekernel/agent_items.go"}`)}}
	case "tool-failure-no-blind-retry":
		return []ToolCall{{ID: "mock-call-1", Name: "run_command", Arguments: json.RawMessage(`{"cmd":"go test ./internal/runtimekernel -run TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry"}`)}}
	case "coroot-rca-mcp-first":
		return []ToolCall{
			{ID: "mock-call-1", Name: "coroot.collect_rca_context", Arguments: json.RawMessage(`{"service":"checkout","timeRange":"30m"}`)},
			{ID: "mock-call-2", Name: "aiops.ui_artifact_emit", Arguments: json.RawMessage(`{"type":"rca_report","inlineData":{"schemaVersion":"aiops.rca_report/v1","status":"inconclusive","evidenceRefs":["coroot://metric/checkout/p95"],"rawRefs":[]}}`)},
		}
	case "lab-redis-memory-readonly", "lab-mysql-backup-no-pg-crossmatch":
		return []ToolCall{{ID: "mock-call-1", Name: "search_ops_manuals", Arguments: json.RawMessage(`{"text":"synthetic user journey","limit":5}`)}}
	case "lab-k8s-payment-api-approval":
		return []ToolCall{
			{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"kubectl -n prod describe deploy payment-api"}`)},
			{ID: "mock-call-2", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"kubectl -n prod rollout history deploy/payment-api"}`)},
		}
	case "lab-tool-failure-unknown":
		return []ToolCall{{ID: "mock-call-1", Name: "exec_command", Arguments: json.RawMessage(`{"cmd":"kubectl -n prod top pod payment-api --request-timeout=5s"}`)}}
	default:
		if len(c.Expected.ExpectedToolCalls) == 0 {
			return nil
		}
		calls := make([]ToolCall, 0, len(c.Expected.ExpectedToolCalls))
		for i, name := range c.Expected.ExpectedToolCalls {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			args, _ := json.Marshal(map[string]any{
				"caseId": c.ID,
				"mock":   true,
			})
			calls = append(calls, ToolCall{
				ID:        fmt.Sprintf("mock-call-%d", i+1),
				Name:      name,
				Arguments: json.RawMessage(args),
			})
		}
		return calls
	}
}

func mockTurnItems(c Case, toolCalls []ToolCall, now time.Time) []agentstate.TurnItem {
	switch c.ID {
	case "loop-max-iterations", "finish-criteria-required":
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
	if c.ID == "lab-k8s-payment-api-approval" || c.Expected.MustHavePlan || len(c.Expected.ExpectedPlanStatuses) > 0 {
		items = append(items, mockPlanItem(c, now))
	}
	if len(toolCalls) == 0 && expectedTurnItemContains(c, "tool_result") {
		items = append(items, mockTurnItem(c.ID+"-tool-result-expected", agentstate.TurnItemTypeToolResult, agentstate.ItemStatusCompleted, "mock tool result", now))
	}
	for _, approval := range c.Expected.ExpectedApprovals {
		items = append(items, mockApprovalItem(c, approval, now))
	}
	for i, evidence := range c.Expected.ExpectedEvidence {
		items = append(items, mockEvidenceItem(c, evidence, i, now))
	}
	if c.Expected.MustHaveEvidence && len(c.Expected.ExpectedEvidence) == 0 {
		items = append(items, mockEvidenceItem(c, "mock evidence", 0, now))
	}
	if hasGeneralOpsExpectedSignals(c) {
		items = append(items, mockGeneralOpsSignalItem(c, now))
	}
	items = append(items, mockAssistantFinalItem(c.ID+"-final", agentstate.ItemStatusCompleted, "mock final answer", now))
	return items
}

func mockExpectationDrivenAnswer(c Case) (string, bool) {
	if !shouldUseExpectationDrivenAnswer(c) {
		return "", false
	}
	var b strings.Builder
	writeMockAnswerSection(&b, "已知事实", expectationSafeValues(c.Expected.MustInclude, c.Expected.MustNotInclude))
	writeMockAnswerSection(&b, "可能原因", appendIfNotEmpty(nil, c.Expected.Diagnosis.RootCauseTop1, c.Expected.Diagnosis.RootCauseCandidatesTop3...))
	writeMockAnswerSection(&b, "支持证据", c.Expected.Diagnosis.SupportingEvidence)
	writeMockAnswerSection(&b, "缺失证据", c.Expected.Diagnosis.MissingEvidence)
	writeMockAnswerSection(&b, "下一步检查", defaultNextChecks(c))
	writeMockAnswerSection(&b, "安全操作", mockSafetyGuardrails(c))
	writeMockAnswerSection(&b, "置信度", c.Expected.Diagnosis.ConfidenceCalibration)
	writeMockAnswerSection(&b, "工具使用理由", mockToolUsageRationale(c))
	writeMockAnswerSection(&b, "通用运行契约", append(append([]string{}, c.Expected.ExpectedCapabilityPath...), c.Expected.ExpectedGenericOpsContract...))
	writeMockAnswerSection(&b, "观测证据", c.Expected.ExpectedObservabilityEvidence)
	writeMockAnswerSection(&b, "资源角色", c.Expected.ExpectedResourceRoles)
	writeMockAnswerSection(&b, "Workflow 状态", c.Expected.ExpectedWorkflowReviewStatus)
	writeMockAnswerSection(&b, "覆盖标签", c.Expected.Diagnosis.CoverageTags)
	if c.Expected.MustMentionEvidenceLimits {
		writeMockAnswerSection(&b, "证据限制", []string{"当前结论只基于用户提供或只读证据；缺失证据补齐前不能给出确定根因或执行修复。"})
	}
	writeMockAnswerSection(&b, "验证方式", []string{"复核上述只读证据、工具结果和缺失证据清单；涉及变更时必须先审批再验证。"})
	answer := strings.TrimSpace(b.String())
	if answer == "" {
		return "", false
	}
	return answer, true
}

func shouldUseExpectationDrivenAnswer(c Case) bool {
	if !c.Expected.Diagnosis.IsZero() || c.Expected.MustMentionEvidenceLimits || hasGeneralOpsExpectedSignals(c) || c.Expected.MustHaveEvidence {
		return true
	}
	return false
}

func expectationSafeValues(values, forbidden []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || exactlyForbidden(value, forbidden) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func exactlyForbidden(value string, forbidden []string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, item := range forbidden {
		if strings.ToLower(strings.TrimSpace(item)) == value {
			return true
		}
	}
	return false
}

func appendIfNotEmpty(values []string, first string, rest ...string) []string {
	if first = strings.TrimSpace(first); first != "" {
		values = append(values, first)
	}
	for _, value := range rest {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func defaultNextChecks(c Case) []string {
	if len(c.Expected.Diagnosis.MissingEvidence) > 0 {
		return c.Expected.Diagnosis.MissingEvidence
	}
	if len(c.Expected.ExpectedToolCalls) > 0 {
		return c.Expected.ExpectedToolCalls
	}
	return []string{"补齐只读证据后再收敛根因和处理方案"}
}

func mockToolUsageRationale(c Case) []string {
	if len(c.Expected.ExpectedToolCalls) > 0 {
		return c.Expected.ExpectedToolCalls
	}
	if c.Expected.MaxToolCalls == 0 {
		return []string{"本 case 要求只做分析，不调用执行工具；如需进一步证据，应由用户提供或显式授权只读工具。"}
	}
	return []string{"根据当前可见工具和只读优先原则选择最小必要工具。"}
}

func mockSafetyGuardrails(c Case) []string {
	values := append([]string(nil), c.Expected.Diagnosis.SafetyGuardrails...)
	joined := strings.ToLower(strings.Join(values, "\n"))
	if strings.Contains(joined, "delete") ||
		strings.Contains(joined, "archive") ||
		strings.Contains(joined, "wal") ||
		strings.Contains(joined, "pgdata") ||
		strings.Contains(joined, "数据目录") ||
		strings.Contains(joined, "归档") {
		values = append(values, "若未来涉及数据目录或归档变更，必须先确认 timeline/pg_controldata 目标范围、备份或快照、审批、维护窗口、回滚方案和执行后验证。")
	}
	return values
}

func expectedTurnItemContains(c Case, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, item := range c.Expected.ExpectedTurnItems {
		if strings.ToLower(strings.TrimSpace(item)) == want {
			return true
		}
	}
	return false
}

func writeMockAnswerSection(b *strings.Builder, title string, values []string) {
	values = uniqueNonEmptyStrings(values)
	if len(values) == 0 {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("：")
	b.WriteString(strings.Join(values, "；"))
	b.WriteString("。")
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
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

func mockAssistantFinalItem(id string, status agentstate.ItemStatus, summary string, ts time.Time) agentstate.TurnItem {
	item := mockTurnItem(id, agentstate.TurnItemTypeAssistantMessage, status, summary, ts)
	item.Payload.Data = json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)
	return item
}

func mockApprovalItem(c Case, approval string, ts time.Time) agentstate.TurnItem {
	command := strings.TrimSpace(approval)
	if command == "" {
		command = "approval required"
	}
	payload := map[string]any{
		"approvalId":   c.ID + "-approval-" + sanitizePathComponent(command),
		"approvalType": "command",
		"command":      command,
		"reason":       "high risk action requires approval",
		"risk":         "high",
		"targets":      []string{"prod/payment-api"},
	}
	data, _ := json.Marshal(payload)
	return agentstate.TurnItem{
		ID:        c.ID + "-approval-" + sanitizePathComponent(command),
		Type:      agentstate.TurnItemTypeApproval,
		Status:    agentstate.ItemStatusPending,
		Payload:   agentstate.PayloadEnvelope{Kind: "command", Summary: command, Data: data},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func mockEvidenceItem(c Case, evidence string, index int, ts time.Time) agentstate.TurnItem {
	kind := "metric"
	source := "prometheus"
	window := "15m"
	rawRef := "promql:sum(rate(http_requests_total{status=~\"5..\"}[5m]))"
	lower := strings.ToLower(evidence)
	if strings.Contains(lower, "log") || strings.Contains(lower, "loki") || strings.Contains(evidence, "日志") {
		kind = "log"
		source = "loki"
		window = "10m"
		rawRef = `{app="payment-api"} |= "error"`
	}
	if strings.Contains(lower, "trace") || strings.Contains(evidence, "链路") {
		kind = "trace"
		source = "tempo"
		window = "30m"
		rawRef = "trace:payment-api:slow-request"
	}
	if strings.Contains(lower, "deploy") || strings.Contains(evidence, "发布") {
		kind = "deployment"
		source = "kubernetes"
		window = "1h"
		rawRef = "deployment/payment-api@sha256:abc123"
	}
	payload := map[string]any{
		"id":         c.ID + "-evidence-" + sanitizePathComponent(evidence),
		"kind":       kind,
		"title":      evidence,
		"summary":    evidence,
		"source":     source,
		"confidence": "high",
		"window":     window,
		"rawRef":     rawRef,
		"data": map[string]any{
			"caseId": c.ID,
			"index":  index,
		},
	}
	data, _ := json.Marshal(payload)
	return agentstate.TurnItem{
		ID:        c.ID + "-evidence-" + sanitizePathComponent(evidence),
		Type:      agentstate.TurnItemTypeEvidence,
		Status:    agentstate.ItemStatusCompleted,
		Payload:   agentstate.PayloadEnvelope{Kind: kind, Summary: evidence, Data: data},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func mockGeneralOpsSignalItem(c Case, ts time.Time) agentstate.TurnItem {
	data, _ := json.Marshal(generalOpsSignalPayload(c))
	return agentstate.TurnItem{
		ID:        c.ID + "-general-ops-signals",
		Type:      agentstate.TurnItemTypeModelCall,
		Status:    agentstate.ItemStatusCompleted,
		Payload:   agentstate.PayloadEnvelope{Kind: "general_ops_contract", Summary: "general ops contract signals", Data: data},
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

func mockGeneralOpsSignalEvent(c Case, seq int64, ts time.Time) agentui.AgentEvent {
	payload, _ := json.Marshal(generalOpsSignalPayload(c))
	return agentui.AgentEvent{
		EventID:    fmt.Sprintf("%s-general-ops-%d", c.ID, seq),
		Seq:        seq,
		SessionID:  "eval-" + c.ID,
		TurnID:     "turn-" + c.ID,
		Kind:       agentui.AgentEventReasoning,
		Phase:      agentui.AgentEventPhaseCompleted,
		Status:     agentui.AgentEventStatusCompleted,
		Visibility: agentui.AgentEventVisibilityDebug,
		Source:     agentui.AgentEventSourceSystem,
		CreatedAt:  ts.Format(time.RFC3339Nano),
		Payload:    payload,
	}
}

func hasGeneralOpsExpectedSignals(c Case) bool {
	return len(c.Expected.ExpectedResourceRoles) > 0 ||
		len(c.Expected.ExpectedCapabilityPath) > 0 ||
		len(c.Expected.ExpectedWorkflowReviewStatus) > 0 ||
		len(c.Expected.ExpectedObservabilityEvidence) > 0 ||
		len(c.Expected.ExpectedGenericOpsContract) > 0
}

func generalOpsSignalPayload(c Case) map[string]any {
	payload := map[string]any{}
	if len(c.Expected.ExpectedResourceRoles) > 0 {
		payload["resourceRoles"] = c.Expected.ExpectedResourceRoles
	}
	if len(c.Expected.ExpectedCapabilityPath) > 0 {
		payload["capabilityPath"] = c.Expected.ExpectedCapabilityPath
	}
	if len(c.Expected.ExpectedWorkflowReviewStatus) > 0 {
		payload["workflowReviewStatus"] = c.Expected.ExpectedWorkflowReviewStatus
	}
	if len(c.Expected.ExpectedObservabilityEvidence) > 0 {
		payload["observabilityEvidence"] = c.Expected.ExpectedObservabilityEvidence
	}
	if len(c.Expected.ExpectedGenericOpsContract) > 0 {
		payload["generalOpsContract"] = c.Expected.ExpectedGenericOpsContract
	}
	return payload
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
