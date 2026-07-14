package appui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/opssemantic"
	"aiops-v2/internal/resourcebinding"
)

var ErrHostOpsInvalidMission = errors.New("invalid host operation mission")

type defaultHostOpsService struct {
	missions     hostops.MissionStore
	transcripts  hostops.TranscriptStore
	orchestrator *hostops.Orchestrator
}

func NewHostOpsService(missions hostops.MissionStore, transcripts hostops.TranscriptStore, orchestrator *hostops.Orchestrator) HostOpsService {
	return &defaultHostOpsService{
		missions:     missions,
		transcripts:  transcripts,
		orchestrator: orchestrator,
	}
}

func (s *defaultHostOpsService) CreateMission(ctx context.Context, command HostMissionCreateCommand) (HostOperationView, error) {
	if s == nil || s.missions == nil {
		return HostOperationView{}, hostops.ErrMissionNotFound
	}
	goal := strings.TrimSpace(command.Goal)
	mentions := normalizeMissionMentions(command)
	if goal == "" && len(mentions) == 0 {
		return HostOperationView{}, ErrHostOpsInvalidMission
	}
	semantic := opssemantic.ParseTask(opssemantic.ParseInput{
		ID:        strings.TrimSpace(command.ID),
		SessionID: strings.TrimSpace(command.SessionID),
		TurnID:    strings.TrimSpace(command.UserTurnID),
		Text:      goal,
	})
	semantic.HostScope = semanticHostScopeFromMentions(mentions)
	if len(semantic.HostScope) > 0 {
		semantic.MissingSlots = nil
		semantic.PlanRequired = len(semantic.HostScope) > 1 || semantic.ExecutionPolicy.RequiresApproval
		semantic.ExecutionPolicy.AllowParallel = len(semantic.HostScope) > 1
	}
	hostCount := len(hostops.UniqueMentionKeys(mentions))
	planRequired := hostCount > 1 || semantic.PlanRequired
	status := hostops.HostMissionStatusPlanning
	if planRequired {
		status = hostops.HostMissionStatusWaitingPlanAcceptance
	}
	mission := hostops.HostOperationMission{
		ID:                           firstNonEmptyHostOpsString(command.ID, deterministicMissionID(command, mentions)),
		SessionID:                    strings.TrimSpace(command.SessionID),
		ThreadID:                     firstNonEmptyHostOpsString(command.ThreadID, command.SessionID),
		UserTurnID:                   strings.TrimSpace(command.UserTurnID),
		ManagerAgentID:               strings.TrimSpace(command.ManagerAgentID),
		Status:                       status,
		SemanticTask:                 semantic,
		PlanRequired:                 planRequired,
		PlanAccepted:                 false,
		Mentions:                     mentions,
		RoleBindings:                 append([]resourcebinding.ResourceRoleBinding(nil), command.RoleBindings...),
		RoleConflicts:                append([]resourcebinding.RoleBindingConflict(nil), command.RoleConflicts...),
		RoleBindingAssignmentEnabled: command.RoleBindingAssignmentEnabled,
	}
	if hostCount > 0 && planRequired {
		plan, err := hostops.BuildPlanForMission(mission)
		if err != nil {
			return HostOperationView{}, err
		}
		mission.Plan = plan
	}
	if err := s.missions.SaveMission(ctx, mission); err != nil {
		return HostOperationView{}, err
	}
	s.appendMissionAudit(ctx, mission.ID, hostops.TranscriptItem{
		Type:    hostops.TranscriptItemManagerMessage,
		Content: "host operation mission created",
		Status:  string(mission.Status),
		Payload: map[string]any{
			"missionId":    mission.ID,
			"hostCount":    hostCount,
			"planRequired": mission.PlanRequired,
		},
	})
	if !mission.PlanRequired && hostCount > 0 && s.orchestrator != nil {
		if _, err := s.orchestrator.SpawnChildren(ctx, mission.ID, autoSpawnAssignmentsForMission(mission, goal)); err != nil {
			return HostOperationView{}, err
		}
		if latest, err := s.missions.GetMission(ctx, mission.ID); err == nil {
			latest.Status = hostops.HostMissionStatusRunning
			if err := s.missions.SaveMission(ctx, latest); err != nil {
				return HostOperationView{}, err
			}
		}
	}
	return s.operationView(ctx, mission.ID)
}

func (s *defaultHostOpsService) GetMission(ctx context.Context, missionID string) (HostOperationView, error) {
	return s.operationView(ctx, strings.TrimSpace(missionID))
}

func (s *defaultHostOpsService) AcceptPlan(ctx context.Context, missionID, planID string) (HostOperationView, error) {
	missionID = strings.TrimSpace(missionID)
	if s == nil || s.orchestrator == nil {
		return HostOperationView{}, hostops.ErrMissionNotFound
	}
	if err := s.orchestrator.AcceptPlan(ctx, missionID, strings.TrimSpace(planID)); err != nil {
		return HostOperationView{}, err
	}
	return s.operationView(ctx, missionID)
}

