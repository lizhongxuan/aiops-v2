package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/tooling"
)

func TestFinalEvidenceRejectsHighConfidenceAfterFailedTool(t *testing.T) {
	state := FinalEvidenceState{
		Checked: []CheckedEvidence{{
			ToolCallID: "call-ok",
			ToolName:   "synthetic.read",
			Summary:    "read returned partial evidence",
		}},
		FailedTools: []FailedToolImpact{{
			ToolCallID:   "call-failed",
			ToolName:     "synthetic.read",
			FailureClass: "timeout",
			Impact:       "required evidence is missing",
		}},
		Confidence: FinalEvidenceConfidenceHigh,
	}

	decision := VerifyFinalEvidence("已确认全部检查完成，结论高置信。", state)
	if decision.Action != FinalEvidenceActionDowngrade {
		t.Fatalf("decision action = %q, want downgrade: %#v", decision.Action, decision)
	}
	if decision.Confidence == FinalEvidenceConfidenceHigh {
		t.Fatalf("decision confidence = high, want lowered: %#v", decision)
	}
	if !containsString(decision.Reasons, "failed_tool_requires_lower_confidence") {
		t.Fatalf("decision reasons = %v, want failed tool reason", decision.Reasons)
	}
}

func TestFinalEvidenceAllowsLowConfidenceUnknownAfterFailedTool(t *testing.T) {
	state := FinalEvidenceState{
		FailedTools: []FailedToolImpact{{
			ToolCallID:   "call-coroot",
			ToolName:     "coroot_collect_rca_context",
			FailureClass: "not_configured",
			Impact:       "required evidence is missing",
		}},
		Confidence: FinalEvidenceConfidenceLow,
	}

	answer := "根因（置信度：低）：无法确定（Coroot 未配置，缺乏依赖边、指标、日志、链路等直接证据）。"
	decision := VerifyFinalEvidence(answer, state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision action = %q, want allow for low-confidence unknown blocker: %#v", decision.Action, decision)
	}
}

func TestFinalEvidenceDowngradesHighConfidenceMissingEvidenceClaim(t *testing.T) {
	state := FinalEvidenceState{
		Checked: []CheckedEvidence{{
			ToolCallID: "call-mcp-list",
			ToolName:   "list_mcp_resources",
			Summary:    "resources empty",
		}},
		FailedTools: []FailedToolImpact{{
			ToolCallID:   "call-coroot",
			ToolName:     "coroot_collect_rca_context",
			FailureClass: "tool_business_error",
			Impact:       "required evidence is missing",
		}},
		Confidence: FinalEvidenceConfidenceMedium,
	}

	answer := "根因：Coroot未配置，无法收集环境A的A服务RCA证据。\n置信度：高\n缺失证据：A服务的依赖链、指标、日志、链路追踪。"
	decision := VerifyFinalEvidence(answer, state)
	if decision.Action != FinalEvidenceActionDowngrade || decision.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("decision = %#v, want low-confidence downgrade", decision)
	}
	if !containsString(decision.Reasons, "missing_evidence_claim_requires_low_confidence") {
		t.Fatalf("reasons = %v, want missing evidence reason", decision.Reasons)
	}
}

func TestFinalEvidenceAllowsCheckedClaims(t *testing.T) {
	state := FinalEvidenceState{
		Checked: []CheckedEvidence{{
			ToolCallID: "call-ok",
			ToolName:   "synthetic.read",
			Summary:    "read completed",
		}},
		Confidence: FinalEvidenceConfidenceHigh,
	}

	decision := VerifyFinalEvidence("已检查 synthetic.read 的只读结果，结论高置信。", state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision action = %q, want allow: %#v", decision.Action, decision)
	}
	if decision.Confidence != FinalEvidenceConfidenceHigh {
		t.Fatalf("decision confidence = %q, want high", decision.Confidence)
	}
}

