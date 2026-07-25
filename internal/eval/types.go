package eval

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/agentui"
)

// Case is the on-disk JSON format for one agent evaluation case.
type Case struct {
	ID                 string                     `json:"id"`
	Category           string                     `json:"category"`
	RootCauseCategory  string                     `json:"rootCauseCategory,omitempty"`
	Priority           string                     `json:"priority,omitempty"`
	Input              string                     `json:"input"`
	Expected           Expected                   `json:"expected"`
	ScoreRules         ScoreRules                 `json:"scoreRules,omitempty"`
	WorkflowGeneration *WorkflowGenerationFixture `json:"workflowGeneration,omitempty"`
}

// ScoreRules configures case-level scoring behavior.
type ScoreRules struct {
	Weights map[string]float64 `json:"weights,omitempty"`
}

// Expected captures deterministic checks that can be scored locally.
type Expected struct {
	MustInclude                   []string                     `json:"mustInclude"`
	MustNotInclude                []string                     `json:"mustNotInclude"`
	ExpectedToolCalls             []string                     `json:"expectedToolCalls"`
	MustMentionFiles              []string                     `json:"mustMentionFiles"`
	ExpectedTurnItems             []string                     `json:"expectedTurnItems,omitempty"`
	ExpectedPlanStatuses          []string                     `json:"expectedPlanStatuses,omitempty"`
	ExpectedPlanModeState         []string                     `json:"expectedPlanModeState,omitempty"`
	ExpectedPlanRequirement       []string                     `json:"expectedPlanRequirement,omitempty"`
	ExpectedPlanCompletionGate    []string                     `json:"expectedPlanCompletionGate,omitempty"`
	ExpectedTaskClaims            []string                     `json:"expectedTaskClaims,omitempty"`
	ExpectedPlanApprovalScope     []string                     `json:"expectedPlanApprovalScope,omitempty"`
	ExpectedPlanRejectionEvents   []string                     `json:"expectedPlanRejectionEvents,omitempty"`
	ExpectedVerificationStatus    []string                     `json:"expectedVerificationStatus,omitempty"`
	ExpectedCompletionGate        []string                     `json:"expectedCompletionGate,omitempty"`
	ExpectedSafetySignals         []string                     `json:"expectedSafetySignals,omitempty"`
	ExpectedUnexpectedStateGate   []string                     `json:"expectedUnexpectedStateGate,omitempty"`
	ExpectedApprovalScope         []string                     `json:"expectedApprovalScope,omitempty"`
	ExpectedTraceEvidence         []string                     `json:"expectedTraceEvidence,omitempty"`
	ExpectedTaskDepth             []string                     `json:"expectedTaskDepth,omitempty"`
	ExpectedRequiredGates         []string                     `json:"expectedRequiredGates,omitempty"`
	ExpectedCoverageAction        []string                     `json:"expectedCoverageAction,omitempty"`
	ExpectedReasoningFallback     []string                     `json:"expectedReasoningFallback,omitempty"`
	ExpectedResumeAction          []string                     `json:"expectedResumeAction,omitempty"`
	ExpectedManagerSynthesis      []string                     `json:"expectedManagerSynthesis,omitempty"`
	ExpectedFailureAction         []string                     `json:"expectedFailureAction,omitempty"`
	ExpectedGenericityFindings    []string                     `json:"expectedGenericityFindings,omitempty"`
	ExpectedResourceIDSource      []string                     `json:"expectedResourceIdSource,omitempty"`
	ExpectedResourceRoles         []string                     `json:"expectedResourceRoles,omitempty"`
	ExpectedCapabilityPath        []string                     `json:"expectedCapabilityPath,omitempty"`
	ExpectedWorkflowReviewStatus  []string                     `json:"expectedWorkflowReviewStatus,omitempty"`
	ExpectedObservabilityEvidence []string                     `json:"expectedObservabilityEvidence,omitempty"`
	ExpectedGenericOpsContract    []string                     `json:"expectedGenericOpsContract,omitempty"`
	ExpectedApprovals             []string                     `json:"expectedApprovals,omitempty"`
	ExpectedEvidence              []string                     `json:"expectedEvidence,omitempty"`
	ExpectedAssembly              *ExpectedAssemblyTrace       `json:"expectedAssembly,omitempty"`
	ExpectedResources             *ExpectedResourceTrace       `json:"expectedResources,omitempty"`
	ExpectedToolSurface           *ExpectedToolSurfaceTrace    `json:"expectedToolSurface,omitempty"`
	ExpectedSessionTargets        *ExpectedSessionTargetTrace  `json:"expectedSessionTargets,omitempty"`
	ExpectedRoleBindings          []ExpectedRoleBindingTrace   `json:"expectedRoleBindings,omitempty"`
	ExpectedFinalReport           *ExpectedFinalReportTrace    `json:"expectedFinalReport,omitempty"`
	ExpectedTraceExplainability   *ExpectedTraceExplainability `json:"expectedTraceExplainability,omitempty"`
	Diagnosis                     DiagnosisExpected            `json:"diagnosis,omitempty"`
	MaxIterations                 int                          `json:"maxIterations,omitempty"`
	MaxToolCalls                  int                          `json:"maxToolCalls,omitempty"`
	MustHavePlan                  bool                         `json:"mustHavePlan,omitempty"`
	MustNotHavePlan               bool                         `json:"mustNotHavePlan,omitempty"`
	MustHaveEvidence              bool                         `json:"mustHaveEvidence,omitempty"`
	ForbidFirstTurnNoToolFinal    bool                         `json:"forbidFirstTurnNoToolFinal,omitempty"`
	MustMentionEvidenceLimits     bool                         `json:"mustMentionEvidenceLimits,omitempty"`
}

