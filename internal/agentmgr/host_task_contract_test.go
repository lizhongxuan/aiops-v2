package agentmgr

import (
	"testing"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/opssemantic"
)

func TestHostSubTaskToAssignmentBuildsSelfContainedWorkerTask(t *testing.T) {
	task := hostops.HostSubTask{
		MissionID:            "mission-1",
		PlanStepID:           "step-1",
		HostAgentID:          "child-a",
		HostID:               "host-a",
		Goal:                 "执行通用主机操作并返回证据",
		Constraints:          []string{"仅操作绑定主机", "非白名单命令先申请审批"},
		RiskLevel:            opssemantic.RiskMediumWrite,
		EvidenceRequirements: []string{"command_result", "status_summary"},
	}

	assignment := HostSubTaskToAssignment(task)
	if assignment.Objective != task.Goal {
		t.Fatalf("objective = %q, want goal", assignment.Objective)
	}
	if len(assignment.Scope.ResourceRefs) != 1 || assignment.Scope.ResourceRefs[0] != "host:host-a" {
		t.Fatalf("scope = %#v, want host resource scope", assignment.Scope)
	}
	if assignment.EvidenceRequirement.MinEvidenceRefs != 2 {
		t.Fatalf("min evidence refs = %d, want 2", assignment.EvidenceRequirement.MinEvidenceRefs)
	}
	if !containsString(assignment.Constraints, "仅操作绑定主机") || !containsString(assignment.Constraints, "risk=medium_write") {
		t.Fatalf("constraints = %#v, want original constraints and risk marker", assignment.Constraints)
	}
	if result := ValidateAgentAssignment(assignment); result.Status != AssignmentLintPass {
		t.Fatalf("assignment lint = %#v, want pass", result)
	}
}

func TestHostTaskReportToEvidenceReportPreservesEvidenceAndErrors(t *testing.T) {
	report := hostops.HostTaskReport{
		HostAgentID:  "child-a",
		HostID:       "host-a",
		Status:       "blocked",
		Summary:      "操作被审批拒绝阻塞",
		EvidenceRefs: []string{"transcript:child-a:approval-1"},
		Errors:       []string{"command approval denied"},
		Blockers:     []string{"waiting_for_approval"},
		NextSteps:    []string{"调整计划或重新申请审批"},
	}

	evidence := HostTaskReportToEvidenceReport(report)
	if evidence.AgentID != "child-a" || evidence.Summary != "操作被审批拒绝阻塞" {
		t.Fatalf("evidence report = %#v, want agent summary", evidence)
	}
	if !containsString(evidence.EvidenceRefs, "transcript:child-a:approval-1") {
		t.Fatalf("evidence refs = %#v, want report evidence refs", evidence.EvidenceRefs)
	}
	if !containsString(evidence.Errors, "command approval denied") || !containsString(evidence.Errors, "blocker: waiting_for_approval") {
		t.Fatalf("errors = %#v, want errors plus blockers", evidence.Errors)
	}
	if !containsString(evidence.NextQuestions, "调整计划或重新申请审批") {
		t.Fatalf("next questions = %#v, want next steps", evidence.NextQuestions)
	}
	if err := evidence.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
