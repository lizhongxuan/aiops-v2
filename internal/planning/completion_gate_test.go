package planning

import "testing"

func TestCompletionGateBlocksSuccessFinalWhenPlanHasPendingEvidenceApprovalOrFailedTool(t *testing.T) {
	cases := []struct {
		name      string
		state     PlanState
		context   CompletionGateContext
		want      string
		wantCause string
	}{
		{
			name: "pending step",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:     "step-1",
				Text:   "读取现有计划状态并整理结构化字段",
				Status: StepStatusPending,
			}}},
			want:      CompletionGateDenySuccessFinal,
			wantCause: "pending_step",
		},
		{
			name: "blocked step",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:        "step-1",
				Text:      "等待用户批准计划执行范围",
				Status:    StepStatusBlocked,
				BlockedBy: []string{"approval-synthetic-1"},
			}}},
			want:      CompletionGateRequireBlockerFinal,
			wantCause: "blocked_step",
		},
		{
			name: "failed step",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:      "step-1",
				Text:    "运行 focused tests 并记录失败原因",
				Status:  StepStatusFailed,
				Summary: "测试失败",
			}}},
			want:      CompletionGateRequireBlockerFinal,
			wantCause: "failed_step",
		},
		{
			name: "required approval missing",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:                "step-1",
				Text:              "根据批准范围执行写操作",
				Status:            StepStatusCompleted,
				RequiredApprovals: []string{"approval-synthetic-1"},
			}}},
			want:      CompletionGateDenySuccessFinal,
			wantCause: "missing_required_approval",
		},
		{
			name: "verification skipped",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:                 "step-1",
				Text:               "运行 Playwright 验证页面状态",
				Status:             StepStatusCompleted,
				VerificationStatus: "skipped",
			}}},
			want:      CompletionGateRequireBlockerFinal,
			wantCause: "verification_skipped",
		},
		{
			name: "pending evidence",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:           "step-1",
				Text:         "运行 focused tests 并记录验证证据",
				Status:       StepStatusCompleted,
				EvidenceRefs: []string{"trace-synthetic-1"},
			}}},
			context:   CompletionGateContext{PendingEvidenceRefs: []string{"evidence-synthetic-2"}},
			want:      CompletionGateDenySuccessFinal,
			wantCause: "pending_evidence",
		},
		{
			name: "failed tool",
			state: PlanState{Status: PlanStatusActive, Steps: []PlanStep{{
				ID:           "step-1",
				Text:         "读取工具输出并记录证据",
				Status:       StepStatusCompleted,
				EvidenceRefs: []string{"trace-synthetic-1"},
			}}},
			context:   CompletionGateContext{FailedToolRefs: []string{"tool-synthetic-1"}},
			want:      CompletionGateDenySuccessFinal,
			wantCause: "failed_tool",
		},
		{
			name: "allow complete",
			state: PlanState{Status: PlanStatusCompleted, Steps: []PlanStep{{
				ID:                 "step-1",
				Text:               "运行 focused tests 并记录验证证据",
				Status:             StepStatusCompleted,
				EvidenceRefs:       []string{"trace-synthetic-1"},
				VerificationStatus: "passed",
			}}},
			context: CompletionGateContext{ApprovedRefs: []string{"approval-synthetic-1"}},
			want:    CompletionGateAllow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateCompletionGate(tc.state, tc.context)
			if decision.Action != tc.want {
				t.Fatalf("action = %q, want %q: %#v", decision.Action, tc.want, decision)
			}
			if tc.wantCause != "" && !containsPlanningString(decision.Reasons, tc.wantCause) {
				t.Fatalf("reasons = %#v, want %q", decision.Reasons, tc.wantCause)
			}
		})
	}
}
