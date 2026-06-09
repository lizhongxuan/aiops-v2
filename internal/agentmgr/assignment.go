package agentmgr

import (
	"fmt"
	"strings"
)

type AgentAssignment struct {
	Objective           string              `json:"objective"`
	Background          string              `json:"background"`
	KnownFacts          []string            `json:"knownFacts"`
	Scope               AgentScope          `json:"scope"`
	ExpectedOutput      string              `json:"expectedOutput"`
	EvidenceRequirement EvidenceRequirement `json:"evidenceRequirement"`
	StopCondition       string              `json:"stopCondition"`
	Constraints         []string            `json:"constraints,omitempty"`
}

type AgentScope struct {
	ResourceRefs []string `json:"resourceRefs,omitempty"`
	TimeRange    string   `json:"timeRange,omitempty"`
	Exclusions   []string `json:"exclusions,omitempty"`
}

func (s AgentScope) IsZero() bool {
	return len(s.ResourceRefs) == 0 && strings.TrimSpace(s.TimeRange) == "" && len(s.Exclusions) == 0
}

type EvidenceRequirement struct {
	MinEvidenceRefs int      `json:"minEvidenceRefs"`
	RequiredKinds   []string `json:"requiredKinds,omitempty"`
}

func (r EvidenceRequirement) IsZero() bool {
	return r.MinEvidenceRefs <= 0 && len(r.RequiredKinds) == 0
}

type AssignmentLintStatus string

const (
	AssignmentLintPass AssignmentLintStatus = "pass"
	AssignmentLintFail AssignmentLintStatus = "fail"
)

type AssignmentLintResult struct {
	Status        AssignmentLintStatus `json:"status"`
	MissingFields []string             `json:"missingFields,omitempty"`
	Reasons       []string             `json:"reasons,omitempty"`
}

func ValidateAgentAssignment(a AgentAssignment) AssignmentLintResult {
	var missing []string
	if strings.TrimSpace(a.Objective) == "" {
		missing = append(missing, "objective")
	}
	if strings.TrimSpace(a.Background) == "" {
		missing = append(missing, "background")
	}
	if len(a.KnownFacts) == 0 {
		missing = append(missing, "knownFacts")
	}
	if a.Scope.IsZero() {
		missing = append(missing, "scope")
	}
	if strings.TrimSpace(a.ExpectedOutput) == "" {
		missing = append(missing, "expectedOutput")
	}
	if a.EvidenceRequirement.IsZero() {
		missing = append(missing, "evidenceRequirement")
	}
	if strings.TrimSpace(a.StopCondition) == "" {
		missing = append(missing, "stopCondition")
	}
	if len(missing) > 0 {
		return AssignmentLintResult{
			Status:        AssignmentLintFail,
			MissingFields: missing,
			Reasons:       []string{"provide_self_contained_assignment"},
		}
	}
	return AssignmentLintResult{Status: AssignmentLintPass}
}

func (a AgentAssignment) Summary(maxChars int) string {
	parts := []string{
		"objective=" + strings.TrimSpace(a.Objective),
		"scope=" + strings.Join(a.Scope.ResourceRefs, ","),
		"expectedOutput=" + strings.TrimSpace(a.ExpectedOutput),
		fmt.Sprintf("minEvidenceRefs=%d", a.EvidenceRequirement.MinEvidenceRefs),
	}
	summary := strings.Join(nonEmptyStrings(parts), "; ")
	if maxChars > 0 && len(summary) > maxChars {
		if maxChars <= 3 {
			return summary[:maxChars]
		}
		return summary[:maxChars-3] + "..."
	}
	return summary
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(strings.TrimSuffix(value, "=")) != "" {
			out = append(out, value)
		}
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
