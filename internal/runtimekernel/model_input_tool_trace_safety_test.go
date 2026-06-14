package runtimekernel

import (
	"testing"
	"time"

	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/taskdepth"
)

func TestModelInputToolTraceFieldsCollectVerificationSafetyPermissionState(t *testing.T) {
	expiresAt := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	session := &SessionState{
		ID:   "sess-synthetic-trace",
		Type: SessionTypeWorkspace,
		Mode: ModeExecute,
		PlanMode: PlanModeState{
			State:  PlanModeStateActive,
			PlanID: "plan-synthetic",
		},
		PlanApprovalScopes: []PlanApprovalScope{{
			PlanID:         "plan-synthetic",
			ApprovalID:     "approval-synthetic",
			AllowedActions: []string{"synthetic.write"},
			ResourceScopes: []PlanApprovalResourceScope{{Type: "synthetic_resource", ID: "synthetic-id"}},
			RiskCeiling:    "medium",
			ExpiresAt:      &expiresAt,
			InputHash:      "sha256:synthetic-input",
		}},
		PendingApprovals: []PendingApproval{{
			ID:        "approval-safety",
			SessionID: "sess-synthetic-trace",
			TurnID:    "turn-synthetic-trace",
			ToolName:  "synthetic.write",
			Command:   "synthetic write --force skip validation",
			Reason:    "safety signal requires approval",
			Risk:      "high",
			Status:    "pending",
			CreatedAt: expiresAt,
			UpdatedAt: expiresAt,
		}},
		CurrentTurn: &TurnSnapshot{
			ID:          "turn-synthetic-trace",
			SessionID:   "sess-synthetic-trace",
			SessionType: SessionTypeWorkspace,
			Mode:        ModeExecute,
			TaskDepth:   taskdepth.Profile{Level: taskdepth.LevelOperations, RequiresEvidence: true, RequiresValidation: true},
			Iterations: []IterationState{{
				ToolResults: []ToolResult{{
					ToolCallID: "call-unexpected",
					Content:    `{"status":"unexpected_state","resourceType":"synthetic_resource","resourceId":"synthetic-id","summary":"synthetic state changed"}`,
				}},
			}},
		},
	}

	fields := buildModelInputToolTraceFields(session, session.CurrentTurn, "toolsurface-synthetic", "policy-synthetic")
	if fields.ApprovalScope == nil || fields.ApprovalScope.InputHash != "sha256:synthetic-input" || fields.ApprovalScope.RiskCeiling != "medium" {
		t.Fatalf("approval scope trace = %#v, want approved scope with input hash and risk ceiling", fields.ApprovalScope)
	}
	if len(fields.SafetySignals) == 0 {
		t.Fatalf("safety signals = %#v, want destructive workaround signal", fields.SafetySignals)
	}
	if !traceHasSafetyCategory(fields.SafetySignals, "force") || !traceHasSafetyCategory(fields.SafetySignals, "skip_validation") {
		t.Fatalf("safety signals = %#v, want force and skip_validation", fields.SafetySignals)
	}
	if fields.UnexpectedStateGate == nil || fields.UnexpectedStateGate.Action != UnexpectedStateActionBlockMutation {
		t.Fatalf("unexpected gate = %#v, want block_mutation", fields.UnexpectedStateGate)
	}
	if !containsString(fields.UnexpectedStateGate.Reasons, "unexpected_state") {
		t.Fatalf("unexpected reasons = %#v, want unexpected_state", fields.UnexpectedStateGate.Reasons)
	}
}

func TestPendingApprovalScopeTraceIncludesScopeExpiryAndInputHash(t *testing.T) {
	expiresAt := time.Date(2026, 6, 7, 12, 15, 0, 0, time.UTC)
	session := &SessionState{
		ID:   "sess-synthetic-pending-approval",
		Type: SessionTypeWorkspace,
		Mode: ModeExecute,
		PendingApprovals: []PendingApproval{{
			ID:             "approval-synthetic-pending",
			SessionID:      "sess-synthetic-pending-approval",
			TurnID:         "turn-synthetic-pending-approval",
			ToolName:       "synthetic.write",
			AllowedActions: []string{"synthetic.write"},
			ResourceScopes: []string{"type=synthetic_resource id=synthetic-id"},
			RiskCeiling:    "high",
			ExpiresAt:      &expiresAt,
			InputHash:      "sha256:synthetic-input",
			Status:         "pending",
			CreatedAt:      expiresAt.Add(-15 * time.Minute),
			UpdatedAt:      expiresAt.Add(-15 * time.Minute),
		}},
	}

	trace := approvalScopeTraceFromSession(session)
	if trace == nil || trace.Status != "pending" {
		t.Fatalf("approval trace = %#v, want pending", trace)
	}
	if trace.InputHash != "sha256:synthetic-input" || trace.ExpiresAt != "2026-06-07T12:15:00Z" || trace.RiskCeiling != "high" {
		t.Fatalf("approval trace = %#v, want input hash, expiry and risk ceiling", trace)
	}
	if !containsString(trace.AllowedActions, "synthetic.write") || !containsString(trace.ResourceScopes, "type=synthetic_resource id=synthetic-id") {
		t.Fatalf("approval trace scopes = actions=%v resources=%v", trace.AllowedActions, trace.ResourceScopes)
	}
}

func traceHasSafetyCategory(signals []promptinput.SafetySignalTrace, category string) bool {
	for _, signal := range signals {
		if signal.Category == category {
			return true
		}
	}
	return false
}
