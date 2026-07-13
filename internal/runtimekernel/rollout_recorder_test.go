package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestRuntimeKernelCanonicalRolloutOrdersFirstLoopAndDoesNotRepeatAssembly(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("inspect", []schema.ToolCall{{
			ID: "call-read", Type: "function",
			Function: schema.FunctionCall{Name: "rollout_read", Arguments: `{"token":"args-secret-canary"}`},
		}}),
		schema.AssistantMessage("safe-final", nil),
	}}
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "rollout_read", Description: "read typed state"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: `{"status":"healthy","evidenceRefs":["evidence://health"]}`}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	const sessionID = "session-runtime-rollout-order"
	const turnID = "turn-runtime-rollout-order"
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: turnID, Input: "input-secret-canary",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	wantKinds := []string{
		modeltrace.CanonicalRolloutKindAdmission,
		modeltrace.CanonicalRolloutKindAssembly,
		modeltrace.CanonicalRolloutKindPrompt,
		modeltrace.CanonicalRolloutKindProviderRequest,
		modeltrace.CanonicalRolloutKindProviderResponse,
		modeltrace.CanonicalRolloutKindToolProposed,
		modeltrace.CanonicalRolloutKindToolDispatched,
		modeltrace.CanonicalRolloutKindToolResult,
		modeltrace.CanonicalRolloutKindPrompt,
		modeltrace.CanonicalRolloutKindProviderRequest,
		modeltrace.CanonicalRolloutKindProviderResponse,
	}
	if got := canonicalRolloutFilteredKindsForTest(events, wantKinds); !equalCanonicalRolloutKinds(got, wantKinds) {
		t.Fatalf("core event order = %v, want %v; all=%v", got, wantKinds, canonicalRolloutKindsForTest(events))
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("events[%d] sequence = %d, want %d; all=%v", index, event.Sequence, index+1, canonicalRolloutKindsForTest(events))
		}
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindAdmission) != 1 ||
		countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindAssembly) != 1 {
		t.Fatalf("admission/assembly repeated across existing assembly: %v", canonicalRolloutKindsForTest(events))
	}

	session := kernel.sessions.Get(sessionID)
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.CanonicalRolloutHead == nil {
		t.Fatal("persisted turn is missing canonical rollout head")
	}
	head := session.CurrentTurn.CanonicalRolloutHead
	last := events[len(events)-1]
	if err := head.Validate(); err != nil {
		t.Fatalf("CanonicalRolloutHead.Validate() error = %v", err)
	}
	if head.SchemaVersion != last.SchemaVersion || head.EventID != last.EventID || head.Hash != last.Hash ||
		head.Sequence != last.Sequence || head.Status != RolloutRecordStatusRecorded {
		t.Fatalf("head = %+v, want last event ref %+v", head, last)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("json.Marshal(events) error = %v", err)
	}
	for _, forbidden := range []string{"input-secret-canary", "args-secret-canary", "safe-final", "healthy", `\"Arguments\"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("canonical rollout leaked provider/model/tool payload %q: %s", forbidden, encoded)
		}
	}
	proposed := firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindToolProposed)
	if proposed.Payload["callId"] != "call-read" || proposed.Payload["name"] != "rollout_read" || proposed.Payload["argsHash"] == "" {
		t.Fatalf("tool_proposed payload = %#v, want callId/name/argsHash only", proposed.Payload)
	}
	result := firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindToolResult)
	if result.Payload["callId"] != "call-read" || result.Payload["outcome"] != "complete" || result.Payload["contentHash"] == "" || result.Payload["errorClass"] != "" {
		t.Fatalf("tool_result payload = %#v, want typed safe outcome", result.Payload)
	}
	if refs, ok := result.Payload["evidenceRefs"].([]any); !ok || len(refs) != 1 || refs[0] != "evidence://health" {
		t.Fatalf("tool_result evidenceRefs = %#v, want evidence://health", result.Payload["evidenceRefs"])
	}
}

func TestRuntimeKernelProviderRequestRolloutFailureCallsProviderZeroTimes(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindProviderRequest)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("must-not-run", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-provider-request-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-provider-request-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want fail closed rollout append")
	}
	if len(model.inputs) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(model.inputs))
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), "session-provider-request-fail", "turn-provider-request-fail")
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if got := canonicalRolloutKindsForTest(events); !equalCanonicalRolloutKinds(got, []string{
		modeltrace.CanonicalRolloutKindAdmission,
		modeltrace.CanonicalRolloutKindAssembly,
		modeltrace.CanonicalRolloutKindPrompt,
		modeltrace.CanonicalRolloutKindCheckpoint,
		modeltrace.CanonicalRolloutKindFinalFacts,
		modeltrace.CanonicalRolloutKindTransportProjection,
	}) {
		t.Fatalf("persisted kinds = %v, want pre-provider facts followed by failed terminal", got)
	}
}

func TestRuntimeKernelCanonicalRolloutCommitsFinalFactsThenProjectionWithoutText(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("final-markdown-secret-canary", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	const sessionID, turnID = "session-final-boundary", "turn-final-boundary"
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost, Mode: ModeInspect, TurnID: turnID, Input: "inspect",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	want := []string{modeltrace.CanonicalRolloutKindFinalFacts, modeltrace.CanonicalRolloutKindTransportProjection}
	if got := canonicalRolloutFilteredKindsForTest(events, want); !equalCanonicalRolloutKinds(got, want) {
		t.Fatalf("terminal sequence = %v, want %v; all=%v", got, want, canonicalRolloutKindsForTest(events))
	}
	encoded, _ := json.Marshal([]modeltrace.CanonicalRolloutEvent{
		lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindFinalFacts),
		lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection),
	})
	for _, forbidden := range []string{"final-markdown-secret-canary", "answerText", "markdown", "blocks", "AiopsTransportState", `"text"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("terminal rollout leaked %q: %s", forbidden, encoded)
		}
	}
	snapshot := kernel.sessions.Get(sessionID).CurrentTurn
	assertCanonicalProjectionMatchesSnapshot(t, lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection), snapshot, finalContractForRolloutTest(t, snapshot))
}

func TestRuntimeKernelCanonicalFinalFactHashesIgnoreFinalMarkdown(t *testing.T) {
	facts := FinalRuntimeFacts{CompletionStatus: FinalCompletionStatusSucceeded, PostcheckStatus: FinalPostcheckStatusNotRequired, RollbackStatus: FinalRollbackStatusNotRequired}
	plain := BuildFinalContract("plain answer", facts)
	markdown := BuildFinalContract("# Different markdown\n\nsecret-canary", facts)
	plainHash, err := canonicalFinalContractFactHash(&plain)
	if err != nil {
		t.Fatalf("canonicalFinalContractFactHash(plain) error = %v", err)
	}
	markdownHash, err := canonicalFinalContractFactHash(&markdown)
	if err != nil {
		t.Fatalf("canonicalFinalContractFactHash(markdown) error = %v", err)
	}
	if plainHash != markdownHash {
		t.Fatalf("typed final contract hash changed with markdown: %q != %q", plainHash, markdownHash)
	}
}

func TestRuntimeKernelTargetRequiredRecordsTypedTerminalBeforeProvider(t *testing.T) {
	model := &sequentialLoopModel{}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	intent := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}, Confidence: runtimecontract.ConfidenceHigh}
	const sessionID, turnID = "session-target-required-rollout", "turn-target-required-rollout"
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeWorkspace, Mode: ModeExecute, TurnID: turnID,
		Input: "mutate target-required-secret-canary", IntentFrame: &intent,
	})
	if err != nil || result.Status != "completed" || len(model.inputs) != 0 {
		t.Fatalf("RunTurn() = %#v, %v provider=%d; want completed/0", result, err, len(model.inputs))
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	want := []string{modeltrace.CanonicalRolloutKindAdmission, modeltrace.CanonicalRolloutKindCheckpoint, modeltrace.CanonicalRolloutKindFinalFacts, modeltrace.CanonicalRolloutKindTransportProjection}
	if got := canonicalRolloutKindsForTest(events); !equalCanonicalRolloutKinds(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "target-required-secret-canary") || strings.Contains(string(encoded), admissionTargetRequiredFinalText) {
		t.Fatalf("target-required rollout leaked text: %s", encoded)
	}
}

