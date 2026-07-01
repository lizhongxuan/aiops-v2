package runtimekernel

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/planning"
	"aiops-v2/internal/tooling"
)

func TestRunTurnClaimNextTaskWritesTaskClaimTrace(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	store, err := planning.NewTaskStore(planning.PlanState{
		Status: planning.PlanStatusActive,
		Steps: []planning.PlanStep{{
			ID:     "step-1",
			Text:   "读取现有状态并记录直接证据",
			Status: planning.StepStatusPending,
		}},
	})
	if err != nil {
		t.Fatalf("NewTaskStore() error = %v", err)
	}
	now := time.Date(2026, 6, 7, 1, 0, 0, 0, time.UTC)
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "claim-call-1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "claim_next_task",
				Arguments: `{"owner":"agent:planner","agentId":"agent-synthetic-1"}`,
			},
		}}),
		schema.AssistantMessage("claimed step-1", nil),
	}}
	kernel := newLoopKernel(t, model, []tooling.Tool{planning.NewClaimNextTaskTool(store, func() time.Time { return now })}, nil, nil)

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-task-claim-trace",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-task-claim-trace",
		Input:       "claim the next synthetic task",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	session := kernel.sessions.Get("sess-task-claim-trace")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("missing turn iterations: %#v", session)
	}
	tracePath := session.CurrentTurn.Iterations[1].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	for _, want := range []string{`"taskClaims"`, `"step-1"`, `"agent:planner"`, `"claimed"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace missing %s:\n%s", want, string(data))
		}
	}
}
