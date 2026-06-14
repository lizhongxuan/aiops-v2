package planning

import "testing"

func TestPlanReducerEmitsTransitionLog(t *testing.T) {
	prev := PlanState{
		Status: PlanStatusActive,
		Steps: []PlanStep{{
			ID:     "step-1",
			Text:   "读取现有计划状态并整理结构化字段",
			Status: StepStatusPending,
			Owner:  "agent:planner",
		}},
	}
	args := UpdatePlanArgs{
		Steps: []PlanStep{{
			ID:           "step-1",
			Text:         "读取现有计划状态并整理结构化字段",
			Status:       StepStatusCompleted,
			Owner:        "agent:planner",
			EvidenceRefs: []string{"trace-synthetic-1"},
			Summary:      "已读取计划状态",
		}},
	}

	result, err := ApplyPlanUpdateWithTransitions(prev, args)
	if err != nil {
		t.Fatalf("ApplyPlanUpdateWithTransitions() error = %v", err)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("transitions = %#v, want one", result.Transitions)
	}
	transition := result.Transitions[0]
	if transition.StepID != "step-1" || transition.From != string(StepStatusPending) || transition.To != string(StepStatusCompleted) {
		t.Fatalf("transition status = %#v", transition)
	}
	if transition.Owner != "agent:planner" {
		t.Fatalf("transition owner = %q", transition.Owner)
	}
	if len(transition.EvidenceRefs) != 1 || transition.EvidenceRefs[0] != "trace-synthetic-1" {
		t.Fatalf("transition evidence = %#v", transition.EvidenceRefs)
	}
}

func TestPlanReducerEmitsTransitionForNewStep(t *testing.T) {
	result, err := ApplyPlanUpdateWithTransitions(PlanState{}, UpdatePlanArgs{
		Steps: []PlanStep{{
			ID:     "step-1",
			Text:   "根据当前请求建立可验证执行计划",
			Status: StepStatusInProgress,
			Owner:  "agent:planner",
		}},
	})
	if err != nil {
		t.Fatalf("ApplyPlanUpdateWithTransitions() error = %v", err)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("transitions = %#v, want one", result.Transitions)
	}
	if result.Transitions[0].From != "" || result.Transitions[0].To != string(StepStatusInProgress) {
		t.Fatalf("new step transition = %#v", result.Transitions[0])
	}
}