func TestFinalEvidenceTreatsUserProvidedEvidenceAsChecked(t *testing.T) {
	state := BuildFinalEvidenceState(&TurnSnapshot{
		Metadata: map[string]string{
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "command_output,log",
			"aiops.userEvidence.signals":    "history_branch_id,timeline_mismatch",
			"aiops.userEvidence.rawExcerpt": "主机A timeline 7；主机B timeline 9；requested timeline 7 is not a child of this server's history",
		},
	}, nil)

	if len(state.Checked) != 1 {
		t.Fatalf("checked = %#v, want user-provided evidence item", state.Checked)
	}
	checked := state.Checked[0]
	if checked.ToolName != "user_provided_evidence" {
		t.Fatalf("checked tool = %q, want user_provided_evidence", checked.ToolName)
	}
	for _, want := range []string{"command_output", "timeline_mismatch", "timeline 7"} {
		if !strings.Contains(checked.Summary, want) {
			t.Fatalf("checked summary = %q, want %q", checked.Summary, want)
		}
	}
	if state.Confidence != FinalEvidenceConfidenceHigh {
		t.Fatalf("confidence = %q, want high for explicit pasted evidence", state.Confidence)
	}

	decision := VerifyFinalEvidence("基于用户提供证据已确认：A 是 timeline 7，B 是 timeline 9，结论高置信。", state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision = %#v, want allow", decision)
	}
}

func TestFinalEvidenceAllowsMissingEvidenceWhenAnswerDoesNotClaimHighConfidence(t *testing.T) {
	state := FinalEvidenceState{
		Checked: []CheckedEvidence{{
			ToolName: "user_provided_evidence",
			Summary:  "operator pasted read-only database role, timeline, and log evidence",
		}},
		Confidence: FinalEvidenceConfidenceHigh,
	}
	answer := "结论（置信度：中）：基于用户提供的只读证据，主备 timeline 存在分叉。缺失证据包括 history 文件、恢复来源配置和控制面状态；在缺失证据补齐前不要执行修复。"

	decision := VerifyFinalEvidence(answer, state)

	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision = %#v, want allow because the answer names evidence gaps without claiming high confidence", decision)
	}
}

func TestFinalEvidenceRecordsNotChecked(t *testing.T) {
	session := &SessionState{}
	session.ToolDiscovery.AddRejectedCall(DeferredToolRejectedCall{
		ToolName:       "synthetic.deferred.read",
		ErrorType:      "tool_unloaded",
		Reason:         "tool exists but was not selected",
		RequiredAction: "call tool_search with mode=search, then mode=select",
		ToolCallID:     "call-unloaded",
	}, time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC))

	state := BuildFinalEvidenceState(&TurnSnapshot{}, session)
	if len(state.NotChecked) != 1 {
		t.Fatalf("notChecked = %#v, want one rejected tool", state.NotChecked)
	}
	if state.NotChecked[0].ToolName != "synthetic.deferred.read" || state.NotChecked[0].Reason != "tool_unloaded" {
		t.Fatalf("notChecked item = %#v", state.NotChecked[0])
	}
	if state.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("confidence = %q, want low", state.Confidence)
	}

	decision := VerifyFinalEvidence("已检查所有可用工具，结论确定。", state)
	if decision.Action != FinalEvidenceActionDowngrade || !strings.Contains(strings.Join(decision.Reasons, ","), "not_checked") {
		t.Fatalf("decision = %#v, want not_checked downgrade", decision)
	}
}

