package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestWorkflowRepairBudgetAllowsExactlyTwoRepairRounds(t *testing.T) {
	snapshot := workflowRepairBudgetTestSnapshot(RuntimePromptProfileWorkflowAgent)
	base := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)

	first := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 0, ToolCallID: "compile-initial", Metadata: workflowRepairBudgetTestMetadata("compile"),
		Result:       workflowRepairBudgetFailureResult("UNKNOWN_ACTION", "/steps/0/action"),
		PromptTokens: 40, CompletionTokens: 10, TotalTokens: 50, ModelDuration: 100 * time.Millisecond, ObservedAt: base,
	})
	if !first.Applies || first.Exhausted || first.State.RepairAttempts != 0 {
		t.Fatalf("first decision = %#v, want initial diagnostic without consumed repair", first)
	}

	second := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 1, ToolCallID: "preview-repair-1", Metadata: workflowRepairBudgetTestMetadata("preview", "validate"),
		Result:       workflowRepairBudgetFailureResult("PORT_MISMATCH", "/edges/0/sourcePort"),
		PromptTokens: 45, CompletionTokens: 11, TotalTokens: 56, ModelDuration: 120 * time.Millisecond, ObservedAt: base.Add(time.Second),
	})
	if second.Exhausted || second.State.RepairAttempts != 1 {
		t.Fatalf("second decision = %#v, want one repair attempt and one remaining", second)
	}

	third := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 2, ToolCallID: "validate-repair-2", Metadata: workflowRepairBudgetTestMetadata("validate"),
		Result:       workflowRepairBudgetFailureResult("MISSING_ENVIRONMENT", "/environment"),
		PromptTokens: 50, CompletionTokens: 12, TotalTokens: 62, ModelDuration: 140 * time.Millisecond, ObservedAt: base.Add(2 * time.Second),
	})
	if !third.Exhausted || third.State.RepairAttempts != workflowRepairMaxAttempts {
		t.Fatalf("third decision = %#v, want exhausted after exactly %d repairs", third, workflowRepairMaxAttempts)
	}
	if third.State.TotalTokens != 168 || third.State.ModelDurationMillis != 360 {
		t.Fatalf("budget totals = tokens:%d duration:%d, want 168/360", third.State.TotalTokens, third.State.ModelDurationMillis)
	}
	for _, want := range []string{"MISSING_ENVIRONMENT", "/environment", "retained", "new turn"} {
		if !strings.Contains(third.BlockReason, want) {
			t.Fatalf("block reason %q missing %q", third.BlockReason, want)
		}
	}

	replayed := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 2, ToolCallID: "validate-repair-2", Metadata: workflowRepairBudgetTestMetadata("validate"),
		Result:       workflowRepairBudgetFailureResult("MISSING_ENVIRONMENT", "/environment"),
		PromptTokens: 50, CompletionTokens: 12, TotalTokens: 62, ModelDuration: 140 * time.Millisecond, ObservedAt: base.Add(3 * time.Second),
	})
	if replayed.State.FailureCount != 3 || replayed.State.TotalTokens != 168 || len(replayed.State.History) != 3 {
		t.Fatalf("replayed state = %#v, want idempotent persisted observation", replayed.State)
	}
}

func TestWorkflowRepairBudgetPersistsAcrossSnapshotResume(t *testing.T) {
	snapshot := workflowRepairBudgetTestSnapshot(RuntimePromptProfileWorkflowAgent)
	first := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 4, ToolCallID: "call-initial", Metadata: workflowRepairBudgetTestMetadata("compile"),
		Result: workflowRepairBudgetFailureResult("UNKNOWN_ACTION", "/steps/0/action"), ObservedAt: time.Now().UTC(),
	})
	if first.Exhausted {
		t.Fatal("initial failure unexpectedly exhausted budget")
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var restored TurnSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	second := observeWorkflowRepairBudget(&restored, workflowRepairBudgetObservation{
		Iteration: 5, ToolCallID: "call-repair-1", Metadata: workflowRepairBudgetTestMetadata("preview"),
		Result: workflowRepairBudgetFailureResult("PORT_MISMATCH", "/edges/0"), ObservedAt: time.Now().UTC(),
	})
	third := observeWorkflowRepairBudget(&restored, workflowRepairBudgetObservation{
		Iteration: 6, ToolCallID: "call-repair-2", Metadata: workflowRepairBudgetTestMetadata("validate"),
		Result: workflowRepairBudgetFailureResult("REFERENCE_LOCKED", "/steps/1"), ObservedAt: time.Now().UTC(),
	})
	if second.State.RepairAttempts != 1 || !third.Exhausted || third.State.InitialFailureIteration != 4 {
		t.Fatalf("restored decisions = second:%#v third:%#v, budget was reset across resume", second, third)
	}
}

