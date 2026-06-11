package hostops

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/observability"
	"aiops-v2/internal/opssemantic"
)

var ErrCommandApprovalNotFound = errors.New("host command approval not found")

type CommandApprovalStatus string

const (
	CommandApprovalStatusPending  CommandApprovalStatus = "pending"
	CommandApprovalStatusDenied   CommandApprovalStatus = "denied"
	CommandApprovalStatusApproved CommandApprovalStatus = "approved"
	CommandApprovalStatusExecuted CommandApprovalStatus = "executed"
	CommandApprovalStatusFailed   CommandApprovalStatus = "failed"
)

type CommandApproval struct {
	ID             string                   `json:"id"`
	GroupID        string                   `json:"groupId,omitempty"`
	MissionID      string                   `json:"missionId"`
	ChildAgentID   string                   `json:"childAgentId"`
	PlanStepID     string                   `json:"planStepId"`
	HostID         string                   `json:"hostId"`
	HostAddress    string                   `json:"hostAddress,omitempty"`
	Environment    string                   `json:"environment,omitempty"`
	Command        string                   `json:"command"`
	RiskLevel      opssemantic.OpsRiskLevel `json:"riskLevel"`
	Status         CommandApprovalStatus    `json:"status"`
	Decision       string                   `json:"decision,omitempty"`
	Reason         string                   `json:"reason,omitempty"`
	PolicyDecision CommandPolicyDecision    `json:"policyDecision"`
	Result         HostCommandResult        `json:"result,omitempty"`
	ToolContext    ToolContext              `json:"-"`
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
	DecidedAt      *time.Time               `json:"decidedAt,omitempty"`
	ExecutedAt     *time.Time               `json:"executedAt,omitempty"`
}

type CommandApprovalGroup struct {
	ID          string                   `json:"id"`
	MissionID   string                   `json:"missionId"`
	PlanStepID  string                   `json:"planStepId"`
	HostID      string                   `json:"hostId"`
	RiskLevel   opssemantic.OpsRiskLevel `json:"riskLevel"`
	Status      CommandApprovalStatus    `json:"status"`
	Decision    string                   `json:"decision,omitempty"`
	ApprovalIDs []string                 `json:"approvalIds"`
	Commands    []string                 `json:"commands"`
	Total       int                      `json:"total"`
	Pending     int                      `json:"pending"`
	CreatedAt   time.Time                `json:"createdAt"`
	UpdatedAt   time.Time                `json:"updatedAt"`
}

type CommandApprovalRequest struct {
	ToolContext    ToolContext
	MissionID      string
	ChildAgentID   string
	PlanStepID     string
	HostID         string
	HostAddress    string
	Environment    string
	Command        string
	RiskLevel      opssemantic.OpsRiskLevel
	PolicyDecision CommandPolicyDecision
	Reason         string
}

type CommandApprovalStore interface {
	Save(ctx context.Context, approval CommandApproval) error
	Get(ctx context.Context, approvalID string) (CommandApproval, error)
	List(ctx context.Context) ([]CommandApproval, error)
	ListPending(ctx context.Context) ([]CommandApproval, error)
}

type InMemoryCommandApprovalStore struct {
	mu        sync.RWMutex
	approvals map[string]CommandApproval
}

func NewInMemoryCommandApprovalStore() *InMemoryCommandApprovalStore {
	return &InMemoryCommandApprovalStore{approvals: map[string]CommandApproval{}}
}

func (s *InMemoryCommandApprovalStore) Save(_ context.Context, approval CommandApproval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if approval.ID == "" {
		approval.ID = commandApprovalID(approval)
	}
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = now
	}
	approval.UpdatedAt = now
	s.approvals[approval.ID] = cloneCommandApproval(approval)
	return nil
}

func (s *InMemoryCommandApprovalStore) Get(_ context.Context, approvalID string) (CommandApproval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	approval, ok := s.approvals[strings.TrimSpace(approvalID)]
	if !ok {
		return CommandApproval{}, ErrCommandApprovalNotFound
	}
	return cloneCommandApproval(approval), nil
}

func (s *InMemoryCommandApprovalStore) List(_ context.Context) ([]CommandApproval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]CommandApproval, 0, len(s.approvals))
	for _, approval := range s.approvals {
		items = append(items, cloneCommandApproval(approval))
	}
	return items, nil
}

func (s *InMemoryCommandApprovalStore) ListPending(ctx context.Context) ([]CommandApproval, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	pending := make([]CommandApproval, 0, len(items))
	for _, item := range items {
		if item.Status == CommandApprovalStatusPending {
			pending = append(pending, item)
		}
	}
	return pending, nil
}

