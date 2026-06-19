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
	if result.Output != "synthetic_read 未成功返回证据；该项未完成检查，结论低置信。" {
		t.Fatalf("final output = %q, want verifier rewrite", result.Output)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model calls = %d, want verifier-triggered third call", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-final-evidence")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing iterations: %#v", session)
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
