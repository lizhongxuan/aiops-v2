package experiencepack

import "testing"

func TestRunnerCandidateHasFiveStagesAndGuards(t *testing.T) {
	candidate := GenerateRunnerWorkflowCandidate(Trajectory{
		CaseID:      "case-runner",
		UserGoal:    "部署 pg 主从",
		Commands:    []string{"ssh xxA", "ssh xxB", "install pg", "init", "backup", "validate"},
		Environment: EnvironmentFingerprint{HostCount: 3},
	})
	if len(candidate.Steps) != 5 {
		t.Fatalf("expected precheck/dry_run/execute/validate/rollback, got %#v", candidate.Steps)
	}
	if candidate.Guards["dry_run_required"] != true || candidate.Guards["approval_required"] != true || candidate.Guards["host_lease_required"] != true {
		t.Fatalf("missing required guards: %#v", candidate.Guards)
	}
}
