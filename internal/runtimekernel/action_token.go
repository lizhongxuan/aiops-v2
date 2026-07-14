package runtimekernel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/agentassembly"
)

const (
	ActionTokenSchemaVersion = "aiops.approval-action-token.v1"
	ApprovalContextStaleCode = "approval_context_stale"
)

// ActionToken freezes the server-owned facts that one approval decision binds.
// It is not a bearer credential and never grants authority by itself.
type ActionToken struct {
	SchemaVersion          string    `json:"schemaVersion"`
	ApprovalID             string    `json:"approvalId"`
	TurnID                 string    `json:"turnId"`
	ToolCallID             string    `json:"toolCallId"`
	ToolName               string    `json:"toolName"`
	ArgumentsHash          string    `json:"argumentsHash"`
	TargetRefs             []string  `json:"targetRefs"`
	ToolSurfaceFingerprint string    `json:"toolSurfaceFingerprint"`
	PermissionHash         string    `json:"permissionHash"`
	RollbackHash           string    `json:"rollbackHash"`
	CheckpointID           string    `json:"checkpointId"`
	ExpiresAt              time.Time `json:"expiresAt"`
	Hash                   string    `json:"hash"`
}

type ActionTokenCurrentFacts struct {
	ApprovalID             string
	TurnID                 string
	ToolCallID             string
	ToolName               string
	ArgumentsHash          string
	TargetRefs             []string
	ToolSurfaceFingerprint string
	PermissionHash         string
	RollbackHash           string
	CheckpointID           string
}

type VerifiedActionToken struct {
	token            ActionToken
	current          ActionTokenCurrentFacts
	verificationHash string
}

type ApprovalContextStaleError struct {
	Code           string   `json:"code"`
	MismatchFields []string `json:"mismatchFields"`
}

func (e *ApprovalContextStaleError) Error() string {
	if e == nil {
		return ApprovalContextStaleCode
	}
	fields := uniqueSortedTraceStrings(e.MismatchFields)
	if len(fields) == 0 {
		return ApprovalContextStaleCode
	}
	return ApprovalContextStaleCode + ": " + strings.Join(fields, ",")
}

func FreezeActionToken(input ActionToken) (ActionToken, error) {
	token := normalizeActionToken(input)
	token.SchemaVersion = ActionTokenSchemaVersion
	token.Hash = ""
	token.Hash = actionTokenHash(token)
	if err := token.Validate(); err != nil {
		return ActionToken{}, err
	}
	return token, nil
}

func (token ActionToken) Validate() error {
	token = normalizeActionToken(token)
	if token.SchemaVersion != ActionTokenSchemaVersion {
		return fmt.Errorf("invalid action token schema version")
	}
	fields := []struct{ name, value string }{
		{"approvalId", token.ApprovalID}, {"turnId", token.TurnID}, {"toolCallId", token.ToolCallID},
		{"toolName", token.ToolName}, {"argumentsHash", token.ArgumentsHash}, {"toolSurfaceFingerprint", token.ToolSurfaceFingerprint},
		{"permissionHash", token.PermissionHash}, {"rollbackHash", token.RollbackHash}, {"checkpointId", token.CheckpointID},
	}
	for _, field := range fields {
		if field.value == "" {
			return fmt.Errorf("action token requires %s", field.name)
		}
	}
	if len(token.TargetRefs) == 0 {
		return fmt.Errorf("action token requires targetRefs")
	}
	if token.ExpiresAt.IsZero() {
		return fmt.Errorf("action token requires expiresAt")
	}
	if token.Hash == "" || token.Hash != actionTokenHash(token) {
		return fmt.Errorf("action token hash mismatch")
	}
	return nil
}

func VerifyActionToken(token ActionToken, current ActionTokenCurrentFacts, now time.Time) (VerifiedActionToken, error) {
	if err := token.Validate(); err != nil {
		return VerifiedActionToken{}, newApprovalContextStaleError("token")
	}
	current = normalizeActionTokenCurrentFacts(current)
	mismatches := actionTokenMismatchFields(token, current)
	if now.IsZero() {
		now = time.Now()
	}
	if !now.Before(token.ExpiresAt) {
		mismatches = append(mismatches, "expiry")
	}
	if len(mismatches) > 0 {
		return VerifiedActionToken{}, newApprovalContextStaleError(mismatches...)
	}
	verified := VerifiedActionToken{token: token, current: current}
	verified.verificationHash = verifiedActionTokenHash(token, current)
	return verified, nil
}

func (verified VerifiedActionToken) revalidateDispatch(turnID, toolCallID, toolName, argumentsHash, toolSurfaceFingerprint, permissionHash string, now time.Time) error {
	if verified.verificationHash == "" || verified.verificationHash != verifiedActionTokenHash(verified.token, verified.current) {
		return newApprovalContextStaleError("token")
	}
	current := verified.current
	current.TurnID = strings.TrimSpace(turnID)
	current.ToolCallID = strings.TrimSpace(toolCallID)
	current.ToolName = strings.TrimSpace(toolName)
	current.ArgumentsHash = strings.TrimSpace(argumentsHash)
	current.ToolSurfaceFingerprint = strings.TrimSpace(toolSurfaceFingerprint)
	current.PermissionHash = strings.TrimSpace(permissionHash)
	_, err := VerifyActionToken(verified.token, current, now)
	return err
}

