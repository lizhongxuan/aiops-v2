package hostops

import (
	"context"
	"errors"
	"strings"

	"aiops-v2/internal/opssemantic"
)

var ErrInvalidHostCommandRequest = errors.New("invalid host command tool request")

type HostCommandExecutor interface {
	RunShell(ctx context.Context, toolCtx ToolContext, req HostCommandRequest) (HostCommandResult, error)
}

type HostCommandTool struct {
	executor  HostCommandExecutor
	policy    *CommandPolicy
	approvals *CommandApprovalController
}

func NewHostCommandTool(executor HostCommandExecutor, policy *CommandPolicy) *HostCommandTool {
	return &HostCommandTool{executor: executor, policy: policy}
}

func NewHostCommandToolWithApprovals(executor HostCommandExecutor, policy *CommandPolicy, approvals *CommandApprovalController) *HostCommandTool {
	return &HostCommandTool{executor: executor, policy: policy, approvals: approvals}
}

type HostCommandToolRequest struct {
	ToolContext  ToolContext
	MissionID    string
	ChildAgentID string
	PlanStepID   string
	HostID       string
	HostAddress  string
	Environment  string
	Command      string
	RiskLevel    opssemantic.OpsRiskLevel
}

type HostCommandToolResult struct {
	Executed         bool
	ApprovalRequired bool
	ApprovalID       string
	MissionID        string
	ChildAgentID     string
	PlanStepID       string
	HostID           string
	Command          string
	PolicyDecision   CommandPolicyDecision
	CommandResult    HostCommandResult
}

func (t *HostCommandTool) Run(ctx context.Context, req HostCommandToolRequest) (HostCommandToolResult, error) {
	req = normalizeHostCommandToolRequest(req)
	if err := validateHostCommandToolRequest(req); err != nil {
		return HostCommandToolResult{}, err
	}
	if req.ToolContext.AgentKind != AgentKindHostChild {
		t.auditSecurityRefusal(ctx, req, ErrManagerDirectHostDenied)
		return HostCommandToolResult{}, ErrManagerDirectHostDenied
	}
	if err := EnforceHostBinding(req.ToolContext, req.HostID); err != nil {
		t.auditSecurityRefusal(ctx, req, err)
		return HostCommandToolResult{}, err
	}
	policy := t.policy
	if policy == nil {
		policy = NewCommandPolicy(CommandPolicyConfig{})
	}
	decision := policy.Evaluate(CommandPolicyContext{
		MissionID:    req.MissionID,
		ChildAgentID: req.ChildAgentID,
		PlanStepID:   req.PlanStepID,
		HostID:       req.HostID,
		Environment:  req.Environment,
		Command:      req.Command,
		RiskLevel:    req.RiskLevel,
	})
	result := HostCommandToolResult{
		MissionID:        req.MissionID,
		ChildAgentID:     req.ChildAgentID,
		PlanStepID:       req.PlanStepID,
		HostID:           req.HostID,
		Command:          req.Command,
		PolicyDecision:   decision,
		ApprovalRequired: decision.RequiresApproval,
	}
	if !decision.Allowed || decision.RequiresApproval {
		if t != nil && t.approvals != nil {
			approval, err := t.approvals.RequestApproval(ctx, CommandApprovalRequest{
				ToolContext:    req.ToolContext,
				MissionID:      req.MissionID,
				ChildAgentID:   req.ChildAgentID,
				PlanStepID:     req.PlanStepID,
				HostID:         req.HostID,
				HostAddress:    req.HostAddress,
				Environment:    req.Environment,
				Command:        req.Command,
				RiskLevel:      req.RiskLevel,
				PolicyDecision: decision,
			})
			if err != nil {
				return HostCommandToolResult{}, err
			}
			result.ApprovalID = approval.ID
		}
		return result, nil
	}
	if t == nil || t.executor == nil {
		return HostCommandToolResult{}, errors.New("host command executor is unavailable")
	}
	commandResult, err := t.executor.RunShell(ctx, req.ToolContext, HostCommandRequest{
		HostID:      req.HostID,
		HostAddress: req.HostAddress,
		Script:      req.Command,
	})
	if err != nil {
		return HostCommandToolResult{}, err
	}
	result.Executed = true
	result.ApprovalRequired = false
	result.CommandResult = commandResult
	return result, nil
}

func (t *HostCommandTool) auditSecurityRefusal(ctx context.Context, req HostCommandToolRequest, cause error) {
	if t == nil || t.approvals == nil || t.approvals.transcripts == nil || strings.TrimSpace(req.ChildAgentID) == "" {
		return
	}
	_ = t.approvals.transcripts.Append(ctx, req.ChildAgentID, TranscriptItem{
		Type:     TranscriptItemError,
		ToolName: "host_command",
		Status:   "security_refused",
		Content:  cause.Error(),
		Payload: map[string]any{
			"missionId":    req.MissionID,
			"childAgentId": req.ChildAgentID,
			"planStepId":   req.PlanStepID,
			"hostId":       req.HostID,
			"command":      req.Command,
		},
	})
}

func normalizeHostCommandToolRequest(req HostCommandToolRequest) HostCommandToolRequest {
	req.MissionID = strings.TrimSpace(req.MissionID)
	req.ChildAgentID = strings.TrimSpace(req.ChildAgentID)
	req.PlanStepID = strings.TrimSpace(req.PlanStepID)
	req.HostID = strings.TrimSpace(req.HostID)
	req.HostAddress = strings.TrimSpace(req.HostAddress)
	req.Environment = strings.TrimSpace(req.Environment)
	req.Command = normalizeCommand(req.Command)
	if req.RiskLevel == "" {
		req.RiskLevel = opssemantic.ClassifyRisk(req.Command)
	}
	return req
}

func validateHostCommandToolRequest(req HostCommandToolRequest) error {
	if req.MissionID == "" ||
		req.ChildAgentID == "" ||
		req.PlanStepID == "" ||
		req.HostID == "" ||
		req.Command == "" {
		return ErrInvalidHostCommandRequest
	}
	return nil
}
