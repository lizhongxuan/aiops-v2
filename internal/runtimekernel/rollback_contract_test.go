package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func TestActionRollbackContractValidatorRejectsMissingRequiredFields(t *testing.T) {
	base := validActionRollbackContractForTest()
	tests := []struct {
		name   string
		mutate func(*ActionRollbackContract)
		want   string
	}{
		{name: "targetRefs", mutate: func(c *ActionRollbackContract) { c.TargetRefs = nil }, want: "targetRefs"},
		{name: "expectedEffect", mutate: func(c *ActionRollbackContract) { c.ExpectedEffect = "" }, want: "expectedEffect"},
		{name: "preChangeEvidenceRefs", mutate: func(c *ActionRollbackContract) { c.PreChangeEvidenceRefs = nil }, want: "preChangeEvidenceRefs"},
		{name: "validation", mutate: func(c *ActionRollbackContract) { c.Validation = "" }, want: "validation"},
		{name: "rollback", mutate: func(c *ActionRollbackContract) { c.Rollback = "" }, want: "rollback"},
		{name: "inputHash", mutate: func(c *ActionRollbackContract) { c.InputHash = "" }, want: "inputHash"},
		{name: "toolSurfaceFingerprint", mutate: func(c *ActionRollbackContract) { c.ToolSurfaceFingerprint = "" }, want: "toolSurfaceFingerprint"},
		{name: "permissionSnapshotHash", mutate: func(c *ActionRollbackContract) { c.PermissionSnapshotHash = "" }, want: "permissionSnapshotHash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := base
			tt.mutate(&contract)
			err := contract.ValidateMutating()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateMutating() error = %v, want missing %s", err, tt.want)
			}
		})
	}
}

func TestActionRollbackContractAllowsManualTakeoverInsteadOfRollback(t *testing.T) {
	contract := validActionRollbackContractForTest()
	contract.Rollback = ""
	contract.ManualTakeover = "operator will take manual control if rollback is not deterministic"

	if err := contract.ValidateMutating(); err != nil {
		t.Fatalf("ValidateMutating() error = %v, want manual takeover accepted", err)
	}
}

func TestMarkTurnBlockedRejectsMutatingApprovalWithoutRollbackContract(t *testing.T) {
	now := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)
	kernel := &RuntimeKernel{sessions: NewSessionManager()}
	session := kernel.sessions.GetOrCreate("sess-rollback-invalid", SessionTypeHost, ModeExecute)
	session.HostID = "host-a"
	snapshot := &TurnSnapshot{
		ID:          "turn-rollback-invalid",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   0,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	session.CurrentTurn = snapshot
	kernel.sessions.Update(session)

	result := DispatchResult{
		Blocked: true,
		Reason:  "restart requires approval",
		Outcome: "approval_needed",
		Source:  "tool",
		Metadata: tooling.ToolMetadata{
			Name:             "restart_service",
			Layer:            tooling.ToolLayerMutation,
			Mutating:         true,
			RequiresApproval: true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType: "service",
				ResourceID:   "demo.service",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{"systemctl status demo.service"},
			},
		},
		Approval: &tooling.PermissionApprovalPayload{
			ExpectedEffect: "restart demo.service",
			Validation:     "systemctl status demo.service",
		},
		DecisionTrace: promptinput.DispatchDecisionTrace{
			ArgumentsHash:          "sha256:args",
			ToolSurfaceFingerprint: "surface-1",
			PermissionSnapshotHash: "permission-1",
		},
	}
	err := kernel.markTurnBlocked(session, snapshot, ToolCall{
		ID:        "call-restart",
		Name:      "restart_service",
		Arguments: json.RawMessage(`{"service":"demo.service"}`),
	}, result)
	if err != nil {
		t.Fatalf("markTurnBlocked() error = %v", err)
	}
	if len(session.PendingApprovals) != 0 {
		t.Fatalf("pending approvals = %#v, want none for invalid rollback contract", session.PendingApprovals)
	}
	if len(session.PendingEvidence) != 1 || !strings.Contains(session.PendingEvidence[0].Reason, "rollback_contract_invalid") {
		t.Fatalf("pending evidence = %#v, want rollback_contract_invalid evidence gate", session.PendingEvidence)
	}
}

