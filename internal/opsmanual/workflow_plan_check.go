package opsmanual

import "context"

type WorkflowPlanCheckRequest struct {
	Manual         OpsManual      `json:"manual"`
	WorkflowID     string         `json:"workflow_id"`
	OperationFrame OperationFrame `json:"operation_frame"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	RequestedBy    string         `json:"requested_by,omitempty"`
	TriggeredBy    string         `json:"triggered_by,omitempty"`
}

type WorkflowPlanCheckResult struct {
	Status           string                 `json:"status"`
	WorkflowDigest   string                 `json:"workflow_digest,omitempty"`
	WorkflowStatus   string                 `json:"workflow_status,omitempty"`
	TargetHosts      []string               `json:"target_hosts,omitempty"`
	ActionsUsed      []string               `json:"actions_used,omitempty"`
	RequiresApproval bool                   `json:"requires_approval,omitempty"`
	RiskLevel        string                 `json:"risk_level,omitempty"`
	Warnings         []PreflightPlanWarning `json:"warnings,omitempty"`
	Summary          string                 `json:"summary,omitempty"`
}

type WorkflowPlanChecker interface {
	CheckWorkflowPlan(context.Context, WorkflowPlanCheckRequest) (WorkflowPlanCheckResult, error)
}

type WorkflowPlanCheckerFunc func(context.Context, WorkflowPlanCheckRequest) (WorkflowPlanCheckResult, error)

func (f WorkflowPlanCheckerFunc) CheckWorkflowPlan(ctx context.Context, req WorkflowPlanCheckRequest) (WorkflowPlanCheckResult, error) {
	return f(ctx, req)
}