type CommandApprovalControllerConfig struct {
	Store       CommandApprovalStore
	Missions    MissionStore
	Transcripts TranscriptStore
	Executor    HostCommandExecutor
}

type CommandApprovalController struct {
	store       CommandApprovalStore
	missions    MissionStore
	transcripts TranscriptStore
	executor    HostCommandExecutor
}

func NewCommandApprovalController(cfg CommandApprovalControllerConfig) *CommandApprovalController {
	return &CommandApprovalController{
		store:       cfg.Store,
		missions:    cfg.Missions,
		transcripts: cfg.Transcripts,
		executor:    cfg.Executor,
	}
}

func (c *CommandApprovalController) RequestApproval(ctx context.Context, req CommandApprovalRequest) (CommandApproval, error) {
	if c == nil || c.store == nil {
		return CommandApproval{}, ErrCommandApprovalNotFound
	}
	approval := normalizeCommandApproval(CommandApproval{
		MissionID:      req.MissionID,
		ChildAgentID:   req.ChildAgentID,
		PlanStepID:     req.PlanStepID,
		HostID:         req.HostID,
		HostAddress:    req.HostAddress,
		Environment:    req.Environment,
		Command:        req.Command,
		RiskLevel:      req.RiskLevel,
		Status:         CommandApprovalStatusPending,
		Reason:         firstNonEmptyString(req.Reason, req.PolicyDecision.Reason, "command requires approval"),
		PolicyDecision: req.PolicyDecision,
		ToolContext:    req.ToolContext,
	})
	if approval.ID == "" {
		approval.ID = commandApprovalID(approval)
	}
	if approval.GroupID == "" {
		approval.GroupID = commandApprovalGroupID(approval)
	}
	if err := c.store.Save(ctx, approval); err != nil {
		return CommandApproval{}, err
	}
	c.markWaitingApproval(ctx, approval)
	c.appendApprovalTranscript(ctx, approval, "pending", "command approval requested")
	return c.store.Get(ctx, approval.ID)
}

func (c *CommandApprovalController) DecideGroup(ctx context.Context, groupID, decision string) (CommandApprovalGroup, []HostCommandResult, error) {
	groupID = strings.TrimSpace(groupID)
	if c == nil || c.store == nil || groupID == "" {
		return CommandApprovalGroup{}, nil, ErrCommandApprovalNotFound
	}
	items, err := c.store.List(ctx)
	if err != nil {
		return CommandApprovalGroup{}, nil, err
	}
	targets := make([]CommandApproval, 0)
	for _, item := range items {
		if strings.TrimSpace(item.GroupID) != groupID || item.Status != CommandApprovalStatusPending {
			continue
		}
		targets = append(targets, item)
	}
	if len(targets) == 0 {
		return CommandApprovalGroup{}, nil, ErrCommandApprovalNotFound
	}
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].ID < targets[j].ID
	})
	results := make([]HostCommandResult, 0, len(targets))
	for _, item := range targets {
		_, result, err := c.Decide(ctx, item.ID, decision)
		if err != nil {
			return CommandApprovalGroup{}, results, err
		}
		if normalizeApprovalDecision(decision) == "approved" || normalizeApprovalDecision(decision) == "approved_for_session" {
			results = append(results, result)
		}
	}
	group, err := c.GetGroup(ctx, groupID)
	if err != nil {
		return CommandApprovalGroup{}, results, err
	}
	return group, results, nil
}

func (c *CommandApprovalController) GetGroup(ctx context.Context, groupID string) (CommandApprovalGroup, error) {
	groupID = strings.TrimSpace(groupID)
	if c == nil || c.store == nil || groupID == "" {
		return CommandApprovalGroup{}, ErrCommandApprovalNotFound
	}
	items, err := c.store.List(ctx)
	if err != nil {
		return CommandApprovalGroup{}, err
	}
	groupItems := make([]CommandApproval, 0)
	for _, item := range items {
		if strings.TrimSpace(item.GroupID) == groupID {
			groupItems = append(groupItems, item)
		}
	}
	if len(groupItems) == 0 {
		return CommandApprovalGroup{}, ErrCommandApprovalNotFound
	}
	sort.SliceStable(groupItems, func(i, j int) bool {
		return groupItems[i].ID < groupItems[j].ID
	})
	return commandApprovalGroupFromItems(groupID, groupItems), nil
}

