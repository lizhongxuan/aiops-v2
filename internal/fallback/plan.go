package fallback

import (
	"encoding/json"
	"time"

	"aiops-v2/internal/actionproposal"
)

type ProposedAction struct {
	ToolName       string                            `json:"toolName"`
	ToolInput      json.RawMessage                   `json:"toolInput"`
	Risk           actionproposal.Risk               `json:"risk,omitempty"`
	TargetSummary  string                            `json:"targetSummary,omitempty"`
	ActionSummary  string                            `json:"actionSummary,omitempty"`
	RiskSummary    string                            `json:"riskSummary,omitempty"`
	Reason         string                            `json:"reason,omitempty"`
	ExpectedEffect string                            `json:"expectedEffect,omitempty"`
	Rollback       string                            `json:"rollback,omitempty"`
	Verification   []actionproposal.VerificationStep `json:"verification,omitempty"`
}

type RunbookMatchSummary struct {
	RunbookID string `json:"runbookId"`
	Score     int    `json:"score"`
	Coverage  string `json:"coverage,omitempty"`
}

type PlanExecRequest struct {
	SessionID      string                `json:"sessionId"`
	TurnID         string                `json:"turnId"`
	TenantID       string                `json:"tenantId,omitempty"`
	UserID         string                `json:"userId,omitempty"`
	IncidentID     string                `json:"incidentId"`
	Goal           string                `json:"goal"`
	WhyNoRunbook   string                `json:"whyNoRunbook"`
	EvidenceRefs   []string              `json:"evidenceRefs,omitempty"`
	Actions        []ProposedAction      `json:"actions"`
	RunbookMatches []RunbookMatchSummary `json:"runbookMatches,omitempty"`
}

type FallbackPlan struct {
	ID           string                          `json:"id"`
	IncidentID   string                          `json:"incidentId"`
	Goal         string                          `json:"goal"`
	WhyNoRunbook string                          `json:"whyNoRunbook"`
	EvidenceRefs []string                        `json:"evidenceRefs,omitempty"`
	Actions      []actionproposal.ActionProposal `json:"actions"`
	Risk         actionproposal.Risk             `json:"risk"`
	CreatedAt    time.Time                       `json:"createdAt"`
}

type PlanExecResult struct {
	Plan FallbackPlan `json:"plan"`
}

type ObserveResultRequest struct {
	PlanID        string `json:"planId"`
	ActionToken   string `json:"actionToken,omitempty"`
	ToolResultRef string `json:"toolResultRef,omitempty"`
	EvidenceRef   string `json:"evidenceRef,omitempty"`
	Failed        bool   `json:"failed,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ActionObservation struct {
	ActionToken   string    `json:"actionToken,omitempty"`
	ToolResultRef string    `json:"toolResultRef,omitempty"`
	EvidenceRef   string    `json:"evidenceRef,omitempty"`
	Failed        bool      `json:"failed,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	ObservedAt    time.Time `json:"observedAt"`
}
