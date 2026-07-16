package appui

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestUpsertCanonicalTransportBlockUpdatesWithoutMoving(t *testing.T) {
	order := []string{}
	blocks := map[string]AiopsTransportBlock{}
	upsertCanonicalTransportBlock(&order, blocks, AiopsTransportBlock{AiopsProcessBlock: AiopsProcessBlock{ID: "first", Text: "running"}})
	upsertCanonicalTransportBlock(&order, blocks, AiopsTransportBlock{AiopsProcessBlock: AiopsProcessBlock{ID: "second", Text: "later"}})
	upsertCanonicalTransportBlock(&order, blocks, AiopsTransportBlock{AiopsProcessBlock: AiopsProcessBlock{ID: "first", Text: "completed"}})

	if want := []string{"first", "second"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	if blocks["first"].Text != "completed" {
		t.Fatalf("first block = %#v, want updated payload", blocks["first"])
	}
}

func TestCanonicalTransportBlockArtifactCollisionKeepsFirstVisiblePosition(t *testing.T) {
	acc := newCanonicalTransportBlockAccumulator()
	artifact := AiopsTransportAgentUIArtifact{ID: "shared", Type: "result", Summary: "artifact"}
	acc.observeTurn(AiopsTransportTurn{AgentUIArtifacts: []AiopsTransportAgentUIArtifact{artifact}})
	acc.reconcileTurn(AiopsTransportTurn{
		Process:          []AiopsProcessBlock{{ID: "shared", Kind: AiopsTransportProcessKindTool, Text: "tool"}},
		AgentUIArtifacts: []AiopsTransportAgentUIArtifact{artifact},
	})

	want := []string{"artifact:shared", "shared"}
	if !reflect.DeepEqual(acc.order, want) {
		t.Fatalf("order = %#v, want collision correction without movement %#v", acc.order, want)
	}
	if acc.blocks["artifact:shared"].Type != AiopsTransportBlockTypeArtifact {
		t.Fatalf("relocated artifact = %#v", acc.blocks["artifact:shared"])
	}
}

func TestTransportProjectorStableOrderAcrossReplayAndApprovalResume(t *testing.T) {
	now := time.Date(2026, 7, 16, 11, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-stable-order",
		SessionID: "session-stable-order",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(3 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "commentary", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "先做预检。", Data: json.RawMessage(`{"phase":"commentary","streamState":"complete"}`)}},
			{ID: "tool-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "运行预检", Data: json.RawMessage(`{"toolCallId":"call-1","toolName":"run_ops_manual_preflight","displayKind":"ops_manual_preflight_result"}`)}},
			{ID: "approval", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "等待审批", Data: json.RawMessage(`{"approvalId":"approval-1","approvalType":"tool"}`)}},
		},
		PendingApprovals: []runtimekernel.PendingApproval{{ID: "approval-1", TurnID: "turn-stable-order", Status: "pending", Reason: "确认执行"}},
	}

	projector := NewTransportProjector()
	state, err := projector.ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-stable-order"), turn)
	if err != nil {
		t.Fatalf("blocked ProjectTurnSnapshot() error = %v", err)
	}
	blockedOrder := append([]string(nil), state.Turns[turn.ID].BlockOrder...)

	turn.Lifecycle = runtimekernel.TurnLifecycleCompleted
	turn.UpdatedAt = now.Add(6 * time.Second)
	turn.PendingApprovals = nil
	turn.AgentItems[1].Status = agentstate.ItemStatusCompleted
	turn.AgentItems[2].Status = agentstate.ItemStatusCompleted
	turn.AgentItems = append(turn.AgentItems,
		agentstate.TurnItem{ID: "tool-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "预检通过", Data: json.RawMessage(`{"toolCallId":"call-1","toolName":"run_ops_manual_preflight","displayKind":"ops_manual_preflight_result","outputPreview":{"status":"passed"}}`)}},
		agentstate.TurnItem{ID: "final", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "预检已通过。", Data: json.RawMessage(`{"phase":"final_answer","streamState":"complete"}`)}},
	)

	state, err = projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("resumed ProjectTurnSnapshot() error = %v", err)
	}
	wantOrder := append([]string(nil), state.Turns[turn.ID].BlockOrder...)
	if !reflect.DeepEqual(wantOrder[:len(blockedOrder)], blockedOrder) {
		t.Fatalf("resume moved existing blocks: blocked=%#v resumed=%#v", blockedOrder, wantOrder)
	}

	fresh, err := projector.ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-stable-order"), turn)
	if err != nil {
		t.Fatalf("fresh ProjectTurnSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(fresh.Turns[turn.ID].BlockOrder, wantOrder) {
		t.Fatalf("fresh order = %#v, persisted replay order = %#v", fresh.Turns[turn.ID].BlockOrder, wantOrder)
	}
	for iteration := 0; iteration < 10; iteration++ {
		state, err = projector.ProjectTurnSnapshot(state, turn)
		if err != nil {
			t.Fatalf("replay %d error = %v", iteration, err)
		}
		if got := state.Turns[turn.ID].BlockOrder; !reflect.DeepEqual(got, wantOrder) {
			t.Fatalf("replay %d order = %#v, want %#v", iteration, got, wantOrder)
		}
	}
}
