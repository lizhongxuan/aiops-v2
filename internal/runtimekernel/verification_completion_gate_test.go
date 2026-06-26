package runtimekernel

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
	"aiops-v2/internal/verification"
)

func TestVerificationCompletionGateRequiresExecutionReportForComplexTask(t *testing.T) {
	decision := EvaluateVerificationCompletionGate(taskdepth.Profile{
		Level:              taskdepth.LevelOperations,
		RequiresEvidence:   true,
		RequiresValidation: true,
	}, &TurnSnapshot{})

	if decision.Action != VerificationCompletionActionBlockSuccessFinal {
		t.Fatalf("action = %q, want block_success_final: %#v", decision.Action, decision)
	}
	if !containsString(decision.Reasons, "missing_verification_report") || !containsString(decision.Reasons, "execution_required") {
		t.Fatalf("reasons = %v, want missing verification report and execution requirement", decision.Reasons)
	}
}

func TestVerificationCompletionGateAllowsAnalysisOnlyFinalWithoutExecutionReport(t *testing.T) {
	decision := EvaluateVerificationCompletionGate(taskdepth.Profile{
		Level:               taskdepth.LevelInvestigation,
		RequiresPlan:        true,
		RequiresEvidence:    true,
		AnalysisOnly:        true,
		ExecutionProhibited: true,
	}, &TurnSnapshot{})

	if decision.Action != VerificationCompletionActionAllow {
		t.Fatalf("action = %q, want allow for analysis-only no-exec task: %#v", decision.Action, decision)
	}
	if decision.Requirement != verification.VerificationAnalysisAllowed {
		t.Fatalf("requirement = %q, want analysis_allowed: %#v", decision.Requirement, decision)
	}
}

func TestVerificationCompletionGateValidatesPassPartialAndFail(t *testing.T) {
	pass := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-pass",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     "synthetic task",
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultPass,
			RawRef: "artifact://synthetic/pass",
		}},
	})
	passDecision := EvaluateVerificationCompletionGate(taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}, pass)
	if passDecision.Action != VerificationCompletionActionAllow || passDecision.Status != verification.StatusPass {
		t.Fatalf("pass decision = %#v, want allow/PASS", passDecision)
	}

	partial := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-partial",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPartial,
		Subject:     "synthetic task",
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultBlocked,
			RawRef: "artifact://synthetic/partial",
		}},
		Blockers: []verification.VerificationBlocker{{
			Source:       verification.BlockerPermission,
			Reason:       "synthetic permission missing",
			BlockedScope: "synthetic mutation",
			NextAction:   "request synthetic approval",
		}},
	})
	partialDecision := EvaluateVerificationCompletionGate(taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}, partial)
	if partialDecision.Action != VerificationCompletionActionRequireBlockerFinal || partialDecision.Status != verification.StatusPartial {
		t.Fatalf("partial decision = %#v, want require blocker/PARTIAL", partialDecision)
	}
	if verificationCompletionGateAllowsFinal("已完成，可以收尾。", partialDecision, nil) {
		t.Fatal("partial decision allowed success final, want blocker-only final")
	}
	if !verificationCompletionGateAllowsFinal("权限缺少，无法继续；需要 request synthetic approval。", partialDecision, nil) {
		t.Fatal("partial decision rejected explicit blocker final")
	}

	fail := verificationReportSnapshot(t, verification.VerificationReport{
		ID:          "vr-fail",
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusFail,
		Subject:     "synthetic task",
		Expected:    "synthetic expected",
		Actual:      "synthetic actual",
		RawRefs:     []string{"artifact://synthetic/fail"},
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultFail,
			RawRef: "artifact://synthetic/fail",
		}},
		ContractChecks: []verification.ContractCheck{{
			Name:    "synthetic contract",
			Checked: true,
			Result:  verification.EvidenceResultFail,
		}},
	})
	failDecision := EvaluateVerificationCompletionGate(taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresValidation: true}, fail)
	if failDecision.Action != VerificationCompletionActionBlockSuccessFinal || failDecision.Status != verification.StatusFail {
		t.Fatalf("fail decision = %#v, want block success/FAIL", failDecision)
	}
	if verificationCompletionGateAllowsFinal("已完成，已验证。", failDecision, nil) {
		t.Fatal("fail decision allowed success final")
	}
}

