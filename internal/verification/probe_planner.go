package verification

type TaskKind string

const (
	TaskKindUnknown          TaskKind = ""
	TaskKindStateChanging    TaskKind = "state_changing"
	TaskKindParsingSelection TaskKind = "parsing_selection"
	TaskKindFailureHandling  TaskKind = "failure_handling"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ProbePlanningInput struct {
	TaskKind       TaskKind
	RiskLevel      RiskLevel
	ExecutedAction string
	SupportsRevert bool
	ProbeBlocked   bool
	BlockerSource  string
	BlockerReason  string
	BlockedScope   string
}

type ProbePlan struct {
	Candidates            []ProbeCandidate         `json:"candidates"`
	Blockers              []VerificationBlocker    `json:"blockers,omitempty"`
	RequiredStatusOnBlock ReportStatus             `json:"requiredStatusOnBlock,omitempty"`
	Trace                 []ProbePlanningTraceItem `json:"trace,omitempty"`
}

type ProbeCandidate struct {
	Type            ProbeType `json:"type"`
	Reason          string    `json:"reason"`
	RequiredForPass bool      `json:"requiredForPass"`
	Blocked         bool      `json:"blocked,omitempty"`
	SkippedReason   string    `json:"skippedReason,omitempty"`
}

type ProbePlanningTraceItem struct {
	Type          ProbeType `json:"type"`
	Selected      bool      `json:"selected"`
	Reason        string    `json:"reason,omitempty"`
	SkippedReason string    `json:"skippedReason,omitempty"`
}

func PlanProbes(input ProbePlanningInput) ProbePlan {
	candidate := ProbeCandidate{
		Type:            selectedProbeType(input),
		Reason:          selectedProbeReason(input),
		RequiredForPass: true,
	}
	plan := ProbePlan{
		Candidates: []ProbeCandidate{candidate},
		Trace: []ProbePlanningTraceItem{{
			Type:     candidate.Type,
			Selected: true,
			Reason:   candidate.Reason,
		}},
	}

	if !input.ProbeBlocked {
		return plan
	}

	source := normalizeBlockerSource(input.BlockerSource)
	if source == "" {
		source = BlockerEnvironment
	}
	reason := normalizeReportString(input.BlockerReason)
	if reason == "" {
		reason = "required probe is blocked"
	}
	scope := normalizeReportString(input.BlockedScope)
	if scope == "" {
		scope = "probe:" + string(candidate.Type)
	}
	nextAction := "resolve blocker and rerun required probe"

	plan.Candidates[0].Blocked = true
	plan.Candidates[0].SkippedReason = reason
	plan.RequiredStatusOnBlock = StatusPartial
	plan.Blockers = []VerificationBlocker{{
		Reason:       reason,
		Source:       source,
		BlockedScope: scope,
		NextAction:   nextAction,
	}}
	plan.Trace[0].SkippedReason = reason
	return plan
}

func selectedProbeType(input ProbePlanningInput) ProbeType {
	switch input.TaskKind {
	case TaskKindStateChanging:
		if input.SupportsRevert {
			return ProbeReverse
		}
		return ProbeIdempotency
	case TaskKindParsingSelection:
		return ProbeBoundary
	case TaskKindFailureHandling:
		return ProbeErrorPath
	default:
		return ProbeBoundary
	}
}

func selectedProbeReason(input ProbePlanningInput) string {
	switch input.TaskKind {
	case TaskKindStateChanging:
		if input.SupportsRevert {
			return "state-changing task has a reversible action, so reverse probe is required for PASS"
		}
		return "state-changing task requires idempotency evidence before PASS"
	case TaskKindParsingSelection:
		return "parsing, selection, or filtering task requires boundary probe evidence"
	case TaskKindFailureHandling:
		return "failure-handling task requires error-path probe evidence"
	default:
		return "task requires a generic boundary probe before PASS"
	}
}