func TestFinalEvidenceTreatsMCPUnavailableAsNotChecked(t *testing.T) {
	session := &SessionState{}
	session.ToolDiscovery.AddRejectedCall(DeferredToolRejectedCall{
		ToolName:       "synthetic.observability_metrics",
		ErrorType:      "mcp_unavailable",
		Reason:         "skipped due to mcp_unavailable: server synthetic_obs health=unavailable",
		RequiredAction: "use another direct evidence source or wait until the external source is healthy",
		ToolCallID:     "call-mcp-unavailable",
	}, time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC))

	state := BuildFinalEvidenceState(&TurnSnapshot{
		Iterations: []IterationState{{
			ToolCalls: []ToolCall{{ID: "call-direct", Name: "exec_command"}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-direct",
				Content:    "cpu idle 80%",
				Summary:    "direct host CPU snapshot",
			}},
		}},
	}, session)
	if len(state.NotChecked) != 1 || state.NotChecked[0].Reason != "mcp_unavailable" {
		t.Fatalf("notChecked = %#v, want mcp_unavailable", state.NotChecked)
	}
	if state.Confidence != FinalEvidenceConfidenceMedium {
		t.Fatalf("confidence = %q, want medium because direct evidence exists but external source unavailable", state.Confidence)
	}
	decision := VerifyFinalEvidence("CPU status normal, confirmed by all evidence.", state)
	if decision.Action != FinalEvidenceActionDowngrade || decision.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("decision = %#v, want downgrade to low when unavailable evidence is ignored", decision)
	}
	if !containsString(decision.Reasons, "not_checked_item_requires_lower_confidence") {
		t.Fatalf("decision reasons = %v, want not_checked reason", decision.Reasons)
	}
}

func TestFailureClassifierRecognizesStructuredMCPUnavailable(t *testing.T) {
	result := DispatchResult{
		Error:   `{"errorType":"mcp_unavailable","reason":"skipped due to mcp_unavailable"}`,
		Outcome: "tool_failed",
		Source:  "runtime",
	}
	if got := failureKindForDispatchResult(result); got != string(toolfailure.KindMCPServerUnavailable) {
		t.Fatalf("failure kind = %q, want %q", got, toolfailure.KindMCPServerUnavailable)
	}
}

func TestFinalEvidenceDowngradesRiskyOperationalAdvice(t *testing.T) {
	state := FinalEvidenceState{
		Checked: []CheckedEvidence{
			{ToolName: "web_search", Summary: "read official docs"},
		},
		Confidence: FinalEvidenceConfidenceHigh,
	}
	decision := VerifyFinalEvidence("建议直接清理 archive 中冲突 WAL：rm -rf /repo/archive/*", state)
	if decision.Action != FinalEvidenceActionDowngrade || decision.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("decision = %+v, want downgrade low confidence", decision)
	}
	if !strings.Contains(strings.Join(decision.Reasons, ","), "risky_operational_advice_requires_evidence_gate") {
		t.Fatalf("reasons = %#v, want risky advice reason", decision.Reasons)
	}
}

func TestFinalEvidenceBlocksUngatedMutationCommandAdviceWithoutTarget(t *testing.T) {
	state := FinalEvidenceState{
		Confidence:         FinalEvidenceConfidenceLow,
		ExecCommandAllowed: false,
		TargetBound:        false,
	}
	answer := "可以先查看状态，然后执行 sudo systemctl restart nginx，最后再检查服务是否恢复。"

	decision := VerifyFinalEvidence(answer, state)

	if decision.Action != FinalEvidenceActionBlock || decision.Confidence != FinalEvidenceConfidenceLow {
		t.Fatalf("decision = %+v, want block low confidence", decision)
	}
	for _, want := range []string{
		"risky_operational_advice_requires_evidence_gate",
		"ungated_mutation_command_advice",
		"exec_command_not_allowed",
		"no_explicit_target_binding",
	} {
		if !containsString(decision.Reasons, want) {
			t.Fatalf("reasons = %#v, want %q", decision.Reasons, want)
		}
	}
}

func TestRiskyAdviceIgnoresOOMKillEvidenceText(t *testing.T) {
	answer := "已确认事实：Exit Code: 1，应用主动退出；可排除 OOM Kill 137 或 Segfault 139。结论：需要继续补充只读日志证据。"

	decision := EvaluateRiskyOperationalAdvice(answer)

	if decision.RequiresEvidenceGate {
		t.Fatalf("decision = %+v, want OOM Kill evidence text treated as non-command evidence", decision)
	}
}