func (c *CommandApprovalController) ListPendingGroups(ctx context.Context) ([]CommandApprovalGroup, error) {
	if c == nil || c.store == nil {
		return []CommandApprovalGroup{}, nil
	}
	items, err := c.store.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	byGroup := map[string][]CommandApproval{}
	for _, item := range items {
		groupID := strings.TrimSpace(item.GroupID)
		if groupID == "" {
			groupID = commandApprovalGroupID(item)
		}
		byGroup[groupID] = append(byGroup[groupID], item)
	}
	groups := make([]CommandApprovalGroup, 0, len(byGroup))
	for groupID, approvals := range byGroup {
		groups = append(groups, commandApprovalGroupFromItems(groupID, approvals))
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].CreatedAt.Equal(groups[j].CreatedAt) {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].CreatedAt.Before(groups[j].CreatedAt)
	})
	return groups, nil
}

func (c *CommandApprovalController) Decide(ctx context.Context, approvalID, decision string) (CommandApproval, HostCommandResult, error) {
	if c == nil || c.store == nil {
		return CommandApproval{}, HostCommandResult{}, ErrCommandApprovalNotFound
	}
	approval, err := c.store.Get(ctx, strings.TrimSpace(approvalID))
	if err != nil {
		return CommandApproval{}, HostCommandResult{}, err
	}
	decision = normalizeApprovalDecision(decision)
	now := time.Now().UTC()
	approval.Decision = decision
	approval.DecidedAt = &now
	if decision != "approved" && decision != "approved_for_session" {
		observability.RecordOpsMetric(observability.OpsMetricCommandApproval, false)
		approval.Status = CommandApprovalStatusDenied
		if err := c.store.Save(ctx, approval); err != nil {
			return CommandApproval{}, HostCommandResult{}, err
		}
		c.markDenied(ctx, approval)
		c.appendApprovalTranscript(ctx, approval, "denied", "command approval denied")
		updated, _ := c.store.Get(ctx, approval.ID)
		return updated, HostCommandResult{}, nil
	}
	approval.Status = CommandApprovalStatusApproved
	observability.RecordOpsMetric(observability.OpsMetricCommandApproval, true)
	if err := c.store.Save(ctx, approval); err != nil {
		return CommandApproval{}, HostCommandResult{}, err
	}
	c.appendApprovalTranscript(ctx, approval, "approved", "command approval approved")
	result, err := c.executeApproved(ctx, approval)
	if err != nil {
		observability.RecordOpsMetric(observability.OpsMetricCommandExecution, false)
		approval.Status = CommandApprovalStatusFailed
		approval.Result = HostCommandResult{Status: "failed", Error: err.Error()}
		_ = c.store.Save(ctx, approval)
		c.markExecutionFailed(ctx, approval, err.Error())
		c.appendApprovalTranscript(ctx, approval, "failed", err.Error())
		return approval, HostCommandResult{}, err
	}
	observability.RecordOpsMetric(observability.OpsMetricCommandExecution, result.Error == "" && result.Status != "failed")
	executedAt := time.Now().UTC()
	approval.Status = CommandApprovalStatusExecuted
	approval.Result = result
	approval.ExecutedAt = &executedAt
	if err := c.store.Save(ctx, approval); err != nil {
		return CommandApproval{}, HostCommandResult{}, err
	}
	c.markApprovedExecuted(ctx, approval, result)
	c.appendCommandResultTranscript(ctx, approval, result)
	updated, _ := c.store.Get(ctx, approval.ID)
	return updated, result, nil
}

func (c *CommandApprovalController) ListPending(ctx context.Context) ([]CommandApproval, error) {
	if c == nil || c.store == nil {
		return []CommandApproval{}, nil
	}
	return c.store.ListPending(ctx)
}

func (c *CommandApprovalController) executeApproved(ctx context.Context, approval CommandApproval) (HostCommandResult, error) {
	if c == nil || c.executor == nil {
		return HostCommandResult{}, errors.New("host command executor is unavailable")
	}
	toolCtx := approval.ToolContext
	if toolCtx.AgentKind == "" {
		toolCtx = ToolContext{AgentKind: AgentKindHostChild, BoundHostID: approval.HostID}
	}
	return c.executor.RunShell(ctx, toolCtx, HostCommandRequest{
		HostID:      approval.HostID,
		HostAddress: approval.HostAddress,
		Script:      approval.Command,
	})
}

