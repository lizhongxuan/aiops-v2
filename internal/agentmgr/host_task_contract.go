package agentmgr

import (
	"fmt"
	"strings"

	"aiops-v2/internal/hostops"
)

func HostSubTaskToAssignment(task hostops.HostSubTask) AgentAssignment {
	hostID := strings.TrimSpace(task.HostID)
	boundRole := strings.TrimSpace(task.BoundRole)
	roleBindingHash := strings.TrimSpace(task.RoleBindingHash)
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		goal = "operate on the assigned host and return bounded evidence"
	}
	evidenceKinds := cleanStringList(task.EvidenceRequirements)
	minEvidenceRefs := len(evidenceKinds)
	if minEvidenceRefs == 0 {
		minEvidenceRefs = 1
	}
	constraints := cleanStringList(task.Constraints)
	if task.RiskLevel != "" {
		constraints = append(constraints, "risk="+string(task.RiskLevel))
	}
	if roleBindingHash != "" {
		constraints = append(constraints, "role_binding_hash="+roleBindingHash)
	}
	constraints = append(constraints, "host_binding_required")
	knownFacts := []string{
		"host_agent_id=" + strings.TrimSpace(task.HostAgentID),
		"bound_host_id=" + hostID,
	}
	if boundRole != "" {
		knownFacts = append(knownFacts, "bound_role="+boundRole)
	}
	if roleBindingHash != "" {
		knownFacts = append(knownFacts, "role_binding_hash="+roleBindingHash)
	}
	return AgentAssignment{
		Objective: goal,
		Background: strings.Join(nonEmptyStrings([]string{
			"mission=" + strings.TrimSpace(task.MissionID),
			"planStep=" + strings.TrimSpace(task.PlanStepID),
			"hostAgent=" + strings.TrimSpace(task.HostAgentID),
		}), "; "),
		KnownFacts: knownFacts,
		Scope: AgentScope{
			ResourceRefs: []string{"host:" + hostID},
		},
		ExpectedOutput: "Return a HostTaskReport with status, bound host id, bound role, role binding hash, executed commands, evidence refs, errors, blockers, and next step suggestions.",
		EvidenceRequirement: EvidenceRequirement{
			MinEvidenceRefs: minEvidenceRefs,
			RequiredKinds:   evidenceKinds,
		},
		StopCondition: "Stop after the assigned host task is completed, blocked, or unsafe to continue.",
		Constraints:   constraints,
	}
}

func HostTaskReportToEvidenceReport(report hostops.HostTaskReport) EvidenceReport {
	errors := cleanStringList(report.Errors)
	for _, blocker := range cleanStringList(report.Blockers) {
		errors = append(errors, "blocker: "+blocker)
	}
	return EvidenceReport{
		AgentID:       strings.TrimSpace(report.HostAgentID),
		Summary:       strings.TrimSpace(report.Summary),
		EvidenceRefs:  cleanStringList(report.EvidenceRefs),
		Confidence:    confidenceFromHostTaskStatus(report.Status),
		NextQuestions: cleanStringList(report.NextSteps),
		Errors:        errors,
	}.Normalize()
}

func HostTaskReportFromAgentResult(result AgentResult, task hostops.HostSubTask) hostops.HostTaskReport {
	status := strings.TrimSpace(string(result.Status))
	if status == "" {
		status = "unknown"
	}
	report := hostops.HostTaskReport{
		MissionID:       strings.TrimSpace(task.MissionID),
		PlanStepID:      strings.TrimSpace(task.PlanStepID),
		HostAgentID:     firstNonEmptyAgentString(strings.TrimSpace(task.HostAgentID), strings.TrimSpace(result.AgentID)),
		HostID:          firstNonEmptyAgentString(strings.TrimSpace(task.HostID), strings.TrimSpace(result.HostID)),
		BoundRole:       strings.TrimSpace(task.BoundRole),
		RoleBindingHash: strings.TrimSpace(task.RoleBindingHash),
		Status:          status,
		Summary:         strings.TrimSpace(result.Output),
		EvidenceRefs:    append([]string(nil), result.ResultRefs...),
	}
	if strings.TrimSpace(result.Error) != "" {
		report.Errors = []string{strings.TrimSpace(result.Error)}
	}
	return report
}

func confidenceFromHostTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(AgentStatusCompleted):
		return "high"
	case "blocked", string(AgentStatusFailed), string(AgentStatusKilled):
		return "low"
	default:
		return "unknown"
	}
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmptyAgentString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatHostSubTaskID(task hostops.HostSubTask) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		strings.TrimSpace(task.MissionID),
		strings.TrimSpace(task.PlanStepID),
		strings.TrimSpace(task.HostAgentID),
		strings.TrimSpace(task.HostID),
	)
}
