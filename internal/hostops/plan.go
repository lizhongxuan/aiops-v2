package hostops

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/opssemantic"
)

var (
	ErrPlanMissingHost            = errors.New("host operation plan requires at least one host")
	ErrCompletedPlanStepImmutable = errors.New("completed plan step cannot be changed")
	ErrPlanRevisionRequiresReason = errors.New("plan revision requires a reason")
)

type PlanRevisionRequest struct {
	Reason          string
	AffectedHostIDs []string
	Steps           []PlanStep
}

func BuildPlanForMission(mission HostOperationMission) (HostOperationPlan, error) {
	hostIDs := missionPlanHostIDs(mission)
	if len(hostIDs) == 0 {
		return HostOperationPlan{}, ErrPlanMissingHost
	}
	risk := mission.SemanticTask.RiskLevel
	if risk == "" {
		risk = opssemantic.RiskReadOnly
	}
	action := mission.SemanticTask.ActionType
	if action == "" {
		action = opssemantic.ActionReadOnly
		if risk != opssemantic.RiskReadOnly {
			action = opssemantic.ActionWrite
		}
	}
	evidence := semanticEvidenceDescriptions(mission.SemanticTask.EvidenceRequirements)
	steps := make([]PlanStep, 0, len(hostIDs))
	for i, hostID := range hostIDs {
		index := i + 1
		steps = append(steps, PlanStep{
			ID:               fmt.Sprintf("step-%s-%d", sanitizeIDPart(mission.ID), index),
			Index:            index,
			Title:            "Operate on assigned host",
			Summary:          "Run the requested operation on the assigned host and report evidence.",
			Status:           PlanStepStatusPending,
			HostIDs:          []string{hostID},
			ActionType:       action,
			RiskLevel:        risk,
			EvidenceRequired: evidence,
			ApprovalRequired: opssemantic.RiskRequiresApproval(risk),
		})
	}
	return HostOperationPlan{
		ID:      "plan-" + sanitizeIDPart(mission.ID),
		Version: 1,
		Status:  PlanStatusWaitingAcceptance,
		Steps:   steps,
	}, nil
}

func PlanProgress(plan HostOperationPlan) (completed, total int) {
	total = len(plan.Steps)
	for _, step := range plan.Steps {
		if step.Status == PlanStepStatusCompleted {
			completed++
		}
	}
	return completed, total
}

func ReviseMissionPlan(mission HostOperationMission, req PlanRevisionRequest) (HostOperationMission, error) {
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return HostOperationMission{}, ErrPlanRevisionRequiresReason
	}
	currentByID := map[string]PlanStep{}
	for _, step := range mission.Plan.Steps {
		currentByID[step.ID] = step
	}
	for _, next := range req.Steps {
		current, ok := currentByID[next.ID]
		if !ok || current.Status != PlanStepStatusCompleted {
			continue
		}
		if !reflect.DeepEqual(frozenCompletedStep(current), frozenCompletedStep(next)) {
			return HostOperationMission{}, ErrCompletedPlanStepImmutable
		}
	}

	requiresAcceptance := revisionRequiresAcceptance(mission.Plan.Steps, req.Steps)
	fromVersion := mission.Plan.Version
	if fromVersion == 0 {
		fromVersion = 1
	}
	toVersion := fromVersion + 1
	mission.Plan.Version = toVersion
	mission.Plan.Steps = cloneRevisionSteps(req.Steps)
	for i := range mission.Plan.Steps {
		if mission.Plan.Steps[i].Index == 0 {
			mission.Plan.Steps[i].Index = i + 1
		}
		if mission.Plan.Steps[i].Status == "" {
			mission.Plan.Steps[i].Status = PlanStepStatusPending
		}
	}
	mission.Plan.Revisions = append(mission.Plan.Revisions, PlanRevision{
		ID:                 fmt.Sprintf("revision-%s-%d", sanitizeIDPart(mission.ID), toVersion),
		FromVersion:        fromVersion,
		ToVersion:          toVersion,
		Reason:             reason,
		AffectedHostIDs:    append([]string(nil), req.AffectedHostIDs...),
		Changes:            revisionChanges(currentByID, req.Steps),
		RequiresAcceptance: requiresAcceptance,
		CreatedAt:          time.Now().UTC(),
	})
	if requiresAcceptance {
		mission.Plan.Status = PlanStatusWaitingAcceptance
		mission.PlanAccepted = false
		mission.Status = HostMissionStatusWaitingPlanAcceptance
		mission.Plan.AcceptedAt = nil
		mission.Plan.AcceptedBy = ""
	}
	return mission, nil
}

