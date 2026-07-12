package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
)

type cancelAwareBlockingModel struct {
	started chan struct{}
}

func newCancelAwareBlockingModel() *cancelAwareBlockingModel {
	return &cancelAwareBlockingModel{started: make(chan struct{}, 1)}
}

func (m *cancelAwareBlockingModel) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	select {
	case m.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *cancelAwareBlockingModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *cancelAwareBlockingModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func TestCancelTurn_PersistsCanceledLifecycle(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	kernel.sessions.Update(session)

	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn result status = %q, want cancelled", result.Status)
	}

	updated := kernel.sessions.Get(session.ID)
	if updated == nil || updated.CurrentTurn == nil {
		t.Fatal("updated session/current turn is nil")
	}
	if updated.CurrentTurn.Lifecycle != TurnLifecycleCanceled {
		t.Fatalf("CurrentTurn lifecycle = %q, want %q", updated.CurrentTurn.Lifecycle, TurnLifecycleCanceled)
	}
	if updated.CurrentTurn.CompletedAt == nil {
		t.Fatal("CurrentTurn.CompletedAt is nil after cancel")
	}
}

func TestCancelTurn_MarksRunningAgentItemsCancelled(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel-agent-items", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel-agent-items",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			newAgentItem("model-running", agentstate.TurnItemTypeModelCall, agentstate.ItemStatusRunning, "calling model", nil),
			newAgentItem("assistant-running", agentstate.TurnItemTypeAssistantMessage, agentstate.ItemStatusRunning, "正在分析已有证据", map[string]any{"displayKind": "assistant.message", "phase": "commentary", "streamState": "streaming"}),
			newAgentItem("tool-completed", agentstate.TurnItemTypeToolCall, agentstate.ItemStatusCompleted, "web_search", nil),
		},
	}
	kernel.sessions.Update(session)

	if _, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    session.CurrentTurn.ID,
		Reason:    "user stop",
	}); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}

	updated := kernel.sessions.Get(session.ID)
	if updated == nil || updated.CurrentTurn == nil {
		t.Fatal("updated session/current turn is nil")
	}
	for _, item := range updated.CurrentTurn.AgentItems {
		switch item.ID {
		case "model-running", "assistant-running":
			if item.Status != agentstate.ItemStatusCancelled {
				t.Fatalf("item %s status = %q, want cancelled", item.ID, item.Status)
			}
		case "tool-completed":
			if item.Status != agentstate.ItemStatusCompleted {
				t.Fatalf("completed item status = %q, want unchanged", item.Status)
			}
		}
	}
}

func TestCancelTurn_EmitsTurnAbortedProjection(t *testing.T) {
	kernel := newTestKernel(nil)
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel-event", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel-event",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	kernel.sessions.Update(session)

	if _, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    session.CurrentTurn.ID,
		Reason:    "user stop",
	}); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}

	if len(emitter.events) == 0 {
		t.Fatal("expected projection event after cancel")
	}
	last := emitter.events[len(emitter.events)-1]
	if last.Type != EventTurnAborted {
		t.Fatalf("last event type = %q, want %q", last.Type, EventTurnAborted)
	}
	if last.SessionID != session.ID || last.TurnID != session.CurrentTurn.ID {
		t.Fatalf("last event = %+v, want session %q turn %q", last, session.ID, session.CurrentTurn.ID)
	}
}