func TestRuntimeKernelTargetRequiredFinalFactsAppendFailureLeavesFinalUncommitted(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindFinalFacts)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder
	intent := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindChange, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite}, Confidence: runtimecontract.ConfidenceHigh}
	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-target-required-fail", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		TurnID: "turn-target-required-fail", Input: "mutate", IntentFrame: &intent,
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want final_facts append failure")
	}
	snapshot := kernel.sessions.Get("session-target-required-fail").CurrentTurn
	if snapshot.Lifecycle == TurnLifecycleCompleted || strings.TrimSpace(snapshot.FinalOutput) != "" {
		t.Fatalf("failed append committed final state: lifecycle=%s final=%q", snapshot.Lifecycle, snapshot.FinalOutput)
	}
	if len(model.inputs) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(model.inputs))
	}
}

func TestRuntimeKernelProviderResponseRolloutFailureStopsBeforeToolAndFinal(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	toolCalls := 0
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "rollout_mutation", Description: "must not execute"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			toolCalls++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-mutation", Type: "function",
			Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
		}}),
	}}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-provider-response-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-provider-response-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want provider_response append failure")
	}
	if toolCalls != 0 {
		t.Fatalf("tool calls = %d, want 0", toolCalls)
	}
	session := kernel.sessions.Get("session-provider-response-fail")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing persisted turn snapshot")
	}
	if len(session.CurrentTurn.Iterations) != 0 || strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" {
		t.Fatalf("turn entered tool/final processing after recorder failure: iterations=%d final=%q", len(session.CurrentTurn.Iterations), session.CurrentTurn.FinalOutput)
	}
}

func TestRuntimeKernelToolDispatchedAppendFailureCallsExecutorZeroTimes(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindToolDispatched)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	var executed atomic.Int32
	toolDef := rolloutReadTool("read_rollout_dispatch_fail", &executed, false)
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-dispatch-fail", Type: "function",
		Function: schema.FunctionCall{Name: "read_rollout_dispatch_fail", Arguments: `{"secret":"dispatch-secret-canary"}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-dispatch-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-dispatch-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want tool_dispatched append failure")
	}
	if executed.Load() != 0 {
		t.Fatalf("executor calls = %d, want 0", executed.Load())
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), "session-dispatch-fail", "turn-dispatch-fail")
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolProposed) != 1 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolDispatched) != 0 {
		t.Fatalf("events = %v, want proposed without false dispatched", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelParallelDispatchUsesOneStableBatchCommit(t *testing.T) {
	var executed atomic.Int32
	tools := []tooling.Tool{
		rolloutReadTool("read_rollout_parallel_a", &executed, true),
		rolloutReadTool("read_rollout_parallel_b", &executed, true),
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{ID: "call-b", Type: "function", Function: schema.FunctionCall{Name: "read_rollout_parallel_b", Arguments: `{"order":2,"secret":"parallel-secret-b"}`}},
			{ID: "call-a", Type: "function", Function: schema.FunctionCall{Name: "read_rollout_parallel_a", Arguments: `{"order":1,"secret":"parallel-secret-a"}`}},
		}),
		schema.AssistantMessage("complete", nil),
	}}
	kernel := newLoopKernel(t, model, tools, nil, nil)

	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-parallel-stable", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-parallel-stable", Input: "inspect",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if executed.Load() != 2 {
		t.Fatalf("executor calls = %d, want 2", executed.Load())
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), "session-parallel-stable", "turn-parallel-stable")
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolDispatched) != 1 {
		t.Fatalf("events = %v, want one atomic batch dispatched event", canonicalRolloutKindsForTest(events))
	}
	dispatched := firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindToolDispatched)
	calls, ok := dispatched.Payload["calls"].([]any)
	if !ok || len(calls) != 2 {
		t.Fatalf("tool_dispatched calls = %#v, want ordered two-call batch", dispatched.Payload["calls"])
	}
	first, firstOK := calls[0].(map[string]any)
	second, secondOK := calls[1].(map[string]any)
	if !firstOK || !secondOK || first["callId"] != "call-b" || second["callId"] != "call-a" || first["argsHash"] == "" || second["argsHash"] == "" {
		t.Fatalf("tool_dispatched calls = %#v, want provider order call-b/call-a with hashes", calls)
	}
	encoded, _ := json.Marshal(dispatched)
	if strings.Contains(string(encoded), "parallel-secret") {
		t.Fatalf("batch dispatched event leaked arguments: %s", encoded)
	}
}

func TestRuntimeKernelParallelDispatchAppendFailureKeepsWholeBatchAtZero(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindToolDispatched)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	var executed atomic.Int32
	tools := []tooling.Tool{
		rolloutReadTool("read_rollout_parallel_fail_a", &executed, true),
		rolloutReadTool("read_rollout_parallel_fail_b", &executed, true),
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{
		{ID: "call-a", Type: "function", Function: schema.FunctionCall{Name: "read_rollout_parallel_fail_a", Arguments: `{}`}},
		{ID: "call-b", Type: "function", Function: schema.FunctionCall{Name: "read_rollout_parallel_fail_b", Arguments: `{}`}},
	})}}
	kernel := newLoopKernel(t, model, tools, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-parallel-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-parallel-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want atomic batch append failure")
	}
	if executed.Load() != 0 {
		t.Fatalf("parallel executor calls = %d, want whole batch 0", executed.Load())
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), "session-parallel-fail", "turn-parallel-fail")
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolProposed) != 2 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolDispatched) != 0 {
		t.Fatalf("events = %v, want two proposals and no partial dispatched fact", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelApprovalRolloutOrdersRequestedDecisionDispatchAndResult(t *testing.T) {
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-approval", Type: "function",
			Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{"path":"/tmp/approval-secret-canary"}`},
		}}),
		schema.AssistantMessage("approved complete", nil),
	}}
	toolDef := rolloutApprovalTool(&executed)
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-approval-rollout", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-approval-rollout", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "file_absent", "aiops.userEvidence.rawExcerpt": "safe typed evidence",
		},
	})
	if err != nil || blocked.Status != "blocked" || executed != 0 {
		t.Fatalf("RunTurn() = %#v, %v, executed=%d; want pending approval", blocked, err, executed)
	}
	session := kernel.sessions.Get("session-approval-rollout")
	if session == nil || len(session.PendingApprovals) != 1 {
		t.Fatalf("pending approval missing: %#v", session)
	}
	pendingApproval := session.PendingApprovals[0]
	if pendingApproval.ActionToken == nil {
		t.Fatal("pending approval action token is nil")
	}
	approvalID := pendingApproval.ID
	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: approvalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || resumed.Status != "completed" || executed != 1 {
		t.Fatalf("ResumeTurn() = %#v, %v, executed=%d; want completed/1", resumed, err, executed)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	wantOrdered := []string{
		modeltrace.CanonicalRolloutKindToolProposed,
		modeltrace.CanonicalRolloutKindToolDispatched,
		modeltrace.CanonicalRolloutKindApprovalRequested,
		modeltrace.CanonicalRolloutKindApprovalDecided,
		modeltrace.CanonicalRolloutKindToolDispatched,
		modeltrace.CanonicalRolloutKindToolResult,
	}
	if got := canonicalRolloutFilteredKindsForTest(events, wantOrdered); !equalCanonicalRolloutKinds(got, wantOrdered) {
		t.Fatalf("tool/approval sequence = %v, want %v; all=%v", got, wantOrdered, canonicalRolloutKindsForTest(events))
	}
	requested := firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindApprovalRequested)
	token := *pendingApproval.ActionToken
	wantTokenFacts := map[string]string{
		"approvalId": token.ApprovalID, "toolCallId": token.ToolCallID, "toolName": token.ToolName,
		"argsHash": token.ArgumentsHash, "actionTokenHash": token.Hash,
		"toolSurfaceFingerprint": token.ToolSurfaceFingerprint, "permissionHash": token.PermissionHash,
		"checkpointId": token.CheckpointID, "rollbackHash": token.RollbackHash,
	}
	for key, want := range wantTokenFacts {
		if got := requested.Payload[key]; got != want {
			t.Fatalf("approval_requested %s = %#v, want ActionToken value %q; payload=%#v", key, got, want, requested.Payload)
		}
	}
	if got := compactStringList(anyStringSlice(requested.Payload["targetRefs"])); strings.Join(got, "\x00") != strings.Join(token.TargetRefs, "\x00") {
		t.Fatalf("approval_requested targetRefs = %#v, want ActionToken value %#v", got, token.TargetRefs)
	}
	encoded, _ := json.Marshal(events)
	if strings.Contains(string(encoded), "/tmp/approval-secret-canary") || strings.Contains(string(encoded), "approved complete") {
		t.Fatalf("approval rollout leaked raw payload: %s", encoded)
	}
}

