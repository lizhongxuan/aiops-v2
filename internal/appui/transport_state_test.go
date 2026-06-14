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

func TestChildAgentTransportPreservesFullRuntimeTraceFields(t *testing.T) {
	child := AiopsTransportChildAgent{
		ID:               "child-host-a",
		MissionID:        "mission-1",
		SessionID:        "host-child:mission-1:host-a",
		HostID:           "host-a",
		HostDisplayName:  "Host A",
		Status:           "running",
		RuntimeProfile:   "host_agent_full_runtime",
		ActiveSubtaskID:  "subtask-1",
		QueueReason:      "same_host_write_task_running",
		TraceSummary:     "base runtime with host-bound scope",
		AgentMessageRefs: []string{"agent-message-1"},
		PromptSections: []AiopsTransportPromptSectionTrace{{
			ID:             "host_agent.binding.v1",
			Kind:           "dynamic",
			Source:         "host-task",
			RetentionRank:  "P0",
			RetentionClass: "must_keep",
			CompactAction:  "kept_original",
			SourceRef:      "agent-message-1",
			Redaction:      "not_required",
		}},
		ContextDecisions: []AiopsTransportContextDecision{{
			Kind:          "host_fact",
			Decision:      "included",
			Reason:        "bound_host",
			RetentionRank: "P2",
			SourceRef:     "artifact://host-facts/summary",
		}},
		ToolSurface: []AiopsTransportToolSurfaceEntry{{
			Name:    "host_command",
			Visible: true,
			Reason:  "bound_host_tool",
		}},
		McpInstructionDeltas: []AiopsTransportScopedTraceEntry{{
			ID:     "docs-readonly",
			Status: "available",
			Reason: "readonly_resource_in_scope",
		}},
		SkillActivationTrace: []AiopsTransportScopedTraceEntry{{
			ID:     "generic-service-inspection",
			Status: "recommended",
			Reason: "manager_recommended_not_loaded",
		}},
		ApprovalTrace: []AiopsTransportScopedTraceEntry{{
			ID:     "approval-1",
			Status: "pending",
			Reason: "non_whitelisted_command",
		}},
		EvidenceTrace: []AiopsTransportEvidenceTrace{{
			ID:        "evidence-1",
			Source:    "host_agent_tool",
			Ref:       "artifact://evidence/1",
			Redaction: "applied",
			Summary:   "command result summary",
		}},
		ReportTimeline: []AiopsTransportHostTaskReportTrace{{
			ID:         "report-1",
			Status:     "blocked",
			HostID:     "host-a",
			PlanStepID: "step-1",
			Summary:    "waiting for approval",
		}},
	}

	raw, err := json.Marshal(child)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var roundTrip AiopsTransportChildAgent
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !reflect.DeepEqual(roundTrip, child) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", roundTrip, child)
	}
}
