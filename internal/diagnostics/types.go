package diagnostics

type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

type ToolFailureSemantic string

const (
	ToolFailurePolicyBlocked     ToolFailureSemantic = "policy_blocked"
	ToolFailureCommandNotAllowed ToolFailureSemantic = "command_not_allowed"
	ToolFailurePermissionDenied  ToolFailureSemantic = "permission_denied"
	ToolFailureTimeout           ToolFailureSemantic = "timeout"
	ToolFailureNonZeroExit       ToolFailureSemantic = "non_zero_exit"
	ToolFailureEmptyOutput       ToolFailureSemantic = "empty_output"
)

type DiagnosticTrace struct {
	TurnID           string          `json:"turnId,omitempty"`
	ScopeHash        string          `json:"scopeHash,omitempty"`
	ScopeSummary     string          `json:"scopeSummary,omitempty"`
	Hypotheses       []string        `json:"hypotheses,omitempty"`
	ObservedEvidence []string        `json:"observedEvidence,omitempty"`
	RefutingEvidence []string        `json:"refutingEvidence,omitempty"`
	MissingEvidence  []string        `json:"missingEvidence,omitempty"`
	ToolFailures     []ToolFailure   `json:"toolFailures,omitempty"`
	ManualBindingID  string          `json:"manualBindingId,omitempty"`
	Confidence       ConfidenceLevel `json:"confidence,omitempty"`
	ConfidenceReason string          `json:"confidenceReason,omitempty"`
	RequiresApproval bool            `json:"requiresApproval,omitempty"`
}

type ToolFailure struct {
	ToolName  string              `json:"toolName,omitempty"`
	Semantic  ToolFailureSemantic `json:"semantic,omitempty"`
	Detail    string              `json:"detail,omitempty"`
	Critical  bool                `json:"critical,omitempty"`
	ScopeHash string              `json:"scopeHash,omitempty"`
}

type EvidenceMatrixRow struct {
	Claim    string `json:"claim,omitempty"`
	Support  string `json:"support,omitempty"`
	Refute   string `json:"refute,omitempty"`
	Missing  string `json:"missing,omitempty"`
	Critical bool   `json:"critical,omitempty"`
}

type DiagnosticScope struct {
	Hash      string `json:"hash,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
}

type EvidenceStatus string

const (
	EvidenceStatusActive  EvidenceStatus = "active"
	EvidenceStatusStale   EvidenceStatus = "stale"
	EvidenceStatusBlocked EvidenceStatus = "blocked"
	EvidenceStatusMissing EvidenceStatus = "missing"
)

type DiagnosticFact struct {
	ScopeHash     string         `json:"scopeHash,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Status        EvidenceStatus `json:"status,omitempty"`
	DirectSupport bool           `json:"directSupport,omitempty"`
	Critical      bool           `json:"critical,omitempty"`
}

type ScopedText struct {
	ScopeHash string `json:"scopeHash,omitempty"`
	Text      string `json:"text,omitempty"`
}

type TraceBuildInput struct {
	TurnID               string
	CurrentScope         DiagnosticScope
	Facts                []DiagnosticFact
	Hypotheses           []ScopedText
	RefutingEvidence     []ScopedText
	ToolFailures         []ToolFailure
	ManualBindingID      string
	ManualBindingIsStale bool
	RequiresApproval     bool
}