func TestParseVerificationReportJSONIgnoresGenericStatusPayload(t *testing.T) {
	if report, ok := parseVerificationReportJSON([]byte(`{"summary":"synthetic metrics checked","status":"ok"}`)); ok {
		t.Fatalf("generic status payload parsed as verification report: %#v", report)
	}

	report, ok := parseVerificationReportJSON([]byte(`{
		"requirement":"execution_required",
		"status":"FAIL",
		"subject":"synthetic task",
		"expected":"synthetic expected state",
		"actual":"synthetic actual state"
	}`))
	if !ok {
		t.Fatal("expected/actual payload was not recognized as verification report")
	}
	if report.Status != verification.StatusFail || report.Expected == "" || report.Actual == "" {
		t.Fatalf("report = %#v, want FAIL report with expected/actual", report)
	}
}

func TestRunTurnVerificationCompletionGateRetriesMissingReport(t *testing.T) {
	traceDir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", traceDir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-synthetic-check",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_check",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("已完成，已验证，结论明确。", nil),
		schema.AssistantMessage("缺少结构化 VerificationReport，当前不能给完成结论。", nil),
	}}
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.check", Description: "synthetic execution evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: `{"result":"synthetic check completed without report"}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-verification-completion-missing",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-verification-completion-missing",
		Input:       "执行 synthetic 操作并验证结果",
		Metadata:    map[string]string{"taskDepth": "operations"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output != "缺少结构化 VerificationReport，当前不能给完成结论。" {
		t.Fatalf("final output = %q, want blocker final", result.Output)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model calls = %d, want gate-triggered third call", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-verification-completion-missing")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing iterations: %#v", session)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeEvidence, agentstate.ItemStatusBlocked) {
		t.Fatalf("agent items missing blocked verification gate item: %#v", session.CurrentTurn.AgentItems)
	}
	data := readRuntimeTrace(t, session.CurrentTurn.Iterations[1].ModelInputTraceFile)
	for _, want := range []string{`"completionGate"`, `"block_success_final"`, `"missing_verification_report"`, `"execution_required"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("trace missing %q:\n%s", want, data)
		}
	}
}

