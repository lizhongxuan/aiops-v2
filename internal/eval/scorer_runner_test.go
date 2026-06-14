package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
)

func TestScoreCaseAppliesContentToolFileAndQualityChecks(t *testing.T) {
	tc := Case{
		ID:       "code-analysis",
		Category: "代码分析",
		Input:    "分析 runtime kernel",
		Expected: Expected{
			MustInclude:       []string{"RuntimeKernel", "AgentEvent"},
			MustNotInclude:    []string{"不知道"},
			ExpectedToolCalls: []string{"read_file"},
			MustMentionFiles:  []string{"internal/runtimekernel/eino_kernel.go"},
			ExpectedTurnItems: []string{"user_message", "model_call", "final_answer"},
		},
	}
	output := RunOutput{
		Answer: "结论：RuntimeKernel 通过 internal/runtimekernel/eino_kernel.go 驱动 turn，并把 AgentEvent 作为验证链路。验证方式：go test ./internal/runtimekernel ./internal/eval。",
		ToolCalls: []ToolCall{
			{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"internal/runtimekernel/eino_kernel.go"}`)},
		},
		TurnItems: []agentstate.TurnItem{
			{ID: "item-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted},
			{ID: "item-2", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted},
			{ID: "item-3", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected score to pass, got %#v", score)
	}
	if score.Score != 1 {
		t.Fatalf("expected full score, got %v", score.Score)
	}
	for _, check := range score.Checks {
		if !check.Passed {
			t.Fatalf("expected check %q to pass: %#v", check.Name, check)
		}
	}
}

func TestScoreCaseDetectsRegressions(t *testing.T) {
	tc := Case{
		ID:       "debug",
		Category: "Debug 排错",
		Input:    "定位失败原因",
		Expected: Expected{
			MustInclude:       []string{"根因"},
			MustNotInclude:    []string{"直接重启"},
			ExpectedToolCalls: []string{"run_command"},
			MustMentionFiles:  []string{"internal/runtimekernel/dispatch.go"},
		},
	}
	output := RunOutput{
		Answer:    "可能是环境问题，建议直接重启。",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file"}},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected regression score to fail, got %#v", score)
	}
	failed := map[string]bool{}
	for _, check := range score.Checks {
		if !check.Passed {
			failed[check.Name] = true
		}
	}
	for _, name := range []string{"mustInclude", "mustNotInclude", "expectedToolCalls", "mustMentionFiles", "notVague", "hasVerification"} {
		if !failed[name] {
			t.Fatalf("expected %s to fail, failed checks: %#v", name, failed)
		}
	}
}

func TestScoreCaseExpectedToolCallsAllowExtraTools(t *testing.T) {
	tc := Case{
		ID:       "tool-subset",
		Category: "工具调用",
		Input:    "检查工具调用",
		Expected: Expected{
			ExpectedToolCalls: []string{"exec_command"},
		},
	}
	output := RunOutput{
		Answer: "已用 exec_command 检查，并补充验证方式：go test ./internal/eval。",
		ToolCalls: []ToolCall{
			{ID: "call-1", Name: "update_plan"},
			{ID: "call-2", Name: "exec_command"},
		},
	}

	score := ScoreCase(tc, output)

	check := findCheck(score.Checks, "expectedToolCalls")
	if !check.Passed {
		t.Fatalf("expectedToolCalls should pass when required tool is present: %#v", check)
	}
	if len(check.Unexpected) != 1 || check.Unexpected[0] != "update_plan" {
		t.Fatalf("expected unexpected tool to be recorded for diagnostics: %#v", check)
	}
}

func TestScoreCaseDetectsMissingExpectedTurnItems(t *testing.T) {
	tc := Case{
		ID:       "turn-items",
		Category: "协议状态",
		Input:    "检查 turn item",
		Expected: Expected{
			ExpectedTurnItems: []string{"user_message", "model_call", "tool_call", "tool_result", "final_answer"},
		},
	}
	output := RunOutput{
		Answer: "已记录 user_message 和 model_call，验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "item-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted},
			{ID: "item-2", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected missing turn items to fail: %#v", score)
	}
	found := false
	for _, check := range score.Checks {
		if check.Name == "expectedTurnItems" {
			found = true
			if check.Passed {
				t.Fatalf("expectedTurnItems should fail: %#v", check)
			}
			if len(check.Missing) != 3 {
				t.Fatalf("missing turn items = %#v, want 3 missing", check.Missing)
			}
		}
	}
	if !found {
		t.Fatalf("expected expectedTurnItems check, got %#v", score.Checks)
	}
}

func TestScoreCaseChecksPlanPresenceAndStatuses(t *testing.T) {
	planPayload, _ := json.Marshal(planning.PlanState{
		Status: planning.PlanStatusActive,
		Steps: []planning.PlanStep{
			{ID: "inspect", Text: "Inspect host", Status: planning.StepStatusInProgress},
			{ID: "summarize", Text: "Summarize", Status: planning.StepStatusPending},
		},
	})
	tc := Case{
		ID:       "plan-required",
		Category: "计划协议",
		Input:    "复杂任务必须有 plan",
		Expected: Expected{
			MustHavePlan:         true,
			ExpectedPlanStatuses: []string{"in_progress", "pending"},
		},
	}
	output := RunOutput{
		Answer: "复杂任务已经生成结构化 plan。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "plan-1", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: planPayload}},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected plan checks to pass, got %#v", score)
	}
}

func TestScorerChecksPlanModeTraceExpectations(t *testing.T) {
	modelPayload, _ := json.Marshal(map[string]any{
		"planModeState": map[string]any{
			"state":          "active",
			"planId":         "plan-synthetic-1",
			"approvalStatus": "pending_exit_approval",
		},
		"planRequirementDecision": map[string]any{
			"required": true,
			"reason":   "multi_step",
		},
		"planCompletionGate": map[string]any{
			"decision": "block",
			"reasons":  []string{"pending_evidence"},
		},
		"taskClaims": []map[string]any{{
			"taskId": "step-2",
			"owner":  "agent:planner",
		}},
		"planApprovalScope": map[string]any{
			"approvedScopes": []string{"internal/promptcompiler"},
		},
		"planRejectionEvents": []map[string]any{{
			"reason": "scope too broad",
		}},
	})
	tc := Case{
		ID:       "plan-trace",
		Category: "plan mode",
		Input:    "复杂任务需要 plan mode trace",
		Expected: Expected{
			ExpectedPlanModeState:       []string{"active", "pending_exit_approval"},
			ExpectedPlanRequirement:     []string{"multi_step"},
			ExpectedPlanCompletionGate:  []string{"block", "pending_evidence"},
			ExpectedTaskClaims:          []string{"step-2", "agent:planner"},
			ExpectedPlanApprovalScope:   []string{"internal/promptcompiler"},
			ExpectedPlanRejectionEvents: []string{"scope too broad"},
			MustInclude:                 []string{"Plan Mode"},
			MustMentionEvidenceLimits:   false,
			ForbidFirstTurnNoToolFinal:  false,
			MustHavePlan:                false,
			MustHaveEvidence:            false,
			ExpectedPlanStatuses:        nil,
			ExpectedApprovals:           nil,
			ExpectedEvidence:            nil,
			ExpectedTurnItems:           nil,
			ExpectedToolCalls:           nil,
			MustMentionFiles:            nil,
			MustNotInclude:              nil,
		},
	}
	output := RunOutput{
		Answer: "Plan Mode trace 已记录；验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: modelPayload}},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected plan trace checks to pass, got %#v", score)
	}
	for _, name := range []string{"expectedPlanModeState", "expectedPlanRequirement", "expectedPlanCompletionGate", "expectedTaskClaims", "expectedPlanApprovalScope", "expectedPlanRejectionEvents"} {
		if check := findCheck(score.Checks, name); !check.Passed {
			t.Fatalf("check %s = %#v, want pass", name, check)
		}
	}
}

func TestScoreCaseFailsWhenPlanIsForbidden(t *testing.T) {
	tc := Case{
		ID:       "simple-no-plan",
		Category: "简单问答",
		Input:    "简单问答不应强制 plan",
		Expected: Expected{
			MustNotHavePlan: true,
		},
	}
	output := RunOutput{
		Answer: "简单问答直接回答。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "plan-1", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected forbidden plan to fail, got %#v", score)
	}
	if check := findCheck(score.Checks, "planPresence"); check.Passed || len(check.Unexpected) == 0 {
		t.Fatalf("planPresence check = %#v, want unexpected plan", check)
	}
}

func TestScoreCaseChecksApprovalsEvidenceAndBudgets(t *testing.T) {
	approvalData, _ := json.Marshal(map[string]any{
		"approvalId":   "approval-1",
		"approvalType": "command",
		"command":      "kubectl rollout undo deploy/payment-api -n prod",
		"reason":       "restart approval",
		"risk":         "high",
		"targets":      []string{"prod/payment-api"},
	})
	evidenceData, _ := json.Marshal(map[string]any{
		"kind":       "log",
		"summary":    "error logs",
		"source":     "loki",
		"window":     "10m",
		"rawRef":     `{app="payment-api"} |= "error"`,
		"confidence": "high",
	})
	tc := Case{
		ID:       "governance",
		Category: "治理",
		Input:    "检查审批和证据",
		Expected: Expected{
			ExpectedApprovals: []string{"restart approval", "kubectl rollout undo", "high"},
			ExpectedEvidence:  []string{"error logs", "loki", "10m", `{app="payment-api"}`},
			MaxIterations:     1,
			MaxToolCalls:      1,
		},
	}
	output := RunOutput{
		Answer:    "已收集 error logs 并等待 restart approval。验证方式：go test ./internal/eval。",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read_logs"}},
		TurnItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted},
			{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusPending, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "restart approval", Data: approvalData}},
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "log", Summary: "error logs", Data: evidenceData}},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected governance checks to pass, got %#v", score)
	}
	for _, name := range []string{"expectedApprovals", "expectedEvidence", "maxIterations", "maxToolCalls"} {
		if check := findCheck(score.Checks, name); !check.Passed {
			t.Fatalf("check %s = %#v, want pass", name, check)
		}
	}
}

func TestScoreCaseFailsWhenStructuredEvidenceOrApprovalIsMissing(t *testing.T) {
	tc := Case{
		ID:       "governance-missing",
		Category: "治理",
		Input:    "检查审批和证据",
		Expected: Expected{
			ExpectedApprovals: []string{"kubectl rollout undo"},
			ExpectedEvidence:  []string{"prometheus", "rawRef"},
		},
	}
	output := RunOutput{
		Answer: "只有普通回答。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "5xx spike"}},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected missing structured evidence/approval to fail, got %#v", score)
	}
	for _, name := range []string{"expectedApprovals", "expectedEvidence"} {
		check := findCheck(score.Checks, name)
		if check.Passed || len(check.Missing) == 0 {
			t.Fatalf("check %s = %#v, want missing structured field", name, check)
		}
	}
}

func TestScoreCaseRequiresStructuredEvidenceWhenConfigured(t *testing.T) {
	tc := Case{
		ID:       "evidence-required",
		Category: "诊断",
		Input:    "分析目标服务在指定时间窗内的关键指标异常。",
		Expected: Expected{
			MustHaveEvidence: true,
		},
	}
	output := RunOutput{
		Answer: "结论基于目标服务的关键指标摘要，但当前输出没有结构化证据记录。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "item-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted},
			{ID: "item-2", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted},
			{ID: "item-3", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected missing evidence to fail, got %#v", score)
	}
	if check := findCheck(score.Checks, "mustHaveEvidence"); check.Passed || len(check.Missing) == 0 {
		t.Fatalf("mustHaveEvidence check = %#v, want missing evidence", check)
	}
}

func TestScoreCaseForbidsFirstTurnFinalWithoutToolUse(t *testing.T) {
	tc := Case{
		ID:       "premature-final",
		Category: "诊断",
		Input:    "分析复杂告警并给出证据链。",
		Expected: Expected{
			ForbidFirstTurnNoToolFinal: true,
		},
	}
	output := RunOutput{
		Answer: "结论直接给出目标资源异常，但没有先读取指定时间窗内的关键指标或事件记录。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "item-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted},
			{ID: "item-2", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted},
			{ID: "item-3", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected premature final to fail, got %#v", score)
	}
	if check := findCheck(score.Checks, "prematureFinal"); check.Passed || len(check.Unexpected) == 0 {
		t.Fatalf("prematureFinal check = %#v, want unexpected final answer", check)
	}
}

func TestScoreCaseRequiresEvidenceLimitsWhenConfigured(t *testing.T) {
	evidenceData, _ := json.Marshal(map[string]any{
		"kind":    "metric",
		"summary": "关键指标在指定时间窗内升高",
		"source":  "目标观测面",
		"window":  "指定时间窗",
	})
	tc := Case{
		ID:       "evidence-limits",
		Category: "诊断",
		Input:    "基于目标服务证据输出诊断。",
		Expected: Expected{
			MustMentionEvidenceLimits: true,
		},
	}
	output := RunOutput{
		Answer: "结论：目标服务的关键指标在指定时间窗内升高，目标资源需要继续排查。验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "item-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "metric", Summary: "关键指标在指定时间窗内升高", Data: evidenceData}},
			{ID: "item-2", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted},
		},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected missing evidence limits to fail, got %#v", score)
	}
	if check := findCheck(score.Checks, "evidenceLimits"); check.Passed || len(check.Missing) == 0 {
		t.Fatalf("evidenceLimits check = %#v, want missing evidence limitation", check)
	}
}

func TestMockAgentEmitsStructuredAIOpsEvidenceAndApproval(t *testing.T) {
	tc := Case{
		ID:       "aiops-structured-evidence",
		Category: "aiops",
		Input:    "支付服务 5xx 上升，判断是否需要回滚。",
		Expected: Expected{
			ExpectedApprovals: []string{"kubectl rollout undo deploy/payment-api -n prod"},
			ExpectedEvidence:  []string{"payment-api 5xx metric", "loki error logs"},
		},
	}

	output, err := MockAgent{}.Run(context.Background(), tc)
	if err != nil {
		t.Fatalf("run mock agent: %v", err)
	}
	score := ScoreCase(tc, output)
	if !score.Passed {
		t.Fatalf("expected mock aiops evidence case to pass, got %#v", score)
	}

	var evidenceSources []string
	var approvalCommand string
	for _, item := range output.TurnItems {
		switch item.Type {
		case agentstate.TurnItemTypeEvidence:
			var payload map[string]any
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("decode evidence payload: %v", err)
			}
			evidenceSources = append(evidenceSources, strings.TrimSpace(fmt.Sprint(payload["source"])))
		case agentstate.TurnItemTypeApproval:
			var payload map[string]any
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("decode approval payload: %v", err)
			}
			approvalCommand = strings.TrimSpace(fmt.Sprint(payload["command"]))
		}
	}
	if !stringSliceContainsFold(evidenceSources, "prometheus") || !stringSliceContainsFold(evidenceSources, "loki") {
		t.Fatalf("evidence sources = %#v, want prometheus and loki", evidenceSources)
	}
	if !strings.Contains(approvalCommand, "kubectl rollout undo") {
		t.Fatalf("approval command = %q, want rollout undo command", approvalCommand)
	}
}

func TestScoreCaseUsesNormalizedScoreWeights(t *testing.T) {
	tc := Case{
		ID:       "weights",
		Category: "评分",
		Input:    "检查权重",
		Expected: Expected{
			MustInclude:       []string{"required-answer"},
			ExpectedToolCalls: []string{"read_file"},
		},
		ScoreRules: ScoreRules{Weights: map[string]float64{
			"answer": 3,
			"tools":  1,
		}},
	}
	output := RunOutput{
		Answer:    "缺少关键内容，但有验证方式：go test ./internal/eval。",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file"}},
	}

	score := ScoreCase(tc, output)

	if score.ScoreWeights["answer"] <= score.ScoreWeights["tools"] {
		t.Fatalf("score weights = %#v, want answer weight greater than tools", score.ScoreWeights)
	}
	if score.Score <= 0 || score.Score >= 1 {
		t.Fatalf("weighted score = %f, want partial score", score.Score)
	}
}

func TestRunnerWritesArtifactsAndReport(t *testing.T) {
	casesDir := t.TempDir()
	outDir := t.TempDir()
	writeJSON(t, filepath.Join(casesDir, "code-analysis.json"), Case{
		ID:       "code-analysis",
		Category: "代码分析",
		Input:    "请分析 runtime kernel 的事件链路",
		Expected: Expected{
			MustInclude:       []string{"RuntimeKernel", "AgentEvent"},
			MustNotInclude:    []string{"无法判断"},
			ExpectedToolCalls: []string{"read_file"},
			MustMentionFiles:  []string{"internal/runtimekernel/eino_kernel.go"},
		},
	})

	report, err := Runner{
		CasesDir:  casesDir,
		OutputDir: outDir,
		Agent:     MockAgent{},
		RunID:     "unit-run",
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.Summary.Total != 1 || report.Summary.Passed != 1 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}

	caseDir := filepath.Join(outDir, "code-analysis")
	for _, name := range []string{"answer.txt", "agent_events.json", "tool_calls.json", "turn_items.json"} {
		if _, err := os.Stat(filepath.Join(caseDir, name)); err != nil {
			t.Fatalf("expected artifact %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "report.json")); err != nil {
		t.Fatalf("expected report.json: %v", err)
	}
}

func TestRunnerRepetitionsAggregateAverageMinimumAndArtifacts(t *testing.T) {
	casesDir := t.TempDir()
	outDir := t.TempDir()
	writeJSON(t, filepath.Join(casesDir, "repeat.json"), Case{
		ID:       "repeat",
		Category: "诊断",
		Input:    "repeat this case",
		Expected: Expected{MustInclude: []string{"stable-answer"}},
	})
	agent := &scriptedEvalAgent{outputs: []RunOutput{
		{Answer: "stable-answer。验证方式：go test ./internal/eval。"},
		{Answer: "missing expected phrase。验证方式：go test ./internal/eval。"},
		{Answer: "stable-answer。验证方式：go test ./internal/eval。"},
	}}

	report, err := Runner{
		CasesDir:    casesDir,
		OutputDir:   outDir,
		Agent:       agent,
		RunID:       "repeat-run",
		RunPhase:    "candidate",
		Repetitions: 3,
		Metadata:    map[string]string{"AIOPS_DIAGNOSTIC_PROTOCOL": "1"},
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.RunPhase != "candidate" || report.Repetitions != 3 || report.Metadata["AIOPS_DIAGNOSTIC_PROTOCOL"] != "1" {
		t.Fatalf("report metadata not preserved: %#v", report)
	}
	if len(report.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(report.Cases))
	}
	score := report.Cases[0]
	if score.Iterations != 3 || len(score.IterationScores) != 3 || len(score.IterationArtifacts) != 3 {
		t.Fatalf("iteration metadata missing: %#v", score)
	}
	if score.MinScore >= score.AvgScore {
		t.Fatalf("min score = %.2f avg = %.2f, want min below avg", score.MinScore, score.AvgScore)
	}
	if score.Passed {
		t.Fatalf("aggregate should fail when any repetition fails: %#v", score)
	}
	if report.Summary.LowestScoreAverage != score.MinScore || report.Summary.MinScore != score.MinScore {
		t.Fatalf("summary min fields = %#v, case min %.2f", report.Summary, score.MinScore)
	}
	for _, name := range []string{
		filepath.Join(outDir, "repeat", "1", "answer.txt"),
		filepath.Join(outDir, "repeat", "2", "answer.txt"),
		filepath.Join(outDir, "repeat", "3", "answer.txt"),
	} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("expected repetition artifact %s: %v", name, err)
		}
	}
}

func TestP0SmokeCasesExistAndPassMockAgent(t *testing.T) {
	casesDir := filepath.Clean("../../testdata/eval_cases")
	outDir := t.TempDir()

	report, err := Runner{
		CasesDir:  casesDir,
		OutputDir: outDir,
		Agent:     MockAgent{},
		RunID:     "p0-smoke",
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("run p0 smoke eval cases: %v", err)
	}

	required := map[string]bool{
		"approval-blocked":        false,
		"approval-denied":         false,
		"context-compaction-goal": false,
		"high-risk-blocked":       false,
		"plan-required":           false,
		"prompt-trace-diff":       false,
		"simple-no-plan":          false,
		"memory-hit":              false,
		"memory-miss":             false,
		"stale-memory-ignored":    false,
		"tool-failure-fallback":   false,
		"synthesis-only":          false,
		"simple-chat-no-plan":     false,
	}
	for _, c := range report.Cases {
		if _, ok := required[c.CaseID]; ok {
			required[c.CaseID] = true
			if !c.Passed {
				t.Fatalf("required P0 smoke case %s did not pass: %#v", c.CaseID, c)
			}
		}
	}
	for id, found := range required {
		if !found {
			t.Fatalf("required P0 smoke case %s was not loaded; report cases=%#v", id, report.Cases)
		}
	}
}

func TestCompareReportsFlagsBetterWorseAndSame(t *testing.T) {
	baseline := Report{Cases: []CaseScore{
		{CaseID: "better", Score: 0.5, Passed: false},
		{CaseID: "worse", Score: 1, Passed: true},
		{CaseID: "same", Score: 0.75, Passed: false},
	}}
	current := Report{Cases: []CaseScore{
		{CaseID: "better", Score: 1, Passed: true},
		{CaseID: "worse", Score: 0.5, Passed: false},
		{CaseID: "same", Score: 0.75, Passed: false},
	}}

	diff := CompareReports(baseline, current)

	got := map[string]string{}
	for _, item := range diff.Cases {
		got[item.CaseID] = item.Status
	}
	want := map[string]string{"better": "better", "worse": "worse", "same": "same"}
	for id, status := range want {
		if got[id] != status {
			t.Fatalf("case %s status = %q, want %q; diff=%#v", id, got[id], status, diff)
		}
	}
	if diff.Summary.Better != 1 || diff.Summary.Worse != 1 || diff.Summary.Same != 1 {
		t.Fatalf("unexpected diff summary: %#v", diff.Summary)
	}
}

func TestCompareReportsIncludesRegressedChecksAndMarkdown(t *testing.T) {
	baseline := Report{RunID: "base", Cases: []CaseScore{{
		CaseID: "case-1",
		Score:  1,
		Passed: true,
		Checks: []CheckResult{{Name: "mustInclude", Passed: true}},
	}}}
	current := Report{RunID: "current", Cases: []CaseScore{{
		CaseID: "case-1",
		Score:  0,
		Passed: false,
		Checks: []CheckResult{{Name: "mustInclude", Passed: false}},
	}}}

	comparison := CompareReports(baseline, current)
	if len(comparison.Cases) != 1 || len(comparison.Cases[0].RegressedChecks) != 1 || comparison.Cases[0].RegressedChecks[0] != "mustInclude" {
		t.Fatalf("comparison = %#v, want regressed mustInclude check", comparison)
	}
	current.BaselineComparison = &comparison
	markdown := RenderMarkdownReport(current)
	if !strings.Contains(markdown, "mustInclude") || !strings.Contains(markdown, "worse") {
		t.Fatalf("markdown report missing regression detail:\n%s", markdown)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func findCheck(checks []CheckResult, name string) CheckResult {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return CheckResult{}
}

type scriptedEvalAgent struct {
	outputs []RunOutput
	calls   int
}

func (a *scriptedEvalAgent) Run(_ context.Context, _ Case) (RunOutput, error) {
	if a.calls >= len(a.outputs) {
		return RunOutput{Answer: "no scripted output。验证方式：go test ./internal/eval。"}, nil
	}
	out := a.outputs[a.calls]
	a.calls++
	return out, nil
}
