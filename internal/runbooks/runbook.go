package runbooks

import (
	"encoding/json"

	"aiops-v2/internal/actionproposal"
)

type Scope struct {
	Modules      []string `json:"modules,omitempty" yaml:"modules,omitempty"`
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Services     []string `json:"services,omitempty" yaml:"services,omitempty"`
	Environments []string `json:"environments,omitempty" yaml:"environments,omitempty"`
}

type VerifyStep struct {
	Tool     string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	Input    map[string]any `json:"input,omitempty" yaml:"input,omitempty"`
	Expected map[string]any `json:"expected,omitempty" yaml:"expected,omitempty"`
}

type Step struct {
	ID               string              `json:"id" yaml:"id"`
	Title            string              `json:"title,omitempty" yaml:"title,omitempty"`
	Tool             string              `json:"tool" yaml:"tool"`
	Input            map[string]any      `json:"input,omitempty" yaml:"input,omitempty"`
	Condition        string              `json:"condition,omitempty" yaml:"condition,omitempty"`
	Required         *bool               `json:"required,omitempty" yaml:"required,omitempty"`
	Risk             actionproposal.Risk `json:"risk,omitempty" yaml:"risk,omitempty"`
	ApprovalRequired bool                `json:"approvalRequired,omitempty" yaml:"approvalRequired,omitempty"`
	ExpectedEffect   string              `json:"expectedEffect,omitempty" yaml:"expectedEffect,omitempty"`
	Rollback         string              `json:"rollback,omitempty" yaml:"rollback,omitempty"`
	Verify           []VerifyStep        `json:"verify,omitempty" yaml:"verify,omitempty"`
}

func (s Step) IsRequired() bool {
	if s.Required == nil {
		return true
	}
	return *s.Required
}

type Runbook struct {
	ID          string              `json:"id" yaml:"id"`
	Name        string              `json:"name" yaml:"name"`
	Description string              `json:"description,omitempty" yaml:"description,omitempty"`
	Scope       Scope               `json:"scope" yaml:"scope"`
	Risk        actionproposal.Risk `json:"risk,omitempty" yaml:"risk,omitempty"`
	Steps       []Step              `json:"steps" yaml:"steps"`
}

type Candidate struct {
	Runbook Runbook `json:"runbook"`
	Score   int     `json:"score"`
	Reason  string  `json:"reason"`
}

type MatchRequest struct {
	Symptom     string `json:"symptom,omitempty"`
	Capability  string `json:"capability,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

type StepState string

const (
	StepPending  StepState = "pending"
	StepSkipped  StepState = "skipped"
	StepProposed StepState = "proposed"
	StepObserved StepState = "observed"
	StepFailed   StepState = "failed"
)

type StepProgress struct {
	State         StepState `json:"state"`
	ToolResultRef string    `json:"toolResultRef,omitempty"`
	EvidenceRef   string    `json:"evidenceRef,omitempty"`
	Reason        string    `json:"reason,omitempty"`
}

type RunbookInstance struct {
	ID            string                  `json:"id"`
	RunbookID     string                  `json:"runbookId"`
	IncidentID    string                  `json:"incidentId,omitempty"`
	Status        string                  `json:"status"`
	Context       map[string]any          `json:"context,omitempty"`
	Evidence      map[string]any          `json:"evidence,omitempty"`
	StepProgress  map[string]StepProgress `json:"stepProgress,omitempty"`
	CreatedAtUnix int64                   `json:"createdAtUnix"`
	UpdatedAtUnix int64                   `json:"updatedAtUnix"`
}

func cloneInstance(instance RunbookInstance) RunbookInstance {
	instance.Context = cloneMap(instance.Context)
	instance.Evidence = cloneMap(instance.Evidence)
	instance.StepProgress = cloneStepProgress(instance.StepProgress)
	return instance
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	data, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func cloneStepProgress(in map[string]StepProgress) map[string]StepProgress {
	if in == nil {
		return nil
	}
	out := make(map[string]StepProgress, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