func TestRiskyAdviceDetectsStandaloneKillCommandAdvice(t *testing.T) {
	answer := "可以执行 kill 1234 结束进程，然后观察服务是否恢复。"

	decision := EvaluateRiskyOperationalAdvice(answer)

	if !decision.RequiresEvidenceGate || decision.Category != riskyAdviceCategoryUngatedMutationCommandAdvice {
		t.Fatalf("decision = %+v, want standalone kill command advice blocked", decision)
	}
}

func TestFinalEvidenceBlocksMutationIntentWithoutTargetWhenAnswerOmitsBinding(t *testing.T) {
	state := FinalEvidenceState{
		Confidence:                  FinalEvidenceConfidenceLow,
		ExecCommandAllowed:          false,
		TargetBound:                 false,
		MutationIntentWithoutTarget: true,
	}
	answer := "当前 chat 模式不允许变更操作。请切换到 execute 模式后继续。"

	decision := VerifyFinalEvidence(answer, state)

	if decision.Action != FinalEvidenceActionBlock {
		t.Fatalf("decision = %+v, want block", decision)
	}
	for _, want := range []string{"mutation_intent_requires_explicit_target_binding", "no_explicit_target_binding"} {
		if !containsString(decision.Reasons, want) {
			t.Fatalf("reasons = %#v, want %q", decision.Reasons, want)
		}
	}
}

func TestContainsOperationalMutationIntentTreatsCausalCommandQuestionAsAnalysis(t *testing.T) {
	analysisQuestion := "为什么从节点执行命令 pg_autoctl create postgres 加入集群后 timeline 比主机A还要高，导致无法同步？有什么原因会导致？"
	if containsOperationalMutationIntent(analysisQuestion) {
		t.Fatalf("containsOperationalMutationIntent(%q) = true, want false for causal analysis question", analysisQuestion)
	}
	directRequest := "请执行 systemctl restart nginx，然后分析为什么没有恢复。"
	if !containsOperationalMutationIntent(directRequest) {
		t.Fatalf("containsOperationalMutationIntent(%q) = false, want true for direct execution request", directRequest)
	}
}

func TestFinalEvidenceBlockedFallbackPrefersRiskReviewForDestructiveDataAdvice(t *testing.T) {
	decision := FinalEvidenceVerification{
		Action:     FinalEvidenceActionBlock,
		Confidence: FinalEvidenceConfidenceLow,
		Reasons: []string{
			"destructive_archive_or_data_deletion",
			"mutation_intent_requires_explicit_target_binding",
			"no_explicit_target_binding",
			"exec_command_not_allowed",
		},
	}

	fallback := finalEvidenceBlockedFallback(decision)

	for _, want := range []string{
		"安全结论",
		"只读证据",
		"备份",
		"审批",
		"维护/停服窗口",
		"回滚",
		"验收",
		"数据目录",
		"归档/WAL",
	} {
		if !strings.Contains(fallback, want) {
			t.Fatalf("fallback missing %q:\n%s", want, fallback)
		}
	}
	for _, notWant := range []string{"@host", "@IP", "绑定"} {
		if strings.Contains(fallback, notWant) {
			t.Fatalf("fallback contains target-binding text %q:\n%s", notWant, fallback)
		}
	}
}

func TestBuildFinalEvidenceStateDetectsMutationIntentWithoutTarget(t *testing.T) {
	session := &SessionState{
		Messages: []Message{{
			Role:    "user",
			Content: "在 host-a 上重启 nginx。需要先审批。",
		}},
	}
	state := BuildFinalEvidenceState(&TurnSnapshot{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Metadata: map[string]string{
			"aiops.target.binding":          "none",
			"aiops.tool.execCommandAllowed": "false",
		},
	}, session)

	if state.TargetBound {
		t.Fatalf("TargetBound = true, want false")
	}
	if !state.MutationIntentWithoutTarget {
		t.Fatalf("MutationIntentWithoutTarget = false, want true")
	}
}

