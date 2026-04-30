package appui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type sessionSourceStub struct {
	list   []*runtimekernel.SessionState
	latest *runtimekernel.SessionState
}

func (s sessionSourceStub) Get(id string) *runtimekernel.SessionState {
	for _, item := range s.list {
		if item != nil && item.ID == id {
			return item
		}
	}
	return nil
}

func (s sessionSourceStub) GetLatest() *runtimekernel.SessionState { return s.latest }
func (s sessionSourceStub) List() []*runtimekernel.SessionState {
	return append([]*runtimekernel.SessionState(nil), s.list...)
}

type runtimeStub struct {
	run    runtimekernel.TurnResult
	resume runtimekernel.TurnResult
	cancel runtimekernel.TurnResult
}

func (r runtimeStub) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return r.run, nil
}
func (r runtimeStub) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return r.resume, nil
}
func (r runtimeStub) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return r.cancel, nil
}

func TestSnapshotBuilderProjectsRuntimeSessionToWebSnapshot(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-host",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeChat,
		HostID:    "host-a",
		UpdatedAt: now,
		Messages: []runtimekernel.Message{
			{ID: "msg-user", Role: "user", Content: "hello from host", Timestamp: now},
			{ID: "msg-assistant", Role: "assistant", Content: "world", Timestamp: now.Add(time.Second)},
		},
		PendingApprovals: []runtimekernel.PendingApproval{
			{ID: "approval-1", SessionID: "sess-host", TurnID: "turn-1", ToolName: "exec_command", HostID: "host-a", Command: "free -h", Reason: "needs approval", CreatedAt: now},
		},
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:               "turn-1",
			Lifecycle:        runtimekernel.TurnLifecycleSuspended,
			PendingApprovals: []runtimekernel.PendingApproval{{ID: "approval-1"}},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	if snapshot.SessionID != "sess-host" {
		t.Fatalf("SessionID = %q, want sess-host", snapshot.SessionID)
	}
	if snapshot.Kind != "single_host" {
		t.Fatalf("Kind = %q, want single_host", snapshot.Kind)
	}
	if snapshot.SelectedHostID != "host-a" {
		t.Fatalf("SelectedHostID = %q, want host-a", snapshot.SelectedHostID)
	}
	if len(snapshot.Cards) != 2 {
		t.Fatalf("len(Cards) = %d, want 2", len(snapshot.Cards))
	}
	if snapshot.Cards[0].Type != "UserMessageCard" {
		t.Fatalf("Cards[0].Type = %q, want UserMessageCard", snapshot.Cards[0].Type)
	}
	if len(snapshot.Approvals) != 1 || snapshot.Approvals[0].ToolName != "exec_command" {
		t.Fatalf("Approvals = %+v, want exec_command approval", snapshot.Approvals)
	}
	if snapshot.Approvals[0].Command != "free -h" {
		t.Fatalf("Approvals[0].Command = %q, want real command", snapshot.Approvals[0].Command)
	}
	if snapshot.Approvals[0].Reason != "needs approval" {
		t.Fatalf("Approvals[0].Reason = %q, want policy reason", snapshot.Approvals[0].Reason)
	}
	if snapshot.Runtime.Turn.Phase != "waiting_approval" {
		t.Fatalf("Runtime.Turn.Phase = %q, want waiting_approval", snapshot.Runtime.Turn.Phase)
	}
}

func TestSnapshotBuilderExposesConfiguredLLMModelOnlyFromSettingsRepo(t *testing.T) {
	builder := NewSnapshotBuilderWithSettings(nil, &settingsRepoStub{
		llm: &store.LLMConfig{
			Provider: "openai",
			Model:    "claude-sonnet-4-20250514",
		},
	})

	snapshot := builder.BuildStateSnapshot(nil)

	if snapshot.Config["model"] != "claude-sonnet-4-20250514" {
		t.Fatalf("Config[model] = %q, want configured model", snapshot.Config["model"])
	}
}

