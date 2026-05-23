package selfopt

import "time"

type Options struct {
	AllowRealLLM    bool
	LLMSuggestions  bool
	AllowRemoteHost bool
	ServerURL       string
}

type Config struct {
	ServerURL       string    `json:"serverUrl,omitempty"`
	AllowRealLLM    bool      `json:"allowRealLLM"`
	AllowRemoteHost bool      `json:"allowRemoteHost"`
	ServerLLM       LLMConfig `json:"serverLLM"`
	LabLLM          LLMConfig `json:"labLLM"`
}

type LLMConfig struct {
	Enabled          bool   `json:"enabled"`
	BaseURL          string `json:"baseUrl,omitempty"`
	BaseURLHash      string `json:"baseURLHash,omitempty"`
	Model            string `json:"model,omitempty"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
	Source           string `json:"source,omitempty"`
}

type Manifest struct {
	RunID     string    `json:"runId"`
	StartedAt time.Time `json:"startedAt"`
	Config    Config    `json:"config"`
	Cases     []string  `json:"cases"`
	Safety    string    `json:"safety"`
}

type Case struct {
	ID             string             `json:"id"`
	Category       string             `json:"category,omitempty"`
	Priority       string             `json:"priority,omitempty"`
	Input          string             `json:"input,omitempty"`
	Expected       Expected           `json:"expected,omitempty"`
	Metadata       CaseMetadata       `json:"metadata,omitempty"`
	ScoreWeights   map[string]float64 `json:"scoreWeights,omitempty"`
	BaselinePolicy BaselinePolicy     `json:"baselinePolicy,omitempty"`
}

type Expected struct {
	MustInclude       []string `json:"mustInclude,omitempty"`
	MustNotInclude    []string `json:"mustNotInclude,omitempty"`
	ExpectedToolCalls []string `json:"expectedToolCalls,omitempty"`
	MaxToolCalls      int      `json:"maxToolCalls,omitempty"`
}

type CaseMetadata struct {
	CaseType        string             `json:"caseType,omitempty"`
	AreaTags        []string           `json:"areaTags,omitempty"`
	FeatureTags     []string           `json:"featureTags,omitempty"`
	RiskLevel       string             `json:"riskLevel,omitempty"`
	RequiresLLM     bool               `json:"requiresLLM,omitempty"`
	RequiresBrowser bool               `json:"requiresBrowser,omitempty"`
	RequiresNetwork bool               `json:"requiresNetwork,omitempty"`
	BaselinePolicy  BaselinePolicy     `json:"baselinePolicy,omitempty"`
	ScoreWeights    map[string]float64 `json:"scoreWeights,omitempty"`
}

type BaselinePolicy string

const (
	BaselineObserve           BaselinePolicy = "observe"
	BaselineBlockOnRegression BaselinePolicy = "block_on_regression"
)

type ImpactMatrix struct {
	ChangedFiles    []string `json:"changedFiles"`
	MatchedAreaTags []string `json:"matchedAreaTags"`
	SelectedCaseIDs []string `json:"selectedCaseIds"`
	SkippedCaseIDs  []string `json:"skippedCaseIds,omitempty"`
	FullSuite       bool     `json:"fullSuite"`
}

type Veto struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Detail   string `json:"detail,omitempty"`
}

type GateDecision string

const (
	GatePass  GateDecision = "pass"
	GateBlock GateDecision = "block"
	GateWarn  GateDecision = "warn"
)

type GatePolicy struct {
	CaseScoreDropThreshold  float64
	SuiteScoreDropThreshold float64
}

type CaseComparison struct {
	CaseID          string   `json:"caseId"`
	Priority        string   `json:"priority,omitempty"`
	BaselinePassed  bool     `json:"baselinePassed"`
	CurrentPassed   bool     `json:"currentPassed"`
	BaselineScore   float64  `json:"baselineScore"`
	CurrentScore    float64  `json:"currentScore"`
	Delta           float64  `json:"delta"`
	Movement        string   `json:"movement"`
	RegressedChecks []string `json:"regressedChecks,omitempty"`
	ChangedAreas    []string `json:"changedAreas,omitempty"`
}

type GateResult struct {
	Decision GateDecision `json:"decision"`
	Reasons  []string     `json:"reasons,omitempty"`
}

type CaseScore struct {
	CaseID string             `json:"caseId"`
	Passed bool               `json:"passed"`
	Score  float64            `json:"score"`
	Phases map[string]float64 `json:"phases,omitempty"`
	Checks []string           `json:"checks,omitempty"`
}

type Scorecard struct {
	RunID       string             `json:"runId"`
	Overall     float64            `json:"overall"`
	CaseScores  []CaseScore        `json:"caseScores"`
	PhaseScores map[string]float64 `json:"phaseScores,omitempty"`
	Vetoes      []Veto             `json:"vetoes,omitempty"`
	Gate        GateResult         `json:"gate"`
	AIOpsTests  *AIOpsTestSummary  `json:"aiopsTests,omitempty"`
}

type RunOptions struct {
	RunID           string
	CasesDir        string
	OutDir          string
	Changed         []string
	Dashboard       bool
	AssetDraft      bool
	RealAIOpsRunDir string
	Config          Config
}

type RunResult struct {
	RunDir      string
	Manifest    Manifest
	Scorecard   Scorecard
	Impact      ImpactMatrix
	Comparisons []CaseComparison
	Gate        GateResult
}

type AIOpsTestSummary struct {
	RunDir  string            `json:"runDir"`
	Reports []AIOpsTestReport `json:"reports"`
	Total   int               `json:"total"`
	Passed  int               `json:"passed"`
	Failed  int               `json:"failed"`
	Worse   int               `json:"worse"`
}

type AIOpsTestReport struct {
	Name        string            `json:"name"`
	Total       int               `json:"total"`
	Passed      int               `json:"passed"`
	Failed      int               `json:"failed"`
	AvgScore    float64           `json:"avgScore"`
	Worse       int               `json:"worse,omitempty"`
	FailedCases []AIOpsFailedCase `json:"failedCases,omitempty"`
	ReportPath  string            `json:"reportPath,omitempty"`
	DiagPath    string            `json:"diagnosisPath,omitempty"`
}

type AIOpsFailedCase struct {
	CaseID          string   `json:"caseId"`
	Score           float64  `json:"score"`
	Movement        string   `json:"movement,omitempty"`
	LikelyRootCause string   `json:"likelyRootCause,omitempty"`
	FailedChecks    []string `json:"failedChecks,omitempty"`
}
