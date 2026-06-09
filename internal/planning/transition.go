package planning

type PlanUpdateResult struct {
	State       PlanState        `json:"state"`
	Transitions []PlanTransition `json:"transitions,omitempty"`
}

type PlanTransition struct {
	StepID       string   `json:"stepId,omitempty"`
	From         string   `json:"from,omitempty"`
	To           string   `json:"to"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Owner        string   `json:"owner,omitempty"`
}

func ApplyPlanUpdateWithTransitions(prev PlanState, args UpdatePlanArgs) (PlanUpdateResult, error) {
	next, err := ApplyPlanUpdate(prev, args)
	if err != nil {
		return PlanUpdateResult{}, err
	}
	return PlanUpdateResult{
		State:       next,
		Transitions: BuildPlanTransitions(prev, next),
	}, nil
}

func BuildPlanTransitions(prev, next PlanState) []PlanTransition {
	previous := map[string]PlanStep{}
	for _, step := range prev.Steps {
		key := planStepTransitionKey(step)
		if key != "" {
			previous[key] = step
		}
	}
	var transitions []PlanTransition
	for _, step := range next.Steps {
		key := planStepTransitionKey(step)
		if key == "" {
			continue
		}
		prevStep, ok := previous[key]
		if ok && prevStep.Status == step.Status {
			continue
		}
		transition := PlanTransition{
			StepID:       step.ID,
			To:           string(step.Status),
			Reason:       step.Summary,
			EvidenceRefs: append([]string(nil), step.EvidenceRefs...),
			Owner:        step.Owner,
		}
		if ok {
			transition.From = string(prevStep.Status)
		}
		transitions = append(transitions, transition)
	}
	return transitions
}

func planStepTransitionKey(step PlanStep) string {
	if step.ID != "" {
		return "id:" + step.ID
	}
	if step.Text != "" {
		return "text:" + step.Text
	}
	return ""
}