func TestRuntimeKernelNilActionTokenReissueRecordsFreshApprovalRequest(t *testing.T) {
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-stale", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-stale-token", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-stale-token", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v, want pending approval", blocked, err)
	}
	session := kernel.sessions.Get("session-stale-token")
	oldID := session.PendingApprovals[0].ID
	session.PendingApprovals[0].ActionToken = nil
	session.CurrentTurn.PendingApprovals[0].ActionToken = nil
	stale, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: oldID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || stale.Status != "blocked" || executed != 0 {
		t.Fatalf("ResumeTurn() = %#v, %v, executed=%d; want stale blocked/0", stale, err, executed)
	}
	session = kernel.sessions.Get(session.ID)
	if len(session.PendingApprovals) != 1 || session.PendingApprovals[0].ID == oldID || session.PendingApprovals[0].ActionToken == nil {
		t.Fatalf("fresh approval missing: %#v", session.PendingApprovals)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindApprovalRequested) != 2 || events[len(events)-1].Kind != modeltrace.CanonicalRolloutKindApprovalRequested {
		t.Fatalf("events = %v, want fresh approval_requested tail", canonicalRolloutKindsForTest(events))
	}
	if got := events[len(events)-1].Payload["approvalId"]; got != session.PendingApprovals[0].ID {
		t.Fatalf("fresh approval_requested id = %#v, want %q", got, session.PendingApprovals[0].ID)
	}
}

func TestRuntimeKernelReissuedApprovalAppendFailureLeavesOldCheckpointAndLedger(t *testing.T) {
	store := newNthKindFailingRolloutStore(modeltrace.CanonicalRolloutKindApprovalRequested, 2)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-stale-fail", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	kernel.rolloutRecorder = recorder
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-stale-append-fail", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-stale-append-fail", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v, want pending approval", blocked, err)
	}
	session := kernel.sessions.Get("session-stale-append-fail")
	oldID := session.PendingApprovals[0].ID
	oldCheckpointID := session.LatestCheckpoint.ID
	session.PendingApprovals[0].ActionToken = nil
	session.CurrentTurn.PendingApprovals[0].ActionToken = nil
	_, err = kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: oldID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err == nil {
		t.Fatal("ResumeTurn() error = nil, want reissued approval append failure")
	}
	if executed != 0 {
		t.Fatalf("executor calls = %d, want 0", executed)
	}
	session = kernel.sessions.Get(session.ID)
	if session.LatestCheckpoint == nil || session.LatestCheckpoint.ID != oldCheckpointID || session.CurrentTurn.LatestCheckpoint == nil || session.CurrentTurn.LatestCheckpoint.ID != oldCheckpointID {
		t.Fatalf("checkpoint changed before critical append: session=%#v turn=%#v want=%q", session.LatestCheckpoint, session.CurrentTurn.LatestCheckpoint, oldCheckpointID)
	}
	if len(session.PendingApprovals) != 1 || session.PendingApprovals[0].ID != oldID {
		t.Fatalf("approval ledger changed before critical append: %#v", session.PendingApprovals)
	}
}

func TestRuntimeKernelApprovalDecisionAppendFailurePrecedesGrantAndExecution(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindApprovalDecided)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-decision-fail", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	kernel.rolloutRecorder = recorder
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-decision-fail", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-decision-fail", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v, want pending approval", blocked, err)
	}
	session := kernel.sessions.Get("session-decision-fail")
	approvalID := session.PendingApprovals[0].ID
	_, err = kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: approvalID,
		Decision: "approved_for_session", ResumeState: TurnResumeStatePendingApproval,
	})
	if err == nil {
		t.Fatal("ResumeTurn() error = nil, want approval_decided append failure")
	}
	if executed != 0 || len(session.ApprovalGrants) != 0 {
		t.Fatalf("append failure crossed decision commit: executor=%d grants=%#v", executed, session.ApprovalGrants)
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindApprovalDecided) != 0 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolResult) != 0 {
		t.Fatalf("events = %v, want no false decision/result", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelApprovedResumeDispatchAppendFailureCallsExecutorZeroTimes(t *testing.T) {
	store := newNthKindFailingRolloutStore(modeltrace.CanonicalRolloutKindToolDispatched, 2)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-resume-dispatch-fail", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	kernel.rolloutRecorder = recorder
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-resume-dispatch-fail", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-resume-dispatch-fail", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err != nil || blocked.Status != "blocked" {
		t.Fatalf("RunTurn() = %#v, %v, want pending approval", blocked, err)
	}
	session := kernel.sessions.Get("session-resume-dispatch-fail")
	oldApprovalID := session.PendingApprovals[0].ID
	oldCheckpointID := session.LatestCheckpoint.ID
	_, err = kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: oldApprovalID,
		Decision: "approved", ResumeState: TurnResumeStatePendingApproval,
	})
	if err == nil {
		t.Fatal("ResumeTurn() error = nil, want approved dispatch append failure")
	}
	if executed != 0 {
		t.Fatalf("approved executor calls = %d, want 0", executed)
	}
	session = kernel.sessions.Get(session.ID)
	if session.CurrentTurn.Lifecycle != TurnLifecycleSuspended || session.CurrentTurn.ResumeState != TurnResumeStatePendingApproval ||
		len(session.PendingApprovals) != 1 || len(session.CurrentTurn.PendingApprovals) != 1 || session.PendingApprovals[0].ID != oldApprovalID ||
		session.LatestCheckpoint == nil || session.CurrentTurn.LatestCheckpoint == nil || session.LatestCheckpoint.ID != oldCheckpointID || session.CurrentTurn.LatestCheckpoint.ID != oldCheckpointID {
		t.Fatalf("dispatch append failure consumed approval state: lifecycle=%s resume=%s sessionApprovals=%#v turnApprovals=%#v checkpoints=%#v/%#v",
			session.CurrentTurn.Lifecycle, session.CurrentTurn.ResumeState, session.PendingApprovals, session.CurrentTurn.PendingApprovals, session.LatestCheckpoint, session.CurrentTurn.LatestCheckpoint)
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindApprovalDecided) != 1 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolDispatched) != 1 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolResult) != 0 {
		t.Fatalf("events = %v, want decision plus only pre-approval dispatch", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelCanonicalRolloutOrdersToolResultCheckpointProjection(t *testing.T) {
	var executed atomic.Int32
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{ID: "call-checkpoint", Type: "function", Function: schema.FunctionCall{Name: "read_checkpoint", Arguments: `{}`}}}),
		schema.AssistantMessage("done", nil),
	}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutReadTool("read_checkpoint", &executed, false)}, nil, nil)
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-tool-checkpoint", SessionType: SessionTypeHost, Mode: ModeInspect,
		TurnID: "turn-tool-checkpoint", Input: "inspect",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), "session-tool-checkpoint", "turn-tool-checkpoint")
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	want := []string{modeltrace.CanonicalRolloutKindToolResult, modeltrace.CanonicalRolloutKindCheckpoint, modeltrace.CanonicalRolloutKindTransportProjection}
	if got := canonicalRolloutFilteredKindsForTest(events, want); len(got) < len(want) || !equalCanonicalRolloutKinds(got[:len(want)], want) {
		t.Fatalf("tool-result boundary = %v, want prefix %v; all=%v", got, want, canonicalRolloutKindsForTest(events))
	}
	checkpoint := firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindCheckpoint)
	for _, key := range []string{"checkpointId", "checkpointKind", "lifecycle", "resumeState", "checkpointHash", "checkpointRef"} {
		if _, ok := checkpoint.Payload[key]; !ok {
			t.Fatalf("checkpoint missing %q: %#v", key, checkpoint.Payload)
		}
	}
	if len(checkpoint.Payload) != 6 {
		t.Fatalf("checkpoint payload = %#v, want six typed fields only", checkpoint.Payload)
	}
	checkpointID, _ := checkpoint.Payload["checkpointId"].(string)
	if checkpointID == "" {
		t.Fatalf("checkpoint id missing: %#v", checkpoint.Payload)
	}
	count := 0
	for _, event := range events {
		if event.Kind == modeltrace.CanonicalRolloutKindCheckpoint && event.Payload["checkpointId"] == checkpointID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("checkpoint %q recorded %d times, want once", checkpointID, count)
	}
}

