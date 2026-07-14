package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modeltrace"
)

func TestCommitSystemTurnOwnsCanonicalTerminalFactsWithoutModelOrTools(t *testing.T) {
	sessions := NewSessionManager()
	kernel := NewRuntimeKernel(RuntimeKernelConfig{Sessions: sessions})
	req := validSystemTurnRequest()
	req.Output.AgentItems = []agentstate.TurnItem{{
		ID:     "turn-system-1-plan",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "plan",
			Summary: "deterministic handoff plan",
		},
	}}

	result, err := kernel.CommitSystemTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("CommitSystemTurn() error = %v", err)
	}
	if result.Status != "completed" || result.Output != req.Output.FinalText {
		t.Fatalf("result = %#v, want completed system output", result)
	}
	if result.SessionID != req.Turn.SessionID || result.TurnID != req.Turn.TurnID ||
		result.ClientTurnID != req.Turn.ClientTurnID || result.ClientMessageID != req.Turn.ClientMessageID {
		t.Fatalf("result identity = %#v, want request identity", result)
	}

	session := sessions.Get(req.Turn.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("system turn did not persist current turn")
	}
	snapshot := session.CurrentTurn
	if snapshot.Lifecycle != TurnLifecycleCompleted || snapshot.ResumeState != TurnResumeStateNone || snapshot.CompletedAt == nil {
		t.Fatalf("system turn lifecycle = %#v, want terminal completed", snapshot)
	}
	if snapshot.FinalOutput != req.Output.FinalText {
		t.Fatalf("FinalOutput = %q, want %q", snapshot.FinalOutput, req.Output.FinalText)
	}
	if snapshot.SystemTurn == nil || snapshot.SystemTurn.Kind != req.Output.Kind || snapshot.SystemTurn.ContractStatus != req.Output.ContractStatus {
		t.Fatalf("SystemTurn = %#v, want typed durable system-turn facts", snapshot.SystemTurn)
	}
	if snapshot.LatestCheckpoint == nil || snapshot.LatestCheckpoint.Kind != "system_turn" || snapshot.LatestCheckpoint.Source != "runtimekernel" {
		t.Fatalf("LatestCheckpoint = %#v, want runtimekernel system_turn checkpoint", snapshot.LatestCheckpoint)
	}
	if session.LatestCheckpoint == nil || session.LatestCheckpoint.ID != snapshot.LatestCheckpoint.ID {
		t.Fatalf("session checkpoint = %#v, want turn checkpoint", session.LatestCheckpoint)
	}
	if len(session.PendingApprovals) != 0 || len(session.PendingEvidence) != 0 || len(snapshot.PendingApprovals) != 0 || len(snapshot.PendingEvidence) != 0 {
		t.Fatalf("terminal system turn retained pending state: session=%#v turn=%#v", session, snapshot)
	}
	if len(session.Messages) != 2 || session.Messages[0].Role != "user" || session.Messages[0].Content != req.Turn.Input ||
		session.Messages[1].Role != "assistant" || session.Messages[1].Content != req.Output.FinalText {
		t.Fatalf("messages = %#v, want canonical user then assistant", session.Messages)
	}

	wantTypes := map[agentstate.TurnItemType]int{
		agentstate.TurnItemTypeUserMessage:      1,
		agentstate.TurnItemTypePlan:             1,
		agentstate.TurnItemTypeAssistantMessage: 1,
		agentstate.TurnItemTypeFinalResponse:    1,
	}
	gotTypes := map[agentstate.TurnItemType]int{}
	var assistant agentstate.TurnItem
	for _, item := range snapshot.AgentItems {
		gotTypes[item.Type]++
		if item.Type == agentstate.TurnItemTypeAssistantMessage {
			assistant = item
		}
		if item.Type == agentstate.TurnItemTypeModelCall || item.Type == agentstate.TurnItemTypeToolCall || item.Type == agentstate.TurnItemTypeToolResult {
			t.Fatalf("system turn fabricated model/tool item: %#v", item)
		}
	}
	for typ, want := range wantTypes {
		if gotTypes[typ] != want {
			t.Fatalf("agent item type %s count = %d, want %d; items=%#v", typ, gotTypes[typ], want, snapshot.AgentItems)
		}
	}
	var assistantPayload struct {
		Phase         string        `json:"phase"`
		StreamState   string        `json:"streamState"`
		FinalContract FinalContract `json:"finalContract"`
	}
	if err := json.Unmarshal(assistant.Payload.Data, &assistantPayload); err != nil {
		t.Fatalf("decode assistant payload: %v", err)
	}
	if assistantPayload.Phase != "final_answer" || assistantPayload.StreamState != "complete" ||
		assistantPayload.FinalContract.Status != req.Output.ContractStatus || assistantPayload.FinalContract.AnswerText != req.Output.FinalText {
		t.Fatalf("assistant payload = %#v, want canonical typed final contract", assistantPayload)
	}
	if !hasAcceptedSystemTurnOwnerTrace(snapshot.OwnerWriteTraces, OwnerWriteTurnLifecycle) ||
		!hasAcceptedSystemTurnOwnerTrace(snapshot.OwnerWriteTraces, OwnerWriteAssistantMessage) {
		t.Fatalf("owner write traces = %#v, want runtimekernel lifecycle and assistant ownership", snapshot.OwnerWriteTraces)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("persisted system turn snapshot invalid: %v", err)
	}

	events, err := kernel.CanonicalRolloutEvents(context.Background(), req.Turn.SessionID, req.Turn.TurnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	wantKinds := []string{
		modeltrace.CanonicalRolloutKindCheckpoint,
		modeltrace.CanonicalRolloutKindFinalFacts,
		modeltrace.CanonicalRolloutKindTransportProjection,
	}
	if len(events) != len(wantKinds) {
		t.Fatalf("rollout events = %#v, want checkpoint/final facts/transport only", events)
	}
	for i, event := range events {
		if event.Kind != wantKinds[i] {
			t.Fatalf("rollout event[%d] kind = %q, want %q", i, event.Kind, wantKinds[i])
		}
	}
}