func TestBuildFinalEvidenceStateDoesNotTreatUserEvidenceRCAAsMutationIntent(t *testing.T) {
	session := &SessionState{
		Messages: []Message{{
			Role: "user",
			Content: "线上 Kubernetes Pod 一直 CrashLoopBackOff。\n" +
				"kubectl describe 里看到 Last State: Terminated, Exit Code: 1，Back-off restarting failed container。\n" +
				"应用日志最后一行是 failed to connect database。请分析",
		}},
	}
	state := BuildFinalEvidenceState(&TurnSnapshot{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Metadata: map[string]string{
			"aiops.route.mode":              "evidence_rca",
			"aiops.target.binding":          "none",
			"aiops.tool.execCommandAllowed": "false",
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "log",
			"taskDepth.analysisOnly":        "true",
			"taskDepth.executionProhibited": "true",
		},
	}, session)

	if state.TargetBound {
		t.Fatalf("TargetBound = true, want false")
	}
	if state.MutationIntentWithoutTarget {
		t.Fatalf("MutationIntentWithoutTarget = true, want false for analysis-only user evidence RCA")
	}
	decision := VerifyFinalEvidence("结论（置信度：中）：基于用户提供日志，最可能是应用启动时数据库连接失败。缺失证据：命名空间、Pod 名称、数据库端点和最近事件。", state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision = %#v, want allow for analysis answer without target binding blocker", decision)
	}
}

func TestFinalEvidenceAllowsExplanatoryPostgresTimelineAnswerWithoutDomainCompleteness(t *testing.T) {
	state := FinalEvidenceState{
		Checked:    []CheckedEvidence{{ToolName: "user_provided_evidence", Summary: "pg timeline evidence"}},
		Confidence: FinalEvidenceConfidenceLow,
	}
	answer := "结论（置信度：低）：A 与 B timeline 分叉，B 为 standby，pg_is_in_recovery()=t，standby.signal 存在，not a child。缺失证据：.history 文件、restore_command、recovery_target_*、HA 控制面。安全方向：从 A 重建 B。验收命令：检查 pg_stat_replication。"
	decision := VerifyFinalEvidence(answer, state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision = %#v, want allow because PG timeline completeness is not part of the generic final gate", decision)
	}
	if containsString(decision.Reasons, "postgres_timeline_rca_missing_complete_evidence") {
		t.Fatalf("reasons = %v, should not include PG timeline domain gate reason", decision.Reasons)
	}
}

func TestFinalEvidenceAllowsCompletePostgresTimelineAnswerThroughGenericGate(t *testing.T) {
	state := FinalEvidenceState{
		Checked:    []CheckedEvidence{{ToolName: "user_provided_evidence", Summary: "pg timeline evidence"}},
		Confidence: FinalEvidenceConfidenceLow,
	}
	answer := "结论（置信度：低）：A 与 B timeline 分叉，B 为 standby，pg_is_in_recovery()=t，standby.signal 存在，not a child。缺失证据：timeline history files (.history)、restore_command、recovery_target_*、primary_conninfo、WAL receiver/sender、WAL archive、pg_auto_failover/Patroni/repmgr。安全方向：从 A 重建 B。验收命令：检查 pg_stat_replication。"
	decision := VerifyFinalEvidence(answer, state)
	if decision.Action != FinalEvidenceActionAllow {
		t.Fatalf("decision = %#v, want allow because generic final gate has checked evidence and low confidence", decision)
	}
}