type ExpectedAssemblyTrace struct {
	AgentKind           string `json:"agentKind,omitempty"`
	Profile             string `json:"profile,omitempty"`
	RuntimeRole         string `json:"runtimeRole,omitempty"`
	RouteReasonContains string `json:"routeReasonContains,omitempty"`
}

type ExpectedResourceTrace struct {
	VerifiedIDs []string `json:"verifiedIds,omitempty"`
	RejectedIDs []string `json:"rejectedIds,omitempty"`
}

type ExpectedToolSurfaceTrace struct {
	VisibleTools        []string `json:"visibleTools,omitempty"`
	DispatchableTools   []string `json:"dispatchableTools,omitempty"`
	HiddenTools         []string `json:"hiddenTools,omitempty"`
	RequireSingleSource bool     `json:"requireSingleSource,omitempty"`
}

type ExpectedSessionTargetTrace struct {
	BindingMode  string   `json:"bindingMode,omitempty"`
	HostIDs      []string `json:"hostIds,omitempty"`
	SourceTurnID string   `json:"sourceTurnId,omitempty"`
}

type ExpectedRoleBindingTrace struct {
	ResourceID    string `json:"resourceId,omitempty"`
	Role          string `json:"role,omitempty"`
	ConflictState string `json:"conflictState,omitempty"`
}

