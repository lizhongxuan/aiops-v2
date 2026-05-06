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

func TestSnapshotBuilderDoesNotUseApprovalReasonAsCommandFallback(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-host",
		Type:      runtimekernel.SessionTypeHost,
		UpdatedAt: now,
		PendingApprovals: []runtimekernel.PendingApproval{
			{ID: "approval-1", SessionID: "sess-host", TurnID: "turn-1", ToolName: "exec_command", Reason: "needs approval", CreatedAt: now},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	if len(snapshot.Approvals) != 1 {
		t.Fatalf("Approvals = %+v, want one approval", snapshot.Approvals)
	}
	if snapshot.Approvals[0].Command != "" {
		t.Fatalf("Approvals[0].Command = %q, want empty command when runtime did not provide a real command", snapshot.Approvals[0].Command)
	}
	if snapshot.Approvals[0].Reason != "needs approval" {
		t.Fatalf("Approvals[0].Reason = %q, want reason preserved", snapshot.Approvals[0].Reason)
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

func TestSnapshotBuilderProjectsAgentItemsIntoAgentEventProjection(t *testing.T) {
	now := time.Date(2026, 4, 28, 9, 30, 0, 0, time.UTC)
	planData := json.RawMessage(`{"steps":[{"id":"inspect","text":"Inspect payment-api","status":"in_progress"}]}`)
	approvalData := json.RawMessage(`{"approvalId":"approval-1","approvalType":"command","command":"kubectl rollout undo deployment/payment-api -n prod","reason":"5xx rose after deploy","risk":"high","targets":["prod/payment-api"]}`)
	evidenceData := json.RawMessage(`{"id":"metric-1","kind":"metric","title":"5xx rate","summary":"payment-api 5xx increased","source":"prometheus","window":"15m","rawRef":"promql:5xx"}`)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-agent-items-projection",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeInspect,
		UpdatedAt: now,
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:          "turn-agent-items-projection",
			SessionID:   "sess-agent-items-projection",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeInspect,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			ResumeState: runtimekernel.TurnResumeStateNone,
			StartedAt:   now,
			UpdatedAt:   now,
			AgentItems: []agentstate.TurnItem{
				{ID: "plan-1", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "plan updated", Data: planData}, CreatedAt: now},
				{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "payment-api 5xx"}, CreatedAt: now},
				{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "payment-api 5xx increased", Data: evidenceData}, CreatedAt: now},
				{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "Rollback payment-api", Data: approvalData}, CreatedAt: now},
				{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "Final answer"}, CreatedAt: now},
			},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	if snapshot.AgentEventProjection == nil {
		t.Fatal("AgentEventProjection is nil, want projection from AgentItems")
	}
	proj := snapshot.AgentEventProjection
	if got := len(proj.ProcessGroups["turn-agent-items-projection"]); got < 3 {
		t.Fatalf("ProcessGroups length = %d, want plan/tool/evidence rows", got)
	}
	if got := len(proj.Approvals); got != 1 {
		t.Fatalf("Approvals length = %d, want 1", got)
	}
	if got := proj.Approvals[0].Command; got != "kubectl rollout undo deployment/payment-api -n prod" {
		t.Fatalf("approval command = %q", got)
	}
	if got := proj.FinalMessages["turn-agent-items-projection"].Text; got != "Final answer" {
		t.Fatalf("final text = %q", got)
	}
}

