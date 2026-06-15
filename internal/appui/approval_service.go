package appui

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

type defaultApprovalService struct {
	runtime      RuntimeGateway
	sessions     SessionSource
	builder      *SnapshotBuilder
	baseContext  context.Context
	hostCommands HostCommandApprovalGateway
}

type HostCommandApprovalGateway interface {
	ListPending(ctx context.Context) ([]hostops.CommandApproval, error)
	Decide(ctx context.Context, approvalID, decision string) (hostops.CommandApproval, hostops.HostCommandResult, error)
}

type HostCommandApprovalGroupGateway interface {
	ListPendingGroups(ctx context.Context) ([]hostops.CommandApprovalGroup, error)
	DecideGroup(ctx context.Context, groupID, decision string) (hostops.CommandApprovalGroup, []hostops.HostCommandResult, error)
}

type HostCommandApprovalAsyncGateway interface {
	DecideAsync(ctx context.Context, approvalID, decision string) (hostops.CommandApproval, error)
}

type HostCommandApprovalGroupAsyncGateway interface {
	DecideGroupAsync(ctx context.Context, groupID, decision string) (hostops.CommandApprovalGroup, error)
}

func NewApprovalService(runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder) ApprovalService {
	return NewApprovalServiceWithContext(context.Background(), runtime, sessions, builder)
}

func NewApprovalServiceWithContext(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder) ApprovalService {
	return NewApprovalServiceWithHostCommandApprovals(baseContext, runtime, sessions, builder, nil)
}

func NewApprovalServiceWithHostCommandApprovals(baseContext context.Context, runtime RuntimeGateway, sessions SessionSource, builder *SnapshotBuilder, hostCommands HostCommandApprovalGateway) ApprovalService {
	return &defaultApprovalService{
		runtime:      runtime,
		sessions:     sessions,
		builder:      builder,
		baseContext:  normalizeBaseContext(baseContext),
		hostCommands: hostCommands,
	}
}

func (s *defaultApprovalService) List(ctx context.Context) ([]ApprovalView, error) {
	if s.sessions == nil {
		return s.hostCommandApprovalViews(ctx, []ApprovalView{})
	}
	approvals := make([]ApprovalView, 0)
	seen := map[string]struct{}{}
	for _, session := range sortSessionsByActivity(s.sessions.List()) {
		for _, approval := range buildApprovals(session.PendingApprovals) {
			if _, ok := seen[approval.ID]; ok {
				continue
			}
			seen[approval.ID] = struct{}{}
			approvals = append(approvals, approval)
		}
		if session.CurrentTurn != nil {
			for _, approval := range buildApprovals(session.CurrentTurn.PendingApprovals) {
				if _, ok := seen[approval.ID]; ok {
					continue
				}
				seen[approval.ID] = struct{}{}
				approvals = append(approvals, approval)
			}
		}
	}
	return s.hostCommandApprovalViews(ctx, approvals)
}

func (s *defaultApprovalService) Decide(ctx context.Context, decision ApprovalDecision) (ActionResult, error) {
	_, req, err := s.approvalResumeRequest(decision)
	if err != nil {
		if result, hostErr := s.decideHostCommandApproval(ctx, decision); hostErr == nil {
			return result, nil
		}
		return ActionResult{}, err
	}
	if s.runtime == nil {
		return ActionResult{}, fmt.Errorf("runtime is not configured")
	}
	result, err := s.runtime.ResumeTurn(ctx, req)
	if err != nil {
		return ActionResult{}, err
	}
	return ActionResult{
		Status:    result.Status,
		SessionID: result.SessionID,
		TurnID:    result.TurnID,
	}, nil
}

func (s *defaultApprovalService) DecideAsync(_ context.Context, decision ApprovalDecision) (ActionResult, error) {
	session, req, err := s.approvalResumeRequest(decision)
	if err != nil {
		if result, hostErr := s.decideHostCommandApprovalAsync(normalizeBaseContext(s.baseContext), decision); hostErr == nil {
			return result, nil
		}
		if result, hostErr := s.decideHostCommandApproval(normalizeBaseContext(s.baseContext), decision); hostErr == nil {
			return result, nil
		}
		return ActionResult{}, err
	}
	if s.runtime == nil {
		return ActionResult{}, fmt.Errorf("runtime is not configured")
	}
	go s.resumeApprovalDecision(req)
	return ActionResult{
		Status:    "accepted",
		SessionID: session.ID,
		TurnID:    req.TurnID,
	}, nil
}

func (s *defaultApprovalService) resumeApprovalDecision(req runtimekernel.ResumeRequest) {
	if s == nil || s.runtime == nil {
		return
	}
	ctx := normalizeBaseContext(s.baseContext)
	defer func() {
		_ = recover()
	}()
	_, _ = s.runtime.ResumeTurn(ctx, req)
}

