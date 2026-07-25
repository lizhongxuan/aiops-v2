package edit

import (
	"fmt"
	"strings"
)

type WorkflowPlanDiagnosticCode string

const (
	PlanDiagnosticUnknownAction      WorkflowPlanDiagnosticCode = "UNKNOWN_ACTION"
	PlanDiagnosticArgumentType       WorkflowPlanDiagnosticCode = "ARGUMENT_TYPE_MISMATCH"
	PlanDiagnosticPortMismatch       WorkflowPlanDiagnosticCode = "PORT_MISMATCH"
	PlanDiagnosticMissingSecret      WorkflowPlanDiagnosticCode = "MISSING_SECRET"
	PlanDiagnosticMissingEnvironment WorkflowPlanDiagnosticCode = "MISSING_ENVIRONMENT"
	PlanDiagnosticIllegalCycle       WorkflowPlanDiagnosticCode = "ILLEGAL_CYCLE"
	PlanDiagnosticUnresolved         WorkflowPlanDiagnosticCode = "UNRESOLVED_AMBIGUITY"
	PlanDiagnosticReferenceLocked    WorkflowPlanDiagnosticCode = "REFERENCE_LOCKED"
)

type WorkflowPlanDiagnostic struct {
	Code        WorkflowPlanDiagnosticCode `json:"code"`
	Severity    string                     `json:"severity"`
	Message     string                     `json:"message"`
	ClientRef   string                     `json:"clientRef,omitempty"`
	Path        string                     `json:"path,omitempty"`
	Suggestion  string                     `json:"suggestion,omitempty"`
	Requirement *WorkflowPlanUnresolved    `json:"requirement,omitempty"`
}

func (diagnostic WorkflowPlanDiagnostic) Validate() error {
	switch diagnostic.Code {
	case PlanDiagnosticUnknownAction, PlanDiagnosticArgumentType, PlanDiagnosticPortMismatch,
		PlanDiagnosticMissingSecret, PlanDiagnosticMissingEnvironment, PlanDiagnosticIllegalCycle,
		PlanDiagnosticUnresolved, PlanDiagnosticReferenceLocked:
	default:
		return fmt.Errorf("unknown workflow plan diagnostic code %q", diagnostic.Code)
	}
	if diagnostic.Severity != "error" && diagnostic.Severity != "warning" {
		return fmt.Errorf("workflow plan diagnostic severity must be error or warning")
	}
	if strings.TrimSpace(diagnostic.Message) == "" {
		return fmt.Errorf("workflow plan diagnostic message is required")
	}
	if diagnostic.Path != "" && !strings.HasPrefix(diagnostic.Path, "/") {
		return fmt.Errorf("workflow plan diagnostic path must be a JSON pointer")
	}
	if diagnostic.Requirement != nil {
		probe := WorkflowPlanIR{Unresolved: []WorkflowPlanUnresolved{*diagnostic.Requirement}, Risk: WorkflowPlanRiskExpectation{Level: "low"}}
		if err := validateWorkflowPlanRequirements(probe); err != nil {
			return err
		}
	}
	return nil
}
