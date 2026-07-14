package runtimekernel

import (
	"fmt"
	"strings"
)

const ActionRollbackContractSchemaVersion = "aiops.rollback.action.v1"

type ActionRollbackContract struct {
	SchemaVersion          string   `json:"schemaVersion"`
	ActionID               string   `json:"actionId,omitempty"`
	ToolName               string   `json:"toolName,omitempty"`
	TargetRefs             []string `json:"targetRefs,omitempty"`
	InputHash              string   `json:"inputHash,omitempty"`
	Risk                   string   `json:"risk,omitempty"`
	ExpectedEffect         string   `json:"expectedEffect,omitempty"`
	PreChangeEvidenceRefs  []string `json:"preChangeEvidenceRefs,omitempty"`
	ApprovalScope          string   `json:"approvalScope,omitempty"`
	ResourceScopes         []string `json:"resourceScopes,omitempty"`
	Rollback               string   `json:"rollback,omitempty"`
	Validation             string   `json:"validation,omitempty"`
	PostCheck              string   `json:"postCheck,omitempty"`
	StopCondition          string   `json:"stopCondition,omitempty"`
	IdempotencyKey         string   `json:"idempotencyKey,omitempty"`
	ToolSurfaceFingerprint string   `json:"toolSurfaceFingerprint,omitempty"`
	PermissionSnapshotHash string   `json:"permissionSnapshotHash,omitempty"`
	ManualTakeover         string   `json:"manualTakeover,omitempty"`
}

func (c ActionRollbackContract) Normalize() ActionRollbackContract {
	c.SchemaVersion = firstNonEmptyString(c.SchemaVersion, ActionRollbackContractSchemaVersion)
	c.ActionID = strings.TrimSpace(c.ActionID)
	c.ToolName = strings.TrimSpace(c.ToolName)
	c.TargetRefs = compactStringList(c.TargetRefs)
	c.InputHash = strings.TrimSpace(c.InputHash)
	c.Risk = strings.TrimSpace(c.Risk)
	c.ExpectedEffect = strings.TrimSpace(c.ExpectedEffect)
	c.PreChangeEvidenceRefs = compactStringList(c.PreChangeEvidenceRefs)
	c.ApprovalScope = strings.TrimSpace(c.ApprovalScope)
	c.ResourceScopes = compactStringList(c.ResourceScopes)
	c.Rollback = strings.TrimSpace(c.Rollback)
	c.Validation = strings.TrimSpace(c.Validation)
	c.PostCheck = strings.TrimSpace(c.PostCheck)
	c.StopCondition = strings.TrimSpace(c.StopCondition)
	c.IdempotencyKey = strings.TrimSpace(c.IdempotencyKey)
	c.ToolSurfaceFingerprint = strings.TrimSpace(c.ToolSurfaceFingerprint)
	c.PermissionSnapshotHash = strings.TrimSpace(c.PermissionSnapshotHash)
	c.ManualTakeover = strings.TrimSpace(c.ManualTakeover)
	return c
}

func (c ActionRollbackContract) ValidateMutating() error {
	c = c.Normalize()
	missing := make([]string, 0, 8)
	if len(c.TargetRefs) == 0 {
		missing = append(missing, "targetRefs")
	}
	if c.ExpectedEffect == "" {
		missing = append(missing, "expectedEffect")
	}
	if len(c.PreChangeEvidenceRefs) == 0 {
		missing = append(missing, "preChangeEvidenceRefs")
	}
	if c.Validation == "" {
		missing = append(missing, "validation")
	}
	if c.Rollback == "" && c.ManualTakeover == "" {
		missing = append(missing, "rollback")
	}
	if c.InputHash == "" {
		missing = append(missing, "inputHash")
	}
	if c.ToolSurfaceFingerprint == "" {
		missing = append(missing, "toolSurfaceFingerprint")
	}
	if c.PermissionSnapshotHash == "" {
		missing = append(missing, "permissionSnapshotHash")
	}
	if len(missing) > 0 {
		return fmt.Errorf("rollback_contract_invalid: missing %s", strings.Join(missing, ", "))
	}
	return nil
}

func BuildActionRollbackContractFromApproval(approval PendingApproval) ActionRollbackContract {
	contract := approval.RollbackContract.Normalize()
	contract.SchemaVersion = firstNonEmptyString(contract.SchemaVersion, ActionRollbackContractSchemaVersion)
	contract.ActionID = firstNonEmptyString(contract.ActionID, approval.ID, approval.ToolCallID)
	contract.ToolName = firstNonEmptyString(contract.ToolName, approval.ToolName)
	contract.TargetRefs = firstNonEmptyStringSlice(contract.TargetRefs, approval.TargetRefs)
	contract.InputHash = firstNonEmptyString(contract.InputHash, approval.InputHash, approval.ArgumentsHash)
	contract.Risk = firstNonEmptyString(contract.Risk, approval.Risk)
	contract.ExpectedEffect = firstNonEmptyString(contract.ExpectedEffect, approval.ExpectedEffect)
	contract.PreChangeEvidenceRefs = firstNonEmptyStringSlice(contract.PreChangeEvidenceRefs, approval.PreChangeEvidenceRefs)
	contract.ApprovalScope = firstNonEmptyString(contract.ApprovalScope, approval.ApprovalScope, approval.RequestedScope)
	contract.ResourceScopes = firstNonEmptyStringSlice(contract.ResourceScopes, approval.ResourceScopes, approval.TargetRefs)
	contract.Rollback = firstNonEmptyString(contract.Rollback, approval.Rollback)
	contract.Validation = firstNonEmptyString(contract.Validation, approval.Validation)
	contract.PostCheck = firstNonEmptyString(contract.PostCheck, approval.PostCheck)
	contract.StopCondition = firstNonEmptyString(contract.StopCondition, approval.StopCondition)
	contract.IdempotencyKey = firstNonEmptyString(contract.IdempotencyKey, approval.IdempotencyKey, approval.InputHash, approval.ArgumentsHash)
	contract.ToolSurfaceFingerprint = firstNonEmptyString(contract.ToolSurfaceFingerprint, approval.ToolSurfaceFingerprint)
	contract.PermissionSnapshotHash = firstNonEmptyString(contract.PermissionSnapshotHash, approval.PermissionSnapshotHash)
	contract.ManualTakeover = firstNonEmptyString(contract.ManualTakeover, approval.ManualTakeover)
	return contract.Normalize()
}

func ValidatePendingApprovalRollbackContract(approval PendingApproval) error {
	if !approval.Mutating {
		return nil
	}
	return BuildActionRollbackContractFromApproval(approval).ValidateMutating()
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if compact := compactStringList(value); len(compact) > 0 {
			return compact
		}
	}
	return nil
}