func (s *defaultApprovalService) approvalResumeRequest(decision ApprovalDecision) (*runtimekernel.SessionState, runtimekernel.ResumeRequest, error) {
	session, approval, err := findApprovalTargetScoped(s.sessions, decision.SessionID, decision.TurnID, decision.ID)
	if err != nil {
		return nil, runtimekernel.ResumeRequest{}, err
	}
	resumeState := runtimekernel.TurnResumeStatePendingApproval
	if strings.EqualFold(strings.TrimSpace(approval.Source), "pending_evidence") {
		resumeState = runtimekernel.TurnResumeStatePendingEvidence
	}
	return session, runtimekernel.ResumeRequest{
		SessionID:    session.ID,
		TurnID:       firstNonEmpty(strings.TrimSpace(approval.TurnID), currentTurnID(session)),
		ApprovalID:   approval.ID,
		CheckpointID: currentCheckpointID(session),
		ResumeState:  resumeState,
		Decision:     normalizeApprovalDecision(decision.Decision),
	}, nil
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

func (s *defaultApprovalService) hostCommandApprovalViews(ctx context.Context, approvals []ApprovalView) ([]ApprovalView, error) {
	if s == nil || s.hostCommands == nil {
		return approvals, nil
	}
	groupViews, groupedApprovalIDs, err := s.hostCommandApprovalGroupViews(ctx)
	if err != nil {
		return nil, err
	}
	if len(groupViews) > 0 {
		approvals = append(approvals, groupViews...)
	}
	items, err := s.hostCommands.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, approval := range approvals {
		if strings.TrimSpace(approval.ID) != "" {
			seen[approval.ID] = struct{}{}
		}
	}
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		if _, grouped := groupedApprovalIDs[item.ID]; grouped {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		approvals = append(approvals, hostCommandApprovalView(item))
	}
	return approvals, nil
}

func (s *defaultApprovalService) hostCommandApprovalGroupViews(ctx context.Context) ([]ApprovalView, map[string]struct{}, error) {
	groupedApprovalIDs := map[string]struct{}{}
	groupGateway, ok := s.hostCommands.(HostCommandApprovalGroupGateway)
	if !ok {
		return nil, groupedApprovalIDs, nil
	}
	groups, err := groupGateway.ListPendingGroups(ctx)
	if err != nil {
		return nil, groupedApprovalIDs, err
	}
	views := make([]ApprovalView, 0, len(groups))
	for _, group := range groups {
		if group.Total <= 1 || strings.TrimSpace(group.ID) == "" {
			continue
		}
		views = append(views, hostCommandApprovalGroupView(group))
		for _, approvalID := range group.ApprovalIDs {
			if strings.TrimSpace(approvalID) != "" {
				groupedApprovalIDs[approvalID] = struct{}{}
			}
		}
	}
	return views, groupedApprovalIDs, nil
}

func hostCommandApprovalView(item hostops.CommandApproval) ApprovalView {
	return ApprovalView{
		ID:           item.ID,
		MissionID:    item.MissionID,
		ChildAgentID: item.ChildAgentID,
		PlanStepID:   item.PlanStepID,
		GroupID:      item.GroupID,
		GroupSize:    1,
		ToolName:     "host_command",
		Command:      item.Command,
		Reason:       firstNonEmpty(strings.TrimSpace(item.Reason), "host command requires approval"),
		Risk:         string(item.RiskLevel),
		Source:       "host_command_policy",
		HostID:       item.HostID,
		Status:       string(item.Status),
		CreatedAt:    isoStamp(item.CreatedAt),
	}
}

func hostCommandApprovalGroupView(group hostops.CommandApprovalGroup) ApprovalView {
	command := strings.Join(group.Commands, "\n")
	return ApprovalView{
		ID:         group.ID,
		MissionID:  group.MissionID,
		PlanStepID: group.PlanStepID,
		GroupID:    group.ID,
		GroupSize:  group.Total,
		ToolName:   "host_command",
		Command:    command,
		Reason:     fmt.Sprintf("approve %d host commands in the same plan step", group.Total),
		Risk:       string(group.RiskLevel),
		Source:     "host_command_group_policy",
		HostID:     group.HostID,
		Status:     string(group.Status),
		CreatedAt:  isoStamp(group.CreatedAt),
	}
}

func (s *defaultApprovalService) decideHostCommandApproval(ctx context.Context, decision ApprovalDecision) (ActionResult, error) {
	if s == nil || s.hostCommands == nil {
		return ActionResult{}, fmt.Errorf("host command approval service is not configured")
	}
	approval, _, err := s.hostCommands.Decide(ctx, strings.TrimSpace(decision.ID), normalizeApprovalDecision(decision.Decision))
	if err == nil {
		return ActionResult{Status: string(approval.Status)}, nil
	}
	if groupGateway, ok := s.hostCommands.(HostCommandApprovalGroupGateway); ok {
		group, _, groupErr := groupGateway.DecideGroup(ctx, strings.TrimSpace(decision.ID), normalizeApprovalDecision(decision.Decision))
		if groupErr == nil {
			return ActionResult{Status: string(group.Status)}, nil
		}
	}
	return ActionResult{}, err
}

func (s *defaultApprovalService) decideHostCommandApprovalAsync(ctx context.Context, decision ApprovalDecision) (ActionResult, error) {
	if s == nil || s.hostCommands == nil {
		return ActionResult{}, fmt.Errorf("host command approval service is not configured")
	}
	if async, ok := s.hostCommands.(HostCommandApprovalAsyncGateway); ok {
		approval, err := async.DecideAsync(ctx, strings.TrimSpace(decision.ID), normalizeApprovalDecision(decision.Decision))
		if err == nil {
			return ActionResult{Status: string(approval.Status)}, nil
		}
	}
	if groupGateway, ok := s.hostCommands.(HostCommandApprovalGroupAsyncGateway); ok {
		group, err := groupGateway.DecideGroupAsync(ctx, strings.TrimSpace(decision.ID), normalizeApprovalDecision(decision.Decision))
		if err == nil {
			return ActionResult{Status: string(group.Status)}, nil
		}
	}
	return ActionResult{}, fmt.Errorf("host command async approval service is not configured")
}

func currentTurnID(session *runtimekernel.SessionState) string {
	if session == nil || session.CurrentTurn == nil {
		return ""
	}
	return session.CurrentTurn.ID
}

func currentCheckpointID(session *runtimekernel.SessionState) string {
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.LatestCheckpoint == nil {
		return ""
	}
	return session.CurrentTurn.LatestCheckpoint.ID
}