func TestRunTurnVerificationCompletionGateRetriesProseApprovalForScopedMutation(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-synthetic-status",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "exec_command",
				Arguments: `{"command":"systemctl","args":["status","synthetic-service"]}`,
			},
		}}),
		schema.AssistantMessage("变更前状态正常。请确认是否批准执行这个重启操作？批准后我将继续执行。", nil),
		schema.AssistantMessage("tool_unavailable: runtime approval gate was required but no mutating tool call was issued; next action is to invoke the scoped mutating tool so the runtime can request approval.", nil),
	}}
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "exec_command", Description: "synthetic host command"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: `{"schemaVersion":"aiops.terminal/v1","status":"ok","command":"systemctl status synthetic-service","stdout":"active (running)","exitCode":0}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-prose-approval-mutation",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-prose-approval-mutation",
		Input:       "在 @host-a 上重启服务。需要先展示审批，用户批准后继续同一个 turn。",
		Metadata: map[string]string{
			"taskDepth":                       "operations",
			"aiops.intent.kind":               "change",
			"aiops.intent.riskBudget":         "host_exec,write",
			"aiops.route.mode":                "host_bound_ops",
			"aiops.route.allowsExecCommand":   "true",
			"aiops.route.requiresHostBinding": "true",
			"aiops.tool.execCommandAllowed":   "true",
			"aiops.tool.hostMutationAllowed":  "true",
			"aiops.target.binding":            "host",
			"aiops.target.hostId":             "host-a",
			"aiops.target.refs":               "host:host-a",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output != "tool_unavailable: runtime approval gate was required but no mutating tool call was issued; next action is to invoke the scoped mutating tool so the runtime can request approval." {
		t.Fatalf("final output = %q, want gate retry output", result.Output)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model calls = %d, want completion-gate retry after prose approval", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-prose-approval-mutation")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	decision := EvaluateVerificationCompletionGate(session.CurrentTurn.TaskDepth, session.CurrentTurn)
	if !containsString(decision.Reasons, "missing_runtime_approval_gate") {
		t.Fatalf("completion gate reasons = %v, want missing_runtime_approval_gate", decision.Reasons)
	}
	if prompt := verificationCompletionGateRetryPrompt(decision); !strings.Contains(prompt, "Do not ask for approval in prose") {
		t.Fatalf("retry prompt missing runtime approval instruction:\n%s", prompt)
	}
}

func TestRunTurnVerificationCompletionGateAllowsEvidenceBackedVerificationSummary(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "call-docker-run",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "synthetic_terminal",
					Arguments: `{"command":"docker","args":["run","-d","--name","synthetic-nginx","-p","18081:80","nginx:latest"]}`,
				},
			},
			{
				ID:   "call-docker-ps",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "synthetic_terminal",
					Arguments: `{"command":"docker","args":["ps","--filter","name=synthetic-nginx","--format","{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Ports}}"]}`,
				},
			},
			{
				ID:   "call-curl",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "synthetic_terminal",
					Arguments: `{"command":"curl","args":["-fsS","http://127.0.0.1:18081"]}`,
				},
			},
		}),
		schema.AssistantMessage("完成。nginx 临时测试容器已成功启动并验证通过。\n\n验证结果：\n- docker ps：容器正常运行\n- curl http://127.0.0.1:18081：连通性正常", nil),
		schema.AssistantMessage("不应该触发第三次模型调用", nil),
	}}
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic_terminal", Description: "synthetic terminal execution"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			command := strings.TrimSpace(req.Command + " " + strings.Join(req.Args, " "))
			payload := map[string]any{
				"schemaVersion": "aiops.terminal/v1",
				"tool":          "exec_command",
				"status":        "ok",
				"command":       command,
				"stdout":        "synthetic success",
				"exitCode":      0,
			}
			data, _ := json.Marshal(payload)
			return tooling.ToolResult{Content: string(data)}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-verification-completion-evidence-backed-summary",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-verification-completion-evidence-backed-summary",
		Input:       "启动一个临时 nginx 容器并验证结果",
		Metadata:    map[string]string{"taskDepth": "operations"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if !strings.Contains(result.Output, "验证通过") {
		t.Fatalf("final output = %q, want evidence-backed verification summary", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want no completion-gate retry after evidence-backed verification summary", len(model.inputs))
	}
}

func TestRunTurnAnalysisOnlyOpsQuestionDoesNotRetryForMissingExecutionReport(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("结论（置信度：低）：这是原理分析，未连接环境也未执行命令。可能原因是恢复后的节点历史状态与目标主节点不一致。\n\n证据清单：需要后续采集两端状态、恢复参数和加入流程日志确认。", nil),
		schema.AssistantMessage("不应该触发第二次模型调用", nil),
	}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-analysis-only-no-exec",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-analysis-only-no-exec",
		Input:       "我做了几次备份并恢复了一个节点，现在从节点加入后同步异常，为什么会这样？先只做原理分析和证据清单，不要连接或执行任何主机命令。",
		Metadata: map[string]string{
			"aiops.route.mode":                   "chat_advisory",
			"aiops.tool.execCommandAllowed":      "false",
			"aiops.route.userProhibitedHostExec": "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if !strings.Contains(result.Output, "原理分析") {
		t.Fatalf("final output = %q, want first model answer", result.Output)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want no completion-gate retry", len(model.inputs))
	}
}

func TestRunTurnVerificationCompletionGateCompletesEvidenceBackedStatusAnswer(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-synthetic-check-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_check",
				Arguments: `{"scope":"cpu"}`,
			},
		}}),
		schema.AssistantMessage("CPU 状态正常：CPU 使用率 10.54%，系统态 14.69%，空闲 74.76%。", nil),
	}}
	calls := 0
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.check", Description: "synthetic execution evidence"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			calls++
			return tooling.ToolResult{Content: `{"cpu":"normal","idle":"74.76%"}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-verification-completion-status-answer",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-verification-completion-status-answer",
		Input:       "检查 CPU 状态",
		Metadata:    map[string]string{"taskDepth": "operations"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if calls != 1 {
		t.Fatalf("tool calls = %d, want 1", calls)
	}
	if !strings.Contains(result.Output, "CPU 状态正常") {
		t.Fatalf("final output = %q, want evidence-backed status answer", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want no completion-gate retry after evidence-backed status answer", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-verification-completion-status-answer")
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing session turn: %#v", session)
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted {
		t.Fatalf("turn lifecycle = %q, want completed", session.CurrentTurn.Lifecycle)
	}
	if len(session.PendingApprovals) != 0 || len(session.CurrentTurn.PendingApprovals) != 0 {
		t.Fatalf("pending approvals should be cleared after completed status answer: session=%#v turn=%#v", session.PendingApprovals, session.CurrentTurn.PendingApprovals)
	}
}

func TestRunTurnVerificationCompletionGateAllowsPassReport(t *testing.T) {
	traceDir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", traceDir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-synthetic-verify",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_verify",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("已完成，VerificationReport 为 PASS。", nil),
	}}
	tool := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.verify", Description: "synthetic verification report"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			report := verification.VerificationReport{
				ID:          "vr-synthetic-pass",
				Requirement: verification.VerificationExecutionRequired,
				Status:      verification.StatusPass,
				Subject:     "synthetic operation",
				Evidence: []verification.EvidenceRecord{{
					Kind:   verification.EvidenceExecution,
					Result: verification.EvidenceResultPass,
					RawRef: "artifact://synthetic/pass",
				}},
			}
			data, _ := json.Marshal(map[string]any{"verificationReport": report})
			return tooling.ToolResult{Content: string(data)}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{tool}, nil, nil)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-verification-completion-pass",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-verification-completion-pass",
		Input:       "执行 synthetic 操作并验证结果",
		Metadata:    map[string]string{"taskDepth": "operations"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output != "已完成，VerificationReport 为 PASS。" {
		t.Fatalf("final output = %q, want pass final", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want no gate retry", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-verification-completion-pass")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing iterations: %#v", session)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeEvidence, agentstate.ItemStatusCompleted) {
		t.Fatalf("agent items missing completed verification gate item: %#v", session.CurrentTurn.AgentItems)
	}
	data := readRuntimeTrace(t, session.CurrentTurn.Iterations[1].ModelInputTraceFile)
	for _, want := range []string{`"verificationReportRef"`, `"vr-synthetic-pass"`, `"verificationStatus"`, `"PASS"`, `"verification_pass"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("trace missing %q:\n%s", want, data)
		}
	}
}

func verificationReportSnapshot(t *testing.T, report verification.VerificationReport) *TurnSnapshot {
	t.Helper()
	data, err := json.Marshal(map[string]any{"verificationReport": report})
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	return &TurnSnapshot{
		Iterations: []IterationState{{
			ToolResults: []ToolResult{{
				ToolCallID: "call-" + report.ID,
				Content:    string(data),
			}},
		}},
	}
}

func syntheticPassVerificationReportContent(t *testing.T, id, subject string) string {
	t.Helper()
	report := verification.VerificationReport{
		ID:          id,
		Requirement: verification.VerificationExecutionRequired,
		Status:      verification.StatusPass,
		Subject:     subject,
		Evidence: []verification.EvidenceRecord{{
			Kind:   verification.EvidenceExecution,
			Result: verification.EvidenceResultPass,
			RawRef: "artifact://synthetic/" + id,
		}},
	}
	data, err := json.Marshal(map[string]any{"verificationReport": report})
	if err != nil {
		t.Fatalf("marshal synthetic verification report: %v", err)
	}
	return string(data)
}

func readRuntimeTrace(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace %q: %v", path, err)
	}
	return string(data)
}