func TestCommitSystemTurnUsesActiveTurnGuard(t *testing.T) {
	sessions := NewSessionManager()
	kernel := NewRuntimeKernel(RuntimeKernelConfig{Sessions: sessions})
	now := time.Now().UTC()
	session := sessions.GetOrCreate("sess-system-active", SessionTypeHost, ModeChat)
	session.CurrentTurn = &TurnSnapshot{
		ID:          "turn-active",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	sessions.Update(session)
	req := validSystemTurnRequest()
	req.Turn.SessionID = session.ID
	req.Turn.TurnID = "turn-system-queued"

	result, err := kernel.CommitSystemTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("CommitSystemTurn() error = %v", err)
	}
	if result.Status != "pending_input" || result.TurnID != "turn-active" {
		t.Fatalf("result = %#v, want pending input on active regular turn", result)
	}
	updated := sessions.Get(session.ID)
	if updated.CurrentTurn == nil || updated.CurrentTurn.ID != "turn-active" || updated.CurrentTurn.Lifecycle != TurnLifecycleRunning {
		t.Fatalf("CurrentTurn = %#v, want active turn preserved", updated.CurrentTurn)
	}
	if len(updated.CurrentTurn.PendingInputs) != 1 || updated.CurrentTurn.PendingInputs[0].Content != req.Turn.Input {
		t.Fatalf("PendingInputs = %#v, want queued user input", updated.CurrentTurn.PendingInputs)
	}
	if updated.CurrentTurn.SystemTurn != nil || updated.CurrentTurn.FinalOutput != "" {
		t.Fatalf("active turn received system terminal facts: %#v", updated.CurrentTurn)
	}
}

