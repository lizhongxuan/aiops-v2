package actionproposal

import (
	"encoding/json"
	"time"
)

type Source string

const (
	SourceRunbook        Source = "runbook"
	SourceOpsManual      Source = "ops_manual"
	SourceWorkflow       Source = "workflow"
	SourceAIChatDirect   Source = "ai_chat_direct"
	SourceMultiHostAgent Source = "multi_host_agent"
	SourceFallback       Source = "fallback"
	SourceBreakGlass     Source = "break_glass"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type VerificationStep struct {
	ToolName string          `json:"toolName,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}

type ActionProposal struct {
	SessionID        string             `json:"sessionId"`
	TurnID           string             `json:"turnId"`
	TenantID         string             `json:"tenantId,omitempty"`
	UserID           string             `json:"userId,omitempty"`
	IncidentID       string             `json:"incidentId,omitempty"`
	Source           Source             `json:"source"`
	ToolName         string             `json:"toolName"`
	ToolInput        json.RawMessage    `json:"toolInput"`
	TargetSummary    string             `json:"targetSummary,omitempty"`
	ActionSummary    string             `json:"actionSummary,omitempty"`
	Risk             Risk               `json:"risk"`
	RiskSummary      string             `json:"riskSummary,omitempty"`
	ApprovalRequired bool               `json:"approvalRequired"`
	Reason           string             `json:"reason,omitempty"`
	EvidenceRefs     []string           `json:"evidenceRefs,omitempty"`
	RunbookID        string             `json:"runbookId,omitempty"`
	RunbookStepID    string             `json:"runbookStepId,omitempty"`
	RunbookStepTitle string             `json:"runbookStepTitle,omitempty"`
	ExpectedEffect   string             `json:"expectedEffect,omitempty"`
	Rollback         string             `json:"rollback,omitempty"`
	Verification     []VerificationStep `json:"verification,omitempty"`
	ActionToken      string             `json:"actionToken"`
	ExpiresAt        time.Time          `json:"expiresAt"`
}

type ActionTokenClaims struct {
	SessionID        string    `json:"sessionId"`
	TurnID           string    `json:"turnId"`
	TenantID         string    `json:"tenantId,omitempty"`
	UserID           string    `json:"userId,omitempty"`
	IncidentID       string    `json:"incidentId,omitempty"`
	ToolName         string    `json:"toolName"`
	InputHash        string    `json:"inputHash"`
	Source           Source    `json:"source"`
	TargetSummary    string    `json:"targetSummary,omitempty"`
	ActionSummary    string    `json:"actionSummary,omitempty"`
	Risk             Risk      `json:"risk"`
	RiskSummary      string    `json:"riskSummary,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	RunbookID        string    `json:"runbookId,omitempty"`
	RunbookStepID    string    `json:"runbookStepId,omitempty"`
	RunbookStepTitle string    `json:"runbookStepTitle,omitempty"`
	ExpectedEffect   string    `json:"expectedEffect,omitempty"`
	Rollback         string    `json:"rollback,omitempty"`
	ExpiresAt        time.Time `json:"expiresAt"`
}