type ExpectedFinalReportTrace struct {
	HostIDs      []string `json:"hostIds,omitempty"`
	Roles        []string `json:"roles,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ExpectedTraceExplainability struct {
	RequireAssemblySource         bool `json:"requireAssemblySource,omitempty"`
	RequirePromptCompilerSource   bool `json:"requirePromptCompilerSource,omitempty"`
	RequireToolSurfaceSource      bool `json:"requireToolSurfaceSource,omitempty"`
	RequireAdapterName            bool `json:"requireAdapterName,omitempty"`
	RequirePromptSectionHashes    bool `json:"requirePromptSectionHashes,omitempty"`
	RequireToolSurfaceFingerprint bool `json:"requireToolSurfaceFingerprint,omitempty"`
}

// DiagnosisExpected captures opt-in deterministic checks for AIOps diagnosis accuracy.
// A zero value is ignored so older eval cases keep their existing scoring behavior.
type DiagnosisExpected struct {
	RootCauseTop1             string   `json:"rootCauseTop1,omitempty"`
	RootCauseCandidatesTop3   []string `json:"rootCauseCandidatesTop3,omitempty"`
	SupportingEvidence        []string `json:"supportingEvidence,omitempty"`
	RefutingEvidence          []string `json:"refutingEvidence,omitempty"`
	MissingEvidence           []string `json:"missingEvidence,omitempty"`
	ToolFailureSemantics      []string `json:"toolFailureSemantics,omitempty"`
	ConfidenceCalibration     []string `json:"confidenceCalibration,omitempty"`
	SafetyGuardrails          []string `json:"safetyGuardrails,omitempty"`
	PromptContextPollution    []string `json:"promptContextPollution,omitempty"`
	ForbiddenStaleScopeTerms  []string `json:"forbiddenStaleScopeTerms,omitempty"`
	ForbiddenSensitiveTerms   []string `json:"forbiddenSensitiveTerms,omitempty"`
	ForbiddenWriteActions     []string `json:"forbiddenWriteActions,omitempty"`
	ToolFailureTargetStates   []string `json:"toolFailureTargetStates,omitempty"`
	RequireApprovedHighRisk   bool     `json:"requireApprovedHighRisk,omitempty"`
	ForbidHighConfidenceScope bool     `json:"forbidHighConfidenceScope,omitempty"`
	CoverageTags              []string `json:"coverageTags,omitempty"`
}

// IsZero reports whether diagnosis scoring should be skipped.
func (d DiagnosisExpected) IsZero() bool {
	return strings.TrimSpace(d.RootCauseTop1) == "" &&
		len(d.RootCauseCandidatesTop3) == 0 &&
		len(d.SupportingEvidence) == 0 &&
		len(d.RefutingEvidence) == 0 &&
		len(d.MissingEvidence) == 0 &&
		len(d.ToolFailureSemantics) == 0 &&
		len(d.ConfidenceCalibration) == 0 &&
		len(d.SafetyGuardrails) == 0 &&
		len(d.PromptContextPollution) == 0 &&
		len(d.ForbiddenStaleScopeTerms) == 0 &&
		len(d.ForbiddenSensitiveTerms) == 0 &&
		len(d.ForbiddenWriteActions) == 0 &&
		len(d.ToolFailureTargetStates) == 0 &&
		!d.RequireApprovedHighRisk &&
		!d.ForbidHighConfidenceScope &&
		len(d.CoverageTags) == 0
}

// ToolCall records the model-visible tool call surface used by eval reports.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// RunOutput is the agent result that the scorer consumes and the runner saves.
type RunOutput struct {
	Answer    string                `json:"answer"`
	Events    []agentui.AgentEvent  `json:"events,omitempty"`
	ToolCalls []ToolCall            `json:"toolCalls,omitempty"`
	TurnItems []agentstate.TurnItem `json:"turnItems,omitempty"`
}

// Agent is the minimal interface needed by the local eval runner.
type Agent interface {
	Run(ctx context.Context, c Case) (RunOutput, error)
}

// CheckResult is the outcome of one scorer rule.
type CheckResult struct {
	Name       string   `json:"name"`
	Passed     bool     `json:"passed"`
	Detail     string   `json:"detail,omitempty"`
	Missing    []string `json:"missing,omitempty"`
	Matched    []string `json:"matched,omitempty"`
	Unexpected []string `json:"unexpected,omitempty"`
}

// CaseScore is one scored case in a report.
type CaseScore struct {
	CaseID             string              `json:"caseId"`
	Category           string              `json:"category,omitempty"`
	RootCauseCategory  string              `json:"rootCauseCategory,omitempty"`
	Priority           string              `json:"priority,omitempty"`
	Passed             bool                `json:"passed"`
	Score              float64             `json:"score"`
	AvgScore           float64             `json:"avgScore"`
	MinScore           float64             `json:"minScore"`
	Iterations         int                 `json:"iterations"`
	IterationScores    []float64           `json:"iterationScores,omitempty"`
	IterationArtifacts []IterationArtifact `json:"iterationArtifacts,omitempty"`
	ScoreWeights       map[string]float64  `json:"scoreWeights,omitempty"`
	PassedChecks       int                 `json:"passedChecks"`
	TotalChecks        int                 `json:"totalChecks"`
	Checks             []CheckResult       `json:"checks"`
	AnswerPath         string              `json:"answerPath,omitempty"`
	EventsPath         string              `json:"eventsPath,omitempty"`
	ToolCallsPath      string              `json:"toolCallsPath,omitempty"`
	TurnItemsPath      string              `json:"turnItemsPath,omitempty"`
	PromptFingerprints []map[string]string `json:"promptFingerprints,omitempty"`
	Error              string              `json:"error,omitempty"`
}

// IterationArtifact records per-repetition output paths for an eval case.
type IterationArtifact struct {
	Iteration     int     `json:"iteration"`
	Score         float64 `json:"score"`
	Passed        bool    `json:"passed"`
	AnswerPath    string  `json:"answerPath,omitempty"`
	EventsPath    string  `json:"eventsPath,omitempty"`
	ToolCallsPath string  `json:"toolCallsPath,omitempty"`
	TurnItemsPath string  `json:"turnItemsPath,omitempty"`
	Error         string  `json:"error,omitempty"`
}

// ReportSummary aggregates a run.
type ReportSummary struct {
	Total              int     `json:"total"`
	Passed             int     `json:"passed"`
	Failed             int     `json:"failed"`
	AvgScore           float64 `json:"avgScore"`
	MinScore           float64 `json:"minScore"`
	LowestScoreAverage float64 `json:"lowestScoreAverage"`
}

// Report is the JSON score report emitted by the runner.
type Report struct {
	RunID              string                    `json:"runId"`
	RunPhase           string                    `json:"runPhase,omitempty"`
	Agent              string                    `json:"agent,omitempty"`
	CasesDir           string                    `json:"casesDir,omitempty"`
	OutputDir          string                    `json:"outputDir,omitempty"`
	Repetitions        int                       `json:"repetitions"`
	Metadata           map[string]string         `json:"metadata,omitempty"`
	StartedAt          time.Time                 `json:"startedAt"`
	CompletedAt        time.Time                 `json:"completedAt"`
	Summary            ReportSummary             `json:"summary"`
	Cases              []CaseScore               `json:"cases"`
	BaselineComparison *ComparisonReport         `json:"baselineComparison,omitempty"`
	WorkflowGeneration *WorkflowGenerationReport `json:"workflowGeneration,omitempty"`
}

type WorkflowGenerationFixture struct {
	SchemaVersion         string                                 `json:"schemaVersion"`
	RequiredNodeTypes     []string                               `json:"requiredNodeTypes,omitempty"`
	AllowedNodeTypes      []string                               `json:"allowedNodeTypes,omitempty"`
	RequiredActions       []string                               `json:"requiredActions,omitempty"`
	AllowedActions        []string                               `json:"allowedActions,omitempty"`
	ArgumentSchemas       map[string]WorkflowEvalArgumentSchema  `json:"argumentSchemas,omitempty"`
	RequiredPorts         map[string]WorkflowEvalPortExpectation `json:"requiredPorts,omitempty"`
	RequiredPaths         []string                               `json:"requiredPaths,omitempty"`
	ForbiddenPaths        []string                               `json:"forbiddenPaths,omitempty"`
	ForbiddenPlaceholders []string                               `json:"forbiddenPlaceholders,omitempty"`
	Validation            WorkflowEvalValidationExpectation      `json:"validation"`
	Risk                  WorkflowEvalRiskExpectation            `json:"risk"`
	Invariants            []WorkflowEvalInvariant                `json:"invariants,omitempty"`
	Budgets               WorkflowEvalBudgets                    `json:"budgets"`
	RequireDeterministic  bool                                   `json:"requireDeterministic,omitempty"`
	PerformanceFixture    string                                 `json:"performanceFixture,omitempty"`
	RecordedAttempts      []WorkflowGenerationAttempt            `json:"recordedAttempts,omitempty"`
}
type WorkflowEvalArgumentSchema struct {
	Required             []string          `json:"required,omitempty"`
	Properties           map[string]string `json:"properties"`
	AdditionalProperties bool              `json:"additionalProperties"`
}
type WorkflowEvalPortExpectation struct {
	Inputs  []string `json:"inputs,omitempty"`
	Outputs []string `json:"outputs,omitempty"`
}
type WorkflowEvalValidationExpectation struct {
	FirstPreviewStatus string `json:"firstPreviewStatus"`
	FinalStatus        string `json:"finalStatus"`
	RateEligible       bool   `json:"rateEligible"`
}
type WorkflowEvalRiskExpectation struct {
	Level            string `json:"level"`
	RequiresApproval bool   `json:"requiresApproval"`
	MustBeGranted    bool   `json:"mustBeGranted,omitempty"`
}
type WorkflowEvalInvariant struct {
	Kind       string   `json:"kind"`
	Path       string   `json:"path,omitempty"`
	Name       string   `json:"name,omitempty"`
	Source     string   `json:"source,omitempty"`
	SourcePort string   `json:"sourcePort,omitempty"`
	Target     string   `json:"target,omitempty"`
	TargetPort string   `json:"targetPort,omitempty"`
	NodeIDs    []string `json:"nodeIds,omitempty"`
}
type WorkflowEvalBudgets struct {
	MaxRepairCount     int   `json:"maxRepairCount"`
	MaxFirstFeedbackMS int64 `json:"maxFirstFeedbackMs"`
	MaxPreviewP95MS    int64 `json:"maxPreviewP95Ms"`
	MaxValidateP95MS   int64 `json:"maxValidateP95Ms"`
	MaxInputTokens     int   `json:"maxInputTokens"`
	MaxContextTokens   int   `json:"maxContextTokens"`
}
type WorkflowGenerationAttempt struct {
	SchemaVersion string                `json:"schemaVersion"`
	PlanHash      string                `json:"planHash,omitempty"`
	CompileHash   string                `json:"compileHash,omitempty"`
	Baseline      *WorkflowEvalSnapshot `json:"baseline,omitempty"`
	FirstPreview  WorkflowEvalSnapshot  `json:"firstPreview"`
	Final         WorkflowEvalSnapshot  `json:"final"`
	Metrics       WorkflowEvalMetrics   `json:"metrics"`
	SemanticFacts map[string]bool       `json:"semanticFacts,omitempty"`
	TextContent   []string              `json:"textContent,omitempty"`
}
type WorkflowEvalSnapshot struct {
	Graph      WorkflowEvalGraph      `json:"graph"`
	Validation WorkflowEvalValidation `json:"validation"`
	Risk       string                 `json:"risk"`
	Approval   WorkflowEvalApproval   `json:"approval"`
}
type WorkflowEvalGraph struct {
	Nodes []WorkflowEvalNode `json:"nodes"`
	Edges []WorkflowEvalEdge `json:"edges"`
}
type WorkflowEvalNode struct {
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	Label       string             `json:"label,omitempty"`
	Action      string             `json:"action,omitempty"`
	Args        map[string]any     `json:"args,omitempty"`
	InputPorts  []string           `json:"inputPorts,omitempty"`
	OutputPorts []string           `json:"outputPorts,omitempty"`
	Position    map[string]float64 `json:"position,omitempty"`
	Reference   string             `json:"reference,omitempty"`
	Pinned      bool               `json:"pinned,omitempty"`
}
type WorkflowEvalEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	SourcePort string `json:"sourcePort,omitempty"`
	Target     string `json:"target"`
	TargetPort string `json:"targetPort,omitempty"`
}
type WorkflowEvalValidation struct {
	Status      string   `json:"status"`
	Valid       bool     `json:"valid"`
	Diagnostics []string `json:"diagnostics,omitempty"`
}
type WorkflowEvalApproval struct {
	Required bool `json:"required"`
	Granted  bool `json:"granted"`
}
type WorkflowEvalMetrics struct {
	FirstFeedbackMS int64 `json:"firstFeedbackMs"`
	PreviewMS       int64 `json:"previewMs"`
	ValidateMS      int64 `json:"validateMs"`
	RepairCount     int   `json:"repairCount"`
	InputTokens     int   `json:"inputTokens"`
	OutputTokens    int   `json:"outputTokens"`
	ContextTokens   int   `json:"contextTokens"`
}
type WorkflowGenerationCaseScore struct {
	CaseID            string                      `json:"caseId"`
	Passed            bool                        `json:"passed"`
	MeanScore         float64                     `json:"meanScore"`
	ScoreVariance     float64                     `json:"scoreVariance"`
	AttemptScores     []float64                   `json:"attemptScores"`
	Checks            []CheckResult               `json:"checks"`
	PlaceholderCount  int                         `json:"placeholderCount"`
	FirstPreviewValid int                         `json:"firstPreviewValid"`
	FinalValid        int                         `json:"finalValid"`
	RateEligible      int                         `json:"rateEligible"`
	FailedIterations  []int                       `json:"failedIterations,omitempty"`
	Attempts          []WorkflowGenerationAttempt `json:"-"`
}
type WorkflowGenerationMetrics struct {
	MeanScore                float64 `json:"meanScore"`
	ScoreVariance            float64 `json:"scoreVariance"`
	FirstPreviewValidRate    float64 `json:"firstPreviewValidRate"`
	FinalValidRate           float64 `json:"finalValidRate"`
	PlaceholderCount         int     `json:"placeholderCount"`
	PreviewP95MS             int64   `json:"previewP95Ms"`
	ValidateP95MS            int64   `json:"validateP95Ms"`
	FirstFeedbackP95MS       int64   `json:"firstFeedbackP95Ms"`
	MaxRepairCount           int     `json:"maxRepairCount"`
	MeanInputTokens          float64 `json:"meanInputTokens"`
	MeanOutputTokens         float64 `json:"meanOutputTokens"`
	MeanContextTokens        float64 `json:"meanContextTokens"`
	GraphCopiesInTextContent int     `json:"graphCopiesInTextContent"`
}
type WorkflowGenerationGate struct {
	Passed                     bool     `json:"passed"`
	FinalValidThreshold        float64  `json:"finalValidThreshold"`
	FirstPreviewValidThreshold float64  `json:"firstPreviewValidThreshold"`
	PlaceholderThreshold       int      `json:"placeholderThreshold"`
	Failures                   []string `json:"failures,omitempty"`
}
type WorkflowGenerationFailureSample struct {
	CaseID       string   `json:"caseId"`
	Iteration    int      `json:"iteration"`
	FailedChecks []string `json:"failedChecks"`
}
type WorkflowGenerationReport struct {
	SchemaVersion  string                            `json:"schemaVersion"`
	Mode           string                            `json:"mode"`
	Repetitions    int                               `json:"repetitions"`
	Gate           WorkflowGenerationGate            `json:"gate"`
	Metrics        WorkflowGenerationMetrics         `json:"metrics"`
	Cases          []WorkflowGenerationCaseScore     `json:"cases"`
	FailureSamples []WorkflowGenerationFailureSample `json:"failureSamples,omitempty"`
}
type WorkflowGenerationScoreOptions struct {
	Mode    string
	Secrets []string
}

// ComparisonCase captures baseline-vs-current movement for one case.
type ComparisonCase struct {
	CaseID          string   `json:"caseId"`
	BaselineScore   float64  `json:"baselineScore"`
	CurrentScore    float64  `json:"currentScore"`
	Delta           float64  `json:"delta"`
	BaselinePassed  bool     `json:"baselinePassed"`
	CurrentPassed   bool     `json:"currentPassed"`
	Status          string   `json:"status"`
	RegressedChecks []string `json:"regressedChecks,omitempty"`
	ImprovedChecks  []string `json:"improvedChecks,omitempty"`
}

// ComparisonSummary aggregates baseline movement.
type ComparisonSummary struct {
	Better  int `json:"better"`
	Worse   int `json:"worse"`
	Same    int `json:"same"`
	New     int `json:"new"`
	Missing int `json:"missing"`
}

// ComparisonReport lists baseline-vs-current movement.
type ComparisonReport struct {
	Summary ComparisonSummary `json:"summary"`
	Cases   []ComparisonCase  `json:"cases"`
}
