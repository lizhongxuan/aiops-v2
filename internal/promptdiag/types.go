package promptdiag

import "time"

const (
	RootCauseUnknown                 = "unknown"
	RootCausePrompt                  = "prompt"
	RootCauseToolOrPolicy            = "tool_or_policy"
	RootCausePromptOrToolDescription = "prompt_or_tool_description"
	RootCauseContext                 = "context"
	RootCauseCompletionGate          = "completion_gate"
	RootCauseDeploymentOrConfig      = "deployment_or_config"
	RootCauseRegression              = "regression"
	RootCauseModelOrProvider         = "model_or_provider"
)

// Config describes the read-only inputs for a prompt diagnosis run.
type Config struct {
	ReportPath        string
	BaselinePath      string
	CasesDir          string
	TraceDir          string
	OutputDir         string
	DraftCasesOutDir  string
	HistoryPath       string
	LLMSuggestions    bool
	LLMBaseURL        string
	LLMAPIKey         string
	LLMModel          string
	PromptSizeWarning int
	GeneratedAt       time.Time
}

type RunDiagnosis struct {
	RunID       string            `json:"runId"`
	Agent       string            `json:"agent,omitempty"`
	ReportPath  string            `json:"reportPath,omitempty"`
	Baseline    string            `json:"baselinePath,omitempty"`
	CasesDir    string            `json:"casesDir,omitempty"`
	TraceDir    string            `json:"traceDir,omitempty"`
	GeneratedAt time.Time         `json:"generatedAt"`
	Summary     DiagnosisSummary  `json:"summary"`
	Cases       []CaseDiagnosis   `json:"cases"`
	TraceLinks  []TraceLink       `json:"traceLinks,omitempty"`
	Suggestions []Suggestion      `json:"suggestions,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type DiagnosisSummary struct {
	Total                   int            `json:"total"`
	Passed                  int            `json:"passed"`
	Failed                  int            `json:"failed"`
	AvgScore                float64        `json:"avgScore"`
	BaselineFailed          int            `json:"baselineFailed,omitempty"`
	BaselineAvgScore        float64        `json:"baselineAvgScore,omitempty"`
	FailedIncreased         bool           `json:"failedIncreased,omitempty"`
	AvgScoreDropped         bool           `json:"avgScoreDropped,omitempty"`
	Better                  int            `json:"better,omitempty"`
	Worse                   int            `json:"worse,omitempty"`
	Same                    int            `json:"same,omitempty"`
	New                     int            `json:"new,omitempty"`
	Missing                 int            `json:"missing,omitempty"`
	PromptChanged           bool           `json:"promptChanged"`
	StableHashChanged       bool           `json:"stableHashChanged"`
	DeveloperHashChanged    bool           `json:"developerHashChanged"`
	ToolRegistryHashChanged bool           `json:"toolRegistryHashChanged"`
	AvgModelCalls           float64        `json:"avgModelCalls,omitempty"`
	AvgToolCalls            float64        `json:"avgToolCalls,omitempty"`
	AvgIterations           float64        `json:"avgIterations,omitempty"`
	RootCauseCounts         map[string]int `json:"rootCauseCounts,omitempty"`
}

type CaseDiagnosis struct {
	CaseID          string            `json:"caseId"`
	Category        string            `json:"category,omitempty"`
	Priority        string            `json:"priority,omitempty"`
	Passed          bool              `json:"passed"`
	Score           float64           `json:"score"`
	Movement        string            `json:"movement,omitempty"`
	FailedChecks    []string          `json:"failedChecks,omitempty"`
	LikelyRootCause string            `json:"likelyRootCause"`
	Evidence        EvidenceSummary   `json:"evidence"`
	RuleHits        []RuleHit         `json:"ruleHits,omitempty"`
	Suggestions     []Suggestion      `json:"suggestions,omitempty"`
	Artifacts       map[string]string `json:"artifacts,omitempty"`
}

type EvidenceSummary struct {
	AnswerPath            string              `json:"answerPath,omitempty"`
	AnswerPreview         string              `json:"answerPreview,omitempty"`
	AnswerCharCount       int                 `json:"answerCharCount,omitempty"`
	AnswerLineCount       int                 `json:"answerLineCount,omitempty"`
	EventsPath            string              `json:"eventsPath,omitempty"`
	ToolCallsPath         string              `json:"toolCallsPath,omitempty"`
	TurnItemsPath         string              `json:"turnItemsPath,omitempty"`
	TraceFiles            []TraceLink         `json:"traceFiles,omitempty"`
	TraceTurns            []TraceTurnSummary  `json:"traceTurns,omitempty"`
	TraceTurnCount        int                 `json:"traceTurnCount,omitempty"`
	TraceIterationCount   int                 `json:"traceIterationCount,omitempty"`
	ToolCalls             []string            `json:"toolCalls,omitempty"`
	VisibleTools          []string            `json:"visibleTools,omitempty"`
	ExpectedTools         []string            `json:"expectedTools,omitempty"`
	MissingExpectedTools  []string            `json:"missingExpectedTools,omitempty"`
	FailedToolNames       []string            `json:"failedToolNames,omitempty"`
	PromptFingerprints    []map[string]string `json:"promptFingerprints,omitempty"`
	StableHash            string              `json:"stableHash,omitempty"`
	DeveloperHash         string              `json:"developerHash,omitempty"`
	ToolRegistryHash      string              `json:"toolRegistryHash,omitempty"`
	MessageCount          int                 `json:"messageCount,omitempty"`
	PromptSizeChars       int                 `json:"promptSizeChars,omitempty"`
	HasUserMessage        bool                `json:"hasUserMessage"`
	ModelCallCount        int                 `json:"modelCallCount,omitempty"`
	ToolCallCount         int                 `json:"toolCallCount,omitempty"`
	ToolResultCount       int                 `json:"toolResultCount,omitempty"`
	FailedToolResultCount int                 `json:"failedToolResultCount,omitempty"`
	PlanCount             int                 `json:"planCount,omitempty"`
	EvidenceCount         int                 `json:"evidenceCount,omitempty"`
	MaxIterationObserved  int                 `json:"maxIterationObserved,omitempty"`
	Error                 string              `json:"error,omitempty"`
}

type TraceLink struct {
	CaseID          string            `json:"caseId,omitempty"`
	SessionID       string            `json:"sessionId,omitempty"`
	TurnID          string            `json:"turnId,omitempty"`
	Iteration       int               `json:"iteration"`
	CreatedAt       string            `json:"createdAt,omitempty"`
	JSONPath        string            `json:"jsonPath,omitempty"`
	MarkdownPath    string            `json:"markdownPath,omitempty"`
	DiffPath        string            `json:"diffPath,omitempty"`
	StableHash      string            `json:"stableHash,omitempty"`
	MessageCount    int               `json:"messageCount,omitempty"`
	PromptSizeChars int               `json:"promptSizeChars,omitempty"`
	HasUserMessage  bool              `json:"hasUserMessage"`
	VisibleTools    []string          `json:"visibleTools,omitempty"`
	Fingerprint     map[string]string `json:"fingerprint,omitempty"`
}

type TraceTurnSummary struct {
	CaseID     string `json:"caseId,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
	Iterations []int  `json:"iterations,omitempty"`
	FirstAt    string `json:"firstAt,omitempty"`
	LastAt     string `json:"lastAt,omitempty"`
}

