package runtimekernel

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func TestRunTurnObserverRecordsTurnAndModelCall(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	observer := &recordingRuntimeObserver{}
	kernel := newTestKernelWithSpanSource(nil)
	kernel.observer = observer

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:       "sess-observer",
		SessionType:     SessionTypeHost,
		Mode:            ModeChat,
		TurnID:          "turn-observer",
		ClientTurnID:    "client-turn-observer",
		ClientMessageID: "client-message-observer",
		Input:           "hello observer",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}
	if len(observer.turns) != 1 {
		t.Fatalf("recorded turns = %d, want 1", len(observer.turns))
	}
	turn := observer.turns[0]
	if turn.SessionID != "sess-observer" || turn.TurnID != "turn-observer" {
		t.Fatalf("turn attrs = %#v", turn)
	}
	if len(observer.modelCalls) != 1 {
		t.Fatalf("recorded model calls = %d, want 1", len(observer.modelCalls))
	}
	modelCall := observer.modelCalls[0]
	if modelCall.SessionID != "sess-observer" || modelCall.TurnID != "turn-observer" {
		t.Fatalf("model attrs = %#v", modelCall)
	}
	if modelCall.PromptStableHash == "" {
		t.Fatal("model call should include prompt stable hash")
	}
	if modelCall.PromptFingerprint["stableHash"] == "" || modelCall.PromptFingerprint["developerHash"] == "" {
		t.Fatalf("model call should include prompt fingerprint, got %#v", modelCall.PromptFingerprint)
	}
	if modelCall.MessageCount == 0 {
		t.Fatal("model call should include input message count")
	}
	if modelCall.TraceFile == "" || !strings.Contains(modelCall.TraceFile, traceDir) || !strings.HasSuffix(modelCall.TraceFile, ".md") {
		t.Fatalf("trace file = %q, want markdown file under %q", modelCall.TraceFile, traceDir)
	}
	if !strings.Contains(modelCall.TraceFile, "iteration-000-") {
		t.Fatalf("trace file = %q, want iteration-000 markdown trace", modelCall.TraceFile)
	}
	if modelCall.TraceDiffFile != "" {
		t.Fatalf("trace diff file = %q, want empty for the first model call", modelCall.TraceDiffFile)
	}
	if len(observer.statuses) == 0 || observer.statuses[len(observer.statuses)-1] != "completed" {
		t.Fatalf("statuses = %v, want final completed", observer.statuses)
	}
	session := kernel.sessions.Get("sess-observer")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) == 0 {
		t.Fatal("expected turn snapshot with iterations")
	}
	if got := session.CurrentTurn.Iterations[0].PromptFingerprint["stableHash"]; got == "" {
		t.Fatalf("iteration prompt fingerprint missing stableHash: %#v", session.CurrentTurn.Iterations[0].PromptFingerprint)
	}
	if !session.CurrentTurn.Iterations[0].PromptShadowParity.Passed {
		t.Fatalf("iteration prompt shadow parity = %#v, want passed", session.CurrentTurn.Iterations[0].PromptShadowParity)
	}
	legacyIteration := session.CurrentTurn.Iterations[0]
	legacyIteration.PromptShadowParity = promptinput.PromptShadowParityReport{}
	if err := legacyIteration.Validate(); err != nil {
		t.Fatalf("legacy zero prompt shadow parity should remain compatible: %v", err)
	}
	partialParity := session.CurrentTurn.Iterations[0]
	partialParity.PromptShadowParity.SchemaVersion = ""
	if err := partialParity.Validate(); err != nil {
		t.Fatalf("deprecated partial prompt shadow parity blocked iteration state: %v", err)
	}
	failedParity := session.CurrentTurn.Iterations[0]
	failedParity.PromptShadowParity.GateViolations = []string{"deprecated_shadow_drift"}
	failedParity.PromptShadowParity.Passed = false
	if err := failedParity.Validate(); err != nil {
		t.Fatalf("deprecated failed prompt shadow parity blocked iteration state: %v", err)
	}
	for _, item := range session.CurrentTurn.AgentItems {
		if item.ID != modelCallItemID("turn-observer", 0) {
			continue
		}
		var data struct {
			PromptFingerprint  map[string]string                    `json:"promptFingerprint"`
			PromptShadowParity promptinput.PromptShadowParityReport `json:"promptShadowParity"`
		}
		if err := json.Unmarshal(item.Payload.Data, &data); err != nil {
			t.Fatalf("unmarshal model item data: %v", err)
		}
		if data.PromptFingerprint["stableHash"] == "" {
			t.Fatalf("model item prompt fingerprint missing stableHash: %#v", data.PromptFingerprint)
		}
		if !data.PromptShadowParity.Passed {
			t.Fatalf("model item prompt shadow parity = %#v, want passed", data.PromptShadowParity)
		}
		return
	}
	t.Fatal("model_call agent item not found")
}

