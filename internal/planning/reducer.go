package planning

import (
	"fmt"
	"strings"
)

func ApplyPlanUpdate(_ PlanState, args UpdatePlanArgs) (PlanState, error) {
	normalized, err := normalizeUpdatePlanArgs(args)
	if err != nil {
		return PlanState{}, err
	}
	return PlanState{
		Status: normalized.Status,
		Steps:  append([]PlanStep(nil), normalized.Steps...),
	}, nil
}

func normalizeUpdatePlanArgs(args UpdatePlanArgs) (UpdatePlanArgs, error) {
	status := args.Status
	if status == "" {
		status = PlanStatusActive
	}
	if !status.IsValid() {
		return UpdatePlanArgs{}, fmt.Errorf("invalid plan status %q", args.Status)
	}
	if len(args.Steps) == 0 {
		return UpdatePlanArgs{}, fmt.Errorf("plan steps are required")
	}

	steps := make([]PlanStep, 0, len(args.Steps))
	seenIDs := map[string]bool{}
	inProgress := 0
	for i, step := range args.Steps {
		normalized := PlanStep{
			ID:      strings.TrimSpace(step.ID),
			Text:    strings.TrimSpace(step.Text),
			Status:  step.Status,
			Summary: strings.TrimSpace(step.Summary),
		}
		if normalized.Text == "" {
			return UpdatePlanArgs{}, fmt.Errorf("step[%d] text is required", i)
		}
		if normalized.Status == "" {
			normalized.Status = StepStatusPending
		}
		if !normalized.Status.IsValid() {
			return UpdatePlanArgs{}, fmt.Errorf("step[%d] invalid status %q", i, normalized.Status)
		}
		if normalized.ID != "" {
			if seenIDs[normalized.ID] {
				return UpdatePlanArgs{}, fmt.Errorf("duplicate step id %q", normalized.ID)
			}
			seenIDs[normalized.ID] = true
		}
		if normalized.Status == StepStatusInProgress {
			inProgress++
		}
		steps = append(steps, normalized)
	}
	if inProgress > 1 {
		return UpdatePlanArgs{}, fmt.Errorf("at most one step can be in_progress")
	}
	if status.IsFinal() && inProgress > 0 {
		return UpdatePlanArgs{}, fmt.Errorf("final plan status cannot contain in_progress steps")
	}
	return UpdatePlanArgs{Status: status, Steps: steps}, nil
}
