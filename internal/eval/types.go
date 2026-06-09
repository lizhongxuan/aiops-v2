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
	ID                string     `json:"id"`
	Category          string     `json:"category"`
	RootCauseCategory string     `json:"rootCauseCategory,omitempty"`
	Priority          string     `json:"priority,omitempty"`
	Input             string     `json:"input"`
	Expected          Expected   `json:"expected"`
	ScoreRules        ScoreRules `json:"scoreRules,omitempty"`
}

// ScoreRules configures case-level scoring behavior.
type ScoreRules struct {
	Weights map[string]float64 `json:"weights,omitempty"`
}

// Expected captures deterministic checks that can be scored locally.
type Expected struct {
	MustInclude                 []string          `json:"mustInclude"`
	MustNotInclude              []string          `json:"mustNotInclude"`
	ExpectedToolCalls           []string          `json:"expectedToolCalls"`
	MustMentionFiles            []string          `json:"mustMentionFiles"`
	ExpectedTurnItems           []string          `json:"expectedTurnItems,omitempty"`
	ExpectedPlanStatuses        []string          `json:"expectedPlanStatuses,omitempty"`
	ExpectedPlanModeState       []string          `json:"expectedPlanModeState,omitempty"`
	ExpectedPlanRequirement     []string          `json:"expectedPlanRequirement,omitempty"`
	ExpectedPlanCompletionGate  []string          `json:"expectedPlanCompletionGate,omitempty"`
	ExpectedTaskClaims          []string          `json:"expectedTaskClaims,omitempty"`
	ExpectedPlanApprovalScope   []string          `json:"expectedPlanApprovalScope,omitempty"`
	ExpectedPlanRejectionEvents []string          `json:"expectedPlanRejectionEvents,omitempty"`
	ExpectedVerificationStatus  []string          `json:"expectedVerificationStatus,omitempty"`
	ExpectedCompletionGate      []string          `json:"expectedCompletionGate,omitempty"`
	ExpectedSafetySignals       []string          `json:"expectedSafetySignals,omitempty"`
	ExpectedUnexpectedStateGate []string          `json:"expectedUnexpectedStateGate,omitempty"`
	ExpectedApprovalScope       []string          `json:"expectedApprovalScope,omitempty"`
	ExpectedTraceEvidence       []string          `json:"expectedTraceEvidence,omitempty"`
	ExpectedTaskDepth           []string          `json:"expectedTaskDepth,omitempty"`
	ExpectedRequiredGates       []string          `json:"expectedRequiredGates,omitempty"`
	ExpectedCoverageAction      []string          `json:"expectedCoverageAction,omitempty"`
	ExpectedReasoningFallback   []string          `json:"expectedReasoningFallback,omitempty"`
	ExpectedResumeAction        []string          `json:"expectedResumeAction,omitempty"`
	ExpectedManagerSynthesis    []string          `json:"expectedManagerSynthesis,omitempty"`
	ExpectedFailureAction       []string          `json:"expectedFailureAction,omitempty"`
	ExpectedGenericityFindings  []string          `json:"expectedGenericityFindings,omitempty"`
	ExpectedResourceIDSource    []string          `json:"expectedResourceIdSource,omitempty"`
	ExpectedApprovals           []string          `json:"expectedApprovals,omitempty"`
	ExpectedEvidence            []string          `json:"expectedEvidence,omitempty"`
	Diagnosis                   DiagnosisExpected `json:"diagnosis,omitempty"`
	MaxIterations               int               `json:"maxIterations,omitempty"`
	MaxToolCalls                int               `json:"maxToolCalls,omitempty"`
	MustHavePlan                bool              `json:"mustHavePlan,omitempty"`
	MustNotHavePlan             bool              `json:"mustNotHavePlan,omitempty"`
	MustHaveEvidence            bool              `json:"mustHaveEvidence,omitempty"`
	ForbidFirstTurnNoToolFinal  bool              `json:"forbidFirstTurnNoToolFinal,omitempty"`
	MustMentionEvidenceLimits   bool              `json:"mustMentionEvidenceLimits,omitempty"`
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
	RunID              string            `json:"runId"`
	RunPhase           string            `json:"runPhase,omitempty"`
	Agent              string            `json:"agent,omitempty"`
	CasesDir           string            `json:"casesDir,omitempty"`
	OutputDir          string            `json:"outputDir,omitempty"`
	Repetitions        int               `json:"repetitions"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	StartedAt          time.Time         `json:"startedAt"`
	CompletedAt        time.Time         `json:"completedAt"`
	Summary            ReportSummary     `json:"summary"`
	Cases              []CaseScore       `json:"cases"`
	BaselineComparison *ComparisonReport `json:"baselineComparison,omitempty"`
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
