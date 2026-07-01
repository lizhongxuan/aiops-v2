package opsrepair

import "aiops-v2/internal/opsmanual"

const (
	PhasePreflight = "preflight"
	PhaseExecute   = "execute"
	PhaseVerify    = "verify"
	PhaseRollback  = "rollback"
)

type PlanRequest struct {
	Frame    opsmanual.OperationFrame `json:"frame"`
	Evidence []RepairEvidence         `json:"evidence,omitempty"`
}

type RepairPlan struct {
	ID               string             `json:"id,omitempty"`
	Capability       string             `json:"capability,omitempty"`
	DiagnosisSummary string             `json:"diagnosis_summary,omitempty"`
	Options          []RepairOption     `json:"options,omitempty"`
	RequiresApproval bool               `json:"requires_approval,omitempty"`
	Verification     RepairVerification `json:"verification,omitempty"`
}

type RepairOption struct {
	ID        string       `json:"id,omitempty"`
	Title     string       `json:"title,omitempty"`
	RiskLevel string       `json:"risk_level,omitempty"`
	DataLoss  bool         `json:"data_loss,omitempty"`
	Steps     []RepairStep `json:"steps,omitempty"`
	WhenToUse []string     `json:"when_to_use,omitempty"`
}

type RepairStep struct {
	ID         string         `json:"id,omitempty"`
	Phase      string         `json:"phase,omitempty"`
	ReadOnly   bool           `json:"read_only,omitempty"`
	ActionRef  string         `json:"action_ref,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type RepairEvidence struct {
	ID         string         `json:"id,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Source     string         `json:"source,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type RepairVerification struct {
	RequiredEvidence []string `json:"required_evidence,omitempty"`
	Independent      bool     `json:"independent,omitempty"`
}