func TestCommitSystemTurnFailsClosedBeforeTerminalPersistence(t *testing.T) {
	sessions := NewSessionManager()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         systemTurnRolloutStoreFunc(func(context.Context, modeltrace.CanonicalRolloutEvent) error { return errors.New("store unavailable") }),
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	kernel := NewRuntimeKernel(RuntimeKernelConfig{Sessions: sessions, RolloutRecorder: recorder})
	req := validSystemTurnRequest()

	if _, err := kernel.CommitSystemTurn(context.Background(), req); err == nil {
		t.Fatal("CommitSystemTurn() error = nil, want fail-closed rollout error")
	}
	session := sessions.Get(req.Turn.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("failed system turn lost its admitted running snapshot")
	}
	if session.CurrentTurn.Lifecycle.IsTerminal() || session.CurrentTurn.FinalOutput != "" || session.CurrentTurn.CompletedAt != nil {
		t.Fatalf("failed recorder committed terminal state: %#v", session.CurrentTurn)
	}
	for _, message := range session.Messages {
		if message.Role == "assistant" {
			t.Fatalf("failed recorder committed assistant message: %#v", message)
		}
	}
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type == agentstate.TurnItemTypeAssistantMessage || item.Type == agentstate.TurnItemTypeFinalResponse {
			t.Fatalf("failed recorder committed final item: %#v", item)
		}
	}
}

func TestSystemTurnRequestRejectsFabricatedRuntimeFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SystemTurnRequest)
		want   string
	}{
		{name: "empty final", mutate: func(req *SystemTurnRequest) { req.Output.FinalText = "" }, want: "final text"},
		{name: "invalid kind", mutate: func(req *SystemTurnRequest) { req.Output.Kind = SystemTurnKind("other") }, want: "kind"},
		{name: "verified without evidence", mutate: func(req *SystemTurnRequest) { req.Output.ContractStatus = FinalContractStatusVerified }, want: "verified"},
		{name: "assistant item", mutate: func(req *SystemTurnRequest) {
			req.Output.AgentItems = []agentstate.TurnItem{{ID: "forged-assistant", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted}}
		}, want: "assistant_message"},
		{name: "model item", mutate: func(req *SystemTurnRequest) {
			req.Output.AgentItems = []agentstate.TurnItem{{ID: "forged-model", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted}}
		}, want: "model_call"},
		{name: "unfinished item", mutate: func(req *SystemTurnRequest) {
			req.Output.AgentItems = []agentstate.TurnItem{{ID: "running-plan", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusRunning}}
		}, want: "completed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := validSystemTurnRequest()
			test.mutate(&req)
			if err := req.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func validSystemTurnRequest() SystemTurnRequest {
	return SystemTurnRequest{
		Turn: TurnRequest{
			SessionType:     SessionTypeHost,
			Mode:            ModeChat,
			SessionID:       "sess-system-1",
			TurnID:          "turn-system-1",
			ClientTurnID:    "client-turn-system-1",
			ClientMessageID: "client-message-system-1",
			Input:           "create a workflow",
		},
		Output: SystemTurnOutput{
			Kind:           SystemTurnKindNotice,
			FinalText:      "Workflow creation moved to Runner Studio.",
			ContractStatus: FinalContractStatusPartial,
			FailureCodes:   []string{"workflow_creation_migrated"},
		},
	}
}

func hasAcceptedSystemTurnOwnerTrace(traces []OwnerWriteTrace, responsibility OwnerWriteResponsibility) bool {
	for _, trace := range traces {
		if trace.Responsibility == responsibility && trace.Writer == OwnerRuntimeKernel && trace.Outcome == OwnerWriteOutcomeAccepted {
			return true
		}
	}
	return false
}

type systemTurnRolloutStoreFunc func(context.Context, modeltrace.CanonicalRolloutEvent) error

func (fn systemTurnRolloutStoreFunc) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	return fn(ctx, event)
}
