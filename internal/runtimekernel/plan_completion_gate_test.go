package runtimekernel

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/planning"
	"aiops-v2/internal/tooling"
)

func TestRunTurnPlanCompletionGateRecordsPendingPlanWithoutProseRetry(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "plan-call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "update_plan",
				Arguments: `{"steps":[{"id":"inspect","text":"读取现有计划状态并记录直接证据","status":"completed","evidenceRefs":["trace-synthetic-1"],"verificationStatus":"passed"},{"id":"verify","text":"运行 focused tests 并记录验证输出","status":"pending"}]}`,
			},
		}}),
		schema.AssistantMessage("已完成，结论明确。", nil),
		schema.AssistantMessage("还有计划步骤 verify 未完成，当前不能给成功结论。", nil),
	}}
	kernel := newLoopKernel(t, model, []tooling.Tool{planning.NewUpdatePlanTool()}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-plan-completion-gate",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-plan-completion-gate",
		Input:       "完成这个多步骤验证任务",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Output != "已完成，结论明确。" {
		t.Fatalf("output = %q, want display answer preserved", result.Output)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, pending plan must not be repaired by prose retry", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-plan-completion-gate")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing turn iterations: %#v", session)
	}
	tracePath := session.CurrentTurn.Iterations[1].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if !strings.Contains(string(data), `"planCompletionGate"`) || !strings.Contains(string(data), `"pending_step"`) {
		t.Fatalf("trace missing plan completion gate:\n%s", string(data))
	}
	if got := goldenFinalContractStatus(session.CurrentTurn); got == string(FinalContractStatusVerified) || got == "" {
		t.Fatalf("final contract status = %q, pending plan must not verify", got)
	}
}