func missionPlanHostIDs(mission HostOperationMission) []string {
	hostIDs := make([]string, 0, len(mission.SemanticTask.HostScope)+len(mission.Mentions))
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			return
		}
		seen[strings.ToLower(value)] = true
		hostIDs = append(hostIDs, value)
	}
	for _, host := range mission.SemanticTask.HostScope {
		add(firstNonEmptyString(host.HostID, host.Address, host.DisplayName, strings.TrimPrefix(host.Raw, "@")))
	}
	for _, mention := range mission.Mentions {
		add(firstNonEmptyString(mention.HostID, mention.Address, mention.DisplayName, strings.TrimPrefix(mention.Raw, "@")))
	}
	return hostIDs
}

func semanticEvidenceDescriptions(requirements []opssemantic.EvidenceRequirement) []string {
	if len(requirements) == 0 {
		return []string{opssemantic.EvidenceCommandOutput}
	}
	out := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		out = append(out, firstNonEmptyString(requirement.Description, requirement.Kind))
	}
	return out
}

type completedStepSnapshot struct {
	ID               string
	Index            int
	Title            string
	Summary          string
	HostIDs          []string
	ChildAgentIDs    []string
	ActionType       opssemantic.OpsActionType
	RiskLevel        opssemantic.OpsRiskLevel
	EvidenceRequired []string
	ApprovalRequired bool
}

func frozenCompletedStep(step PlanStep) completedStepSnapshot {
	return completedStepSnapshot{
		ID:               step.ID,
		Index:            step.Index,
		Title:            step.Title,
		Summary:          step.Summary,
		HostIDs:          sortedStrings(step.HostIDs),
		ChildAgentIDs:    sortedStrings(step.ChildAgentIDs),
		ActionType:       step.ActionType,
		RiskLevel:        step.RiskLevel,
		EvidenceRequired: sortedStrings(step.EvidenceRequired),
		ApprovalRequired: step.ApprovalRequired,
	}
}

func revisionRequiresAcceptance(current, next []PlanStep) bool {
	currentByID := map[string]PlanStep{}
	for _, step := range current {
		currentByID[step.ID] = step
	}
	for _, step := range next {
		old, ok := currentByID[step.ID]
		if !ok {
			if step.ActionType == opssemantic.ActionWrite || step.RiskLevel != "" && step.RiskLevel != opssemantic.RiskReadOnly {
				return true
			}
			continue
		}
		if riskRank(step.RiskLevel) > riskRank(old.RiskLevel) {
			return true
		}
		if old.ActionType != opssemantic.ActionWrite && step.ActionType == opssemantic.ActionWrite {
			return true
		}
	}
	return false
}

func riskRank(risk opssemantic.OpsRiskLevel) int {
	switch risk {
	case opssemantic.RiskLowWrite:
		return 1
	case opssemantic.RiskMediumWrite:
		return 2
	case opssemantic.RiskHighWrite:
		return 3
	case opssemantic.RiskDestructive:
		return 4
	default:
		return 0
	}
}

func cloneRevisionSteps(steps []PlanStep) []PlanStep {
	out := append([]PlanStep(nil), steps...)
	for i := range out {
		out[i] = clonePlanStep(out[i])
	}
	return out
}

func revisionChanges(currentByID map[string]PlanStep, next []PlanStep) []string {
	changes := make([]string, 0, len(next))
	for _, step := range next {
		if _, ok := currentByID[step.ID]; ok {
			changes = append(changes, "updated:"+step.ID)
		} else {
			changes = append(changes, "added:"+step.ID)
		}
	}
	if len(changes) == 0 {
		changes = append(changes, "no_step_changes")
	}
	return changes
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
