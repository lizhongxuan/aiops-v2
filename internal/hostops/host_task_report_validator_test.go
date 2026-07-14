package hostops

import (
	"errors"
	"strings"
	"testing"
)

func TestHostTaskReportValidatorRequiresMissionHostStepAndStatus(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	err := validator.Validate(HostTaskReport{MissionID: "mission-report", PlanStepID: "step-a", HostID: "host-a"})
	if !errors.Is(err, ErrInvalidHostTaskReport) {
		t.Fatalf("err = %v, want ErrInvalidHostTaskReport", err)
	}
}

func TestHostTaskReportValidatorRejectsCrossHostEvidence(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	err := validator.Validate(HostTaskReport{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
		Status:      string(HostTaskReportStatusCompleted),
		Evidence:    []HostTaskEvidence{{ID: "ev-b", HostID: "host-b", Source: EvidenceSourceHostCommandTool, RedactionStatus: RedactionStatusNotRequired}},
	})
	if !errors.Is(err, ErrInvalidHostTaskReport) {
		t.Fatalf("err = %v, want ErrInvalidHostTaskReport", err)
	}
}

func TestHostTaskReportValidatorRequiresBoundRoleAndHashWhenContextHasRoleBinding(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:       "mission-report",
		PlanStepID:      "step-a",
		HostAgentID:     "child-a",
		HostID:          "host-a",
		BoundRole:       "pg_primary",
		RoleBindingHash: "role-hash-a",
	})
	err := validator.Validate(HostTaskReport{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
		Status:      string(HostTaskReportStatusCompleted),
		Evidence:    []HostTaskEvidence{{ID: "ev-a", HostID: "host-a", Source: EvidenceSourceHostCommandTool, RedactionStatus: RedactionStatusNotRequired}},
	})
	if !errors.Is(err, ErrInvalidHostTaskReport) {
		t.Fatalf("err = %v, want missing role binding to be invalid", err)
	}
	err = validator.Validate(HostTaskReport{
		MissionID:       "mission-report",
		PlanStepID:      "step-a",
		HostAgentID:     "child-a",
		HostID:          "host-a",
		BoundRole:       "pg_primary",
		RoleBindingHash: "role-hash-a",
		Status:          string(HostTaskReportStatusCompleted),
		Evidence:        []HostTaskEvidence{{ID: "ev-a", HostID: "host-a", Source: EvidenceSourceHostCommandTool, RedactionStatus: RedactionStatusNotRequired}},
	})
	if err != nil {
		t.Fatalf("Validate with bound role/hash error = %v", err)
	}
}

func TestHostTaskReportValidatorRejectsHumanTerminalEvidenceByDefault(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	err := validator.Validate(HostTaskReport{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
		Status:      string(HostTaskReportStatusCompleted),
		Evidence:    []HostTaskEvidence{{ID: "terminal-ev", HostID: "host-a", Source: EvidenceSourceHumanTerminal, RedactionStatus: RedactionStatusApplied}},
	})
	if !errors.Is(err, ErrHumanTerminalEvidenceRejected) {
		t.Fatalf("err = %v, want ErrHumanTerminalEvidenceRejected", err)
	}
}

func TestHostTaskReportValidatorAllowsExplicitSanitizedArtifactRef(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	err := validator.Validate(HostTaskReport{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
		Status:      string(HostTaskReportStatusCompleted),
		Evidence: []HostTaskEvidence{{
			ID:              "artifact-ev",
			HostID:          "host-a",
			Source:          EvidenceSourceArtifact,
			ArtifactRef:     "artifact://mission-report/host-a/step-a/check",
			RedactionStatus: RedactionStatusApplied,
		}},
	})
	if err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestHostTaskReportValidatorAcceptsRuntimeChildTerminalStatuses(t *testing.T) {
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	for _, status := range []HostTaskReportStatus{
		HostTaskReportStatusCompleted,
		HostTaskReportStatusBlockedApproval,
		HostTaskReportStatusBlockedEvidence,
		HostTaskReportStatusFailed,
		HostTaskReportStatusCancelled,
		HostTaskReportStatusTimeout,
	} {
		t.Run(string(status), func(t *testing.T) {
			err := validator.Validate(HostTaskReport{
				MissionID:   "mission-report",
				PlanStepID:  "step-a",
				HostAgentID: "child-a",
				HostID:      "host-a",
				Status:      string(status),
				Evidence: []HostTaskEvidence{{
					ID:              "ev-" + string(status),
					HostID:          "host-a",
					Source:          EvidenceSourceHostCommandTool,
					RedactionStatus: RedactionStatusNotRequired,
				}},
			})
			if err != nil {
				t.Fatalf("Validate(%s) error = %v", status, err)
			}
		})
	}
}

func TestHostTaskReportValidatorRedactsSensitiveCommandSummaries(t *testing.T) {
	report := HostTaskReport{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
		Status:      string(HostTaskReportStatusCompleted),
		Commands: []HostTaskCommandRecord{{
			Command: "check --token plain-secret",
			Status:  "success",
			Summary: "password=plain-secret was present in environment",
		}},
		Evidence: []HostTaskEvidence{{ID: "ev-a", HostID: "host-a", Source: EvidenceSourceHostCommandTool, RedactionStatus: RedactionStatusApplied}},
	}
	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-report",
		PlanStepID:  "step-a",
		HostAgentID: "child-a",
		HostID:      "host-a",
	})
	sanitized, err := validator.Sanitize(report)
	if err != nil {
		t.Fatalf("Sanitize error = %v", err)
	}
	rendered := sanitized.Commands[0].Command + "\n" + sanitized.Commands[0].Summary
	if strings.Contains(rendered, "plain-secret") || !strings.Contains(rendered, "[REDACTED") {
		t.Fatalf("sanitized command summary = %q", rendered)
	}
}
