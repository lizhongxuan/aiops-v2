package planning

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func ApplyPlanUpdate(_ PlanState, args UpdatePlanArgs) (PlanState, error) {
	normalized, err := normalizeUpdatePlanArgs(args)
	if err != nil {
		return PlanState{}, err
	}
	artifact := normalized.Artifact
	if artifact != nil {
		copyArtifact := *artifact
		copyArtifact.Steps = append([]PlanStep(nil), artifact.Steps...)
		artifact = &copyArtifact
	}
	return PlanState{
		Status:   normalized.Status,
		Steps:    append([]PlanStep(nil), normalized.Steps...),
		Artifact: artifact,
	}, nil
}

func normalizeUpdatePlanArgs(args UpdatePlanArgs) (UpdatePlanArgs, error) {
	status := args.Status
	steps := args.Steps
	artifact := args.Artifact
	if artifact != nil {
		if err := artifact.Validate(); err != nil {
			return UpdatePlanArgs{}, err
		}
		if len(steps) == 0 {
			steps = artifact.Steps
		}
		if status == "" {
			status = planStatusFromArtifactStatus(artifact.Status)
		}
	}
	if status == "" {
		status = PlanStatusActive
	}
	if !status.IsValid() {
		return UpdatePlanArgs{}, fmt.Errorf("invalid plan status %q", args.Status)
	}
	if len(steps) == 0 {
		return UpdatePlanArgs{}, fmt.Errorf("plan steps are required")
	}

	normalizedSteps, err := normalizePlanSteps(steps, status.IsFinal())
	if err != nil {
		return UpdatePlanArgs{}, err
	}
	if err := validatePlanDependencies(normalizedSteps); err != nil {
		return UpdatePlanArgs{}, err
	}
	if artifact != nil {
		copyArtifact := *artifact
		copyArtifact.Steps = append([]PlanStep(nil), normalizedSteps...)
		artifact = &copyArtifact
	}
	return UpdatePlanArgs{Status: status, Steps: normalizedSteps, Artifact: artifact}, nil
}

func normalizePlanSteps(input []PlanStep, finalPlan bool) ([]PlanStep, error) {
	steps := make([]PlanStep, 0, len(input))
	seenIDs := map[string]bool{}
	inProgress := 0
	for i, step := range input {
		normalized := normalizePlanStep(step)
		if normalized.Text == "" {
			return nil, fmt.Errorf("step[%d] text is required", i)
		}
		if isVaguePlanStep(normalized.Text) {
			return nil, fmt.Errorf("step[%d] text is too vague; describe the operation, evidence/tool, and expected output", i)
		}
		if normalized.Status == "" {
			normalized.Status = StepStatusPending
		}
		if !normalized.Status.IsValid() {
			return nil, fmt.Errorf("step[%d] invalid status %q", i, normalized.Status)
		}
		if normalized.ID != "" {
			if seenIDs[normalized.ID] {
				return nil, fmt.Errorf("duplicate step id %q", normalized.ID)
			}
			seenIDs[normalized.ID] = true
		}
		if normalized.Status == StepStatusInProgress {
			inProgress++
		}
		if normalized.Status == StepStatusBlocked && normalized.Summary == "" && len(normalized.BlockedBy) == 0 {
			return nil, fmt.Errorf("step[%d] blocked status requires blockedBy or summary", i)
		}
		if normalized.Status == StepStatusCompleted && normalized.VerificationStatus == "failed" {
			return nil, fmt.Errorf("step[%d] completed status cannot have failed verification", i)
		}
		steps = append(steps, normalized)
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("at most one step can be in_progress")
	}
	if finalPlan && inProgress > 0 {
		return nil, fmt.Errorf("final plan status cannot contain in_progress steps")
	}
	return steps, nil
}

func normalizePlanStep(step PlanStep) PlanStep {
	return PlanStep{
		ID:                 strings.TrimSpace(step.ID),
		Text:               strings.TrimSpace(step.Text),
		Status:             step.Status,
		Summary:            strings.TrimSpace(step.Summary),
		Owner:              strings.TrimSpace(step.Owner),
		AgentID:            strings.TrimSpace(step.AgentID),
		DependsOn:          trimStringSlice(step.DependsOn),
		Blocks:             trimStringSlice(step.Blocks),
		BlockedBy:          trimStringSlice(step.BlockedBy),
		EvidenceRefs:       trimStringSlice(step.EvidenceRefs),
		RequiredApprovals:  trimStringSlice(step.RequiredApprovals),
		VerificationStatus: strings.TrimSpace(step.VerificationStatus),
	}
}

func trimStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func planStatusFromArtifactStatus(status PlanArtifactStatus) PlanStatus {
	switch status {
	case PlanArtifactApproved:
		return PlanStatusCompleted
	case PlanArtifactRejected:
		return PlanStatusFailed
	case PlanArtifactSuperseded:
		return PlanStatusCancelled
	default:
		return PlanStatusActive
	}
}

func isVaguePlanStep(text string) bool {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), "")
	if normalized == "" {
		return true
	}
	for _, phrase := range []string{
		"分析问题",
		"检查服务",
		"处理故障",
		"查看情况",
	} {
		if normalized == phrase {
			return true
		}
	}
	return utf8.RuneCountInString(normalized) < 6
}