func TestWorkflowRepairBudgetResumeMetadataCannotResetPersistedState(t *testing.T) {
	snapshot := workflowRepairBudgetTestSnapshot(RuntimePromptProfileWorkflowAgent)
	observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 0, ToolCallID: "call-initial", Metadata: workflowRepairBudgetTestMetadata("compile"),
		Result: workflowRepairBudgetFailureResult("UNKNOWN_ACTION", "/steps/0/action"), ObservedAt: time.Now().UTC(),
	})
	frozen := snapshot.Metadata[workflowRepairBudgetMetadataKey]
	merged := mergeResumeTurnMetadata(snapshot.Metadata, map[string]string{workflowRepairBudgetMetadataKey: `{}`})
	merged = preserveWorkflowRepairBudgetMetadata(snapshot.Metadata, merged)
	if merged[workflowRepairBudgetMetadataKey] != frozen {
		t.Fatalf("repair budget metadata was reset during resume: got %q want %q", merged[workflowRepairBudgetMetadataKey], frozen)
	}
}

func TestWorkflowRepairBudgetUsesProfileAndDiscoveryMetadataNotToolNames(t *testing.T) {
	failure := workflowRepairBudgetFailureResult("UNKNOWN_ACTION", "/steps/0")
	cases := []struct {
		name     string
		profile  string
		metadata tooling.ToolMetadata
		want     bool
	}{
		{name: "workflow compile capability", profile: RuntimePromptProfileWorkflowAgent, metadata: workflowRepairBudgetTestMetadata("compile"), want: true},
		{name: "other profile", profile: RuntimePromptProfileAdvisor, metadata: workflowRepairBudgetTestMetadata("compile")},
		{name: "apply mutation is outside repair diagnostics", profile: RuntimePromptProfileWorkflowAgent, metadata: workflowRepairBudgetTestMetadata("apply", "mutate")},
		{name: "unscoped tool", profile: RuntimePromptProfileWorkflowAgent, metadata: tooling.ToolMetadata{Discovery: tooling.ToolDiscoveryMetadata{OperationKinds: []string{"validate"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := workflowRepairBudgetTestSnapshot(tc.profile)
			decision := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
				Iteration: 0, ToolCallID: "opaque-call", Metadata: tc.metadata, Result: failure, ObservedAt: time.Now().UTC(),
			})
			if decision.Applies != tc.want {
				t.Fatalf("Applies = %v, want %v; decision=%#v", decision.Applies, tc.want, decision)
			}
		})
	}
}

func TestWorkflowRepairBudgetCancellationIsPersistedAndStopsAccounting(t *testing.T) {
	snapshot := workflowRepairBudgetTestSnapshot(RuntimePromptProfileWorkflowAgent)
	observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 0, ToolCallID: "call-initial", Metadata: workflowRepairBudgetTestMetadata("compile"),
		Result: workflowRepairBudgetFailureResult("UNKNOWN_ACTION", "/steps/0"), ObservedAt: time.Now().UTC(),
	})
	canceledAt := time.Now().UTC().Add(time.Second)
	recordWorkflowRepairCancellation(snapshot, "user stop", canceledAt)

	state, err := workflowRepairBudgetStateFromSnapshot(snapshot)
	if err != nil {
		t.Fatalf("read cancellation state: %v", err)
	}
	if !state.Canceled || state.CancelReason != "user stop" || state.CanceledAt == nil || !state.CanceledAt.Equal(canceledAt) {
		t.Fatalf("cancellation state = %#v", state)
	}
	after := observeWorkflowRepairBudget(snapshot, workflowRepairBudgetObservation{
		Iteration: 1, ToolCallID: "call-after-cancel", Metadata: workflowRepairBudgetTestMetadata("preview"),
		Result: workflowRepairBudgetFailureResult("PORT_MISMATCH", "/edges/0"), ObservedAt: canceledAt.Add(time.Second),
	})
	if !after.Canceled || after.State.FailureCount != 1 || len(after.State.History) != 1 {
		t.Fatalf("post-cancel observation = %#v, want no additional repair accounting", after)
	}
}