func TestRuntimeKernelCanonicalRolloutOrdersApprovalBlockCheckpointProjection(t *testing.T) {
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-denied-terminal", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-denied-terminal", SessionType: SessionTypeHost, Mode: ModeExecute,
		TurnID: "turn-denied-terminal", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err != nil || blocked.Status != "blocked" || executed != 0 {
		t.Fatalf("RunTurn() = %#v, %v, executed=%d; want approval block", blocked, err, executed)
	}
	session := kernel.sessions.Get("session-denied-terminal")
	events, err := kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents(blocked) error = %v", err)
	}
	blockWant := []string{modeltrace.CanonicalRolloutKindApprovalRequested, modeltrace.CanonicalRolloutKindCheckpoint, modeltrace.CanonicalRolloutKindTransportProjection}
	if got := canonicalRolloutFilteredKindsForTest(events, blockWant); !equalCanonicalRolloutKinds(got, blockWant) {
		t.Fatalf("approval block sequence = %v, want %v; all=%v", got, blockWant, canonicalRolloutKindsForTest(events))
	}
	assertCanonicalProjectionMatchesSnapshot(t, firstCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection), session.CurrentTurn, nil)
	approvalID := session.PendingApprovals[0].ID
	denied, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID, ApprovalID: approvalID,
		Decision: "denied", ResumeState: TurnResumeStatePendingApproval,
	})
	if err != nil || denied.Status != "completed" || executed != 0 {
		t.Fatalf("ResumeTurn(denied) = %#v, %v executed=%d; want completed/0", denied, err, executed)
	}
	events, err = kernel.CanonicalRolloutEvents(context.Background(), session.ID, session.CurrentTurn.ID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents(denied) error = %v", err)
	}
	want := []string{
		modeltrace.CanonicalRolloutKindApprovalRequested,
		modeltrace.CanonicalRolloutKindCheckpoint,
		modeltrace.CanonicalRolloutKindTransportProjection,
		modeltrace.CanonicalRolloutKindApprovalDecided,
		modeltrace.CanonicalRolloutKindCheckpoint,
		modeltrace.CanonicalRolloutKindFinalFacts,
		modeltrace.CanonicalRolloutKindTransportProjection,
	}
	if got := canonicalRolloutFilteredKindsForTest(events, want); !equalCanonicalRolloutKinds(got, want) {
		t.Fatalf("approval denied sequence = %v, want %v; all=%v", got, want, canonicalRolloutKindsForTest(events))
	}
	terminalFacts := lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindFinalFacts)
	if terminalFacts.Payload["finalContractStatus"] != string(FinalContractStatusApprovalDenied) {
		t.Fatalf("denied final facts = %#v, want approval_denied", terminalFacts.Payload)
	}
	assertCanonicalProjectionMatchesSnapshot(t, lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection), session.CurrentTurn, finalContractForRolloutTest(t, session.CurrentTurn))
}

func TestRuntimeKernelCanonicalRolloutRecordsCanceledAndFailedTerminals(t *testing.T) {
	newRunningTurn := func(kernel *RuntimeKernel, sessionID, turnID string) (*SessionState, *TurnSnapshot) {
		session := kernel.sessions.GetOrCreate(sessionID, SessionTypeHost, ModeInspect)
		snapshot := &TurnSnapshot{ID: turnID, SessionID: session.ID, SessionType: session.Type, Mode: session.Mode, Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone}
		session.CurrentTurn = snapshot
		return session, snapshot
	}
	t.Run("cancelled", func(t *testing.T) {
		kernel := NewRuntimeKernel(RuntimeKernelConfig{})
		session, snapshot := newRunningTurn(kernel, "session-terminal-cancel", "turn-terminal-cancel")
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if ok, err := kernel.markTurnCanceledRecorded(cancelCtx, session, snapshot, "cancel-reason-secret-canary"); err != nil || !ok {
			t.Fatalf("markTurnCanceledRecorded() = %v, %v; want true, nil", ok, err)
		}
		events, err := kernel.CanonicalRolloutEvents(context.Background(), session.ID, snapshot.ID)
		if err != nil {
			t.Fatalf("CanonicalRolloutEvents() error = %v", err)
		}
		want := []string{modeltrace.CanonicalRolloutKindCheckpoint, modeltrace.CanonicalRolloutKindFinalFacts, modeltrace.CanonicalRolloutKindTransportProjection}
		if got := canonicalRolloutKindsForTest(events); !equalCanonicalRolloutKinds(got, want) {
			t.Fatalf("cancel events = %v, want %v", got, want)
		}
		encoded, _ := json.Marshal(events)
		if strings.Contains(string(encoded), "cancel-reason-secret-canary") {
			t.Fatalf("cancel terminal leaked reason: %s", encoded)
		}
		contract := BuildTerminalFinalContract("", FinalContractStatusCancelled, []string{"turn_cancelled"})
		assertCanonicalProjectionMatchesSnapshot(t, lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection), snapshot, &contract)
	})
	t.Run("failed", func(t *testing.T) {
		kernel := NewRuntimeKernel(RuntimeKernelConfig{})
		session, snapshot := newRunningTurn(kernel, "session-terminal-failed", "turn-terminal-failed")
		if err := kernel.markTurnFailedFromError(session, snapshot, errors.New("failure-text-secret-canary"), "provider_failed"); err != nil {
			t.Fatalf("markTurnFailedFromError() error = %v", err)
		}
		events, err := kernel.CanonicalRolloutEvents(context.Background(), session.ID, snapshot.ID)
		if err != nil {
			t.Fatalf("CanonicalRolloutEvents() error = %v", err)
		}
		want := []string{modeltrace.CanonicalRolloutKindCheckpoint, modeltrace.CanonicalRolloutKindFinalFacts, modeltrace.CanonicalRolloutKindTransportProjection}
		if got := canonicalRolloutKindsForTest(events); !equalCanonicalRolloutKinds(got, want) {
			t.Fatalf("failed events = %v, want %v", got, want)
		}
		encoded, _ := json.Marshal(events)
		if strings.Contains(string(encoded), "failure-text-secret-canary") {
			t.Fatalf("failed terminal leaked error: %s", encoded)
		}
		contract := BuildTerminalFinalContract("", FinalContractStatusFailed, []string{"provider_failed"})
		assertCanonicalProjectionMatchesSnapshot(t, lastCanonicalRolloutEventForTest(events, modeltrace.CanonicalRolloutKindTransportProjection), snapshot, &contract)
	})
}