type RuleHit struct {
	RuleID    string `json:"ruleId"`
	Severity  string `json:"severity"`
	RootCause string `json:"rootCause"`
	Message   string `json:"message"`
	Evidence  string `json:"evidence,omitempty"`
}

type Suggestion struct {
	Area        string `json:"area"`
	Action      string `json:"action"`
	Rationale   string `json:"rationale,omitempty"`
	LLMAssisted bool   `json:"llm_assisted,omitempty"`
}

type RunMetadata struct {
	RunID         string    `json:"runId"`
	Agent         string    `json:"agent,omitempty"`
	GeneratedAt   time.Time `json:"generatedAt"`
	ReportPath    string    `json:"reportPath,omitempty"`
	BaselinePath  string    `json:"baselinePath,omitempty"`
	Total         int       `json:"total"`
	Passed        int       `json:"passed"`
	Failed        int       `json:"failed"`
	AvgScore      float64   `json:"avgScore"`
	Better        int       `json:"better,omitempty"`
	Worse         int       `json:"worse,omitempty"`
	Same          int       `json:"same,omitempty"`
	New           int       `json:"new,omitempty"`
	Missing       int       `json:"missing,omitempty"`
	PromptChanged bool      `json:"promptChanged"`
	AvgModelCalls float64   `json:"avgModelCalls,omitempty"`
	AvgToolCalls  float64   `json:"avgToolCalls,omitempty"`
	AvgIterations float64   `json:"avgIterations,omitempty"`
}