func TestRunTurnWorkflowRepairBudgetBlocksBeforeAnotherMutationAndRetainsCandidate(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		workflowRepairToolCallMessage("opaque.compile", "compile-initial", 41),
		workflowRepairToolCallMessage("opaque.preview", "preview-repair-1", 43),
		workflowRepairToolCallMessage("opaque.validate", "validate-repair-2", 47),
		workflowRepairToolCallMessage("opaque.apply", "apply-must-not-run", 53),
	}}
	var applyExecutions atomic.Int32
	tools := []tooling.Tool{
		workflowRepairFailingTool("opaque.compile", "compile", "UNKNOWN_ACTION", "/steps/0/action"),
		workflowRepairFailingTool("opaque.preview", "preview", "PORT_MISMATCH", "/edges/0/sourcePort"),
		workflowRepairFailingTool("opaque.validate", "validate", "MISSING_ENVIRONMENT", "/environment"),
		&tooling.StaticTool{
			Meta:       tooling.ToolMetadata{Name: "opaque.apply", AlwaysLoad: true, Profiles: []string{RuntimePromptProfileWorkflowAgent}, Discovery: tooling.ToolDiscoveryMetadata{AgentProfiles: []string{RuntimePromptProfileWorkflowAgent}, OperationKinds: []string{"apply", "mutate"}}},
			Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeWorkspace)}, Modes: []string{string(ModeExecute)}},
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				applyExecutions.Add(1)
				return tooling.ToolResult{Content: "unexpected apply"}, nil
			},
		},
	}
	kernel := newLoopKernel(t, model, tools, nil, nil)
	candidate, err := runtimecontract.NewInlineTurnArtifact("workflow.candidate", map[string]string{"changeSetId": "candidate-retained"})
	if err != nil {
		t.Fatalf("candidate artifact: %v", err)
	}

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-workflow-repair-budget", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		TurnID: "turn-workflow-repair-budget", Input: "生成并校验 Workflow Candidate",
		Metadata:      map[string]string{"profile": RuntimePromptProfileWorkflowAgent, "toolProfile": RuntimePromptProfileWorkflowAgent},
		TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{candidate},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "blocked" || !strings.Contains(result.Error, "MISSING_ENVIRONMENT") {
		t.Fatalf("RunTurn() = %#v, want typed repair-budget blocker", result)
	}
	if got := applyExecutions.Load(); got != 0 {
		t.Fatalf("apply executions = %d, want zero after repair exhaustion", got)
	}
	if len(model.inputs) != 3 {
		t.Fatalf("model calls = %d, want exactly initial + two repair rounds", len(model.inputs))
	}

	session := kernel.sessions.Get("sess-workflow-repair-budget")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing persisted turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed || session.CurrentTurn.FinalOutput != "" {
		t.Fatalf("turn lifecycle/final = %q/%q, want failed without completed output", session.CurrentTurn.Lifecycle, session.CurrentTurn.FinalOutput)
	}
	if len(session.CurrentTurn.TurnArtifacts) != 1 || session.CurrentTurn.TurnArtifacts[0].ID != candidate.ID {
		t.Fatalf("turn artifacts = %#v, candidate was not retained", session.CurrentTurn.TurnArtifacts)
	}
	state, stateErr := workflowRepairBudgetStateFromSnapshot(session.CurrentTurn)
	if stateErr != nil || !state.Exhausted || state.RepairAttempts != workflowRepairMaxAttempts {
		t.Fatalf("persisted budget = %#v, %v", state, stateErr)
	}
}

