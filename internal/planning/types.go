package planning

type PlanStatus string

const (
	PlanStatusActive    PlanStatus = "active"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusFailed    PlanStatus = "failed"
	PlanStatusCancelled PlanStatus = "cancelled"
)

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusBlocked    StepStatus = "blocked"
	StepStatusFailed     StepStatus = "failed"
	StepStatusCancelled  StepStatus = "cancelled"
)

type PlanState struct {
	Status PlanStatus `json:"status"`
	Steps  []PlanStep `json:"steps"`
}

type PlanStep struct {
	ID      string     `json:"id,omitempty"`
	Text    string     `json:"text"`
	Status  StepStatus `json:"status"`
	Summary string     `json:"summary,omitempty"`
}

type UpdatePlanArgs struct {
	Status PlanStatus `json:"status,omitempty"`
	Steps  []PlanStep `json:"steps"`
}

func (s PlanStatus) IsValid() bool {
	switch s {
	case PlanStatusActive, PlanStatusCompleted, PlanStatusFailed, PlanStatusCancelled:
		return true
	default:
		return false
	}
}

func (s PlanStatus) IsFinal() bool {
	switch s {
	case PlanStatusCompleted, PlanStatusFailed, PlanStatusCancelled:
		return true
	default:
		return false
	}
}

func (s StepStatus) IsValid() bool {
	switch s {
	case StepStatusPending, StepStatusInProgress, StepStatusCompleted, StepStatusBlocked, StepStatusFailed, StepStatusCancelled:
		return true
	default:
		return false
	}
}
