package planning

import (
	"fmt"
	"strings"
)

type PlanArtifactStatus string

const (
	PlanArtifactDraft           PlanArtifactStatus = "draft"
	PlanArtifactPendingApproval PlanArtifactStatus = "pending_approval"
	PlanArtifactApproved        PlanArtifactStatus = "approved"
	PlanArtifactRejected        PlanArtifactStatus = "rejected"
	PlanArtifactSuperseded      PlanArtifactStatus = "superseded"
)

type PlanArtifact struct {
	ID                  string             `json:"id"`
	Version             int                `json:"version"`
	Status              PlanArtifactStatus `json:"status"`
	Context             PlanContext        `json:"context"`
	RecommendedApproach []PlanApproachStep `json:"recommendedApproach"`
	Scope               PlanScope          `json:"scope"`
	Reuse               PlanReuse          `json:"reuse"`
	Verification        PlanVerification   `json:"verification"`
	OpenQuestions       []PlanQuestion     `json:"openQuestions,omitempty"`
	Approval            *PlanApprovalState `json:"approval,omitempty"`
	Rejections          []PlanRejection    `json:"rejections,omitempty"`
	Steps               []PlanStep         `json:"steps"`
	CreatedAt           string             `json:"createdAt,omitempty"`
	UpdatedAt           string             `json:"updatedAt,omitempty"`
}

type PlanContext struct {
	Summary string   `json:"summary,omitempty"`
	Facts   []string `json:"facts,omitempty"`
	Refs    []string `json:"refs,omitempty"`
}

type PlanApproachStep struct {
	ID        string   `json:"id,omitempty"`
	Summary   string   `json:"summary"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

type PlanScope struct {
	In             []string `json:"in,omitempty"`
	Out            []string `json:"out,omitempty"`
	ResourceScopes []string `json:"resourceScopes,omitempty"`
	AllowedActions []string `json:"allowedActions,omitempty"`
	RiskCeiling    string   `json:"riskCeiling,omitempty"`
}

type PlanReuse struct {
	ExistingPatterns []string `json:"existingPatterns,omitempty"`
	NewCodeNeeded    []string `json:"newCodeNeeded,omitempty"`
}

type PlanVerification struct {
	Checks       []string `json:"checks,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Status       string   `json:"status,omitempty"`
}

type PlanQuestion struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Status  string `json:"status,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type PlanApprovalState struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	ApprovedBy string   `json:"approvedBy,omitempty"`
	Scope      []string `json:"scope,omitempty"`
}

type PlanRejection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func (s PlanArtifactStatus) IsValid() bool {
	switch s {
	case PlanArtifactDraft, PlanArtifactPendingApproval, PlanArtifactApproved, PlanArtifactRejected, PlanArtifactSuperseded:
		return true
	default:
		return false
	}
}

func (a PlanArtifact) Validate() error {
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("plan artifact id is required")
	}
	if a.Version <= 0 {
		return fmt.Errorf("plan artifact version must be positive")
	}
	if a.Status == "" {
		return fmt.Errorf("plan artifact status is required")
	}
	if !a.Status.IsValid() {
		return fmt.Errorf("invalid plan artifact status %q", a.Status)
	}
	if !a.Context.hasContent() {
		return fmt.Errorf("context is required")
	}
	if len(a.RecommendedApproach) == 0 {
		return fmt.Errorf("recommendedApproach is required")
	}
	for i, step := range a.RecommendedApproach {
		if strings.TrimSpace(step.Summary) == "" {
			return fmt.Errorf("recommendedApproach[%d] summary is required", i)
		}
	}
	if !a.Scope.hasContent() {
		return fmt.Errorf("scope is required")
	}
	if !a.Verification.hasContent() {
		return fmt.Errorf("verification is required")
	}
	for i, question := range a.OpenQuestions {
		if strings.TrimSpace(question.ID) == "" {
			return fmt.Errorf("openQuestions[%d] id is required", i)
		}
		if strings.TrimSpace(question.Text) == "" {
			return fmt.Errorf("openQuestions[%d] text is required", i)
		}
	}
	if a.Status == PlanArtifactApproved && a.Approval == nil {
		return fmt.Errorf("approval is required for approved plan artifact")
	}
	if a.Status == PlanArtifactRejected && len(a.Rejections) == 0 {
		return fmt.Errorf("rejections are required for rejected plan artifact")
	}
	if a.Approval != nil {
		if strings.TrimSpace(a.Approval.ID) == "" {
			return fmt.Errorf("approval id is required")
		}
		if strings.TrimSpace(a.Approval.Status) == "" {
			return fmt.Errorf("approval status is required")
		}
	}
	for i, rejection := range a.Rejections {
		if strings.TrimSpace(rejection.ID) == "" {
			return fmt.Errorf("rejections[%d] id is required", i)
		}
		if strings.TrimSpace(rejection.Reason) == "" {
			return fmt.Errorf("rejections[%d] reason is required", i)
		}
	}
	if len(a.Steps) == 0 {
		return fmt.Errorf("plan steps are required")
	}
	if _, err := normalizePlanSteps(a.Steps, false); err != nil {
		return err
	}
	return validatePlanDependencies(a.Steps)
}

func (c PlanContext) hasContent() bool {
	return strings.TrimSpace(c.Summary) != "" || len(trimStringSlice(c.Facts)) > 0 || len(trimStringSlice(c.Refs)) > 0
}

func (s PlanScope) hasContent() bool {
	return len(trimStringSlice(s.In)) > 0 ||
		len(trimStringSlice(s.Out)) > 0 ||
		len(trimStringSlice(s.ResourceScopes)) > 0 ||
		len(trimStringSlice(s.AllowedActions)) > 0 ||
		strings.TrimSpace(s.RiskCeiling) != ""
}

func (v PlanVerification) hasContent() bool {
	return len(trimStringSlice(v.Checks)) > 0 ||
		len(trimStringSlice(v.EvidenceRefs)) > 0 ||
		strings.TrimSpace(v.Status) != ""
}

func validatePlanDependencies(steps []PlanStep) error {
	ids := map[string]bool{}
	for _, step := range steps {
		id := strings.TrimSpace(step.ID)
		if id != "" {
			ids[id] = true
		}
	}
	for i, step := range steps {
		for _, dep := range step.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep != "" && !ids[dep] {
				return fmt.Errorf("step[%d] dependsOn references unknown step %q", i, dep)
			}
		}
		for _, block := range step.Blocks {
			block = strings.TrimSpace(block)
			if block != "" && !ids[block] {
				return fmt.Errorf("step[%d] blocks references unknown step %q", i, block)
			}
		}
	}
	return nil
}
