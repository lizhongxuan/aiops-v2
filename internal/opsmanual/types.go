package opsmanual

type ManualStatus string

const (
	ManualStatusDraft      ManualStatus = "draft"
	ManualStatusVerified   ManualStatus = "verified"
	ManualStatusDeprecated ManualStatus = "deprecated"
)

type DecisionState string

const (
	DecisionDirectExecute DecisionState = "direct_execute"
	DecisionNeedInfo      DecisionState = "need_info"
	DecisionAdapt         DecisionState = "adapt"
	DecisionReference     DecisionState = "reference_only"
	DecisionNoMatch       DecisionState = "no_match"
)

const (
	DecisionDirect       = DecisionDirectExecute
	DecisionNeedMoreInfo = DecisionNeedInfo
)

type WorkflowRef struct {
	WorkflowID      string `json:"workflow_id"`
	WorkflowVersion string `json:"workflow_version,omitempty"`
	WorkflowDigest  string `json:"workflow_digest,omitempty"`
	StorageURI      string `json:"storage_uri,omitempty"`
}

type OpsManual struct {
	ID               string                   `json:"id"`
	ManualFamilyID   string                   `json:"manual_family_id,omitempty"`
	Title            string                   `json:"title"`
	Status           ManualStatus             `json:"status"`
	Version          string                   `json:"version,omitempty"`
	Owner            string                   `json:"owner,omitempty"`
	WorkflowRef      WorkflowRef              `json:"workflow_ref"`
	Operation        OperationProfile         `json:"operation"`
	Applicability    ApplicabilityProfile     `json:"applicability"`
	RequiredContext  RequiredContext          `json:"required_context"`
	ParameterRules   map[string]ParameterRule `json:"parameter_rules,omitempty"`
	Preconditions    []string                 `json:"preconditions"`
	Validation       []string                 `json:"validation"`
	CannotUseWhen    []string                 `json:"cannot_use_when"`
	RiskNotes        []string                 `json:"risk_notes,omitempty"`
	DocumentMarkdown string                   `json:"document_markdown"`
	SearchDoc        string                   `json:"search_doc,omitempty"`
	Metadata         map[string]any           `json:"metadata,omitempty"`
	CreatedAt        string                   `json:"created_at,omitempty"`
	UpdatedAt        string                   `json:"updated_at,omitempty"`
}

type OperationProfile struct {
	TargetType string `json:"target_type"`
	Action     string `json:"action"`
	RiskLevel  string `json:"risk_level,omitempty"`
	Stateful   bool   `json:"stateful,omitempty"`
}

type ApplicabilityProfile struct {
	Middleware         string   `json:"middleware,omitempty"`
	MiddlewareVersions []string `json:"middleware_versions,omitempty"`
	OS                 []string `json:"os,omitempty"`
	Platform           []string `json:"platform,omitempty"`
	ExecutionSurface   []string `json:"execution_surface,omitempty"`
	Topology           []string `json:"topology,omitempty"`
	InternetRequired   string   `json:"internet_required,omitempty"`
}

type RequiredContext struct {
	RequiredInputs   []string `json:"required_inputs,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
	OptionalEvidence []string `json:"optional_evidence,omitempty"`
}

type ParameterRule struct {
	Source     string `json:"source,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Validation string `json:"validation,omitempty"`
}

type OperationFrame struct {
	Intent         string             `json:"intent,omitempty"`
	ObjectType     string             `json:"object_type,omitempty"`
	OperationType  string             `json:"operation_type,omitempty"`
	Target         OperationTarget    `json:"target"`
	Operation      OperationProfile   `json:"operation"`
	TargetScope    TargetScope        `json:"target_scope,omitempty"`
	Environment    EnvironmentProfile `json:"environment"`
	Evidence       EvidenceProfile    `json:"evidence"`
	Risk           RiskProfile        `json:"risk"`
	RequiredParams map[string]any     `json:"required_params,omitempty"`
	RawText        string             `json:"raw_text,omitempty"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
}

type OperationTarget struct {
	Type string `json:"type,omitempty"`
	Name string `json:"name,omitempty"`
}

type TargetScope struct {
	Hosts     []string `json:"hosts,omitempty"`
	Namespace string   `json:"namespace,omitempty"`
	Service   string   `json:"service,omitempty"`
	Cluster   string   `json:"cluster,omitempty"`
}

type EnvironmentProfile struct {
	Env              string `json:"env,omitempty"`
	OS               string `json:"os,omitempty"`
	OSVersion        string `json:"os_version,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Runtime          string `json:"runtime,omitempty"`
	ExecutionSurface string `json:"execution_surface,omitempty"`
	PackageManager   string `json:"package_manager,omitempty"`
	ContainerRuntime string `json:"container_runtime,omitempty"`
}

