package promptcompiler

import (
	"strings"
	"testing"
)

func TestPlanModePromptInjectsFullInstructionOnlyOnceThenSparseReminder(t *testing.T) {
	first, err := NewCompiler().Compile(CompileContext{
		Mode: "plan",
		ProtocolState: ProtocolPromptState{
			PlanMode: &PlanModePromptState{
				State:          "active",
				PlanID:         "plan-synthetic-1",
				ArtifactStatus: "draft",
				ApprovalStatus: "pending_exit_approval",
				OpenQuestions:  2,
			},
		},
	})
	if err != nil {
		t.Fatalf("compile full: %v", err)
	}
	if !first.Dynamic.ProtocolState.PlanMode.FullInstructionInjected {
		t.Fatalf("full instruction flag was not set: %#v", first.Dynamic.ProtocolState.PlanMode)
	}
	for _, want := range []string{
		"## Plan Mode Full Protocol",
		"only inspect, update PlanArtifact, ask the smallest necessary question, or request plan approval",
		"Context, Approach, Scope, Reuse, Verification, and Open Questions",
		"Open Questions",
	} {
		if !strings.Contains(first.Dynamic.Content, want) {
			t.Fatalf("full plan prompt missing %q:\n%s", want, first.Dynamic.Content)
		}
	}

	second, err := NewCompiler().Compile(CompileContext{
		Mode:          "plan",
		ProtocolState: first.Dynamic.ProtocolState,
	})
	if err != nil {
		t.Fatalf("compile sparse: %v", err)
	}
	if strings.Contains(second.Dynamic.Content, "## Plan Mode Full Protocol") {
		t.Fatalf("sparse reminder repeated full protocol:\n%s", second.Dynamic.Content)
	}
	for _, want := range []string{
		"## Plan Mode Sparse Reminder",
		"plan_id: plan-synthetic-1",
		"artifact_status: draft",
		"open_questions: 2",
		"inspect -> update plan -> ask smallest missing question -> request approval",
	} {
		if !strings.Contains(second.Dynamic.Content, want) {
			t.Fatalf("sparse plan prompt missing %q:\n%s", want, second.Dynamic.Content)
		}
	}
}

func TestPlanModePromptResumeReminderCarriesApprovalAndQuestions(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		Mode: "plan",
		ProtocolState: ProtocolPromptState{
			PlanMode: &PlanModePromptState{
				State:            "active",
				PlanID:           "plan-resume-1",
				ArtifactStatus:   "rejected",
				ApprovalStatus:   "rejected",
				ReminderLevel:    "resume",
				PendingQuestions: 1,
				OpenQuestions:    1,
				RejectionReason:  "scope too broad",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile resume: %v", err)
	}
	for _, want := range []string{
		"## Plan Mode Resume Reminder",
		"plan_id: plan-resume-1",
		"approval_status: rejected",
		"pending_questions: 1",
		"rejection_reason: scope too broad",
	} {
		if !strings.Contains(compiled.Dynamic.Content, want) {
			t.Fatalf("resume plan prompt missing %q:\n%s", want, compiled.Dynamic.Content)
		}
	}
}
