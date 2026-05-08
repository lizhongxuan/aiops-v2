package appui

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestTransportStateInitializesDefaults(t *testing.T) {
	state := NewAiopsTransportState("session-1", "thread-1")

	if state.SchemaVersion != AiopsTransportSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", state.SchemaVersion, AiopsTransportSchemaVersion)
	}
	if state.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", state.SessionID)
	}
	if state.ThreadID != "thread-1" {
		t.Fatalf("ThreadID = %q, want thread-1", state.ThreadID)
	}
	if state.Status != AiopsTransportStatusIdle {
		t.Fatalf("Status = %q, want %q", state.Status, AiopsTransportStatusIdle)
	}
	if state.Seq != 0 {
		t.Fatalf("Seq = %d, want 0", state.Seq)
	}
	if state.Turns == nil || state.PendingApprovals == nil || state.McpSurfaces == nil || state.Artifacts == nil {
		t.Fatalf("expected initialized maps, got turns=%v approvals=%v mcp=%v artifacts=%v", state.Turns, state.PendingApprovals, state.McpSurfaces, state.Artifacts)
	}
	if state.RuntimeLiveness.ActiveTurns == nil || state.RuntimeLiveness.PendingApprovals == nil {
		t.Fatalf("expected initialized runtime liveness maps, got %+v", state.RuntimeLiveness)
	}
	if _, err := time.Parse(time.RFC3339Nano, state.UpdatedAt); err != nil {
		t.Fatalf("UpdatedAt = %q is not RFC3339Nano: %v", state.UpdatedAt, err)
	}
}

func TestTransportStateJSONRoundTripPreservesFields(t *testing.T) {
	state := AiopsTransportState{
		SchemaVersion: AiopsTransportSchemaVersion,
		SessionID:     "session-1",
		ThreadID:      "thread-1",
		Status:        AiopsTransportStatusBlocked,
		CurrentTurnID: "turn-1",
		Turns: map[string]AiopsTransportTurn{
			"turn-1": {
				ID:        "turn-1",
				Status:    AiopsTransportTurnStatusBlocked,
				StartedAt: "2026-05-06T10:00:00Z",
				User: &AiopsTransportMessage{
					ID:        "msg-user-1",
					Text:      "rollback payment-api",
					CreatedAt: "2026-05-06T10:00:00Z",
				},
				Intent: &AiopsTransportIntent{
					Text:   "validate rollback target",
					Status: string(AiopsTransportProcessStatusRunning),
				},
				BlockOrder: []string{"approval-1", "text-1"},
				BlocksByID: map[string]AiopsTranscriptBlock{
					"approval-1": {
						ID:   "approval-1",
						Type: AiopsTranscriptBlockTypeApproval,
						Approval: &AiopsApprovalBlock{
							ApprovalID:   "approval-1",
							ApprovalKind: "command",
							Title:        "等待审批",
							Summary:      "Rollback payment-api deployment",
							Command:      "kubectl rollout undo deployment/payment-api -n prod",
							Status:       string(AiopsTransportProcessStatusBlocked),
							RequestedAt:  "2026-05-06T10:00:01Z",
						},
						UpdatedAt: "2026-05-06T10:00:01Z",
					},
					"text-1": {
						ID:   "text-1",
						Type: AiopsTranscriptBlockTypeText,
						Text: &AiopsTextBlock{
							Role:   "assistant",
							Text:   "waiting for approval",
							Status: AiopsTranscriptTextStatusStreaming,
						},
					},
				},
			},
		},
		TurnOrder: []string{"turn-1"},
		PendingApprovals: map[string]AiopsTransportApproval{
			"approval-1": {
				ID:          "approval-1",
				TurnID:      "turn-1",
				Type:        "command",
				Status:      string(AiopsTransportProcessStatusBlocked),
				Command:     "kubectl rollout undo deployment/payment-api -n prod",
				Reason:      "needs approval",
				RequestedAt: "2026-05-06T10:00:01Z",
			},
		},
		McpSurfaces: map[string]AiopsTransportMcpSurface{
			"surface-1": {
				ID:        "surface-1",
				Kind:      "bundle",
				Title:     "Kubernetes remediation",
				Status:    "ready",
				Lifecycle: AiopsTransportLifecycleReady,
				Pinned:    true,
				Cards: []AiopsAgentUICard{{
					ID:         "card-1",
					Kind:       "agent_to_ui",
					Title:      "Rollback diff",
					Summary:    "Review rollback artifact",
					Status:     "ready",
					ArtifactID: "artifact-1",
					Actions: []AiopsTransportActionBinding{{
						ID:               "approve-rollback",
						Label:            "同意",
						Command:          "aiops.approval-decision",
						Target:           "approval-1",
						Params:           map[string]any{"decision": "accept"},
						RequiresApproval: true,
					}},
				}},
				App: &AiopsIframeAppSurface{
					URL:         "app://mcp/kubernetes-remediation",
					Sandbox:     "allow-scripts",
					Height:      640,
					Width:       960,
					Permissions: []string{"clipboard-read"},
				},
				Actions: []AiopsTransportActionBinding{{
					ID:      "refresh",
					Label:   "刷新",
					Command: "aiops.mcp-refresh",
					Target:  "surface-1",
				}},
				ArtifactIDs: []string{"artifact-1"},
				UpdatedAt:   "2026-05-06T10:00:02Z",
			},
		},
		Artifacts: map[string]AiopsTransportArtifact{
			"artifact-1": {
				ID:      "artifact-1",
				TurnID:  "turn-1",
				Kind:    "diff",
				Title:   "rollback diff",
				Preview: "1 file changed",
				PreviewData: &AiopsArtifactPreview{
					ContentType: "text/markdown",
					Text:        "1 file changed",
					RawRef:      "artifact://turn-1/diff-1",
					Metadata:    map[string]string{"format": "diff"},
				},
				RawRef:    "artifact://turn-1/diff-1",
				Lifecycle: AiopsTransportLifecycleReady,
				Actions: []AiopsTransportActionBinding{{
					ID:      "open-diff",
					Label:   "查看变更",
					Command: "aiops.artifact-open",
					Target:  "artifact-1",
				}},
				CreatedAt:  "2026-05-06T10:00:03Z",
				ModifiedAt: "2026-05-06T10:00:03Z",
			},
		},
		RuntimeLiveness: AiopsRuntimeLiveness{
			ActiveTurns:          map[string]bool{"turn-1": true},
			ActiveAgents:         map[string]bool{"agent-main": true},
			PendingApprovals:     map[string]bool{"approval-1": true},
			PendingUserInputs:    map[string]bool{"choice-1": true},
			ActiveCommandStreams: map[string]bool{"cmd-1": true},
		},
		LastError: "approval required",
		Seq:       42,
		UpdatedAt: "2026-05-06T10:00:03Z",
	}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var roundTrip AiopsTransportState
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(roundTrip, state) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", roundTrip, state)
	}
}