func TestSnapshotBuilderTerminalTurnDoesNotExposeBlockedAgentItemApproval(t *testing.T) {
	now := time.Date(2026, 4, 30, 6, 10, 0, 0, time.UTC)
	approvalData := json.RawMessage(`{"approvalId":"approval-stale","approvalType":"command","command":"command -v docker && docker --version","reason":"needs command approval"}`)
	builder := NewSnapshotBuilder()
	session := &runtimekernel.SessionState{
		ID:        "sess-stale-approval",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeInspect,
		UpdatedAt: now,
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:          "turn-stale-approval",
			SessionID:   "sess-stale-approval",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeInspect,
			Lifecycle:   runtimekernel.TurnLifecycleFailed,
			ResumeState: runtimekernel.TurnResumeStateNone,
			StartedAt:   now.Add(-time.Minute),
			UpdatedAt:   now,
			AgentItems: []agentstate.TurnItem{
				{ID: "approval-stale", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "Docker check", Data: approvalData}, CreatedAt: now.Add(-30 * time.Second)},
			},
		},
	}

	snapshot := builder.BuildStateSnapshot(session)

	if snapshot.Runtime.Turn.Active {
		t.Fatal("runtime turn Active = true, want false for terminal failed turn")
	}
	if snapshot.Runtime.Turn.Phase != "failed" {
		t.Fatalf("runtime turn Phase = %q, want failed", snapshot.Runtime.Turn.Phase)
	}
	if snapshot.AgentEventProjection == nil {
		t.Fatal("AgentEventProjection is nil, want sanitized projection")
	}
	proj := snapshot.AgentEventProjection
	if proj.Status == "blocked" {
		t.Fatalf("projection status = blocked, want terminal turn to clear stale approval")
	}
	if len(proj.RuntimeLiveness.PendingApprovals) != 0 || len(proj.RuntimeLiveness.PendingUserInputs) != 0 {
		t.Fatalf("pending liveness = approvals:%+v inputs:%+v, want cleared", proj.RuntimeLiveness.PendingApprovals, proj.RuntimeLiveness.PendingUserInputs)
	}
	if len(proj.RuntimeLiveness.ActiveTurns) != 0 || len(proj.RuntimeLiveness.ActiveAgents) != 0 || len(proj.RuntimeLiveness.ActiveCommandStreams) != 0 {
		t.Fatalf("active liveness = turns:%+v agents:%+v commands:%+v, want cleared", proj.RuntimeLiveness.ActiveTurns, proj.RuntimeLiveness.ActiveAgents, proj.RuntimeLiveness.ActiveCommandStreams)
	}
	if len(proj.Approvals) != 1 {
		t.Fatalf("Approvals length = %d, want 1 historical approval row", len(proj.Approvals))
	}
	if proj.Approvals[0].Status == AgentEventStatusBlocked {
		t.Fatalf("historical approval status = blocked, want terminal status")
	}
}

func TestSanitizeAgentEventProjectionForSnapshotClearsTerminalBlockedApproval(t *testing.T) {
	projection := AgentEventProjection{
		SessionID:          "sess-stale-projection",
		CurrentTurnID:      "turn-stale-projection",
		Status:             "blocked",
		LastSeq:            42,
		LastTerminalFailed: false,
		RuntimeLiveness: RuntimeLiveness{
			ActiveTurns:          map[string]bool{"turn-stale-projection": true},
			ActiveAgents:         map[string]bool{"agent-main": true},
			PendingApprovals:     map[string]bool{"approval-stale": true},
			PendingUserInputs:    map[string]bool{},
			ActiveCommandStreams: map[string]bool{},
		},
		Approvals: []ApprovalProjection{
			{ID: "approval-stale", Status: AgentEventStatusBlocked, Command: "command -v docker && docker --version"},
		},
	}
	snapshot := StateSnapshot{
		SessionID: "sess-stale-projection",
		Runtime: RuntimeSnapshot{
			Turn: RuntimeTurnSnapshot{
				Active: false,
				Phase:  "failed",
			},
		},
	}

	sanitized := SanitizeAgentEventProjectionForSnapshot(projection, snapshot)

	if sanitized.Status == "blocked" {
		t.Fatalf("sanitized status = blocked, want terminal status")
	}
	if len(sanitized.RuntimeLiveness.PendingApprovals) != 0 || len(sanitized.RuntimeLiveness.ActiveTurns) != 0 || len(sanitized.RuntimeLiveness.ActiveAgents) != 0 {
		t.Fatalf("sanitized liveness = %+v, want cleared", sanitized.RuntimeLiveness)
	}
	if len(sanitized.Approvals) != 1 || sanitized.Approvals[0].Status != AgentEventStatusFailed {
		t.Fatalf("sanitized approvals = %+v, want failed historical approval", sanitized.Approvals)
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