func TestSnapshotBuilderProjectsProtocolRuntimeFields(t *testing.T) {
	now := time.Date(2026, 4, 22, 13, 0, 0, 0, time.UTC)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-workspace",
		Type:      runtimekernel.SessionTypeWorkspace,
		Mode:      runtimekernel.ModeExecute,
		HostID:    "host-a",
		UpdatedAt: now,
		Messages: []runtimekernel.Message{
			{ID: "msg-assistant", Role: "assistant", Content: "需要进一步确认。", Timestamp: now},
		},
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:                 "turn-choice",
			SessionID:          "sess-workspace",
			SessionType:        runtimekernel.SessionTypeWorkspace,
			Mode:               runtimekernel.ModeExecute,
			Lifecycle:          runtimekernel.TurnLifecycleSuspended,
			ResumeState:        runtimekernel.TurnResumeStatePendingEvidence,
			Iteration:          0,
			UpdatedAt:          now,
			GovernanceSnapshot: "need external facts before execute",
			PromptSections:     []string{"system", "runtime_policy"},
			HiddenTools:        []string{"write_file"},
			Iterations: []runtimekernel.IterationState{
				{
					ID:          "turn-choice-iter-0",
					SessionID:   "sess-workspace",
					TurnID:      "turn-choice",
					Iteration:   0,
					Lifecycle:   runtimekernel.TurnLifecycleSuspended,
					ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
					ToolCalls: []runtimekernel.ToolCall{
						{
							ID:        "call-1",
							Name:      "ask_user_question",
							Arguments: json.RawMessage(`{"hostId":"host-a","question":"是否继续执行？"}`),
						},
					},
					VisibleTools: []string{"ask_user_question", "web_search"},
					PromptDelta:  "Need more evidence",
					StartedAt:    now,
					UpdatedAt:    now.Add(time.Second),
				},
			},
			PendingEvidence: []runtimekernel.PendingEvidence{
				{
					ID:         "evidence-1",
					SessionID:  "sess-workspace",
					TurnID:     "turn-choice",
					Iteration:  0,
					ToolName:   "ask_user_question",
					ToolCallID: "call-1",
					Reason:     "Need customer confirmation",
					Status:     "pending",
					CreatedAt:  now,
					UpdatedAt:  now,
				},
			},
		},
		PendingEvidence: []runtimekernel.PendingEvidence{
			{
				ID:         "evidence-1",
				SessionID:  "sess-workspace",
				TurnID:     "turn-choice",
				Iteration:  0,
				ToolName:   "ask_user_question",
				ToolCallID: "call-1",
				Reason:     "Need customer confirmation",
				Status:     "pending",
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	if got := len(snapshot.ToolInvocations); got != 1 {
		t.Fatalf("len(ToolInvocations) = %d, want 1", got)
	}
	if snapshot.ToolInvocations[0].ID != "call-1" {
		t.Fatalf("ToolInvocations[0].ID = %q, want call-1", snapshot.ToolInvocations[0].ID)
	}
	if snapshot.ToolInvocations[0].EvidenceID != "evidence-1" {
		t.Fatalf("ToolInvocations[0].EvidenceID = %q, want evidence-1", snapshot.ToolInvocations[0].EvidenceID)
	}
	if got := len(snapshot.EvidenceSummaries); got != 1 {
		t.Fatalf("len(EvidenceSummaries) = %d, want 1", got)
	}
	if snapshot.EvidenceSummaries[0].InvocationID != "call-1" {
		t.Fatalf("EvidenceSummaries[0].InvocationID = %q, want call-1", snapshot.EvidenceSummaries[0].InvocationID)
	}
	if snapshot.CurrentMode != "execute" {
		t.Fatalf("CurrentMode = %q, want execute", snapshot.CurrentMode)
	}
	if snapshot.CurrentLane != "execute" {
		t.Fatalf("CurrentLane = %q, want execute", snapshot.CurrentLane)
	}
	if snapshot.FinalGateStatus != "pending" {
		t.Fatalf("FinalGateStatus = %q, want pending", snapshot.FinalGateStatus)
	}
	if got := snapshot.TurnPolicy.ClassificationReason; got != "need external facts before execute" {
		t.Fatalf("TurnPolicy.ClassificationReason = %q, want governance snapshot", got)
	}
	if got := len(snapshot.PromptEnvelope.VisibleTools); got != 2 {
		t.Fatalf("len(PromptEnvelope.VisibleTools) = %d, want 2", got)
	}
	if got := snapshot.PromptEnvelope.HiddenTools[0].Name; got != "write_file" {
		t.Fatalf("PromptEnvelope.HiddenTools[0].Name = %q, want write_file", got)
	}
}

func TestSnapshotBuilderExposesAgentItemEventsAsShadowDebugConfig(t *testing.T) {
	now := time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-agent-items",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeInspect,
		UpdatedAt: now,
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:          "turn-agent-items",
			SessionID:   "sess-agent-items",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeInspect,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			ResumeState: runtimekernel.TurnResumeStateNone,
			StartedAt:   now,
			UpdatedAt:   now,
			AgentItems: []agentstate.TurnItem{
				{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "read_file"}, CreatedAt: now},
				{ID: "tool-result-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "ok"}, CreatedAt: now},
			},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	raw, ok := snapshot.Config["agentItemEvents"]
	if !ok {
		t.Fatal("expected agentItemEvents in snapshot debug config")
	}
	events, ok := raw.([]AgentEvent)
	if !ok {
		t.Fatalf("agentItemEvents type = %T, want []AgentEvent", raw)
	}
	if len(events) != 2 {
		t.Fatalf("agentItemEvents = %d, want 2", len(events))
	}
	if events[0].Kind != AgentEventTool || events[0].Phase != AgentEventPhaseStarted {
		t.Fatalf("first event = %#v, want tool started", events[0])
	}
	if events[1].Kind != AgentEventTool || events[1].Phase != AgentEventPhaseCompleted {
		t.Fatalf("second event = %#v, want tool completed", events[1])
	}
}

func TestStateAndSessionServicesShareSnapshotBuilder(t *testing.T) {
	now := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)
	hostSession := &runtimekernel.SessionState{
		ID:        "sess-host",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeChat,
		HostID:    "host-a",
		UpdatedAt: now,
		Messages:  []runtimekernel.Message{{ID: "m1", Role: "user", Content: "first", Timestamp: now}},
	}
	workspaceSession := &runtimekernel.SessionState{
		ID:        "sess-workspace",
		Type:      runtimekernel.SessionTypeWorkspace,
		Mode:      runtimekernel.ModeExecute,
		UpdatedAt: now.Add(time.Minute),
		Messages:  []runtimekernel.Message{{ID: "m2", Role: "assistant", Content: "workspace", Timestamp: now.Add(time.Minute)}},
	}
	source := sessionSourceStub{
		list:   []*runtimekernel.SessionState{hostSession, workspaceSession},
		latest: workspaceSession,
	}
	services := NewServices(runtimeStub{}, source)

	state, err := services.StateService().GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.SessionID != "sess-workspace" || state.Kind != "workspace" {
		t.Fatalf("state = %+v, want latest workspace session", state)
	}

	list, err := services.SessionService().ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if list.ActiveSessionID != "sess-workspace" {
		t.Fatalf("ActiveSessionID = %q, want sess-workspace", list.ActiveSessionID)
	}
	if len(list.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(list.Sessions))
	}
	if list.Sessions[0].ID != "sess-workspace" {
		t.Fatalf("Sessions[0].ID = %q, want sess-workspace", list.Sessions[0].ID)
	}
}

func TestChatServiceMapsAppCommandsToRuntimeCalls(t *testing.T) {
	runtime := &chatRuntimeCapture{}
	services := NewServices(runtime, nil)

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionType: "workspace",
		Mode:        "execute",
		SessionID:   "sess-1",
		Content:     "run it",
		HostID:      "host-a",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.SessionID != "sess-1" || result.TurnID == "" || result.Status != "accepted" {
		t.Fatalf("result = %+v, want accepted response for sess-1", result)
	}
	runReq := waitForRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("RunTurn sessionType = %q, want workspace", runReq.SessionType)
	}
	if runReq.Mode != runtimekernel.ModeExecute {
		t.Fatalf("RunTurn mode = %q, want execute", runReq.Mode)
	}
	if runReq.SessionID != "sess-1" || runReq.HostID != "host-a" {
		t.Fatalf("RunTurn target = %+v, want sess-1/host-a", runReq)
	}
	if runReq.Input != "run it" {
		t.Fatalf("RunTurn input = %q, want run it", runReq.Input)
	}
}