func TestCancelTurn_CancelsInFlightRunTurn(t *testing.T) {
	kernel := newTestKernel(nil)
	blockingModel := newCancelAwareBlockingModel()
	kernel.modelRouter = modelrouter.NewRouter("blocking", map[string]modelrouter.ChatModel{"blocking": blockingModel}, nil)

	session := kernel.sessions.GetOrCreate("sess-cancel-live", SessionTypeHost, ModeChat)
	done := make(chan struct {
		result TurnResult
		err    error
	}, 1)

	go func() {
		result, err := kernel.RunTurn(context.Background(), TurnRequest{
			SessionType: SessionTypeHost,
			Mode:        ModeChat,
			SessionID:   session.ID,
			TurnID:      "turn-cancel-live",
			Input:       "请持续生成",
		})
		done <- struct {
			result TurnResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case <-blockingModel.started:
	case <-time.After(time.Second):
		t.Fatal("model did not start before cancel")
	}

	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel-live",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("RunTurn() error = %v, want nil canceled result", outcome.err)
		}
		if outcome.result.Status != "cancelled" {
			t.Fatalf("RunTurn status = %q, want cancelled", outcome.result.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunTurn did not exit after cancel")
	}
	updated := kernel.sessions.Get(session.ID)
	if updated == nil || updated.CurrentTurn == nil {
		t.Fatal("updated session/current turn is nil")
	}
	modelItem := findAgentItem(updated.CurrentTurn.AgentItems, agentstate.TurnItemTypeModelCall)
	if modelItem.ID == "" {
		t.Fatalf("agent items = %#v, want model call item", updated.CurrentTurn.AgentItems)
	}
	if modelItem.Status != agentstate.ItemStatusCancelled {
		t.Fatalf("model item status = %q summary=%q, want cancelled", modelItem.Status, modelItem.Payload.Summary)
	}
	if strings.Contains(modelItem.Payload.Summary, "context canceled") {
		t.Fatalf("model item summary leaked raw cancellation error: %q", modelItem.Payload.Summary)
	}
}

func TestRunTurn_DoesNotStartModelWhenCanceledBeforeExecution(t *testing.T) {
	kernel := newTestKernel(nil)
	blockingModel := newCancelAwareBlockingModel()
	kernel.modelRouter = modelrouter.NewRouter("blocking", map[string]modelrouter.ChatModel{"blocking": blockingModel}, nil)

	session := kernel.sessions.GetOrCreate("sess-cancel-pending", SessionTypeHost, ModeChat)
	result, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel-pending",
		Reason:    "user stop",
	})
	if err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}
	if result.Status != "cancelled" {
		t.Fatalf("CancelTurn status = %q, want cancelled", result.Status)
	}

	runResult, runErr := kernel.RunTurn(context.Background(), TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		SessionID:   session.ID,
		TurnID:      "turn-cancel-pending",
		Input:       "不要启动模型",
	})
	if runErr != nil {
		t.Fatalf("RunTurn() error = %v", runErr)
	}
	if runResult.Status != "cancelled" {
		t.Fatalf("RunTurn status = %q, want cancelled", runResult.Status)
	}
	select {
	case <-blockingModel.started:
		t.Fatal("model started despite pending cancel")
	default:
	}
}

func TestCancelTurn_WritesAbortedToolResultMarker(t *testing.T) {
	kernel := newTestKernel(nil)
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-cancel-tool", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-cancel-tool",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		Iterations: []IterationState{{
			ID:        "turn-cancel-tool-iter-1",
			SessionID: session.ID,
			TurnID:    "turn-cancel-tool",
			Iteration: 1,
			Lifecycle: TurnLifecycleRunning,
			ToolCalls: []ToolCall{{
				ID:   "call-running",
				Name: "exec_command",
			}},
			ToolInvocations: []ToolInvocationState{{
				ID:         "inv-running",
				ToolCallID: "call-running",
				ToolName:   "exec_command",
				Status:     ToolInvocationRunning,
				StartedAt:  now,
				UpdatedAt:  now,
			}},
			StartedAt: now,
			UpdatedAt: now,
		}},
	}
	kernel.sessions.Update(session)

	if _, err := kernel.CancelTurn(context.Background(), CancelRequest{
		SessionID: session.ID,
		TurnID:    "turn-cancel-tool",
		Reason:    "user_cancelled",
	}); err != nil {
		t.Fatalf("CancelTurn() error = %v", err)
	}

	updated := kernel.sessions.Get(session.ID)
	last := latestIteration(updated.CurrentTurn)
	if last == nil || len(last.ToolResults) != 1 {
		t.Fatalf("tool results = %#v, want one aborted result", last)
	}
	result := last.ToolResults[0]
	if result.ToolCallID != "call-running" || !strings.Contains(result.Content, `"schemaVersion":"aiops.tool_aborted/v1"`) {
		t.Fatalf("aborted tool result = %#v, want aiops.tool_aborted/v1", result)
	}
	if !strings.Contains(result.Content, `"partialExecutionRisk":true`) {
		t.Fatalf("aborted tool result missing partialExecutionRisk: %s", result.Content)
	}
	if len(updated.Messages) == 0 || updated.Messages[len(updated.Messages)-1].ToolResult == nil ||
		!strings.Contains(updated.Messages[len(updated.Messages)-1].Content, `"schemaVersion":"aiops.tool_aborted/v1"`) {
		t.Fatalf("session messages missing abort marker: %#v", updated.Messages)
	}
}