func (s *defaultHostOpsService) RevisePlan(ctx context.Context, missionID, instruction string) (HostOperationView, error) {
	missionID = strings.TrimSpace(missionID)
	if s == nil || s.missions == nil {
		return HostOperationView{}, hostops.ErrMissionNotFound
	}
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return HostOperationView{}, err
	}
	if s.orchestrator != nil && mission.Plan.ID != "" {
		affected := make([]string, 0, len(mission.Plan.Steps))
		for _, step := range mission.Plan.Steps {
			affected = append(affected, step.HostIDs...)
		}
		mission, err = s.orchestrator.RevisePlan(ctx, mission.ID, hostops.PlanRevisionRequest{
			Reason:          firstNonEmptyHostOpsString(instruction, "plan revision requested"),
			AffectedHostIDs: affected,
			Steps:           mission.Plan.Steps,
		})
		if err != nil {
			return HostOperationView{}, err
		}
	}
	mission.PlanAccepted = false
	mission.Status = hostops.HostMissionStatusWaitingPlanAcceptance
	if mission.Plan.ID != "" {
		mission.Plan.Status = hostops.PlanStatusWaitingAcceptance
		mission.Plan.AcceptedAt = nil
		mission.Plan.AcceptedBy = ""
	}
	if err := s.missions.SaveMission(ctx, mission); err != nil {
		return HostOperationView{}, err
	}
	return s.operationView(ctx, mission.ID)
}

func (s *defaultHostOpsService) SendChildMessage(ctx context.Context, childAgentID, content string) (HostChildAgentView, error) {
	if s == nil || s.orchestrator == nil {
		return HostChildAgentView{}, hostops.ErrChildSpawnerUnavailable
	}
	child, err := s.orchestrator.SendMessage(ctx, childAgentID, content)
	if err != nil {
		return HostChildAgentView{}, err
	}
	return childAgentView(child), nil
}

func (s *defaultHostOpsService) StopChildAgent(ctx context.Context, childAgentID string) (HostChildAgentView, error) {
	if s == nil || s.orchestrator == nil {
		return HostChildAgentView{}, hostops.ErrChildSpawnerUnavailable
	}
	child, err := s.orchestrator.Stop(ctx, childAgentID)
	if err != nil {
		return HostChildAgentView{}, err
	}
	return childAgentView(child), nil
}

func (s *defaultHostOpsService) ChildTranscript(ctx context.Context, childAgentID string) (HostChildTranscriptView, error) {
	childAgentID = strings.TrimSpace(childAgentID)
	if s == nil || s.transcripts == nil {
		return HostChildTranscriptView{}, hostops.ErrChildAgentNotFound
	}
	items, err := s.transcripts.List(ctx, childAgentID)
	if err != nil {
		return HostChildTranscriptView{}, err
	}
	return HostChildTranscriptView{ChildAgentID: childAgentID, Items: items}, nil
}

func (s *defaultHostOpsService) operationView(ctx context.Context, missionID string) (HostOperationView, error) {
	if s == nil || s.missions == nil {
		return HostOperationView{}, hostops.ErrMissionNotFound
	}
	mission, err := s.missions.GetMission(ctx, missionID)
	if err != nil {
		return HostOperationView{}, err
	}
	children, _ := s.missions.ListChildAgents(ctx, mission.ID)
	childViews := make([]HostChildAgentView, 0, len(children))
	for _, child := range children {
		childViews = append(childViews, childAgentView(child))
	}
	return HostOperationView{
		ID:             mission.ID,
		ThreadID:       mission.ThreadID,
		UserTurnID:     mission.UserTurnID,
		ManagerAgentID: mission.ManagerAgentID,
		Status:         string(mission.Status),
		PlanRequired:   mission.PlanRequired,
		PlanAccepted:   mission.PlanAccepted,
		MentionedHosts: mentionViews(mission.Mentions),
		Plan:           planView(mission.Plan),
		ChildAgents:    childViews,
		CreatedAt:      formatHostOpsTime(mission.CreatedAt),
		UpdatedAt:      formatHostOpsTime(mission.UpdatedAt),
	}, nil
}

func childAgentView(child hostops.HostChildAgent) HostChildAgentView {
	view := HostChildAgentView{
		ID:                child.ID,
		MissionID:         child.MissionID,
		ParentAgentID:     child.ParentAgentID,
		SessionID:         child.SessionID,
		HostID:            child.HostID,
		HostAddress:       child.HostAddress,
		Role:              child.Role,
		Task:              child.Task,
		Status:            string(child.Status),
		PlanStepIDs:       append([]string(nil), child.PlanStepIDs...),
		LastInputPreview:  child.LastInputPreview,
		LastOutputPreview: child.LastOutputPreview,
		Error:             child.Error,
		StartedAt:         formatHostOpsTime(child.StartedAt),
		UpdatedAt:         formatHostOpsTime(child.UpdatedAt),
	}
	if child.CompletedAt != nil {
		view.CompletedAt = formatHostOpsTime(*child.CompletedAt)
	}
	return view
}

