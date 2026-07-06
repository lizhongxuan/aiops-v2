package hostops

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidHostTaskReport         = errors.New("invalid host task report")
	ErrHumanTerminalEvidenceRejected = errors.New("human terminal evidence is not accepted by default")
)

type HostTaskReportValidationContext struct {
	MissionID                  string
	PlanStepID                 string
	HostAgentID                string
	HostID                     string
	BoundRole                  string
	RoleBindingHash            string
	AllowHumanTerminalEvidence bool
}

type HostTaskReportValidator struct {
	ctx HostTaskReportValidationContext
}

func NewHostTaskReportValidator(ctx HostTaskReportValidationContext) *HostTaskReportValidator {
	ctx.MissionID = strings.TrimSpace(ctx.MissionID)
	ctx.PlanStepID = strings.TrimSpace(ctx.PlanStepID)
	ctx.HostAgentID = strings.TrimSpace(ctx.HostAgentID)
	ctx.HostID = strings.TrimSpace(ctx.HostID)
	ctx.BoundRole = strings.TrimSpace(ctx.BoundRole)
	ctx.RoleBindingHash = strings.TrimSpace(ctx.RoleBindingHash)
	return &HostTaskReportValidator{ctx: ctx}
}

func (v *HostTaskReportValidator) Validate(report HostTaskReport) error {
	sanitized, err := v.Sanitize(report)
	if err != nil {
		return err
	}
	if !validHostTaskReportStatus(HostTaskReportStatus(sanitized.Status)) {
		return fmt.Errorf("%w: invalid status", ErrInvalidHostTaskReport)
	}
	if strings.TrimSpace(sanitized.MissionID) == "" || strings.TrimSpace(sanitized.PlanStepID) == "" || strings.TrimSpace(sanitized.HostID) == "" || strings.TrimSpace(sanitized.Status) == "" {
		return fmt.Errorf("%w: missing binding", ErrInvalidHostTaskReport)
	}
	if v.ctx.MissionID != "" && sanitized.MissionID != v.ctx.MissionID {
		return fmt.Errorf("%w: mission mismatch", ErrInvalidHostTaskReport)
	}
	if v.ctx.PlanStepID != "" && sanitized.PlanStepID != v.ctx.PlanStepID {
		return fmt.Errorf("%w: plan step mismatch", ErrInvalidHostTaskReport)
	}
	if v.ctx.HostAgentID != "" && sanitized.HostAgentID != v.ctx.HostAgentID {
		return fmt.Errorf("%w: host agent mismatch", ErrInvalidHostTaskReport)
	}
	if v.ctx.HostID != "" && sanitized.HostID != v.ctx.HostID {
		return fmt.Errorf("%w: host mismatch", ErrInvalidHostTaskReport)
	}
	if v.ctx.BoundRole != "" && sanitized.BoundRole != v.ctx.BoundRole {
		return fmt.Errorf("%w: bound role mismatch", ErrInvalidHostTaskReport)
	}
	if v.ctx.RoleBindingHash != "" && sanitized.RoleBindingHash != v.ctx.RoleBindingHash {
		return fmt.Errorf("%w: role binding hash mismatch", ErrInvalidHostTaskReport)
	}
	for _, evidence := range sanitized.Evidence {
		if evidence.HostID != "" && evidence.HostID != sanitized.HostID {
			return fmt.Errorf("%w: evidence host mismatch", ErrInvalidHostTaskReport)
		}
		if evidence.Source == EvidenceSourceHumanTerminal && !v.ctx.AllowHumanTerminalEvidence {
			return ErrHumanTerminalEvidenceRejected
		}
		if evidence.Source == EvidenceSourceArtifact && evidence.ArtifactRef == "" {
			return fmt.Errorf("%w: artifact evidence missing ref", ErrInvalidHostTaskReport)
		}
		if evidence.RedactionStatus != RedactionStatusApplied && evidence.RedactionStatus != RedactionStatusNotRequired {
			return fmt.Errorf("%w: evidence redaction missing", ErrInvalidHostTaskReport)
		}
	}
	return nil
}

func (v *HostTaskReportValidator) Sanitized(report HostTaskReport) (HostTaskReport, error) {
	return v.Sanitize(report)
}

func (v *HostTaskReportValidator) Sanitize(report HostTaskReport) (HostTaskReport, error) {
	report.MissionID = strings.TrimSpace(report.MissionID)
	report.PlanStepID = strings.TrimSpace(report.PlanStepID)
	report.HostAgentID = strings.TrimSpace(report.HostAgentID)
	report.HostID = strings.TrimSpace(report.HostID)
	report.BoundRole = strings.TrimSpace(report.BoundRole)
	report.RoleBindingHash = strings.TrimSpace(report.RoleBindingHash)
	report.Status = strings.TrimSpace(report.Status)
	report.Summary = RedactSensitiveText(report.Summary)
	for i := range report.Commands {
		report.Commands[i].Command = RedactSensitiveText(report.Commands[i].Command)
		report.Commands[i].RedactedCommand = RedactSensitiveText(firstNonEmptyString(report.Commands[i].RedactedCommand, report.Commands[i].Command))
		report.Commands[i].Summary = RedactSensitiveText(report.Commands[i].Summary)
	}
	for i := range report.Errors {
		report.Errors[i] = RedactSensitiveText(report.Errors[i])
	}
	for i := range report.Blockers {
		report.Blockers[i] = RedactSensitiveText(report.Blockers[i])
	}
	for i := range report.NextSteps {
		report.NextSteps[i] = RedactSensitiveText(report.NextSteps[i])
	}
	for i := range report.Evidence {
		report.Evidence[i].Summary = RedactSensitiveText(report.Evidence[i].Summary)
		report.Evidence[i].ID = strings.TrimSpace(report.Evidence[i].ID)
		report.Evidence[i].HostID = strings.TrimSpace(report.Evidence[i].HostID)
		report.Evidence[i].ArtifactRef = strings.TrimSpace(report.Evidence[i].ArtifactRef)
	}
	return report, nil
}

func validHostTaskReportStatus(status HostTaskReportStatus) bool {
	switch status {
	case HostTaskReportStatusCompleted,
		HostTaskReportStatusFailed,
		HostTaskReportStatusBlocked,
		HostTaskReportStatusBlockedApproval,
		HostTaskReportStatusBlockedEvidence,
		HostTaskReportStatusCancelled,
		HostTaskReportStatusTimeout,
		HostTaskReportStatusNeedsManagerCoordination,
		HostTaskReportStatusNeedsUserApproval:
		return true
	default:
		return false
	}
}