func TestRuntimeKernelTransportProjectionAppendFailureLeavesRegularFinalUncommitted(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindTransportProjection)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("must-not-commit", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder
	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-final-projection-fail", SessionType: SessionTypeHost, Mode: ModeInspect,
		TurnID: "turn-final-projection-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want transport_projection append failure")
	}
	snapshot := kernel.sessions.Get("session-final-projection-fail").CurrentTurn
	if snapshot.Lifecycle == TurnLifecycleCompleted || strings.TrimSpace(snapshot.FinalOutput) != "" {
		t.Fatalf("projection failure committed final: lifecycle=%s final=%q", snapshot.Lifecycle, snapshot.FinalOutput)
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), snapshot.SessionID, snapshot.ID)
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindFinalFacts) < 1 || countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindTransportProjection) != 0 {
		t.Fatalf("events = %v, want durable typed facts without false projection", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelApprovalRequestedRejectsMissingOrInvalidActionToken(t *testing.T) {
	for _, tt := range []struct {
		name  string
		token *ActionToken
	}{
		{name: "missing"},
		{name: "invalid", token: &ActionToken{SchemaVersion: ActionTokenSchemaVersion, ApprovalID: "approval-invalid", Hash: "sha256:forged"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kernel := NewRuntimeKernel(RuntimeKernelConfig{})
			snapshot := &TurnSnapshot{ID: "turn-token-" + tt.name, SessionID: "session-token-" + tt.name}
			err := kernel.recordCanonicalApprovalRequested(context.Background(), snapshot, PendingApproval{
				ID: "approval-" + tt.name, ToolCallID: "call-token", ToolName: "write_file", ActionToken: tt.token,
			})
			if err == nil {
				t.Fatal("recordCanonicalApprovalRequested() error = nil, want invalid ActionToken fail closed")
			}
			events, readErr := kernel.CanonicalRolloutEvents(context.Background(), snapshot.SessionID, snapshot.ID)
			if readErr != nil {
				t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
			}
			if len(events) != 0 {
				t.Fatalf("events = %#v, want no approval_requested for invalid token", events)
			}
		})
	}
}

func TestRuntimeKernelInitialApprovalAppendFailureLeavesRunningStateUntouched(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindApprovalRequested)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	kernel := NewRuntimeKernel(RuntimeKernelConfig{RolloutRecorder: recorder})
	session := kernel.sessions.GetOrCreate("session-initial-request-fail", SessionTypeHost, ModeExecute)
	session.HostID = "host-a"
	oldCheckpoint := newCheckpointMetadata(session.ID, "turn-initial-request-fail", 0, 1, "assistant_response", TurnLifecycleRunning, TurnResumeStateNone)
	snapshot := &TurnSnapshot{
		ID: "turn-initial-request-fail", SessionID: session.ID, SessionType: session.Type, Mode: session.Mode,
		Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone, Iteration: 0, LatestCheckpoint: oldCheckpoint,
		Iterations: []IterationState{{
			ID: "turn-initial-request-fail-iter-0", SessionID: session.ID, TurnID: "turn-initial-request-fail", Iteration: 0,
			Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone, Checkpoint: oldCheckpoint,
		}},
	}
	session.CurrentTurn = snapshot
	session.LatestCheckpoint = oldCheckpoint
	call := ToolCall{ID: "call-initial-request-fail", Name: "write_file", Arguments: json.RawMessage(`{"path":"/tmp/a"}`)}
	dispatch := DispatchResult{
		Blocked: true, Reason: "approval required", Outcome: "approval_needed", Source: "tool",
		Metadata: tooling.ToolMetadata{Name: call.Name},
		DecisionTrace: promptinput.DispatchDecisionTrace{
			ArgumentsHash: toolArgumentsHash(call.Arguments), ToolSurfaceFingerprint: "sha256:surface", PermissionSnapshotHash: "sha256:permission",
		},
	}

	err = kernel.markTurnBlocked(session, snapshot, call, dispatch)
	if err == nil {
		t.Fatal("markTurnBlocked() error = nil, want approval_requested append failure")
	}
	if snapshot.Lifecycle != TurnLifecycleRunning || snapshot.ResumeState != TurnResumeStateNone || snapshot.Error != "" ||
		snapshot.LatestCheckpoint != oldCheckpoint || session.LatestCheckpoint != oldCheckpoint ||
		len(snapshot.PendingApprovals) != 0 || len(session.PendingApprovals) != 0 || len(snapshot.PendingEvidence) != 0 || len(session.PendingEvidence) != 0 {
		t.Fatalf("append failure mutated blocked state: lifecycle=%s resume=%s error=%q checkpoint=%p/%p approvals=%#v/%#v evidence=%#v/%#v",
			snapshot.Lifecycle, snapshot.ResumeState, snapshot.Error, snapshot.LatestCheckpoint, session.LatestCheckpoint,
			snapshot.PendingApprovals, session.PendingApprovals, snapshot.PendingEvidence, session.PendingEvidence)
	}
	last := latestIteration(snapshot)
	if last == nil || last.Lifecycle != TurnLifecycleRunning || last.ResumeState != TurnResumeStateNone || last.Checkpoint != oldCheckpoint {
		t.Fatalf("append failure mutated latest iteration: %#v", last)
	}
}

func TestRuntimeKernelInitialApprovalAppendFailureCallsExecutorZeroTimes(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindApprovalRequested)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	executed := 0
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-initial-request-executor", Type: "function", Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{rolloutApprovalTool(&executed)}, nil, map[policyengine.Mode]policyengine.ModePolicy{})
	kernel.rolloutRecorder = recorder
	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-initial-request-executor", SessionType: SessionTypeHost,
		Mode: ModeExecute, TurnID: "turn-initial-request-executor", HostID: "host-a", Input: "mutate",
		Metadata: map[string]string{
			"aiops.userEvidence.present": "true", "aiops.userEvidence.kinds": "pre_change_snapshot",
			"aiops.userEvidence.signals": "safe", "aiops.userEvidence.rawExcerpt": "safe",
		},
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want approval_requested append failure")
	}
	if executed != 0 {
		t.Fatalf("executor calls = %d, want 0", executed)
	}
}