func TestMarkTurnBlockedStoresRollbackContractForValidMutatingApproval(t *testing.T) {
	now := time.Date(2026, 7, 2, 11, 30, 0, 0, time.UTC)
	kernel := &RuntimeKernel{sessions: NewSessionManager()}
	session := kernel.sessions.GetOrCreate("sess-rollback-valid", SessionTypeHost, ModeExecute)
	session.HostID = "host-a"
	snapshot := &TurnSnapshot{
		ID:          "turn-rollback-valid",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
		Iteration:   0,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{{
			ID:     "tool-result-before",
			Type:   agentstate.TurnItemTypeToolResult,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Data: json.RawMessage(`{"evidenceRefs":["evidence://before-service-status"]}`),
			},
		}},
	}
	session.CurrentTurn = snapshot
	kernel.sessions.Update(session)

	result := DispatchResult{
		Blocked: true,
		Reason:  "restart requires approval",
		Outcome: "approval_needed",
		Source:  "tool",
		Metadata: tooling.ToolMetadata{
			Name:             "restart_service",
			Layer:            tooling.ToolLayerMutation,
			Mutating:         true,
			RequiresApproval: true,
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType: "service",
				ResourceID:   "demo.service",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{"systemctl status demo.service"},
			},
		},
		Approval: &tooling.PermissionApprovalPayload{
			ExpectedEffect: "restart demo.service",
			Rollback:       "systemctl restart demo.service again or restore previous unit state",
			Validation:     "systemctl status demo.service",
		},
		DecisionTrace: promptinput.DispatchDecisionTrace{
			ArgumentsHash:          "sha256:args",
			ToolSurfaceFingerprint: "surface-1",
			PermissionSnapshotHash: "permission-1",
		},
	}
	err := kernel.markTurnBlocked(session, snapshot, ToolCall{
		ID:        "call-restart",
		Name:      "restart_service",
		Arguments: json.RawMessage(`{"service":"demo.service"}`),
	}, result)
	if err != nil {
		t.Fatalf("markTurnBlocked() error = %v", err)
	}
	if len(session.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %#v, want one approval", session.PendingApprovals)
	}
	approval := session.PendingApprovals[0]
	if !approval.Mutating || approval.RollbackContract.SchemaVersion != ActionRollbackContractSchemaVersion {
		t.Fatalf("approval rollback contract = %#v", approval.RollbackContract)
	}
	if err := approval.RollbackContract.ValidateMutating(); err != nil {
		t.Fatalf("rollback contract ValidateMutating() = %v; contract=%#v", err, approval.RollbackContract)
	}
}

func validActionRollbackContractForTest() ActionRollbackContract {
	return ActionRollbackContract{
		SchemaVersion:          ActionRollbackContractSchemaVersion,
		ActionID:               "action-1",
		ToolName:               "restart_service",
		TargetRefs:             []string{"host:host-a", "service:demo.service"},
		InputHash:              "sha256:args",
		Risk:                   "high",
		ExpectedEffect:         "restart demo.service",
		PreChangeEvidenceRefs:  []string{"evidence://before"},
		ApprovalScope:          "host:host-a service:demo.service",
		ResourceScopes:         []string{"host:host-a", "service:demo.service"},
		Rollback:               "restore previous service state",
		Validation:             "systemctl status demo.service",
		PostCheck:              "systemctl status demo.service",
		StopCondition:          "stop if validation fails",
		IdempotencyKey:         "sha256:args",
		ToolSurfaceFingerprint: "surface-1",
		PermissionSnapshotHash: "permission-1",
	}
}
