package opsmanual

type WorkflowManualGenerationRequest struct {
	WorkflowID      string
	WorkflowVersion string
	WorkflowDigest  string
	StorageURI      string
	RawYAML         []byte
	ActionSpecs     []ActionSpecSummary
	RecentRuns      []RunRecord
	Options         WorkflowManualGenerationOptions
}

type WorkflowManualGenerationOptions struct {
	IncludeRecentRunRecords bool
	UseLLMSummary           bool
	GeneratedBy             string
}

type WorkflowManualGenerationResult struct {
	Candidate        ManualCandidate             `json:"candidate"`
	ValidationReport ManualCandidateValidation   `json:"validation_report"`
	UserSummary      ManualGenerationUserSummary `json:"user_summary"`
}

type ActionSpecSummary struct {
	Action       string   `json:"action"`
	Title        string   `json:"title,omitempty"`
	Category     string   `json:"category,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	RequiredArgs []string `json:"required_args,omitempty"`
	Outputs      []string `json:"outputs,omitempty"`
	Deprecated   bool     `json:"deprecated,omitempty"`
}

type WorkflowManualAnalysis struct {
	WorkflowID      string
	WorkflowVersion string
	WorkflowDigest  string
	StorageURI      string
	Name            string
	Description     string
	Operation       OperationProfile
	Applicability   ApplicabilityProfile
	RequiredContext RequiredContext
	ParameterRules  map[string]ParameterRule
	Steps           []WorkflowStepSummary
	GraphStages     []WorkflowGraphStageSummary
	ActionRisks     []WorkflowActionRiskSummary
	SecretFindings  []WorkflowSecretFinding
	ValidationHints []string
	CannotUseHints  []string
	Warnings        []ValidationIssue
	Evidence        map[string][]string
	XOpsManual      map[string]any
	RecentRuns      []RunRecord
}

type WorkflowStepSummary struct {
	Name       string
	Action     string
	Targets    []string
	MustVars   []string
	ExpectVars []string
	ReadOnly   bool
	Risky      bool
	Stage      string
	Evidence   string
}

type WorkflowGraphStageSummary struct {
	ID       string
	Label    string
	Type     string
	StepName string
	Stage    string
	Evidence string
}

type WorkflowActionRiskSummary struct {
	Action           string
	StepName         string
	Risk             string
	DataMutation     bool
	ServiceRestart   bool
	RequiresApproval bool
	Evidence         string
}

type WorkflowSecretFinding struct {
	Field      string
	Kind       string
	HasDefault bool
	SecretRef  bool
	Evidence   string
}

type ManualCandidateValidation struct {
	Status   string            `json:"status,omitempty"`
	Passed   []ValidationIssue `json:"passed,omitempty"`
	Warnings []ValidationIssue `json:"warnings,omitempty"`
	Blocking []ValidationIssue `json:"blocking,omitempty"`
}

type ValidationIssue struct {
	Code     string `json:"code"`
	Field    string `json:"field,omitempty"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

type ManualGenerationUserSummary struct {
	Understood []string `json:"understood,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	NextSteps  []string `json:"next_steps,omitempty"`
}