func TestRunTurnFinalEvidenceVerifierDowngradesAfterFailedTool(t *testing.T) {
	traceDir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", traceDir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-failed",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("已确认全部检查完成，结论高置信。", nil),
		schema.AssistantMessage("synthetic_read 未成功返回证据；该项未完成检查，结论低置信。", nil),
	}}
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.read", Description: "synthetic read evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("timeout while reading synthetic evidence")
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-final-evidence",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-final-evidence",
		Input:       "check synthetic evidence",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	for _, want := range []string{"还不能给最终结论", "synthetic_read 未成功返回证据", "下一步只读检查"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want deterministic constrained final containing %q", result.Output, want)
		}
	}
	if strings.Contains(result.Output, "置信度") || strings.Contains(result.Output, "confidence") {
		t.Fatalf("final output must not expose confidence labels:\n%s", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want no verifier rewrite call", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-final-evidence")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing iterations: %#v", session)
	}
	var finalAssistantMessages []string
	for _, msg := range session.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) == 0 {
			finalAssistantMessages = append(finalAssistantMessages, msg.Content)
		}
	}
	if len(finalAssistantMessages) != 1 {
		t.Fatalf("final assistant messages = %#v, want only accepted final answer persisted", finalAssistantMessages)
	}
	if finalAssistantMessages[0] != result.Output {
		t.Fatalf("persisted final assistant message = %q, want %q", finalAssistantMessages[0], result.Output)
	}
	if strings.Contains(strings.Join(finalAssistantMessages, "\n"), "已确认全部检查完成") {
		t.Fatalf("unconstrained final draft leaked into session messages: %#v", finalAssistantMessages)
	}
	var finalItems []agentstate.TurnItem
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == agentstate.TurnItemTypeAssistantMessage && assistantMessagePhaseForEvidenceTest(item) == "final_answer" && item.Status == agentstate.ItemStatusCompleted {
			finalItems = append(finalItems, item)
		}
	}
	if len(finalItems) != 1 {
		t.Fatalf("assistant final items = %#v, want only accepted final answer item", finalItems)
	}
	if finalItems[0].Status != agentstate.ItemStatusCompleted || finalItems[0].Payload.Summary != result.Output {
		t.Fatalf("assistant final item = %#v, want completed accepted final", finalItems[0])
	}
	tracePath := session.CurrentTurn.Iterations[1].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if !strings.Contains(string(data), `"finalEvidenceState"`) || !strings.Contains(string(data), `"failedTools"`) {
		t.Fatalf("trace missing final evidence failedTools:\n%s", string(data))
	}
}

func TestRunTurnFinalEvidenceRetryIsSynthesisOnlyAndKeepsRejectedDraftHidden(t *testing.T) {
	rejectedDraft := strings.Repeat("已确认全部检查完成，结论高置信。", 20)
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-failed",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage(rejectedDraft, nil),
		schema.AssistantMessage("synthetic_read 未成功返回证据；该项未完成检查，结论低置信。", nil),
	}}
	registry := tooling.NewRegistry()
	if err := registry.Register(&tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.read", Description: "synthetic read evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("timeout while reading synthetic evidence")
		},
	}); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-final-evidence-synthesis-only",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-final-evidence-synthesis-only",
		Input:       "check synthetic evidence",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	for _, want := range []string{"还不能给最终结论", "synthetic_read 未成功返回证据", "下一步只读检查"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want deterministic constrained final containing %q", result.Output, want)
		}
	}
	if strings.Contains(result.Output, "置信度") || strings.Contains(result.Output, "confidence") {
		t.Fatalf("final output must not expose confidence labels:\n%s", result.Output)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want no final evidence rewrite context", len(compiler.contexts))
	}
	if len(compiler.contexts[0].AssembledTools) == 0 {
		t.Fatalf("first iteration should expose the read tool")
	}
	for i, ctx := range compiler.contexts {
		if got := strings.Join(ctx.SkillPromptAssets, "\n"); strings.Contains(got, "Final-revision-only phase") || strings.Contains(got, "Prior final-answer draft excerpt for revision") {
			t.Fatalf("context %d unexpectedly contains final rewrite controls:\n%s", i, got)
		}
	}
	session := kernel.sessions.Get("sess-final-evidence-synthesis-only")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	assertNoLegacyAssistantItems(t, session.CurrentTurn.AgentItems)
}