func TestTransportStableIDsAreRepeatable(t *testing.T) {
	turnA := TransportTurnStableID("thread-1", "turn-1")
	turnB := TransportTurnStableID("thread-1", "turn-1")
	if turnA != turnB {
		t.Fatalf("turn IDs differ: %q vs %q", turnA, turnB)
	}

	blockA := TransportProcessBlockStableID("turn-1", "approval", "approval-1")
	blockB := TransportProcessBlockStableID("turn-1", "approval", "approval-1")
	if blockA != blockB {
		t.Fatalf("block IDs differ: %q vs %q", blockA, blockB)
	}
	if blockA == TransportProcessBlockStableID("turn-1", "approval", "approval-2") {
		t.Fatalf("expected different block IDs for different source IDs")
	}
}

func TestUpsertAiopsTranscriptBlockAppendsOrderOnce(t *testing.T) {
	turn := AiopsTransportTurn{ID: "turn-1"}
	block := AiopsTranscriptBlock{
		ID:   "block-1",
		Type: AiopsTranscriptBlockTypeText,
		Text: &AiopsTextBlock{Text: "hello"},
	}

	turn = UpsertAiopsTranscriptBlock(turn, block)
	turn = UpsertAiopsTranscriptBlock(turn, block)

	if got := len(turn.BlockOrder); got != 1 {
		t.Fatalf("BlockOrder length = %d, want 1: %+v", got, turn.BlockOrder)
	}
	if turn.BlockOrder[0] != "block-1" {
		t.Fatalf("BlockOrder[0] = %q, want block-1", turn.BlockOrder[0])
	}
	if turn.BlocksByID["block-1"].ID != "block-1" {
		t.Fatalf("BlocksByID missing block: %+v", turn.BlocksByID)
	}
}

func TestReplaceVisibleBlocksWithAggregateKeepsTimelinePosition(t *testing.T) {
	turn := AiopsTransportTurn{
		ID:         "turn-1",
		BlockOrder: []string{"text-1", "cmd-1", "cmd-2", "text-2"},
		BlocksByID: map[string]AiopsTranscriptBlock{
			"text-1": {ID: "text-1", Type: AiopsTranscriptBlockTypeText},
			"cmd-1":  {ID: "cmd-1", Type: AiopsTranscriptBlockTypeTool},
			"cmd-2":  {ID: "cmd-2", Type: AiopsTranscriptBlockTypeTool},
			"text-2": {ID: "text-2", Type: AiopsTranscriptBlockTypeText},
		},
	}
	aggregate := AiopsTranscriptBlock{
		ID:   "agg-1",
		Type: AiopsTranscriptBlockTypeAggregate,
		Aggregate: &AiopsAggregateBlock{
			ChildBlockIDs: []string{"cmd-1", "cmd-2"},
		},
	}

	turn = ReplaceVisibleBlocksWithAggregate(turn, []string{"cmd-1", "cmd-2"}, aggregate)

	want := []string{"text-1", "agg-1", "text-2"}
	if !reflect.DeepEqual(turn.BlockOrder, want) {
		t.Fatalf("BlockOrder = %+v, want %+v", turn.BlockOrder, want)
	}
	if _, ok := turn.BlocksByID["cmd-1"]; !ok {
		t.Fatalf("child cmd-1 should remain in BlocksByID for detail lookup")
	}
}