func TestRuntimeKernelResumeTransitionFailureDoesNotRecordToolHandoff(t *testing.T) {
	kernel := NewRuntimeKernel(RuntimeKernelConfig{})
	session := kernel.sessions.GetOrCreate("session-invalid-resume-transition", SessionTypeHost, ModeExecute)
	call := ToolCall{ID: "call-invalid-resume-transition", Name: "write_file", Arguments: json.RawMessage(`{}`)}
	snapshot := &TurnSnapshot{
		ID: "turn-invalid-resume-transition", SessionID: session.ID, SessionType: session.Type, Mode: session.Mode,
		Lifecycle: TurnLifecycleCompleted, ResumeState: TurnResumeStatePendingApproval,
		PendingApprovals: []PendingApproval{{ID: "approval-invalid-transition", ToolCallID: call.ID}},
		Iterations: []IterationState{{
			ID: "turn-invalid-resume-transition-iter-0", SessionID: session.ID, TurnID: "turn-invalid-resume-transition", Iteration: 0,
			Lifecycle: TurnLifecycleCompleted, ResumeState: TurnResumeStatePendingApproval, ToolCalls: []ToolCall{call},
		}},
	}
	session.CurrentTurn = snapshot
	_, err := kernel.resumePendingToolCall(context.Background(), session, snapshot, approvalResumeExecution{})
	if err == nil {
		t.Fatal("resumePendingToolCall() error = nil, want invalid completed-to-running transition")
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), session.ID, snapshot.ID)
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindToolDispatched) != 0 {
		t.Fatalf("events = %v, transition failure must not record handoff", canonicalRolloutKindsForTest(events))
	}
}

func TestRuntimeKernelConfigInjectsRecorderAndNilCreatesReadableFailClosedMemory(t *testing.T) {
	injected, err := NewRolloutRecorder(RolloutRecorderConfig{Store: NewMemoryRolloutStore()})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	if kernel := NewRuntimeKernel(RuntimeKernelConfig{RolloutRecorder: injected}); kernel.rolloutRecorder != injected {
		t.Fatal("RuntimeKernelConfig.RolloutRecorder was not injected")
	}

	kernel := NewRuntimeKernel(RuntimeKernelConfig{})
	if kernel.rolloutRecorder == nil || kernel.rolloutRecorder.failurePolicy != RolloutFailurePolicyFailClosed {
		t.Fatalf("default rollout recorder = %#v, want real fail-closed in-memory recorder", kernel.rolloutRecorder)
	}
	if _, err := kernel.CanonicalRolloutEvents(context.Background(), "missing-session", "missing-turn"); err != nil {
		t.Fatalf("default CanonicalRolloutEvents() error = %v", err)
	}
}

func TestCanonicalRolloutHeadRefRejectsInvalidHashAndEventReference(t *testing.T) {
	input := canonicalRecorderEvent("session-head", "turn-head", modeltrace.CanonicalRolloutKindPrompt)
	input.Sequence = 1
	event, err := modeltrace.FreezeCanonicalRolloutEvent(input)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	valid := CanonicalRolloutHeadRef{
		SchemaVersion: event.SchemaVersion,
		EventID:       event.EventID,
		Hash:          event.Hash,
		Sequence:      event.Sequence,
		Status:        RolloutRecordStatusRecorded,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid head error = %v", err)
	}
	for name, mutate := range map[string]func(*CanonicalRolloutHeadRef){
		"event":  func(ref *CanonicalRolloutHeadRef) { ref.EventID = "event:not-a-digest" },
		"hash":   func(ref *CanonicalRolloutHeadRef) { ref.Hash = "sha256:not-a-digest" },
		"status": func(ref *CanonicalRolloutHeadRef) { ref.Status = RolloutRecordStatusFailed },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("Validate(%s) error = nil, want rejection", name)
			}
		})
	}
}

func TestRuntimeKernelDegradedRecorderContinuesOnlyWithPersistedTypedMarker(t *testing.T) {
	store := newFailKindOnceRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyDegraded})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("typed-final", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder

	const sessionID = "session-runtime-rollout-degraded"
	const turnID = "turn-runtime-rollout-degraded"
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: turnID, Input: "inspect",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindRecorderDegraded) != 1 ||
		countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindFinalFacts) != 1 ||
		countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindTransportProjection) != 1 {
		t.Fatalf("events = %v, want persisted degraded marker followed by terminal facts/projection", canonicalRolloutKindsForTest(events))
	}
	session := kernel.sessions.Get(sessionID)
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.CanonicalRolloutHead == nil ||
		session.CurrentTurn.CanonicalRolloutHead.Status != RolloutRecordStatusRecorded ||
		session.CurrentTurn.CanonicalRolloutHead.EventID != events[len(events)-1].EventID {
		t.Fatalf("canonical rollout head = %#v, want latest recorded projection after observable degraded marker", session)
	}
}

func TestRuntimeKernelDegradedRecorderWithoutPersistedMarkerBlocksSideEffects(t *testing.T) {
	store := newFailKindAndMarkerRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyDegraded})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	toolCalls := 0
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "degraded_mutation", Description: "must not execute"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			toolCalls++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-degraded", Type: "function",
		Function: schema.FunctionCall{Name: "degraded_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-runtime-rollout-unobservable", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-runtime-rollout-unobservable", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want unpersisted degraded marker to fail closed")
	}
	if len(model.inputs) != 1 || toolCalls != 0 {
		t.Fatalf("provider/tool calls = %d/%d, want 1/0", len(model.inputs), toolCalls)
	}
	session := kernel.sessions.Get("session-runtime-rollout-unobservable")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) != 0 || strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" {
		t.Fatalf("turn crossed side-effect boundary after unpersisted marker: %#v", session)
	}
}

func TestRolloutRecorderAssignsStrictSequenceAndStableIdentity(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         store,
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	firstInput := canonicalRecorderEvent("session-1", "turn-1", modeltrace.CanonicalRolloutKindAssembly)
	first, err := recorder.Append(context.Background(), firstInput)
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-1", "turn-1", modeltrace.CanonicalRolloutKindPrompt))
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if first.Status != RolloutRecordStatusRecorded || second.Status != RolloutRecordStatusRecorded {
		t.Fatalf("statuses = %q, %q; want recorded", first.Status, second.Status)
	}
	if first.Event.Sequence != 1 || second.Event.Sequence != 2 {
		t.Fatalf("sequences = %d, %d; want 1, 2", first.Event.Sequence, second.Event.Sequence)
	}
	if first.Event.EventID == "" || first.Event.Hash == "" || first.Event.EventID == second.Event.EventID {
		t.Fatalf("unstable identities: first=%+v second=%+v", first.Event, second.Event)
	}

	// Replaying the same fact at the same sequence in a fresh recorder produces
	// the same identity; wall-clock time and storage do not participate.
	replay, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         NewMemoryRolloutStore(),
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder(replay) error = %v", err)
	}
	replayed, err := replay.Append(context.Background(), firstInput)
	if err != nil {
		t.Fatalf("Append(replay) error = %v", err)
	}
	if replayed.Event.EventID != first.Event.EventID || replayed.Event.Hash != first.Event.Hash {
		t.Fatalf("replayed identity = %q/%q; want %q/%q", replayed.Event.EventID, replayed.Event.Hash, first.Event.EventID, first.Event.Hash)
	}
}

func TestRolloutRecorderKeepsConcurrentTurnsIsolated(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	const eventsPerTurn = 40
	var wg sync.WaitGroup
	for index := 0; index < eventsPerTurn; index++ {
		for _, turnID := range []string{"turn-a", "turn-b"} {
			wg.Add(1)
			go func(index int, turnID string) {
				defer wg.Done()
				event := canonicalRecorderEvent("session-concurrent", turnID, modeltrace.CanonicalRolloutKindCheckpoint)
				event.Payload = map[string]any{"index": index}
				if result, appendErr := recorder.Append(context.Background(), event); appendErr != nil {
					t.Errorf("Append(%s, %d) error = %v (status=%q)", turnID, index, appendErr, result.Status)
				}
			}(index, turnID)
		}
	}
	wg.Wait()

	for _, turnID := range []string{"turn-a", "turn-b"} {
		events, readErr := store.Events(context.Background(), "session-concurrent", turnID)
		if readErr != nil {
			t.Fatalf("Events(%s) error = %v", turnID, readErr)
		}
		if len(events) != eventsPerTurn {
			t.Fatalf("Events(%s) length = %d; want %d", turnID, len(events), eventsPerTurn)
		}
		for index, event := range events {
			if want := int64(index + 1); event.Sequence != want {
				t.Fatalf("Events(%s)[%d].Sequence = %d; want %d", turnID, index, event.Sequence, want)
			}
		}
	}
}

