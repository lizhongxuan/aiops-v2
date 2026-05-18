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
	ID                 string                   `json:"id"`
	ManualFamilyID     string                   `json:"manual_family_id,omitempty"`
	Title              string                   `json:"title"`
	Status             ManualStatus             `json:"status"`
	Tags               []string                 `json:"tags,omitempty"`
	Version            string                   `json:"version,omitempty"`
	Owner              string                   `json:"owner,omitempty"`
	WorkflowRef        WorkflowRef              `json:"workflow_ref"`
	Operation          OperationProfile         `json:"operation"`
	Applicability      ApplicabilityProfile     `json:"applicability"`
	RequiredContext    RequiredContext          `json:"required_context"`
	RetrievalProfile   RetrievalProfile         `json:"retrieval_profile,omitempty"`
	RunnableConditions RunnableConditions       `json:"runnable_conditions,omitempty"`
	PreflightProbe     PreflightProbe           `json:"preflight_probe,omitempty"`
	Diagnosis          DiagnosisProfile         `json:"diagnosis,omitempty"`
	RiskPolicy         RiskPolicy               `json:"risk_policy,omitempty"`
	FallbackGuide      FallbackGuide            `json:"fallback_guide,omitempty"`
	Verification       VerificationProfile      `json:"verification,omitempty"`
	ParameterRules     map[string]ParameterRule `json:"parameter_rules,omitempty"`
	Preconditions      []string                 `json:"preconditions"`
	Validation         []string                 `json:"validation"`
	CannotUseWhen      []string                 `json:"cannot_use_when"`
	RiskNotes          []string                 `json:"risk_notes,omitempty"`
	DocumentMarkdown   string                   `json:"document_markdown"`
	SearchDoc          string                   `json:"search_doc,omitempty"`
	Metadata           map[string]any           `json:"metadata,omitempty"`
	CreatedAt          string                   `json:"created_at,omitempty"`
	UpdatedAt          string                   `json:"updated_at,omitempty"`
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

type DiagnosisProfile struct {
	ApplicableSymptoms       []string `json:"applicable_symptoms,omitempty"`
	NotApplicableWhen        []string `json:"not_applicable_when,omitempty"`
	AllowedEvidenceSources   []string `json:"allowed_evidence_sources,omitempty"`
	RecommendedEvidenceOrder []string `json:"recommended_evidence_order,omitempty"`
	KeyJudgmentRules         []string `json:"key_judgment_rules,omitempty"`
	CommonMisdiagnoses       []string `json:"common_misdiagnoses,omitempty"`
	ConfidenceCriteria       []string `json:"confidence_criteria,omitempty"`
	ConservativeWording      []string `json:"conservative_wording,omitempty"`
	ApprovalRequiredActions  []string `json:"approval_required_actions,omitempty"`
	MinimumRiskNextSteps     []string `json:"minimum_risk_next_steps,omitempty"`
}

type ParameterRule struct {
	Source       string `json:"source,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Validation   string `json:"validation,omitempty"`
	DefaultValue any    `json:"default_value,omitempty"`
}

type ParamRequirement struct {
	ID            string   `json:"id"`
	Label         string   `json:"label,omitempty"`
	Type          string   `json:"type"`
	Required      bool     `json:"required,omitempty"`
	Sensitive     bool     `json:"sensitive,omitempty"`
	DefaultSource string   `json:"default_source,omitempty"`
	DefaultValue  any      `json:"default_value,omitempty"`
	DependsOn     []string `json:"depends_on,omitempty"`
	ResolverHints []string `json:"resolver_hints,omitempty"`
	AskUserWhen   []string `json:"ask_user_when,omitempty"`
	UIControl     string   `json:"ui_control,omitempty"`
}

type ParamCandidate struct {
	Value            any     `json:"value"`
	Label            string  `json:"label,omitempty"`
	Hint             string  `json:"hint,omitempty"`
	Source           string  `json:"source,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`
	Evidence         string  `json:"evidence,omitempty"`
	FreshnessSeconds int     `json:"freshness_seconds,omitempty"`
}