func TestRunTurnWorkflowRepairCancellationDoesNotDispatchPendingMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	model := &cancelBeforeWorkflowMutationModel{cancel: cancel}
	var mutationExecutions atomic.Int32
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "opaque.preview", AlwaysLoad: true, Profiles: []string{RuntimePromptProfileWorkflowAgent},
			Discovery: tooling.ToolDiscoveryMetadata{AgentProfiles: []string{RuntimePromptProfileWorkflowAgent}, OperationKinds: []string{"preview"}},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeWorkspace)}, Modes: []string{string(ModeExecute)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			mutationExecutions.Add(1)
			return tooling.ToolResult{Content: "unexpected mutation"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	result, err := kernel.RunTurn(ctx, TurnRequest{
		SessionID: "sess-workflow-repair-cancel", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		TurnID: "turn-workflow-repair-cancel", Input: "生成 Candidate",
		Metadata: map[string]string{"profile": RuntimePromptProfileWorkflowAgent, "toolProfile": RuntimePromptProfileWorkflowAgent},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "cancelled" || mutationExecutions.Load() != 0 {
		t.Fatalf("RunTurn() = %#v executions=%d, want cancelled without mutation dispatch", result, mutationExecutions.Load())
	}
	session := kernel.sessions.Get("sess-workflow-repair-cancel")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.Lifecycle != TurnLifecycleCanceled || session.CurrentTurn.FinalOutput != "" {
		t.Fatalf("canceled turn = %#v, want canceled without completed output", session)
	}
	state, stateErr := workflowRepairBudgetStateFromSnapshot(session.CurrentTurn)
	if stateErr != nil || !state.Canceled {
		t.Fatalf("cancellation budget state = %#v, %v", state, stateErr)
	}
}

func workflowRepairBudgetTestSnapshot(profile string) *TurnSnapshot {
	return &TurnSnapshot{
		ID: "turn-budget", SessionID: "session-budget", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone, Metadata: map[string]string{},
		TurnAssembly: &agentassembly.TurnAssembly{AdmissionFacts: runtimecontract.AdmissionFacts{Profile: profile}},
	}
}

func workflowRepairBudgetTestMetadata(operationKinds ...string) tooling.ToolMetadata {
	return tooling.ToolMetadata{
		Profiles: []string{RuntimePromptProfileWorkflowAgent},
		Discovery: tooling.ToolDiscoveryMetadata{
			AgentProfiles:  []string{RuntimePromptProfileWorkflowAgent},
			OperationKinds: operationKinds,
		},
	}
}

func workflowRepairBudgetFailureResult(code, path string) tooling.ToolResult {
	raw, _ := json.Marshal(map[string]any{
		"schemaVersion": "aiops.workflow.edit.v2",
		"isError":       true,
		"structuredContent": map[string]any{
			"kind":        "validation",
			"diagnostics": []map[string]string{{"code": code, "severity": "error", "message": "typed diagnostic", "path": path, "suggestion": "repair only this path"}},
		},
	})
	return tooling.ToolResult{Content: "typed workflow diagnostics", StructuredContent: raw, IsError: true}
}

func workflowRepairToolCallMessage(name, id string, totalTokens int) *schema.Message {
	message := schema.AssistantMessage("", []schema.ToolCall{{ID: id, Type: "function", Function: schema.FunctionCall{Name: name, Arguments: `{}`}}})
	message.ResponseMeta = &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: totalTokens - 9, CompletionTokens: 9, TotalTokens: totalTokens}}
	return message
}

func workflowRepairFailingTool(name, operationKind, code, path string) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: name, AlwaysLoad: true, Profiles: []string{RuntimePromptProfileWorkflowAgent}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel,
			Discovery: tooling.ToolDiscoveryMetadata{AgentProfiles: []string{RuntimePromptProfileWorkflowAgent}, OperationKinds: []string{operationKind}},
		},
		Visibility:   tooling.Visibility{SessionTypes: []string{string(SessionTypeWorkspace)}, Modes: []string{string(ModeExecute)}},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return workflowRepairBudgetFailureResult(code, path), nil
		},
	}
}

type cancelBeforeWorkflowMutationModel struct {
	cancel context.CancelFunc
}

func (m *cancelBeforeWorkflowMutationModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.cancel()
	return workflowRepairToolCallMessage("opaque.preview", "preview-canceled", 17), nil
}

func (m *cancelBeforeWorkflowMutationModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.cancel()
	return schema.StreamReaderFromArray([]*schema.Message{workflowRepairToolCallMessage("opaque.preview", "preview-canceled", 17)}), nil
}

func (m *cancelBeforeWorkflowMutationModel) BindTools([]*schema.ToolInfo) error { return nil }