func TestResumeTurnRestoresObservedTurnTraceContext(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-approval",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
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
	observer := &recordingRuntimeObserver{
		turnTraceContext: TraceContextCarrier{
			"traceparent": "00-11111111111111111111111111111111-2222222222222222-01",
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, policyengine.NewDefaultModePolicies())
	kernel.observer = observer

	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-resume-trace",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-resume-trace",
		Input:       "write the file",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("blocked status = %q, want blocked", blocked.Status)
	}
	session := kernel.sessions.Get("sess-resume-trace")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected suspended current turn")
	}
	if got := session.CurrentTurn.TraceContext["traceparent"]; got != observer.turnTraceContext["traceparent"] {
		t.Fatalf("snapshot traceparent = %q, want %q", got, observer.turnTraceContext["traceparent"])
	}

	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:  "sess-resume-trace",
		TurnID:     "turn-resume-trace",
		ApprovalID: session.PendingApprovals[0].ID,
		Decision:   "approved",
	})
	if err != nil {
		t.Fatalf("ResumeTurn failed: %v", err)
	}
	if resumed.Status != "completed" {
		t.Fatalf("resume status = %q, want completed", resumed.Status)
	}
	if len(observer.restoredTraceContexts) == 0 {
		t.Fatal("ResumeTurn did not restore observed trace context")
	}
	if got := observer.restoredTraceContexts[0]["traceparent"]; got != observer.turnTraceContext["traceparent"] {
		t.Fatalf("restored traceparent = %q, want %q", got, observer.turnTraceContext["traceparent"])
	}
	if len(observer.modelCalls) < 2 {
		t.Fatalf("recorded model calls = %d, want resumed model call", len(observer.modelCalls))
	}
	resumedModelCall := observer.modelCalls[len(observer.modelCalls)-1]
	if resumedModelCall.TraceDiffFile == "" {
		t.Fatal("resumed model call should include trace diff file")
	}
	if _, err := os.Stat(resumedModelCall.TraceDiffFile); err != nil {
		t.Fatalf("trace diff file %q not written: %v", resumedModelCall.TraceDiffFile, err)
	}
}

type recordingRuntimeObserver struct {
	turns                 []TurnSpanAttrs
	modelCalls            []ModelCallSpanAttrs
	statuses              []string
	turnTraceContext      TraceContextCarrier
	restoredTraceContexts []TraceContextCarrier
}

func (r *recordingRuntimeObserver) StartTurn(ctx context.Context, attrs TurnSpanAttrs) (context.Context, ObservedSpan) {
	r.turns = append(r.turns, attrs)
	return normalizeObserverContext(ctx), recordingObservedSpan{traceContext: r.turnTraceContext, status: func(status string) {
		r.statuses = append(r.statuses, status)
	}}
}

func (r *recordingRuntimeObserver) ContextWithTraceContext(ctx context.Context, carrier TraceContextCarrier) context.Context {
	r.restoredTraceContexts = append(r.restoredTraceContexts, copyTraceContextCarrier(carrier))
	return normalizeObserverContext(ctx)
}

func (r *recordingRuntimeObserver) StartStage(ctx context.Context, _ StageSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), recordingObservedSpan{}
}

func (r *recordingRuntimeObserver) StartModelCall(ctx context.Context, attrs ModelCallSpanAttrs) (context.Context, ObservedSpan) {
	r.modelCalls = append(r.modelCalls, attrs)
	return normalizeObserverContext(ctx), recordingObservedSpan{status: func(status string) {
		r.statuses = append(r.statuses, status)
	}}
}

func (r *recordingRuntimeObserver) StartToolCall(ctx context.Context, _ ToolCallSpanAttrs) (context.Context, ObservedSpan) {
	return normalizeObserverContext(ctx), recordingObservedSpan{}
}

type recordingObservedSpan struct {
	traceContext TraceContextCarrier
	status       func(string)
}

func (s recordingObservedSpan) SetAttributes(map[string]any) {}

func (s recordingObservedSpan) SetStatus(status string, _ string) {
	if s.status != nil {
		s.status(status)
	}
}

func (s recordingObservedSpan) End() {}

func (s recordingObservedSpan) TraceContext() TraceContextCarrier {
	return copyTraceContextCarrier(s.traceContext)
}