func actionTokenMismatchFields(token ActionToken, current ActionTokenCurrentFacts) []string {
	var mismatches []string
	checks := []struct{ field, expected, actual string }{
		{"approval", token.ApprovalID, current.ApprovalID}, {"turn", token.TurnID, current.TurnID},
		{"tool_call", token.ToolCallID, current.ToolCallID}, {"tool", token.ToolName, current.ToolName},
		{"arguments", token.ArgumentsHash, current.ArgumentsHash}, {"target", strings.Join(token.TargetRefs, "\x00"), strings.Join(current.TargetRefs, "\x00")},
		{"tool_router", token.ToolSurfaceFingerprint, current.ToolSurfaceFingerprint}, {"permission", token.PermissionHash, current.PermissionHash},
		{"rollback", token.RollbackHash, current.RollbackHash}, {"checkpoint", token.CheckpointID, current.CheckpointID},
	}
	for _, check := range checks {
		if check.expected != check.actual {
			mismatches = append(mismatches, check.field)
		}
	}
	return uniqueSortedTraceStrings(mismatches)
}

func normalizeActionToken(token ActionToken) ActionToken {
	token.SchemaVersion = strings.TrimSpace(token.SchemaVersion)
	token.ApprovalID = strings.TrimSpace(token.ApprovalID)
	token.TurnID = strings.TrimSpace(token.TurnID)
	token.ToolCallID = strings.TrimSpace(token.ToolCallID)
	token.ToolName = strings.TrimSpace(token.ToolName)
	token.ArgumentsHash = strings.TrimSpace(token.ArgumentsHash)
	token.TargetRefs = uniqueSortedTraceStrings(token.TargetRefs)
	token.ToolSurfaceFingerprint = strings.TrimSpace(token.ToolSurfaceFingerprint)
	token.PermissionHash = strings.TrimSpace(token.PermissionHash)
	token.RollbackHash = strings.TrimSpace(token.RollbackHash)
	token.CheckpointID = strings.TrimSpace(token.CheckpointID)
	token.Hash = strings.TrimSpace(token.Hash)
	return token
}

func normalizeActionTokenCurrentFacts(facts ActionTokenCurrentFacts) ActionTokenCurrentFacts {
	facts.ApprovalID = strings.TrimSpace(facts.ApprovalID)
	facts.TurnID = strings.TrimSpace(facts.TurnID)
	facts.ToolCallID = strings.TrimSpace(facts.ToolCallID)
	facts.ToolName = strings.TrimSpace(facts.ToolName)
	facts.ArgumentsHash = strings.TrimSpace(facts.ArgumentsHash)
	facts.TargetRefs = uniqueSortedTraceStrings(facts.TargetRefs)
	facts.ToolSurfaceFingerprint = strings.TrimSpace(facts.ToolSurfaceFingerprint)
	facts.PermissionHash = strings.TrimSpace(facts.PermissionHash)
	facts.RollbackHash = strings.TrimSpace(facts.RollbackHash)
	facts.CheckpointID = strings.TrimSpace(facts.CheckpointID)
	return facts
}

func actionTokenHash(token ActionToken) string {
	token.Hash = ""
	return agentassembly.StableHash("approval-action-token", token)
}

func verifiedActionTokenHash(token ActionToken, current ActionTokenCurrentFacts) string {
	return agentassembly.StableHash("verified-approval-action-token", struct {
		Token   ActionToken
		Current ActionTokenCurrentFacts
	}{token, current})
}

func newApprovalContextStaleError(fields ...string) *ApprovalContextStaleError {
	fields = uniqueSortedTraceStrings(fields)
	sort.Strings(fields)
	return &ApprovalContextStaleError{Code: ApprovalContextStaleCode, MismatchFields: fields}
}

func BuildPendingApprovalActionToken(approval PendingApproval, checkpointID string) (ActionToken, error) {
	expiresAt := time.Time{}
	if approval.ExpiresAt != nil {
		expiresAt = *approval.ExpiresAt
	} else if !approval.CreatedAt.IsZero() {
		expiresAt = approval.CreatedAt.Add(15 * time.Minute)
	}
	return FreezeActionToken(ActionToken{
		ApprovalID: approval.ID, TurnID: approval.TurnID, ToolCallID: approval.ToolCallID, ToolName: approval.ToolName,
		ArgumentsHash: firstNonEmptyString(approval.ArgumentsHash, approval.InputHash),
		TargetRefs:    approvalActionTokenTargetRefs(approval), ToolSurfaceFingerprint: approval.ToolSurfaceFingerprint,
		PermissionHash: approval.PermissionSnapshotHash, RollbackHash: approvalRollbackContractHash(approval),
		CheckpointID: checkpointID, ExpiresAt: expiresAt,
	})
}

func approvalActionTokenTargetRefs(approval PendingApproval) []string {
	refs := compactStringList(approval.TargetRefs)
	if len(refs) == 0 {
		refs = compactStringList(approval.ResourceScopes)
	}
	if len(refs) == 0 && strings.TrimSpace(approval.ToolName) != "" {
		refs = []string{"tool:" + strings.TrimSpace(approval.ToolName)}
	}
	return refs
}

func approvalRollbackContractHash(approval PendingApproval) string {
	return agentassembly.StableHash("approval-rollback-contract", BuildActionRollbackContractFromApproval(approval).Normalize())
}
