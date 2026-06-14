package verification

import "testing"

func TestProbePlannerSelectsProbeByTaskShape(t *testing.T) {
	cases := []struct {
		name string
		task ProbePlanningInput
		want ProbeType
	}{
		{
			name: "state changing task requires idempotency",
			task: ProbePlanningInput{
				TaskKind:       TaskKindStateChanging,
				RiskLevel:      RiskMedium,
				ExecutedAction: "updated synthetic settings",
			},
			want: ProbeIdempotency,
		},
		{
			name: "state changing task with rollback requires reverse",
			task: ProbePlanningInput{
				TaskKind:       TaskKindStateChanging,
				RiskLevel:      RiskHigh,
				SupportsRevert: true,
				ExecutedAction: "applied synthetic migration",
			},
			want: ProbeReverse,
		},
		{
			name: "selection task requires boundary",
			task: ProbePlanningInput{
				TaskKind:  TaskKindParsingSelection,
				RiskLevel: RiskLow,
			},
			want: ProbeBoundary,
		},
		{
			name: "failure handling task requires error path",
			task: ProbePlanningInput{
				TaskKind:  TaskKindFailureHandling,
				RiskLevel: RiskMedium,
			},
			want: ProbeErrorPath,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := PlanProbes(tc.task)
			if len(plan.Candidates) == 0 {
				t.Fatalf("Candidates empty")
			}
			if plan.Candidates[0].Type != tc.want {
				t.Fatalf("first probe = %q, want %q: %#v", plan.Candidates[0].Type, tc.want, plan)
			}
			if !plan.Candidates[0].RequiredForPass {
				t.Fatalf("RequiredForPass = false, want true")
			}
			if plan.Candidates[0].Reason == "" {
				t.Fatalf("Reason is empty")
			}
		})
	}
}

func TestProbePlannerMarksBlockedRequiredProbeAsPartialRequirement(t *testing.T) {
	plan := PlanProbes(ProbePlanningInput{
		TaskKind:      TaskKindStateChanging,
		RiskLevel:     RiskMedium,
		ProbeBlocked:  true,
		BlockerSource: BlockerPermission,
		BlockerReason: "synthetic permission missing",
	})

	if len(plan.Candidates) != 1 {
		t.Fatalf("Candidates length = %d, want 1", len(plan.Candidates))
	}
	if !plan.Candidates[0].Blocked {
		t.Fatalf("Blocked = false, want true")
	}
	if plan.Candidates[0].SkippedReason == "" {
		t.Fatalf("SkippedReason is empty")
	}
	if plan.RequiredStatusOnBlock != StatusPartial {
		t.Fatalf("RequiredStatusOnBlock = %q, want PARTIAL", plan.RequiredStatusOnBlock)
	}

	report := validReport(StatusPartial, VerificationExecutionRequired)
	report.Blockers = plan.Blockers
	decision := ValidateReport(report)
	if !decision.Valid {
		t.Fatalf("blocked probe PARTIAL report invalid: %#v", decision)
	}
}