func TestRunTurn_FinalEvidenceRetryPreludeDoesNotCommit(t *testing.T) {
	rejectedDraft := strings.Repeat("已确认全部检查完成，结论高置信。", 20)
	prelude := "Let me try browsing the key PostgreSQL documentation pages directly."
	preludeMsg := schema.AssistantMessage(prelude, nil)
	preludeMsg.Extra = map[string]any{"aiops.intent": "progress"}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-failed",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage(rejectedDraft, nil),
		preludeMsg,
		preludeMsg,
	}}
	registry := tooling.NewRegistry()
	if err := registry.Register(&tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.read", Description: "synthetic read evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("timeout while reading synthetic evidence")
		},
	}); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-final-evidence-prelude",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-final-evidence-prelude",
		Input:       "check synthetic evidence",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output == prelude {
		t.Fatalf("prelude was committed as final output")
	}
	if !strings.Contains(result.Output, "还不能给最终结论") {
		t.Fatalf("final output = %q, want deterministic incomplete final", result.Output)
	}
	session := kernel.sessions.Get("sess-final-evidence-prelude")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	assertNoLegacyAssistantItems(t, session.CurrentTurn.AgentItems)
}

func TestRunTurn_AssistantMessageBoundaryRetriesProcessPreludeBeforeFinalCommit(t *testing.T) {
	prelude := "让我深入查看 PostgreSQL timeline 机制和 pgBackRest 恢复行为的具体文档。"
	final := "根因（置信度：低）：当前问题更像是 pgBackRest 恢复后的 timeline lineage 与 pg_auto_failover 注册状态不一致；需要继续核对 .history、recovery_target_timeline、primary_conninfo、WAL receiver/sender 和 monitor 状态。"
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage(prelude, nil),
		schema.AssistantMessage(final, nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-answer-contract-prelude",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-answer-contract-prelude",
		Input:       "为什么从节点执行 pg_autoctl create postgres 后 timeline 更高？",
		Metadata: map[string]string{
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "log",
			"aiops.userEvidence.signals":    "timeline_mismatch",
			"aiops.userEvidence.rawExcerpt": "主机A timeline 7；主机B timeline 9；standby 无法跟随 primary",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output != final {
		t.Fatalf("final output = %q, want retried final %q", result.Output, final)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want one contract retry", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-answer-contract-prelude")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	if strings.Contains(session.CurrentTurn.FinalOutput, "让我深入查看") {
		t.Fatalf("process prelude leaked into final output:\n%s", session.CurrentTurn.FinalOutput)
	}
}

func TestRunTurn_FinalEvidenceRetryPreludePreservesStructuredRCADraft(t *testing.T) {
	structuredDraft := strings.Join([]string{
		"根因（置信度：高）：从节点 B 加入集群时，PostgreSQL timeline lineage 与当前 primary 不一致，导致 WAL receiver 无法跟随主机 A。",
		"",
		"证据：用户提供的现象说明 pgBackRest 恢复后存在 timeline 分叉；pg_auto_failover 只记录节点角色，不会自动修复 WAL 历史分叉。",
		"",
		"可能原因：",
		"1. pgBackRest restore 使用了 recovery_target_timeline=latest，跟随了归档中更高的历史分支。",
		"2. 主机 B 的 $PGDATA 未彻底清空，残留旧集群或旧 standby 元数据。",
		"3. 主机 A 加入 monitor 前没有完成恢复或没有成为一致的权威 primary。",
		"",
		"下一步只读检查：核对 .history、restore_command、recovery_target_timeline、primary_conninfo、pg_controldata timeline 和 pg_autoctl show state。",
	}, "\n")
	prelude := "现在我会继续查阅 pgBackRest 和 PostgreSQL timeline 的官方文档。"
	preludeMsg := schema.AssistantMessage(prelude, nil)
	preludeMsg.Extra = map[string]any{"aiops.intent": "progress"}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-failed",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage(structuredDraft, nil),
		preludeMsg,
		preludeMsg,
	}}
	registry := tooling.NewRegistry()
	if err := registry.Register(&tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.read", Description: "synthetic read evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{}, errors.New("timeout while reading synthetic evidence")
		},
	}); err != nil {
		t.Fatalf("Register tool failed: %v", err)
	}
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-final-evidence-structured-draft",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-final-evidence-structured-draft",
		Input:       "分析 pgBackRest 恢复后从节点加入 pg_auto_failover 失败的 timeline 原因",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output == prelude {
		t.Fatalf("prelude was committed as final output")
	}
	for _, want := range []string{"根因", "recovery_target_timeline=latest", "$PGDATA 未彻底清空", "下一步只读检查"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want preserved RCA detail %q", result.Output, want)
		}
	}
	for _, forbidden := range []string{"置信度", "confidence", "final contract", "non_substantive_final_answer", "kinds=", "signals=", "{\"content\""} {
		if strings.Contains(result.Output, forbidden) {
			t.Fatalf("final output leaked forbidden text %q:\n%s", forbidden, result.Output)
		}
	}
	if !strings.Contains(result.Output, "证据边界：受限") {
		t.Fatalf("final output = %q, want limited evidence boundary", result.Output)
	}
}