func TestRuntimeKernelCheckpointDedupKeepsHeadAndRecordsChangedFacts(t *testing.T) {
	kernel := NewRuntimeKernel(RuntimeKernelConfig{})
	snapshot := &TurnSnapshot{ID: "turn-checkpoint-dedup", SessionID: "session-checkpoint-dedup"}
	checkpoint := &CheckpointMetadata{ID: "checkpoint-a", Kind: "tool_result", Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateCheckpointReady}
	if err := kernel.recordCanonicalCheckpoint(context.Background(), snapshot, checkpoint); err != nil {
		t.Fatalf("recordCanonicalCheckpoint(first) error = %v", err)
	}
	if err := kernel.appendCanonicalRolloutEvent(context.Background(), snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindTransportProjection, map[string]any{"projectionInputHash": "sha256:later"})); err != nil {
		t.Fatalf("append later event error = %v", err)
	}
	laterHead := *snapshot.CanonicalRolloutHead
	if err := kernel.recordCanonicalCheckpoint(context.Background(), snapshot, checkpoint); err != nil {
		t.Fatalf("recordCanonicalCheckpoint(duplicate) error = %v", err)
	}
	if *snapshot.CanonicalRolloutHead != laterHead {
		t.Fatalf("duplicate checkpoint rewound head: got=%#v want=%#v", snapshot.CanonicalRolloutHead, laterHead)
	}
	changed := *checkpoint
	changed.Lifecycle = TurnLifecycleResumable
	changed.ResumeState = TurnResumeStateResumable
	if err := kernel.recordCanonicalCheckpoint(context.Background(), snapshot, &changed); err != nil {
		t.Fatalf("recordCanonicalCheckpoint(changed) error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), snapshot.SessionID, snapshot.ID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindCheckpoint) != 2 || snapshot.CanonicalRolloutHead.Sequence != laterHead.Sequence+1 {
		t.Fatalf("events/head = %v/%#v, want duplicate suppressed and changed facts recorded", canonicalRolloutKindsForTest(events), snapshot.CanonicalRolloutHead)
	}
}

func TestCanonicalRolloutFactHashFailsClosedOnUnmarshalableFact(t *testing.T) {
	if hash, err := canonicalRolloutFactHash(func() {}); err == nil || hash != "" {
		t.Fatalf("canonicalRolloutFactHash(func) = %q, %v; want empty hash/error", hash, err)
	}
}

func TestRolloutRecorderDeepCopiesAndRedactsBeforeAppend(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	sourceRefs := []string{"source-b", "source-a"}
	payload := map[string]any{
		"apiKey": "secret-canary",
		"nested": map[string]any{"authorization": "Bearer secret-canary", "safe": "before"},
	}
	result, err := recorder.Append(context.Background(), modeltrace.CanonicalRolloutEvent{
		SessionID:  "session-copy",
		TurnID:     "turn-copy",
		Kind:       modeltrace.CanonicalRolloutKindProviderRequest,
		SourceRefs: sourceRefs,
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	sourceRefs[0] = "mutated-source"
	payload["apiKey"] = "mutated-secret"
	payload["nested"].(map[string]any)["safe"] = "after"

	events, err := store.Events(context.Background(), "session-copy", "turn-copy")
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Events() length = %d; want 1", len(events))
	}
	encoded, err := json.Marshal([]any{result.Event, events[0]})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"secret-canary", "mutated-secret", "mutated-source", "after"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("recorded rollout contains %q: %s", forbidden, encoded)
		}
	}
	marker, ok := events[0].Payload["apiKey"].(map[string]any)
	if !ok || marker["redacted"] != true || marker["sha256"] == "" {
		t.Fatalf("Payload[apiKey] = %#v; want hash-backed redaction marker", events[0].Payload["apiKey"])
	}
}

func TestRolloutRecorderDoesNotAliasStoreAndReturnedEvent(t *testing.T) {
	store := &retainingRolloutStore{}
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	input := canonicalRecorderEvent("session-boundary-copy", "turn-boundary-copy", modeltrace.CanonicalRolloutKindCheckpoint)
	input.Payload = map[string]any{"nested": map[string]any{"status": "before"}}
	result, err := recorder.Append(context.Background(), input)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	result.Event.Payload["nested"].(map[string]any)["status"] = "after"
	if got := store.event.Payload["nested"].(map[string]any)["status"]; got != "before" {
		t.Fatalf("store event was mutated through returned result: got %#v, want before", got)
	}
}

func TestRolloutRecorderFailClosedReturnsTypedAppendError(t *testing.T) {
	storeErr := errors.New("store unavailable")
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         rolloutStoreFunc(func(context.Context, modeltrace.CanonicalRolloutEvent) error { return storeErr }),
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-fail", "turn-fail", modeltrace.CanonicalRolloutKindToolResult))
	if err == nil || !errors.Is(err, storeErr) {
		t.Fatalf("Append() error = %v; want store error", err)
	}
	var typed *RolloutRecorderAppendError
	if !errors.As(err, &typed) {
		t.Fatalf("Append() error type = %T; want *RolloutRecorderAppendError", err)
	}
	if result.Status != RolloutRecordStatusFailed || result.ObservedError == nil {
		t.Fatalf("Append() result = %+v; want failed status with observed error", result)
	}
}

func TestRolloutRecorderDegradedPersistsTypedMarker(t *testing.T) {
	storeErr := errors.New("primary append unavailable")
	memory := NewMemoryRolloutStore()
	store := &failFirstRolloutStore{err: storeErr, fallback: memory}
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         store,
		FailurePolicy: RolloutFailurePolicyDegraded,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-degraded", "turn-degraded", modeltrace.CanonicalRolloutKindProviderResponse))
	if err != nil {
		t.Fatalf("Append() fatal error = %v; degraded policy must return a typed outcome", err)
	}
	if result.Status != RolloutRecordStatusDegraded || !errors.Is(result.ObservedError, storeErr) {
		t.Fatalf("Append() result = %+v; want degraded status with observable store error", result)
	}
	if result.Event.Kind != modeltrace.CanonicalRolloutKindRecorderDegraded || !result.MarkerPersisted {
		t.Fatalf("Append() marker = %+v; want persisted recorder_degraded marker", result)
	}
	events, readErr := memory.Events(context.Background(), "session-degraded", "turn-degraded")
	if readErr != nil {
		t.Fatalf("Events() error = %v", readErr)
	}
	if len(events) != 1 || events[0].EventID != result.Event.EventID || events[0].Sequence != 1 {
		t.Fatalf("stored degraded events = %+v; want returned sequence-1 marker", events)
	}
	if got := events[0].Payload["failedKind"]; got != modeltrace.CanonicalRolloutKindProviderResponse {
		t.Fatalf("degraded marker failedKind = %#v; want provider_response", got)
	}
}

func TestRolloutRecorderDegradedCannotMasqueradeAsSuccessWhenStoreIsUnwritable(t *testing.T) {
	storeErr := errors.New("store unavailable password=secret-canary")
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         rolloutStoreFunc(func(context.Context, modeltrace.CanonicalRolloutEvent) error { return storeErr }),
		FailurePolicy: RolloutFailurePolicyDegraded,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-unwritable", "turn-unwritable", modeltrace.CanonicalRolloutKindFinalFacts))
	if err != nil {
		t.Fatalf("Append() fatal error = %v; degraded state is carried by result", err)
	}
	if result.Status != RolloutRecordStatusDegraded || result.MarkerPersisted || result.ObservedError == nil {
		t.Fatalf("Append() result = %+v; want explicit unpersisted degraded state", result)
	}
	if strings.Contains(result.ObservedError.Error(), "secret-canary") {
		t.Fatalf("observable degraded error leaked store secret: %v", result.ObservedError)
	}
	if result.Event.Kind != modeltrace.CanonicalRolloutKindRecorderDegraded || result.Event.Sequence != 1 {
		t.Fatalf("Append() marker = %+v; want sequence-1 degraded marker", result.Event)
	}
	encoded, marshalErr := json.Marshal(result.Event)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(marker) error = %v", marshalErr)
	}
	if strings.Contains(string(encoded), "secret-canary") {
		t.Fatalf("degraded marker persisted raw store error: %s", encoded)
	}
}