func normalizeMissionMentions(command HostMissionCreateCommand) []hostops.HostMention {
	mentions := append([]hostops.HostMention(nil), command.Mentions...)
	if len(mentions) == 0 && strings.TrimSpace(command.Goal) != "" {
		mentions = hostops.ParseHostMentions(command.Goal)
	}
	for _, hostID := range command.HostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID == "" {
			continue
		}
		mentions = append(mentions, hostops.HostMention{
			Raw:         "@" + hostID,
			HostID:      hostID,
			DisplayName: hostID,
			Source:      hostops.HostMentionSourceInventory,
			Resolved:    true,
			Confidence:  1,
			CreatedAt:   time.Now().UTC(),
		})
	}
	out := make([]hostops.HostMention, 0, len(mentions))
	seen := map[string]bool{}
	for _, mention := range mentions {
		mention.Raw = strings.TrimSpace(mention.Raw)
		mention.HostID = strings.TrimSpace(mention.HostID)
		mention.Address = strings.TrimSpace(mention.Address)
		mention.DisplayName = strings.TrimSpace(mention.DisplayName)
		if mention.Raw == "" && firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName) != "" {
			mention.Raw = "@" + firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName)
		}
		key := strings.ToLower(firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@")))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, mention)
	}
	return out
}

func autoSpawnAssignmentsForMission(mission hostops.HostOperationMission, goal string) []hostops.ChildAgentAssignment {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		goal = "Operate on the assigned host and report evidence to the manager."
	}
	assignments := make([]hostops.ChildAgentAssignment, 0, len(mission.Mentions))
	seen := map[string]struct{}{}
	for _, mention := range mission.Mentions {
		hostID := firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@"))
		key := strings.ToLower(strings.TrimSpace(hostID))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		assignments = append(assignments, hostops.ChildAgentAssignment{
			HostID:          hostID,
			HostAddress:     strings.TrimSpace(mention.Address),
			HostDisplayName: firstNonEmptyHostOpsString(mention.DisplayName, strings.TrimPrefix(mention.Raw, "@"), hostID),
			Task:            goal,
			RiskLevel:       mission.SemanticTask.RiskLevel,
		})
	}
	return assignments
}

func mentionViews(mentions []hostops.HostMention) []HostMentionView {
	if len(mentions) == 0 {
		return nil
	}
	views := make([]HostMentionView, 0, len(mentions))
	for _, mention := range mentions {
		views = append(views, HostMentionView{
			Raw:         mention.Raw,
			HostID:      mention.HostID,
			Address:     mention.Address,
			DisplayName: mention.DisplayName,
			Source:      string(mention.Source),
			Resolved:    mention.Resolved,
		})
	}
	return views
}

func planView(plan hostops.HostOperationPlan) *HostPlanView {
	if plan.ID == "" && len(plan.Steps) == 0 {
		return nil
	}
	completed, total := hostops.PlanProgress(plan)
	steps := make([]HostPlanStepView, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, HostPlanStepView{
			ID:               step.ID,
			Index:            step.Index,
			Title:            step.Title,
			Summary:          step.Summary,
			Status:           string(step.Status),
			HostIDs:          append([]string(nil), step.HostIDs...),
			ChildAgentIDs:    append([]string(nil), step.ChildAgentIDs...),
			Risk:             string(step.RiskLevel),
			ApprovalRequired: step.ApprovalRequired,
		})
	}
	return &HostPlanView{
		ID:             plan.ID,
		Version:        plan.Version,
		Status:         string(plan.Status),
		Steps:          steps,
		CompletedCount: completed,
		TotalCount:     total,
	}
}

func semanticHostScopeFromMentions(mentions []hostops.HostMention) []opssemantic.OpsHostRef {
	if len(mentions) == 0 {
		return nil
	}
	refs := make([]opssemantic.OpsHostRef, 0, len(mentions))
	seen := map[string]bool{}
	for _, mention := range mentions {
		key := strings.ToLower(firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@")))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, opssemantic.OpsHostRef{
			HostID:      mention.HostID,
			Address:     mention.Address,
			DisplayName: mention.DisplayName,
			Raw:         mention.Raw,
			Source:      string(mention.Source),
		})
	}
	return refs
}

func deterministicMissionID(command HostMissionCreateCommand, mentions []hostops.HostMention) string {
	h := sha1.New()
	_, _ = h.Write([]byte(strings.TrimSpace(command.ThreadID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(command.SessionID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(command.UserTurnID)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(strings.TrimSpace(command.Goal)))
	for _, mention := range mentions {
		_, _ = h.Write([]byte("|"))
		_, _ = h.Write([]byte(firstNonEmptyHostOpsString(mention.HostID, mention.Address, mention.DisplayName, mention.Raw)))
	}
	return "mission-" + hex.EncodeToString(h.Sum(nil))[:12]
}

func formatHostOpsTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstNonEmptyHostOpsString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *defaultHostOpsService) appendMissionAudit(ctx context.Context, missionID string, item hostops.TranscriptItem) {
	if s == nil || s.transcripts == nil || strings.TrimSpace(missionID) == "" {
		return
	}
	_ = s.transcripts.Append(ctx, hostops.MissionAuditTranscriptID(missionID), item)
}