func (c *CommandApprovalController) markWaitingApproval(ctx context.Context, approval CommandApproval) {
	if c == nil || c.missions == nil {
		return
	}
	child, err := c.missions.GetChildAgent(ctx, approval.ChildAgentID)
	if err == nil {
		child.Status = HostChildAgentStatusApprovalRequired
		child.Error = ""
		_ = c.missions.SaveChildAgent(ctx, child)
	}
	mission, err := c.missions.GetMission(ctx, approval.MissionID)
	if err != nil {
		return
	}
	mission.Status = HostMissionStatusWaitingApproval
	mission.Plan = updatePlanStepForApproval(mission.Plan, approval.PlanStepID, PlanStepStatusBlocked, true)
	_ = c.missions.SaveMission(ctx, mission)
}

func (c *CommandApprovalController) markDenied(ctx context.Context, approval CommandApproval) {
	if c == nil || c.missions == nil {
		return
	}
	child, err := c.missions.GetChildAgent(ctx, approval.ChildAgentID)
	if err == nil {
		child.Status = HostChildAgentStatusBlocked
		child.Error = "command approval denied"
		_ = c.missions.SaveChildAgent(ctx, child)
	}
}

func (c *CommandApprovalController) markExecutionFailed(ctx context.Context, approval CommandApproval, errorText string) {
	if c == nil || c.missions == nil {
		return
	}
	child, err := c.missions.GetChildAgent(ctx, approval.ChildAgentID)
	if err == nil {
		child.Status = HostChildAgentStatusFailed
		child.Error = errorText
		_ = c.missions.SaveChildAgent(ctx, child)
	}
}

func (c *CommandApprovalController) markApprovedExecuted(ctx context.Context, approval CommandApproval, result HostCommandResult) {
	if c == nil || c.missions == nil {
		return
	}
	child, err := c.missions.GetChildAgent(ctx, approval.ChildAgentID)
	if err == nil {
		child.Status = HostChildAgentStatusRunning
		child.Error = ""
		child.LastOutputPreview = firstNonEmptyString(result.Stdout, result.Stderr, result.Error, result.Status)
		_ = c.missions.SaveChildAgent(ctx, child)
	}
	mission, err := c.missions.GetMission(ctx, approval.MissionID)
	if err != nil {
		return
	}
	mission.Status = HostMissionStatusRunning
	mission.Plan = updatePlanStepForApproval(mission.Plan, approval.PlanStepID, PlanStepStatusRunning, true)
	_ = c.missions.SaveMission(ctx, mission)
}

func updatePlanStepForApproval(plan HostOperationPlan, stepID string, status PlanStepStatus, approvalRequired bool) HostOperationPlan {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return plan
	}
	for i := range plan.Steps {
		if strings.TrimSpace(plan.Steps[i].ID) != stepID {
			continue
		}
		plan.Steps[i].Status = status
		plan.Steps[i].ApprovalRequired = approvalRequired
	}
	return plan
}

func (c *CommandApprovalController) appendApprovalTranscript(ctx context.Context, approval CommandApproval, status, content string) {
	if c == nil || c.transcripts == nil || strings.TrimSpace(approval.ChildAgentID) == "" {
		return
	}
	_ = c.transcripts.Append(ctx, approval.ChildAgentID, TranscriptItem{
		Type:       TranscriptItemApproval,
		ApprovalID: approval.ID,
		Status:     strings.TrimSpace(status),
		Content:    strings.TrimSpace(content),
		Payload: map[string]any{
			"missionId":    approval.MissionID,
			"childAgentId": approval.ChildAgentID,
			"planStepId":   approval.PlanStepID,
			"hostId":       approval.HostID,
			"command":      approval.Command,
			"risk":         string(approval.RiskLevel),
		},
	})
}

func (c *CommandApprovalController) appendCommandResultTranscript(ctx context.Context, approval CommandApproval, result HostCommandResult) {
	if c == nil || c.transcripts == nil || strings.TrimSpace(approval.ChildAgentID) == "" {
		return
	}
	_ = c.transcripts.Append(ctx, approval.ChildAgentID, TranscriptItem{
		Type:     TranscriptItemToolResult,
		ToolName: "host_command",
		Status:   firstNonEmptyString(result.Status, "completed"),
		Content:  firstNonEmptyString(result.Stdout, result.Stderr, result.Error),
		Payload: map[string]any{
			"missionId":    approval.MissionID,
			"childAgentId": approval.ChildAgentID,
			"planStepId":   approval.PlanStepID,
			"hostId":       approval.HostID,
			"command":      approval.Command,
			"exitCode":     result.ExitCode,
		},
	})
}

