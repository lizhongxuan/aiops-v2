package planning

import (
	"encoding/json"
	"strings"
	"testing"
)

func validPlanArtifactForTest() PlanArtifact {
	return PlanArtifact{
		ID:      "plan-synthetic-1",
		Version: 1,
		Status:  PlanArtifactDraft,
		Context: PlanContext{
			Summary: "需要把复杂任务拆成可验证的通用执行计划",
		},
		RecommendedApproach: []PlanApproachStep{{
			ID:      "approach-1",
			Summary: "先读取状态，再更新计划，最后请求批准",
		}},
		Scope: PlanScope{
			In:  []string{"计划 artifact 与只读探索"},
			Out: []string{"未批准的写操作"},
		},
		Reuse: PlanReuse{
			ExistingPatterns: []string{"复用 existing runtime plan protocol"},
		},
		Verification: PlanVerification{
			Checks: []string{"运行 focused Go tests"},
		},
		Steps: []PlanStep{{
			ID:     "step-1",
			Text:   "读取现有计划状态并整理需要补齐的结构化字段",
			Status: StepStatusPending,
		}},
	}
}

func TestPlanArtifactValidateRequiresStructuredSections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*PlanArtifact)
		wantErr string
	}{
		{
			name:    "context",
			mutate:  func(a *PlanArtifact) { a.Context = PlanContext{} },
			wantErr: "context is required",
		},
		{
			name:    "recommendedApproach",
			mutate:  func(a *PlanArtifact) { a.RecommendedApproach = nil },
			wantErr: "recommendedApproach is required",
		},
		{
			name:    "scope",
			mutate:  func(a *PlanArtifact) { a.Scope = PlanScope{} },
			wantErr: "scope is required",
		},
		{
			name:    "verification",
			mutate:  func(a *PlanArtifact) { a.Verification = PlanVerification{} },
			wantErr: "verification is required",
		},
		{
			name: "open question id",
			mutate: func(a *PlanArtifact) {
				a.OpenQuestions = []PlanQuestion{{Text: "缺少什么批准范围？"}}
			},
			wantErr: "openQuestions[0] id is required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := validPlanArtifactForTest()
			tc.mutate(&artifact)
			err := artifact.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}

	if err := validPlanArtifactForTest().Validate(); err != nil {
		t.Fatalf("valid artifact failed validation: %v", err)
	}
}

func TestPlanArtifactValidateStatusSpecificFields(t *testing.T) {
	approved := validPlanArtifactForTest()
	approved.Status = PlanArtifactApproved
	if err := approved.Validate(); err == nil || !strings.Contains(err.Error(), "approval is required") {
		t.Fatalf("approved Validate() error = %v, want approval required", err)
	}
	approved.Approval = &PlanApprovalState{ID: "approval-1", Status: "approved", ApprovedBy: "user"}
	if err := approved.Validate(); err != nil {
		t.Fatalf("approved artifact with approval failed validation: %v", err)
	}

	rejected := validPlanArtifactForTest()
	rejected.Status = PlanArtifactRejected
	if err := rejected.Validate(); err == nil || !strings.Contains(err.Error(), "rejections are required") {
		t.Fatalf("rejected Validate() error = %v, want rejections required", err)
	}
	rejected.Rejections = []PlanRejection{{ID: "reject-1", Reason: "scope is too broad"}}
	if err := rejected.Validate(); err != nil {
		t.Fatalf("rejected artifact with rejection failed validation: %v", err)
	}
}

func TestPlanStepOwnerDependenciesBlockedByAndEvidenceRefsRoundTrip(t *testing.T) {
	step := PlanStep{
		ID:                 "step-2",
		Text:               "运行 focused tests 并把验证证据写回计划步骤",
		Status:             StepStatusBlocked,
		Summary:            "等待用户批准执行范围",
		Owner:              "agent:planner",
		AgentID:            "agent-synthetic-1",
		DependsOn:          []string{"step-1"},
		Blocks:             []string{"step-3"},
		BlockedBy:          []string{"approval-synthetic-1"},
		EvidenceRefs:       []string{"trace-synthetic-1"},
		RequiredApprovals:  []string{"approval-synthetic-1"},
		VerificationStatus: "skipped",
	}

	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded PlanStep
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Owner != step.Owner || decoded.AgentID != step.AgentID || decoded.VerificationStatus != step.VerificationStatus {
		t.Fatalf("round-trip lost scalar fields: %#v", decoded)
	}
	if len(decoded.DependsOn) != 1 || decoded.DependsOn[0] != "step-1" {
		t.Fatalf("round-trip lost dependsOn: %#v", decoded.DependsOn)
	}
	if len(decoded.BlockedBy) != 1 || decoded.BlockedBy[0] != "approval-synthetic-1" {
		t.Fatalf("round-trip lost blockedBy: %#v", decoded.BlockedBy)
	}
	if len(decoded.EvidenceRefs) != 1 || decoded.EvidenceRefs[0] != "trace-synthetic-1" {
		t.Fatalf("round-trip lost evidenceRefs: %#v", decoded.EvidenceRefs)
	}
}

func TestPlanArtifactRejectsInvalidDependencies(t *testing.T) {
	artifact := validPlanArtifactForTest()
	artifact.Steps = append(artifact.Steps, PlanStep{
		ID:        "step-2",
		Text:      "根据上一步结果生成计划批准范围",
		Status:    StepStatusPending,
		DependsOn: []string{"missing-step"},
	})
	err := artifact.Validate()
	if err == nil || !strings.Contains(err.Error(), "dependsOn references unknown step") {
		t.Fatalf("Validate() error = %v, want unknown dependency", err)
	}
}