func TestRunTurnBlocksUngatedManualMutationAdviceAfterRetry(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("可以执行 sudo systemctl restart nginx，然后检查 systemctl status nginx。", nil),
		schema.AssistantMessage("执行 sudo systemctl restart nginx 即可。", nil),
		schema.AssistantMessage("执行 sudo systemctl restart nginx 即可。", nil),
		schema.AssistantMessage("执行 sudo systemctl restart nginx 即可。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-ungated-manual-mutation",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-ungated-manual-mutation",
		Input:       "在 host-a 上重启 nginx。需要先展示审批，用户批准后继续同一个 turn。",
		Metadata: map[string]string{
			"aiops.route.mode":              "chat_advisory",
			"aiops.target.binding":          "none",
			"aiops.tool.execCommandAllowed": "false",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(model.inputs) > 2 {
		t.Fatalf("model calls = %d, want no final evidence verifier retry loop", len(model.inputs))
	}
	if strings.Contains(result.Output, "systemctl restart") || strings.Contains(result.Output, "sudo systemctl") {
		t.Fatalf("final output leaked mutation command:\n%s", result.Output)
	}
	for _, want := range []string{"明确绑定", "@host"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want %q", result.Output, want)
		}
	}
	session := kernel.sessions.Get("sess-ungated-manual-mutation")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	if got := session.CurrentTurn.Metadata["finalEvidenceBlocked"]; got != "true" {
		t.Fatalf("finalEvidenceBlocked = %q, want true", got)
	}
}

func TestRunTurnBlocksNoTargetMutationIntentWithoutCommandAdvice(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("无法在当前 chat 模式下执行此操作。请切换到 execute 模式后继续。", nil),
		schema.AssistantMessage("无法在当前 chat 模式下执行此操作。请切换到 execute 模式后继续。", nil),
		schema.AssistantMessage("无法在当前 chat 模式下执行此操作。请切换到 execute 模式后继续。", nil),
		schema.AssistantMessage("无法在当前 chat 模式下执行此操作。请切换到 execute 模式后继续。", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-no-target-mutation-intent",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-no-target-mutation-intent",
		Input:       "在 host-a 上重启 nginx。需要先展示审批，用户批准后继续同一个 turn。",
		Metadata: map[string]string{
			"aiops.route.mode":              "chat_advisory",
			"aiops.target.binding":          "none",
			"aiops.tool.execCommandAllowed": "false",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if strings.Contains(result.Output, "systemctl restart") || strings.Contains(result.Output, "sudo systemctl") {
		t.Fatalf("final output leaked mutation command:\n%s", result.Output)
	}
	for _, want := range []string{"明确绑定", "@host"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want %q", result.Output, want)
		}
	}
}

func assistantMessagePhaseForEvidenceTest(item agentstate.TurnItem) string {
	var payload struct {
		Phase string `json:"phase"`
	}
	if len(item.Payload.Data) > 0 {
		_ = json.Unmarshal(item.Payload.Data, &payload)
	}
	return strings.TrimSpace(payload.Phase)
}
