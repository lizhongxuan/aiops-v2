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
				Process: []AiopsProcessBlock{
					{
						ID:            "block-1",
						Kind:          AiopsTransportProcessKindApproval,
						DisplayKind:   "approval",
						Status:        AiopsTransportProcessStatusBlocked,
						Text:          "Rollback payment-api deployment",
						ApprovalID:    "approval-1",
						OutputPreview: "kubectl rollout undo deployment/payment-api -n prod",
						UpdatedAt:     "2026-05-06T10:00:01Z",
					},
				},
				Final: &AiopsTransportFinal{
					ID:     "final-1",
					Text:   "waiting for approval",
					Status: AiopsTransportFinalStatusRunning,
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
				Pinned:    true,
				UpdatedAt: "2026-05-06T10:00:02Z",
			},
		},
		Artifacts: map[string]AiopsTransportArtifact{
			"artifact-1": {
				ID:         "artifact-1",
				TurnID:     "turn-1",
				Kind:       "diff",
				Title:      "rollback diff",
				Preview:    "1 file changed",
				RawRef:     "artifact://turn-1/diff-1",
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
