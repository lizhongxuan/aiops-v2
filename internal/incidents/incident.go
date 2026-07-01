package incidents

import "time"

type IncidentStatus string

const (
	IncidentStatusOpen   IncidentStatus = "open"
	IncidentStatusClosed IncidentStatus = "closed"
)

type IncidentCase struct {
	ID                   string           `json:"id"`
	ExternalID           string           `json:"externalId,omitempty"`
	Title                string           `json:"title"`
	Severity             string           `json:"severity,omitempty"`
	Status               IncidentStatus   `json:"status"`
	Source               string           `json:"source,omitempty"`
	Environment          string           `json:"environment,omitempty"`
	BusinessCapability   string           `json:"businessCapability,omitempty"`
	BusinessCapabilityID string           `json:"businessCapabilityId,omitempty"`
	AffectedServices     []string         `json:"affectedServices,omitempty"`
	EvidenceRefs         []string         `json:"evidenceRefs,omitempty"`
	Evidence             []EvidenceRef    `json:"evidence,omitempty"`
	Actions              []ActionRecord   `json:"actions,omitempty"`
	Approvals            []ApprovalRecord `json:"approvals,omitempty"`
	Hypotheses           []Hypothesis     `json:"hypotheses,omitempty"`
	Postmortem           *PostmortemDraft `json:"postmortem,omitempty"`
	CreatedAt            time.Time        `json:"createdAt"`
	UpdatedAt            time.Time        `json:"updatedAt"`
	ClosedAt             *time.Time       `json:"closedAt,omitempty"`
}

type ActionRecord struct {
	ID        string    `json:"id,omitempty"`
	ToolName  string    `json:"toolName,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	Risk      string    `json:"risk,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

type ApprovalRecord struct {
	ID        string    `json:"id,omitempty"`
	Command   string    `json:"command,omitempty"`
	Decision  string    `json:"decision,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}