type ResolvedParam struct {
	ID                    string         `json:"id"`
	Value                 any            `json:"value"`
	Source                string         `json:"source,omitempty"`
	Confidence            float64        `json:"confidence,omitempty"`
	Evidence              string         `json:"evidence,omitempty"`
	ConfirmedByUser       bool           `json:"confirmed_by_user,omitempty"`
	NeedsUserConfirmation bool           `json:"needs_user_confirmation,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
}

type MissingParam struct {
	ParamRequirement
	Reason string `json:"reason,omitempty"`
}

type AmbiguousParam struct {
	ParamRequirement
	Reason     string           `json:"reason,omitempty"`
	Candidates []ParamCandidate `json:"candidates,omitempty"`
}

type ParamResolverLog struct {
	Resolver  string `json:"resolver"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

type ParamResolutionNode struct {
	ID           string             `json:"id"`
	Requirement  ParamRequirement   `json:"requirement"`
	Status       string             `json:"status"`
	Resolved     *ResolvedParam     `json:"resolved,omitempty"`
	Missing      *MissingParam      `json:"missing,omitempty"`
	Ambiguous    *AmbiguousParam    `json:"ambiguous,omitempty"`
	Dependencies []string           `json:"dependencies,omitempty"`
	ResolverLog  []ParamResolverLog `json:"resolver_log,omitempty"`
}

type ParamResolutionEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type ParamResolutionGraph struct {
	Nodes []ParamResolutionNode `json:"nodes"`
	Edges []ParamResolutionEdge `json:"edges,omitempty"`
}

type ParamResolutionStatus string

const (
	ParamResolutionResolved      ParamResolutionStatus = "resolved"
	ParamResolutionNeedUserInput ParamResolutionStatus = "need_user_input"
	ParamResolutionAmbiguous     ParamResolutionStatus = "ambiguous"
	ParamResolutionUnresolved    ParamResolutionStatus = "unresolved"
)

type ResolveOpsManualParamsRequest struct {
	RequestText    string         `json:"request_text,omitempty"`
	ManualID       string         `json:"manual_id,omitempty"`
	WorkflowID     string         `json:"workflow_id,omitempty"`
	OperationFrame OperationFrame `json:"operation_frame"`
	KnownParams    map[string]any `json:"known_params,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type ParamResolutionFormField struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Type        string           `json:"type,omitempty"`
	Required    bool             `json:"required,omitempty"`
	Sensitive   bool             `json:"sensitive,omitempty"`
	UIControl   string           `json:"ui_control,omitempty"`
	Placeholder string           `json:"placeholder,omitempty"`
	Default     any              `json:"default,omitempty"`
	Candidates  []ParamCandidate `json:"candidates,omitempty"`
}

type ParamResolutionResult struct {
	Status          ParamResolutionStatus      `json:"status"`
	ManualID        string                     `json:"manual_id,omitempty"`
	WorkflowID      string                     `json:"workflow_id,omitempty"`
	OperationFrame  OperationFrame             `json:"operation_frame"`
	Graph           ParamResolutionGraph       `json:"graph"`
	ResolvedParams  []ResolvedParam            `json:"resolved_params,omitempty"`
	MissingParams   []MissingParam             `json:"missing_params,omitempty"`
	AmbiguousParams []AmbiguousParam           `json:"ambiguous_params,omitempty"`
	Fields          []ParamResolutionFormField `json:"fields,omitempty"`
	NextAction      string                     `json:"next_action,omitempty"`
	ArtifactType    string                     `json:"artifact_type,omitempty"`
	CreatedAt       string                     `json:"created_at,omitempty"`
}

type RetrievalProfile struct {
	Aliases          map[string][]string `json:"aliases,omitempty"`
	Keywords         []string            `json:"keywords,omitempty"`
	NegativeKeywords []string            `json:"negative_keywords,omitempty"`
	EmbeddingText    string              `json:"embedding_text,omitempty"`
	MinScore         ScoreThresholds     `json:"min_score,omitempty"`
}

type ScoreThresholds struct {
	Candidate     float64 `json:"candidate,omitempty"`
	DirectExecute float64 `json:"direct_execute,omitempty"`
}

type RunnableConditions struct {
	RequiredParams      []string `json:"required_params,omitempty"`
	AllowedEnvironments []string `json:"allowed_environments,omitempty"`
	MaxRiskLevel        string   `json:"max_risk_level,omitempty"`
	RequiresApproval    bool     `json:"requires_approval,omitempty"`
}

type PreflightProbe struct {
	ID              string   `json:"id,omitempty"`
	Type            string   `json:"type,omitempty"`
	Action          string   `json:"action,omitempty"`
	ReadOnly        bool     `json:"read_only,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty"`
	RequiredOutputs []string `json:"required_outputs,omitempty"`
}

type RiskPolicy struct {
	BlastRadius          string   `json:"blast_radius,omitempty"`
	DataMutation         bool     `json:"data_mutation,omitempty"`
	ServiceRestart       string   `json:"service_restart,omitempty"`
	ApprovalRequiredWhen []string `json:"approval_required_when,omitempty"`
}

type FallbackGuide struct {
	Mode        string   `json:"mode,omitempty"`
	MarkdownRef string   `json:"markdown_ref,omitempty"`
	Steps       []string `json:"steps,omitempty"`
}

type VerificationProfile struct {
	LastVerifiedAt       string `json:"last_verified_at,omitempty"`
	VerifiedBy           string `json:"verified_by,omitempty"`
	RequiredRunnerDryRun bool   `json:"required_runner_dry_run,omitempty"`
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
	ScoreBreakdown    ScoreBreakdown   `json:"score_breakdown,omitempty"`
	PreflightStatus   PreflightStatus  `json:"preflight_status,omitempty"`
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
	SuccessCount        int    `json:"success_count,omitempty"`
	FailureCount        int    `json:"failure_count,omitempty"`
	RecentResult        string `json:"recent_result,omitempty"`
	LatestStatus        string `json:"latest_status,omitempty"`
	LastRunAt           string `json:"last_run_at,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
	Suppressed          bool   `json:"suppressed,omitempty"`
	SuppressedReason    string `json:"suppressed_reason,omitempty"`
}

type ScoreBreakdown struct {
	StructuralScore float64 `json:"structural_score,omitempty"`
	KeywordScore    float64 `json:"keyword_score,omitempty"`
	VectorScore     float64 `json:"vector_score,omitempty"`
	RunHistoryScore float64 `json:"run_history_score,omitempty"`
	Penalty         float64 `json:"penalty,omitempty"`
	FinalScore      float64 `json:"final_score,omitempty"`
}

type PreflightStatus string

const (
	PreflightStatusNotRun        PreflightStatus = "not_run"
	PreflightStatusPassed        PreflightStatus = "passed"
	PreflightStatusFailed        PreflightStatus = "failed"
	PreflightStatusBlocked       PreflightStatus = "blocked"
	PreflightStatusNotApplicable PreflightStatus = "not_applicable"
	PreflightStatusUnknown       PreflightStatus = "unknown"
)

type PreflightRequest struct {
	ManualID       string         `json:"manual_id"`
	WorkflowID     string         `json:"workflow_id,omitempty"`
	OperationFrame OperationFrame `json:"operation_frame"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	RequestedBy    string         `json:"requested_by,omitempty"`
	TriggeredBy    string         `json:"triggered_by,omitempty"`
}

type PreflightEvidence struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Value  any    `json:"value,omitempty"`
	Note   string `json:"note,omitempty"`
}

type PreflightResult struct {
	Status             PreflightStatus     `json:"status"`
	Ready              bool                `json:"ready"`
	Reason             string              `json:"reason,omitempty"`
	ManualID           string              `json:"manual_id,omitempty"`
	WorkflowID         string              `json:"workflow_id,omitempty"`
	ProbeID            string              `json:"probe_id,omitempty"`
	Evidence           []PreflightEvidence `json:"evidence,omitempty"`
	MissingPermissions []string            `json:"missing_permissions,omitempty"`
	EnvironmentDiffs   []string            `json:"environment_diffs,omitempty"`
	NextAction         string              `json:"next_action,omitempty"`
	CheckedAt          string              `json:"checked_at,omitempty"`
	ArtifactType       string              `json:"artifact_type,omitempty"`
}
