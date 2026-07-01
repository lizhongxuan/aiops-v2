package runtimecontract

import "strings"

const (
	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"
)

type IntentKind string

const (
	IntentKindUnknown          IntentKind = "unknown"
	IntentKindDiagnose         IntentKind = "diagnose"
	IntentKindExplain          IntentKind = "explain"
	IntentKindPlan             IntentKind = "plan"
	IntentKindChange           IntentKind = "change"
	IntentKindVerify           IntentKind = "verify"
	IntentKindResearch         IntentKind = "research"
	IntentKindConfigure        IntentKind = "configure"
	IntentKindRunbookAuthoring IntentKind = "runbook_authoring"
)

type DataScope string

const (
	DataScopeUnknown      DataScope = "unknown"
	DataScopeLocalRuntime DataScope = "local_runtime"
	DataScopeWorkspace    DataScope = "workspace"
	DataScopeOpsKnowledge DataScope = "ops_knowledge"
	DataScopePublicWeb    DataScope = "public_web"
	DataScopeExternalMCP  DataScope = "external_mcp"
)

type ActionRisk string

const (
	ActionRiskUnknown  ActionRisk = "unknown"
	ActionRiskReadOnly ActionRisk = "read_only"
	ActionRiskWrite    ActionRisk = "write"
	ActionRiskHostExec ActionRisk = "host_exec"
	ActionRiskNetwork  ActionRisk = "network"
	ActionRiskDestruct ActionRisk = "destructive"
)

const (
	EvidenceKindLog           = "log"
	EvidenceKindCommandOutput = "command_output"
	EvidenceKindSQLResult     = "sql_result"
	EvidenceKindMonitoring    = "monitoring"
	EvidenceKindStackTrace    = "stack_trace"
	EvidenceKindConfig        = "config"
	EvidenceKindTimeline      = "timeline"
)

const (
	WeakSignalLogLikeText          = "log_like_text"
	WeakSignalConfigLikeText       = "config_like_text"
	WeakSignalTimelineLikeSequence = "timeline_like_sequence"
	WeakSignalCommandOutputLike    = "command_output_like_text"
	WeakSignalMonitoringLikeText   = "monitoring_like_text"
	WeakSignalStackTraceLikeText   = "stack_trace_like_text"
	WeakSignalSQLResultLikeText    = "sql_result_like_text"
)

const (
	MetadataIntentFrame      = "aiops.intent.frame"
	MetadataIntentKind       = "aiops.intent.kind"
	MetadataIntentDataScopes = "aiops.intent.dataScopes"
	MetadataIntentRiskBudget = "aiops.intent.riskBudget"
	MetadataIntentConfidence = "aiops.intent.confidence"
	MetadataEvidenceKinds    = "aiops.intent.evidenceKinds"
	MetadataWeakSignals      = "aiops.intent.weakSignals"
	MetadataLegacyRoute      = "aiops.route.legacy"
	MetadataIntentRoute      = "aiops.route.intent"
	MetadataRouteDiff        = "aiops.route.diff"
)

type IntentConstraint struct {
	Name       string `json:"name"`
	Value      string `json:"value,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	Source     string `json:"source,omitempty"`
}

type CapabilityCandidate struct {
	Name       string       `json:"name"`
	DataScopes []DataScope  `json:"data_scopes,omitempty"`
	Risks      []ActionRisk `json:"risks,omitempty"`
	Reasons    []string     `json:"reasons,omitempty"`
}

type WeakSignal struct {
	Name       string `json:"name"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
	Summary    string `json:"summary,omitempty"`
}

type EvidenceEnvelope struct {
	HasUserProvidedEvidence bool         `json:"has_user_provided_evidence"`
	EvidenceKinds           []string     `json:"evidence_kinds,omitempty"`
	DataScopes              []DataScope  `json:"data_scopes,omitempty"`
	WeakSignals             []WeakSignal `json:"weak_signals,omitempty"`
}

type IntentFrame struct {
	Kind         IntentKind            `json:"kind"`
	DataScopes   []DataScope           `json:"data_scopes,omitempty"`
	RiskBudget   []ActionRisk          `json:"risk_budget,omitempty"`
	Constraints  []IntentConstraint    `json:"constraints,omitempty"`
	Capabilities []CapabilityCandidate `json:"capabilities,omitempty"`
	Evidence     EvidenceEnvelope      `json:"evidence"`
	Confidence   string                `json:"confidence,omitempty"`
	Classifier   string                `json:"classifier,omitempty"`
}

func NormalizeIntentFrame(frame IntentFrame) IntentFrame {
	if frame.Kind == "" {
		frame.Kind = IntentKindUnknown
	}
	if strings.TrimSpace(frame.Confidence) == "" {
		frame.Confidence = ConfidenceLow
	}
	frame.DataScopes = normalizeDataScopes(frame.DataScopes)
	frame.RiskBudget = normalizeActionRisks(frame.RiskBudget)
	frame.Evidence.DataScopes = normalizeDataScopes(frame.Evidence.DataScopes)
	return frame
}

func AppendDataScope(values []DataScope, next ...DataScope) []DataScope {
	out := append([]DataScope(nil), values...)
	for _, scope := range next {
		scope = DataScope(strings.TrimSpace(string(scope)))
		if scope == "" || scope == DataScopeUnknown || ContainsDataScope(out, scope) {
			continue
		}
		out = append(out, scope)
	}
	return out
}

func ContainsDataScope(values []DataScope, want DataScope) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func AppendActionRisk(values []ActionRisk, next ...ActionRisk) []ActionRisk {
	out := append([]ActionRisk(nil), values...)
	for _, risk := range next {
		risk = ActionRisk(strings.TrimSpace(string(risk)))
		if risk == "" || risk == ActionRiskUnknown || ContainsActionRisk(out, risk) {
			continue
		}
		out = append(out, risk)
	}
	return out
}

func ContainsActionRisk(values []ActionRisk, want ActionRisk) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalizeDataScopes(values []DataScope) []DataScope {
	var out []DataScope
	for _, value := range values {
		out = AppendDataScope(out, value)
	}
	return out
}

func normalizeActionRisks(values []ActionRisk) []ActionRisk {
	var out []ActionRisk
	for _, value := range values {
		out = AppendActionRisk(out, value)
	}
	return out
}