type EvidenceProfile struct {
	Provided []string `json:"provided,omitempty"`
	Missing  []string `json:"missing,omitempty"`
}

type RiskProfile struct {
	Level            string `json:"level,omitempty"`
	Reason           string `json:"reason,omitempty"`
	DataMutation     bool   `json:"data_mutation,omitempty"`
	ServiceRestart   bool   `json:"service_restart,omitempty"`
	ProductionImpact string `json:"production_impact,omitempty"`
}

type ManualMatch struct {
	Manual                 OpsManual        `json:"manual"`
	State                  DecisionState    `json:"state"`
	Reasons                []string         `json:"reasons,omitempty"`
	MissingContext         []string         `json:"missing_context,omitempty"`
	CompatibilityGaps      []string         `json:"compatibility_gaps,omitempty"`
	RecommendedNextActions []string         `json:"recommended_next_actions,omitempty"`
	RunRecordSummary       RunRecordSummary `json:"run_record_summary,omitempty"`
}

type SearchOpsManualsRequest struct {
	Text           string         `json:"text,omitempty"`
	OperationFrame OperationFrame `json:"operation_frame,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Limit          int            `json:"limit,omitempty"`
}

type SearchOpsManualsResult struct {
	Decision              DecisionState     `json:"decision"`
	Summary               string            `json:"summary"`
	OperationFrame        OperationFrame    `json:"operation_frame"`
	Manuals               []SearchManualHit `json:"manuals"`
	NextQuestions         []string          `json:"next_questions,omitempty"`
	RecommendedNextAction string            `json:"recommended_next_action,omitempty"`
	SearchedFields        []string          `json:"searched_fields,omitempty"`
}

type SearchManualHit struct {
	Manual            OpsManual        `json:"manual"`
	BoundWorkflowID   string           `json:"bound_workflow_id,omitempty"`
	MatchLevel        string           `json:"match_level"`
	UsableMode        DecisionState    `json:"usable_mode"`
	MatchedFields     []string         `json:"matched_fields,omitempty"`
	MissingFields     []string         `json:"missing_fields,omitempty"`
	EnvironmentDiffs  []string         `json:"environment_diffs,omitempty"`
	BlockedReasons    []string         `json:"blocked_reasons,omitempty"`
	RecommendedAction string           `json:"recommended_action,omitempty"`
	RunRecordSummary  RunRecordSummary `json:"run_record_summary,omitempty"`
}

type ManualCandidate struct {
	ID               string    `json:"id"`
	SourceType       string    `json:"source_type"`
	SourceRefs       []string  `json:"source_refs,omitempty"`
	ProposedManual   OpsManual `json:"proposed_manual"`
	ValidationReport []string  `json:"validation_report,omitempty"`
	ReviewStatus     string    `json:"review_status"`
	Reviewer         string    `json:"reviewer,omitempty"`
	ReviewNote       string    `json:"review_note,omitempty"`
	CreatedAt        string    `json:"created_at,omitempty"`
	UpdatedAt        string    `json:"updated_at,omitempty"`
}

type RunRecord struct {
	ID                  string             `json:"id"`
	ManualID            string             `json:"manual_id,omitempty"`
	WorkflowID          string             `json:"workflow_id"`
	WorkflowVersion     string             `json:"workflow_version,omitempty"`
	WorkflowDigest      string             `json:"workflow_digest,omitempty"`
	OperationFrame      OperationFrame     `json:"operation_frame"`
	EnvironmentSnapshot EnvironmentProfile `json:"environment_snapshot"`
	RedactedParameters  map[string]any     `json:"redacted_parameters,omitempty"`
	ApprovalRef         string             `json:"approval_ref,omitempty"`
	DryRunStatus        string             `json:"dry_run_status,omitempty"`
	ExecutionStatus     string             `json:"execution_status,omitempty"`
	ValidationStatus    string             `json:"validation_status,omitempty"`
	RollbackStatus      string             `json:"rollback_status,omitempty"`
	FailureReason       string             `json:"failure_reason,omitempty"`
	Operator            string             `json:"operator,omitempty"`
	StartedAt           string             `json:"started_at,omitempty"`
	CompletedAt         string             `json:"completed_at,omitempty"`
}

type RunRecordSummary struct {
	SuccessCount int    `json:"success_count,omitempty"`
	FailureCount int    `json:"failure_count,omitempty"`
	RecentResult string `json:"recent_result,omitempty"`
	LastRunAt    string `json:"last_run_at,omitempty"`
}