func normalizeCommandApproval(approval CommandApproval) CommandApproval {
	approval.GroupID = strings.TrimSpace(approval.GroupID)
	approval.MissionID = strings.TrimSpace(approval.MissionID)
	approval.ChildAgentID = strings.TrimSpace(approval.ChildAgentID)
	approval.PlanStepID = strings.TrimSpace(approval.PlanStepID)
	approval.HostID = strings.TrimSpace(approval.HostID)
	approval.HostAddress = strings.TrimSpace(approval.HostAddress)
	approval.Environment = strings.TrimSpace(approval.Environment)
	approval.Command = normalizeCommand(approval.Command)
	approval.Reason = strings.TrimSpace(approval.Reason)
	if approval.RiskLevel == "" {
		approval.RiskLevel = opssemantic.ClassifyRisk(approval.Command)
	}
	if approval.Status == "" {
		approval.Status = CommandApprovalStatusPending
	}
	if approval.GroupID == "" {
		approval.GroupID = commandApprovalGroupID(approval)
	}
	return approval
}

func commandApprovalID(approval CommandApproval) string {
	approval = normalizeCommandApproval(approval)
	h := sha1.New()
	for _, value := range []string{
		approval.MissionID,
		approval.ChildAgentID,
		approval.PlanStepID,
		approval.HostID,
		approval.Command,
		string(approval.RiskLevel),
	} {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return "hostcmd-approval-" + hex.EncodeToString(h.Sum(nil))[:16]
}

func commandApprovalGroupID(approval CommandApproval) string {
	approval = normalizeCommandApprovalForGroupID(approval)
	h := sha1.New()
	for _, value := range []string{
		approval.MissionID,
		approval.PlanStepID,
		approval.HostID,
		string(approval.RiskLevel),
	} {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return "hostcmd-group-" + hex.EncodeToString(h.Sum(nil))[:16]
}

func normalizeCommandApprovalForGroupID(approval CommandApproval) CommandApproval {
	approval.MissionID = strings.TrimSpace(approval.MissionID)
	approval.PlanStepID = strings.TrimSpace(approval.PlanStepID)
	approval.HostID = strings.TrimSpace(approval.HostID)
	if approval.RiskLevel == "" {
		approval.RiskLevel = opssemantic.ClassifyRisk(approval.Command)
	}
	return approval
}

func commandApprovalGroupFromItems(groupID string, items []CommandApproval) CommandApprovalGroup {
	group := CommandApprovalGroup{
		ID:        strings.TrimSpace(groupID),
		Status:    CommandApprovalStatusExecuted,
		Total:     len(items),
		CreatedAt: time.Now().UTC(),
	}
	if len(items) == 0 {
		group.Status = CommandApprovalStatusPending
		return group
	}
	for i, item := range items {
		if i == 0 {
			group.MissionID = item.MissionID
			group.PlanStepID = item.PlanStepID
			group.HostID = item.HostID
			group.RiskLevel = item.RiskLevel
			group.CreatedAt = item.CreatedAt
			group.UpdatedAt = item.UpdatedAt
			group.Decision = item.Decision
		}
		if !item.CreatedAt.IsZero() && item.CreatedAt.Before(group.CreatedAt) {
			group.CreatedAt = item.CreatedAt
		}
		if item.UpdatedAt.After(group.UpdatedAt) {
			group.UpdatedAt = item.UpdatedAt
		}
		if item.Decision != "" && group.Decision == "" {
			group.Decision = item.Decision
		}
		group.ApprovalIDs = append(group.ApprovalIDs, item.ID)
		group.Commands = append(group.Commands, item.Command)
		if item.Status == CommandApprovalStatusPending {
			group.Pending++
		}
		switch item.Status {
		case CommandApprovalStatusPending:
			group.Status = CommandApprovalStatusPending
		case CommandApprovalStatusFailed:
			if group.Status != CommandApprovalStatusPending {
				group.Status = CommandApprovalStatusFailed
			}
		case CommandApprovalStatusDenied:
			if group.Status != CommandApprovalStatusPending && group.Status != CommandApprovalStatusFailed {
				group.Status = CommandApprovalStatusDenied
			}
		}
	}
	return group
}

func cloneCommandApproval(approval CommandApproval) CommandApproval {
	if approval.DecidedAt != nil {
		decidedAt := *approval.DecidedAt
		approval.DecidedAt = &decidedAt
	}
	if approval.ExecutedAt != nil {
		executedAt := *approval.ExecutedAt
		approval.ExecutedAt = &executedAt
	}
	approval.Result.Output = cloneAnyMap(approval.Result.Output)
	return approval
}

func normalizeApprovalDecision(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accept_session", "approved_for_session":
		return "approved_for_session"
	case "accept", "approve", "approved", "allow", "yes":
		return "approved"
	default:
		return "denied"
	}
}
