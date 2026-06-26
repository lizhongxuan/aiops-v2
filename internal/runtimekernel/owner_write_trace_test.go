package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/tooling"
)

func TestOwnerWriteTraceAcceptsCanonicalOwners(t *testing.T) {
	cases := []struct {
		responsibility OwnerWriteResponsibility
		owner          string
	}{
		{OwnerWriteTurnLifecycle, OwnerRuntimeKernel},
		{OwnerWriteAssistantMessage, OwnerRuntimeKernel},
		{OwnerWriteApprovalLedger, OwnerPendingApproval},
		{OwnerWriteToolResult, OwnerToolDispatcher},
		{OwnerWriteContextCompaction, OwnerContextPipeline},
	}

	for _, tc := range cases {
		trace := NewOwnerWriteTrace(OwnerWriteTraceInput{
			Responsibility: tc.responsibility,
			Writer:         tc.owner,
			SessionID:      "sess-owner",
			TurnID:         "turn-owner",
		})
		if trace.Owner != tc.owner {
			t.Fatalf("%s owner = %q, want %q", tc.responsibility, trace.Owner, tc.owner)
		}
		if trace.Outcome != OwnerWriteOutcomeAccepted {
			t.Fatalf("%s outcome = %q, want accepted", tc.responsibility, trace.Outcome)
		}
	}
}

func TestOwnerWriteTraceRejectsNonOwnerWriter(t *testing.T) {
	trace := NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: OwnerWriteAssistantMessage,
		Writer:         "internal/appui.ChatService",
		SessionID:      "sess-owner",
		TurnID:         "turn-owner",
	})

	if trace.Owner != OwnerRuntimeKernel {
		t.Fatalf("owner = %q, want %q", trace.Owner, OwnerRuntimeKernel)
	}
	if trace.Outcome != OwnerWriteOutcomeRejectedNonOwner {
		t.Fatalf("outcome = %q, want rejected_non_owner", trace.Outcome)
	}
}

func TestAppendOwnerWriteTraceCopiesToSessionAndTurn(t *testing.T) {
	session := &SessionState{ID: "sess-owner"}
	turn := &TurnSnapshot{
		ID:          "turn-owner",
		SessionID:   "sess-owner",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
	}

	trace := NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: OwnerWriteToolResult,
		Writer:         OwnerToolDispatcher,
		SessionID:      session.ID,
		TurnID:         turn.ID,
	})
	AppendOwnerWriteTrace(session, turn, trace)

	if len(session.OwnerWriteTraces) != 1 {
		t.Fatalf("session owner traces = %d, want 1", len(session.OwnerWriteTraces))
	}
	if len(turn.OwnerWriteTraces) != 1 {
		t.Fatalf("turn owner traces = %d, want 1", len(turn.OwnerWriteTraces))
	}
	if session.OwnerWriteTraces[0].Responsibility != OwnerWriteToolResult || turn.OwnerWriteTraces[0].Outcome != OwnerWriteOutcomeAccepted {
		t.Fatalf("unexpected owner traces: session=%#v turn=%#v", session.OwnerWriteTraces, turn.OwnerWriteTraces)
	}
}

func TestRunTurnRecordsLifecycleAndAssistantMessageOwnerWriteTrace(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{schema.AssistantMessage("done", nil)},
	}
	kernel := newLoopKernel(t, model, nil, nil, policyengine.NewDefaultModePolicies())

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-final-owner",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-final-owner",
		Input:       "say done",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("RunTurn status = %q, want completed", result.Status)
	}
	session := kernel.sessions.Get("sess-final-owner")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteTurnLifecycle, OwnerRuntimeKernel, OwnerWriteOutcomeAccepted)
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteAssistantMessage, OwnerRuntimeKernel, OwnerWriteOutcomeAccepted)
}

func TestRunTurnApprovalResumeRecordsApprovalAndToolOwnerWriteTrace(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-owner-approval",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"/tmp/owner","content":"ok"}`,
				},
			}}),
			schema.AssistantMessage("write complete", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write a file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "wrote:" + string(input)}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())

	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-approval-owner",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-approval-owner",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("RunTurn status = %q, want blocked", blocked.Status)
	}
	session := kernel.sessions.Get("sess-approval-owner")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected blocked current turn")
	}
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteTurnLifecycle, OwnerRuntimeKernel, OwnerWriteOutcomeAccepted)
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteApprovalLedger, OwnerPendingApproval, OwnerWriteOutcomeAccepted)

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: "sess-approval-owner",
		TurnID:    "turn-approval-owner",
		Decision:  "approved",
	})
	if err != nil {
		t.Fatalf("ResumeTurn() error = %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("ResumeTurn status = %q, want completed", resumed.Status)
	}
	session = kernel.sessions.Get("sess-approval-owner")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected resumed current turn")
	}
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteToolResult, OwnerToolDispatcher, OwnerWriteOutcomeAccepted)
	assertOwnerWriteTrace(t, session.CurrentTurn.OwnerWriteTraces, OwnerWriteAssistantMessage, OwnerRuntimeKernel, OwnerWriteOutcomeAccepted)
}

func TestMarkTurnCanceledRecordsLifecycleOwnerWriteTrace(t *testing.T) {
	now := time.Now()
	session := &SessionState{ID: "sess-cancel-owner"}
	snapshot := &TurnSnapshot{
		ID:          "turn-cancel-owner",
		SessionID:   "sess-cancel-owner",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
	}

	kernel := newTestKernel(nil)
	kernel.sessions.Update(session)
	if ok := kernel.markTurnCanceled(session, snapshot, "user stop"); !ok {
		t.Fatal("markTurnCanceled returned false")
	}
	assertOwnerWriteTrace(t, snapshot.OwnerWriteTraces, OwnerWriteTurnLifecycle, OwnerRuntimeKernel, OwnerWriteOutcomeAccepted)
}

func assertOwnerWriteTrace(t *testing.T, traces []OwnerWriteTrace, responsibility OwnerWriteResponsibility, writer string, outcome OwnerWriteOutcome) {
	t.Helper()
	for _, trace := range traces {
		if trace.Responsibility == responsibility && trace.Writer == writer && trace.Outcome == outcome {
			return
		}
	}
	t.Fatalf("owner write trace missing responsibility=%s writer=%s outcome=%s in %#v", responsibility, writer, outcome, traces)
}