func TestMemoryRolloutStoreRejectsNonIncreasingSequence(t *testing.T) {
	store := NewMemoryRolloutStore()
	event := canonicalRecorderEvent("session-order", "turn-order", modeltrace.CanonicalRolloutKindCheckpoint)
	event.Sequence = 1
	frozen, err := modeltrace.FreezeCanonicalRolloutEvent(event)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	if err := store.Append(context.Background(), frozen); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if err := store.Append(context.Background(), frozen); !errors.Is(err, ErrRolloutSequenceNotIncreasing) {
		t.Fatalf("Append(duplicate) error = %v; want ErrRolloutSequenceNotIncreasing", err)
	}
}

func canonicalRecorderEvent(sessionID, turnID, kind string) modeltrace.CanonicalRolloutEvent {
	return modeltrace.CanonicalRolloutEvent{
		SessionID: sessionID,
		TurnID:    turnID,
		Kind:      kind,
		SourceRefs: []string{
			"turn-assembly:sha256:abc",
		},
		Payload: map[string]any{"fact": "value"},
	}
}

type rolloutStoreFunc func(context.Context, modeltrace.CanonicalRolloutEvent) error

func (fn rolloutStoreFunc) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	return fn(ctx, event)
}

type retainingRolloutStore struct {
	event modeltrace.CanonicalRolloutEvent
}

func (s *retainingRolloutStore) Append(_ context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.event = event
	return nil
}

type failFirstRolloutStore struct {
	mu       sync.Mutex
	failed   bool
	err      error
	fallback RolloutStore
}

func (s *failFirstRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return s.err
	}
	if s.fallback == nil {
		return fmt.Errorf("fallback store is nil")
	}
	return s.fallback.Append(ctx, event)
}

type kindFailingRolloutStore struct {
	mu       sync.Mutex
	failKind string
	memory   *MemoryRolloutStore
}

type failKindOnceRolloutStore struct {
	mu       sync.Mutex
	failKind string
	failed   bool
	memory   *MemoryRolloutStore
}

func newFailKindOnceRolloutStore(kind string) *failKindOnceRolloutStore {
	return &failKindOnceRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *failKindOnceRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind && !s.failed {
		s.failed = true
		return errors.New("injected primary append failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *failKindOnceRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

type failKindAndMarkerRolloutStore struct {
	mu       sync.Mutex
	failKind string
	failing  bool
	memory   *MemoryRolloutStore
}

func newFailKindAndMarkerRolloutStore(kind string) *failKindAndMarkerRolloutStore {
	return &failKindAndMarkerRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *failKindAndMarkerRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind {
		s.failing = true
	}
	if s.failing {
		return errors.New("injected append and marker failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *failKindAndMarkerRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

type nthKindFailingRolloutStore struct {
	mu       sync.Mutex
	failKind string
	failAt   int
	seen     int
	memory   *MemoryRolloutStore
}

func newNthKindFailingRolloutStore(kind string, failAt int) *nthKindFailingRolloutStore {
	return &nthKindFailingRolloutStore{failKind: kind, failAt: failAt, memory: NewMemoryRolloutStore()}
}

func (s *nthKindFailingRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind {
		s.seen++
		if s.seen == s.failAt {
			return errors.New("injected nth canonical rollout append failure")
		}
	}
	return s.memory.Append(ctx, event)
}

func (s *nthKindFailingRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

func newKindFailingRolloutStore(kind string) *kindFailingRolloutStore {
	return &kindFailingRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *kindFailingRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind {
		return errors.New("injected canonical rollout append failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *kindFailingRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

func canonicalRolloutKindsForTest(events []modeltrace.CanonicalRolloutEvent) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func countCanonicalRolloutKind(events []modeltrace.CanonicalRolloutEvent, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func equalCanonicalRolloutKinds(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func firstCanonicalRolloutEventForTest(events []modeltrace.CanonicalRolloutEvent, kind string) modeltrace.CanonicalRolloutEvent {
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	return modeltrace.CanonicalRolloutEvent{}
}

func lastCanonicalRolloutEventForTest(events []modeltrace.CanonicalRolloutEvent, kind string) modeltrace.CanonicalRolloutEvent {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Kind == kind {
			return events[index]
		}
	}
	return modeltrace.CanonicalRolloutEvent{}
}

func finalContractForRolloutTest(t *testing.T, snapshot *TurnSnapshot) *FinalContract {
	t.Helper()
	if snapshot == nil {
		t.Fatal("snapshot is nil")
	}
	for index := len(snapshot.AgentItems) - 1; index >= 0; index-- {
		var payload struct {
			FinalContract *FinalContract `json:"finalContract"`
		}
		if json.Unmarshal(snapshot.AgentItems[index].Payload.Data, &payload) == nil && payload.FinalContract != nil {
			return payload.FinalContract
		}
	}
	t.Fatal("final contract missing from committed agent items")
	return nil
}

func assertCanonicalProjectionMatchesSnapshot(t *testing.T, event modeltrace.CanonicalRolloutEvent, snapshot *TurnSnapshot, contract *FinalContract) {
	t.Helper()
	lifecycle, _ := event.Payload["lifecycle"].(string)
	resume, _ := event.Payload["resumeState"].(string)
	checkpointRef, _ := event.Payload["checkpointRef"].(string)
	expected, err := canonicalTransportProjectionPayload(snapshot, TurnLifecycleState(lifecycle), TurnResumeState(resume), checkpointRef, contract)
	if err != nil {
		t.Fatalf("canonicalTransportProjectionPayload() error = %v", err)
	}
	if event.Payload["projectionInputHash"] != expected["projectionInputHash"] || event.Payload["agentItemsFactHash"] != expected["agentItemsFactHash"] {
		t.Fatalf("projection does not match committed snapshot: event=%#v expected=%#v", event.Payload, expected)
	}
}

func canonicalRolloutFilteredKindsForTest(events []modeltrace.CanonicalRolloutEvent, allowed []string) []string {
	allowedSet := make(map[string]bool, len(allowed))
	for _, kind := range allowed {
		allowedSet[kind] = true
	}
	filtered := make([]string, 0, len(events))
	for _, event := range events {
		if allowedSet[event.Kind] {
			filtered = append(filtered, event.Kind)
		}
	}
	return filtered
}

func rolloutReadTool(name string, executed *atomic.Int32, concurrencySafe bool) *tooling.StaticTool {
	return &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: name, Description: "read typed state"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return concurrencySafe
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			executed.Add(1)
			return tooling.ToolResult{Content: `{"status":"healthy"}`}, nil
		},
	}
}

func rolloutApprovalTool(executed *int) *tooling.StaticTool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "rollout_mutation", Description: "mutate typed state"},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeExecute)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "mutation requires approval",
				Approval: &tooling.PermissionApprovalPayload{
					Risk: "high", Source: "rollout_test", ExpectedEffect: "mutate state",
					Rollback: "restore state", Validation: "verify state",
				},
			}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			*executed++
			return tooling.ToolResult{Content: `{"status":"changed","evidenceRefs":["evidence://postcheck"]}`}, nil
		},
	}
}
