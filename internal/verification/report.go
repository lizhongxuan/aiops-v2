package verification

import (
	"regexp"
	"strings"
)

const maxReportFieldLength = 512

type ReportStatus string

const (
	StatusPass    ReportStatus = "PASS"
	StatusPartial ReportStatus = "PARTIAL"
	StatusFail    ReportStatus = "FAIL"
)

type EvidenceKind string

const (
	EvidenceAnalysis    EvidenceKind = "analysis"
	EvidenceStaticCheck EvidenceKind = "static_check"
	EvidenceExecution   EvidenceKind = "execution"
	EvidenceAdversarial EvidenceKind = "adversarial"
	EvidenceUserBlocker EvidenceKind = "user_blocker"
)

const (
	EvidenceResultPass    = "pass"
	EvidenceResultFail    = "fail"
	EvidenceResultBlocked = "blocked"
)

type VerificationRequirement string

const (
	VerificationNone              VerificationRequirement = "none"
	VerificationAnalysisAllowed   VerificationRequirement = "analysis_allowed"
	VerificationExecutionRequired VerificationRequirement = "execution_required"
)

type ProbeType string

const (
	ProbeBoundary    ProbeType = "boundary"
	ProbeReverse     ProbeType = "reverse"
	ProbeIdempotency ProbeType = "idempotency"
	ProbeErrorPath   ProbeType = "error_path"
)

const (
	BlockerEnvironment     = "environment"
	BlockerPermission      = "permission"
	BlockerToolUnavailable = "tool_unavailable"
	BlockerUserInput       = "user_input"
	BlockerContract        = "contract"
)

type VerificationReport struct {
	ID             string                  `json:"id"`
	PlanID         string                  `json:"planId,omitempty"`
	TaskID         string                  `json:"taskId,omitempty"`
	Requirement    VerificationRequirement `json:"requirement"`
	Status         ReportStatus            `json:"status"`
	Subject        string                  `json:"subject"`
	Evidence       []EvidenceRecord        `json:"evidence"`
	Probes         []ProbeResult           `json:"probes,omitempty"`
	ContractChecks []ContractCheck         `json:"contractChecks,omitempty"`
	Blockers       []VerificationBlocker   `json:"blockers,omitempty"`
	Expected       string                  `json:"expected,omitempty"`
	Actual         string                  `json:"actual,omitempty"`
	RawRefs        []string                `json:"rawRefs,omitempty"`
	CreatedAt      string                  `json:"createdAt,omitempty"`
}

type EvidenceRecord struct {
	Kind       EvidenceKind `json:"kind"`
	ToolName   string       `json:"toolName,omitempty"`
	ToolCallID string       `json:"toolCallId,omitempty"`
	Command    string       `json:"command,omitempty"`
	Expected   string       `json:"expected,omitempty"`
	Actual     string       `json:"actual,omitempty"`
	Result     string       `json:"result"`
	RawRef     string       `json:"rawRef,omitempty"`
}

type ProbeResult struct {
	Type     ProbeType `json:"type"`
	Expected string    `json:"expected"`
	Actual   string    `json:"actual"`
	Result   string    `json:"result"`
	RawRef   string    `json:"rawRef,omitempty"`
}

type ContractCheck struct {
	Name     string `json:"name"`
	Checked  bool   `json:"checked"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Result   string `json:"result"`
}

type VerificationBlocker struct {
	Reason       string `json:"reason"`
	Source       string `json:"source"`
	BlockedScope string `json:"blockedScope,omitempty"`
	NextAction   string `json:"nextAction"`
}

var sensitiveReportPattern = regexp.MustCompile(`(?i)(password|token|secret|api[_-]?key)\s*[:=]|sk-[a-z0-9_-]{8,}`)

func (report VerificationReport) Normalize() VerificationReport {
	report.ID = normalizeReportString(report.ID)
	report.PlanID = normalizeReportString(report.PlanID)
	report.TaskID = normalizeReportString(report.TaskID)
	report.Requirement = normalizeRequirement(report.Requirement)
	report.Status = normalizeStatus(report.Status)
	report.Subject = normalizeReportString(report.Subject)
	report.Expected = normalizeReportString(report.Expected)
	report.Actual = normalizeReportString(report.Actual)
	report.CreatedAt = normalizeReportString(report.CreatedAt)
	report.RawRefs = normalizeRawRefs(report.RawRefs)

	for i := range report.Evidence {
		report.Evidence[i] = report.Evidence[i].Normalize()
	}
	for i := range report.Probes {
		report.Probes[i] = report.Probes[i].Normalize()
	}
	for i := range report.ContractChecks {
		report.ContractChecks[i] = report.ContractChecks[i].Normalize()
	}
	for i := range report.Blockers {
		report.Blockers[i] = report.Blockers[i].Normalize()
	}
	return report
}

func (record EvidenceRecord) Normalize() EvidenceRecord {
	record.Kind = normalizeEvidenceKind(record.Kind)
	record.ToolName = normalizeReportString(record.ToolName)
	record.ToolCallID = normalizeReportString(record.ToolCallID)
	record.Command = normalizeReportString(record.Command)
	record.Expected = normalizeReportString(record.Expected)
	record.Actual = normalizeReportString(record.Actual)
	record.Result = normalizeResult(record.Result)
	record.RawRef = normalizeReportString(record.RawRef)
	return record
}

func (probe ProbeResult) Normalize() ProbeResult {
	probe.Type = normalizeProbeType(probe.Type)
	probe.Expected = normalizeReportString(probe.Expected)
	probe.Actual = normalizeReportString(probe.Actual)
	probe.Result = normalizeResult(probe.Result)
	probe.RawRef = normalizeReportString(probe.RawRef)
	return probe
}

func (check ContractCheck) Normalize() ContractCheck {
	check.Name = normalizeReportString(check.Name)
	check.Expected = normalizeReportString(check.Expected)
	check.Actual = normalizeReportString(check.Actual)
	check.Result = normalizeResult(check.Result)
	return check
}

func (blocker VerificationBlocker) Normalize() VerificationBlocker {
	blocker.Reason = normalizeReportString(blocker.Reason)
	blocker.Source = normalizeBlockerSource(blocker.Source)
	blocker.BlockedScope = normalizeReportString(blocker.BlockedScope)
	blocker.NextAction = normalizeReportString(blocker.NextAction)
	return blocker
}

func normalizeRawRefs(refs []string) []string {
	out := make([]string, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		normalized := normalizeReportString(ref)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeReportString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if sensitiveReportPattern.MatchString(value) {
		return "[redacted]"
	}
	if len(value) > maxReportFieldLength {
		return value[:maxReportFieldLength]
	}
	return value
}

func normalizeStatus(status ReportStatus) ReportStatus {
	return ReportStatus(strings.ToUpper(strings.TrimSpace(string(status))))
}

func normalizeRequirement(requirement VerificationRequirement) VerificationRequirement {
	return VerificationRequirement(strings.ToLower(strings.TrimSpace(string(requirement))))
}

func normalizeEvidenceKind(kind EvidenceKind) EvidenceKind {
	return EvidenceKind(strings.ToLower(strings.TrimSpace(string(kind))))
}

func normalizeProbeType(probe ProbeType) ProbeType {
	return ProbeType(strings.ToLower(strings.TrimSpace(string(probe))))
}

func normalizeResult(result string) string {
	return strings.ToLower(strings.TrimSpace(result))
}

func normalizeBlockerSource(source string) string {
	return strings.ToLower(strings.TrimSpace(source))
}