func TestAbortMarkerAppearsInNextModelInput(t *testing.T) {
	content := `{"schemaVersion":"aiops.tool_aborted/v1","reason":"user_cancelled","partialExecutionRisk":true}`
	messages := promptInputMessagesFromRuntime([]Message{{
		ID:      "msg-abort",
		Role:    "tool",
		Content: content,
		ToolResult: &ToolResult{
			ToolCallID: "call-running",
			Content:    content,
		},
	}})
	if len(messages) != 1 || messages[0].ToolResult == nil || !strings.Contains(messages[0].ToolResult.Content, "aiops.tool_aborted/v1") {
		t.Fatalf("prompt input messages = %#v, want abort marker tool result", messages)
	}
}

func TestResumeTurn_DeniedDecisionEmitsApprovalDecidedProjection(t *testing.T) {
	kernel := newTestKernel(nil)
	emitter, ok := kernel.projector.(*testMockEventEmitter)
	if !ok {
		t.Fatal("expected testMockEventEmitter projector")
	}
	now := time.Now().UTC()
	session := kernel.sessions.GetOrCreate("sess-denied", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-denied",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleSuspended,
		ResumeState: TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now,
		LatestCheckpoint: &CheckpointMetadata{
			ID:          "chk-denied",
			SessionID:   session.ID,
			TurnID:      "turn-denied",
			Iteration:   1,
			Sequence:    1,
			Kind:        "approval_needed",
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		PendingApprovals: []PendingApproval{{
			ID:         "approval-1",
			SessionID:  session.ID,
			TurnID:     "turn-denied",
			Iteration:  1,
			ToolName:   "exec_command",
			ToolCallID: "call-denied",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
		Iterations: []IterationState{{
			ID:          "turn-denied-iteration-1",
			SessionID:   session.ID,
			TurnID:      "turn-denied",
			Iteration:   1,
			Lifecycle:   TurnLifecycleSuspended,
			ResumeState: TurnResumeStatePendingApproval,
			ToolCalls:   []ToolCall{{ID: "call-denied", Name: "exec_command"}},
			StartedAt:   now,
			UpdatedAt:   now,
		}},
	}
	session.PendingApprovals = append([]PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	kernel.sessions.Update(session)

	result, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID:  session.ID,
		TurnID:     session.CurrentTurn.ID,
		ApprovalID: "approval-1",
		Decision:   "denied",
		Metadata:   map[string]string{"rejection.reason": "synthetic rejection reason"},
	})
	if err != nil {
		t.Fatalf("ResumeTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("ResumeTurn status = %q, want completed", result.Status)
	}
	var deniedPayload struct {
		Status           string   `json:"status"`
		ApprovalID       string   `json:"approvalId"`
		Reason           string   `json:"reason"`
		AllowedNextSteps []string `json:"allowedNextSteps"`
	}
	if err := json.Unmarshal([]byte(result.Output), &deniedPayload); err != nil {
		t.Fatalf("ResumeTurn output is not structured denial JSON: %v; output=%q", err, result.Output)
	}
	if deniedPayload.Status != "approval_denied" ||
		deniedPayload.ApprovalID != "approval-1" ||
		deniedPayload.Reason != "synthetic rejection reason" ||
		!containsString(deniedPayload.AllowedNextSteps, "continue_with_existing_evidence") {
		t.Fatalf("denied payload = %#v", deniedPayload)
	}

	if len(emitter.events) == 0 {
		t.Fatal("expected projection event after denied approval")
	}
	for _, event := range emitter.events {
		if event.Type == EventToolStarted {
			t.Fatalf("denied approval must not emit %s", EventToolStarted)
		}
	}
	last := emitter.events[len(emitter.events)-1]
	if last.Type != EventApprovalDecided {
		t.Fatalf("last event type = %q, want %q", last.Type, EventApprovalDecided)
	}
	var payload map[string]string
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatalf("approval.decided payload decode error = %v", err)
	}
	if payload["status"] != "denied" || payload["decision"] != "denied" {
		t.Fatalf("approval.decided payload = %#v, want denied decision/status", payload)
	}
	session = kernel.sessions.Get(session.ID)
	if got := len(session.PendingApprovals); got != 0 {
		t.Fatalf("pending approvals after denied decision = %d, want 0", got)
	}
	if len(session.RejectedApprovals) != 1 || session.RejectedApprovals[0].Reason != "synthetic rejection reason" {
		t.Fatalf("rejected approvals = %#v, want rejection reason recorded", session.RejectedApprovals)
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleCompleted || !strings.Contains(session.CurrentTurn.FinalOutput, `"status":"approval_denied"`) {
		t.Fatalf("turn lifecycle=%q final=%q, want completed structured denial final", session.CurrentTurn.Lifecycle, session.CurrentTurn.FinalOutput)
	}
	decided := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeApprovalDecided)
	if decided.ID == "" || decided.Status != agentstate.ItemStatusCompleted {
		t.Fatalf("approval decided item = %#v, want canonical completed denial", decided)
	}
	var decidedFacts struct {
		ApprovalID string `json:"approvalId"`
		ToolCallID string `json:"toolCallId"`
		Decision   string `json:"decision"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(decided.Payload.Data, &decidedFacts); err != nil {
		t.Fatalf("decode approval decided item: %v; raw=%s", err, string(decided.Payload.Data))
	}
	if decidedFacts.ApprovalID != "approval-1" || decidedFacts.ToolCallID != "call-denied" || decidedFacts.Decision != "denied" || decidedFacts.Status != "denied" {
		t.Fatalf("approval decided facts = %#v, want linked denial", decidedFacts)
	}
	for _, iteration := range session.CurrentTurn.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Mutating && invocation.Status == ToolInvocationCompleted {
				t.Fatalf("denied approval recorded performed mutation: %#v", invocation)
			}
		}
	}
	var deniedFinalContract FinalContract
	for i := len(session.CurrentTurn.AgentItems) - 1; i >= 0; i-- {
		item := session.CurrentTurn.AgentItems[i]
		if item.Type != agentstate.TurnItemTypeAssistantMessage || item.Status != agentstate.ItemStatusCompleted {
			continue
		}
		var finalPayload struct {
			FinalContract FinalContract `json:"finalContract"`
		}
		if json.Unmarshal(item.Payload.Data, &finalPayload) == nil && finalPayload.FinalContract.SchemaVersion != "" {
			deniedFinalContract = finalPayload.FinalContract
			break
		}
	}
	if deniedFinalContract.SchemaVersion == "" || len(deniedFinalContract.PerformedActions) != 0 {
		t.Fatalf("denied final contract = %#v, want typed contract without performed actions", deniedFinalContract)
	}
	protocolState := buildProtocolPromptState(session.CurrentTurn, promptcompiler.ToolPromptDelta{}, nil, nil, session.RejectedApprovals)
	if len(protocolState.Items) != 1 || protocolState.Items[0].Status != "denied" || !strings.Contains(protocolState.Items[0].Text, "synthetic rejection reason") {
		t.Fatalf("protocol state = %#v, want denied approval reason", protocolState)
	}
}

func TestResumeTurn_PlanApprovalsRecordRequestedBeforeDecided(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*SessionState, string, time.Time) (string, error)
	}{
		{
			name: "enter",
			prepare: func(session *SessionState, turnID string, now time.Time) (string, error) {
				result, err := RequestEnterPlanMode(session, turnID, EnterPlanModeRequest{Reason: "prepare implementation plan"}, now)
				return result.ApprovalID, err
			},
		},
		{
			name: "exit",
			prepare: func(session *SessionState, turnID string, now time.Time) (string, error) {
				session.Mode = ModePlan
				session.PlanMode = PlanModeState{State: PlanModeStateActive, AllowDraftPlan: true}
				result, _, err := RequestExitPlanMode(session, turnID, RuntimePlanArtifact{
					ID:        "plan-exit-canonical",
					Objective: "deploy safely",
					Steps:     []RuntimePlanStep{{ID: "step-1", Text: "verify then deploy"}},
				}, now)
				return result.ApprovalID, err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kernel := newTestKernel(nil)
			now := time.Now().UTC()
			session := kernel.sessions.GetOrCreate("sess-plan-canonical-"+tt.name, SessionTypeHost, ModeChat)
			turnID := "turn-plan-canonical-" + tt.name
			session.CurrentTurn = &TurnSnapshot{
				ID:          turnID,
				SessionID:   session.ID,
				SessionType: session.Type,
				Mode:        session.Mode,
				Lifecycle:   TurnLifecycleSuspended,
				ResumeState: TurnResumeStatePendingApproval,
				StartedAt:   now,
				UpdatedAt:   now,
				LatestCheckpoint: &CheckpointMetadata{
					ID:          "checkpoint-plan-" + tt.name,
					SessionID:   session.ID,
					TurnID:      turnID,
					Kind:        "approval_needed",
					Lifecycle:   TurnLifecycleSuspended,
					ResumeState: TurnResumeStatePendingApproval,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			}
			approvalID, err := tt.prepare(session, turnID, now)
			if err != nil || approvalID == "" || len(session.PendingApprovals) != 1 {
				t.Fatalf("prepare plan approval id=%q approvals=%#v err=%v", approvalID, session.PendingApprovals, err)
			}
			kernel.sessions.Update(session)

			result, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
				SessionID:  session.ID,
				TurnID:     turnID,
				ApprovalID: approvalID,
				Decision:   "approved",
			})
			if err != nil || result.Status != "completed" {
				t.Fatalf("ResumeTurn = %#v, %v, want completed plan decision", result, err)
			}
			session = kernel.sessions.Get(session.ID)
			requestedIndex, decidedIndex := -1, -1
			var requestedFacts, decidedFacts approvalAgentItemData
			for index, item := range session.CurrentTurn.AgentItems {
				switch item.Type {
				case agentstate.TurnItemTypeApprovalRequested:
					requestedIndex = index
					if err := json.Unmarshal(item.Payload.Data, &requestedFacts); err != nil {
						t.Fatalf("decode requested item: %v", err)
					}
				case agentstate.TurnItemTypeApprovalDecided:
					decidedIndex = index
					if err := json.Unmarshal(item.Payload.Data, &decidedFacts); err != nil {
						t.Fatalf("decode decided item: %v", err)
					}
				}
			}
			if requestedIndex < 0 || decidedIndex <= requestedIndex {
				t.Fatalf("agent items = %#v, want approval_requested before approval_decided", session.CurrentTurn.AgentItems)
			}
			if requestedFacts.ApprovalID != approvalID || decidedFacts.ApprovalID != approvalID || requestedFacts.ToolName != decidedFacts.ToolName || decidedFacts.Status != "approved" {
				t.Fatalf("requested=%#v decided=%#v, want same plan approval identity", requestedFacts, decidedFacts)
			}
		})
	}
}
