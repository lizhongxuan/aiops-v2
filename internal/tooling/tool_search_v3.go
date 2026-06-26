package tooling

import (
	"sort"
	"strings"
)

type ToolSearchRequest struct {
	Mode               string            `json:"mode,omitempty"`
	Query              string            `json:"query,omitempty"`
	Intent             string            `json:"intent,omitempty"`
	TargetRefs         []string          `json:"targetRefs,omitempty"`
	RequiredCaps       []string          `json:"requiredCaps,omitempty"`
	ForbiddenCaps      []string          `json:"forbiddenCaps,omitempty"`
	RiskLevel          string            `json:"riskLevel,omitempty"`
	SessionType        string            `json:"sessionType,omitempty"`
	RuntimeMode        string            `json:"runtimeMode,omitempty"`
	AgentProfile       string            `json:"agentProfile,omitempty"`
	ResourceScope      string            `json:"resourceScope,omitempty"`
	EvidencePreference string            `json:"evidencePreference,omitempty"`
	Limit              int               `json:"limit,omitempty"`
	IncludeUnavailable bool              `json:"includeUnavailable,omitempty"`
	MCPHealth          map[string]string `json:"mcpHealth,omitempty"`
	EnvironmentFacts   []string          `json:"environmentFacts,omitempty"`
	Ranker             string            `json:"ranker,omitempty"`
}

type ToolCandidate struct {
	Name               string              `json:"name"`
	Source             string              `json:"source,omitempty"`
	Capability         string              `json:"capability,omitempty"`
	Score              float64             `json:"score,omitempty"`
	MatchReasons       []string            `json:"matchReasons,omitempty"`
	RiskLevel          string              `json:"riskLevel,omitempty"`
	RequiresTarget     bool                `json:"requiresTarget,omitempty"`
	RequiresHealthyMCP string              `json:"requiresHealthyMcp,omitempty"`
	LoadableToolSpec   *LoadableToolSpec   `json:"loadableToolSpec,omitempty"`
	SelectablePack     *SelectableToolPack `json:"selectablePack,omitempty"`
}

type LoadableToolSpec struct {
	Name           string   `json:"name"`
	Pack           string   `json:"pack,omitempty"`
	LoadingPolicy  string   `json:"loadingPolicy,omitempty"`
	Capability     string   `json:"capability,omitempty"`
	ResourceTypes  []string `json:"resourceTypes,omitempty"`
	OperationKinds []string `json:"operationKinds,omitempty"`
	RiskLevel      string   `json:"riskLevel,omitempty"`
	RequiresSelect bool     `json:"requiresSelect,omitempty"`
	RequiredAction string   `json:"requiredAction,omitempty"`
}

type SelectableToolPack struct {
	Pack           string   `json:"pack"`
	Tools          []string `json:"tools,omitempty"`
	Capability     string   `json:"capability,omitempty"`
	RequiresSelect bool     `json:"requiresSelect,omitempty"`
	RequiredAction string   `json:"requiredAction,omitempty"`
}

type RejectedToolCandidate struct {
	Name                string   `json:"name"`
	Reason              string   `json:"reason"`
	Status              string   `json:"status,omitempty"`
	Source              string   `json:"source,omitempty"`
	MCPServerID         string   `json:"mcpServerId,omitempty"`
	HealthStatus        string   `json:"healthStatus,omitempty"`
	FilteredReason      string   `json:"filteredReason,omitempty"`
	TargetCompatibility string   `json:"targetCompatibility,omitempty"`
	RiskDecision        string   `json:"riskDecision,omitempty"`
	MatchReasons        []string `json:"matchReasons,omitempty"`
}

type ToolSearchResponse struct {
	Ranker        string                  `json:"ranker,omitempty"`
	MatchCount    int                     `json:"matchCount,omitempty"`
	RejectedCount int                     `json:"rejectedCount,omitempty"`
	Candidates    []ToolCandidate         `json:"candidates,omitempty"`
	Rejected      []RejectedToolCandidate `json:"rejected,omitempty"`
	TraceID       string                  `json:"traceId,omitempty"`
}

func ToolCandidateFromMetadata(meta ToolMetadata) ToolCandidate {
	discovery := meta.EffectiveDiscovery()
	candidate := ToolCandidate{
		Name:               strings.TrimSpace(meta.Name),
		Source:             toolSearchCandidateSource(meta),
		Capability:         discovery.CapabilityKind,
		RiskLevel:          string(discovery.RiskLevel),
		RequiresTarget:     discovery.RequiresExplicitTarget,
		RequiresHealthyMCP: discovery.MCPServerID,
	}
	if ToolRequiresSelect(meta) || discovery.LoadingPolicy != ToolLoadingPolicyCore {
		candidate.LoadableToolSpec = &LoadableToolSpec{
			Name:           strings.TrimSpace(meta.Name),
			Pack:           firstNonEmpty(firstString(discovery.ToolPackIDs), meta.Pack),
			LoadingPolicy:  string(discovery.LoadingPolicy),
			Capability:     discovery.CapabilityKind,
			ResourceTypes:  append([]string(nil), discovery.ResourceTypes...),
			OperationKinds: append([]string(nil), discovery.OperationKinds...),
			RiskLevel:      string(discovery.RiskLevel),
			RequiresSelect: ToolRequiresSelect(meta),
			RequiredAction: "select this tool or its pack before calling it",
		}
	}
	pack := firstNonEmpty(firstString(discovery.ToolPackIDs), meta.Pack)
	if pack != "" && ToolRequiresSelect(meta) {
		candidate.SelectablePack = &SelectableToolPack{
			Pack:           pack,
			Tools:          []string{strings.TrimSpace(meta.Name)},
			Capability:     discovery.CapabilityKind,
			RequiresSelect: true,
			RequiredAction: "select this pack to load its callable tools",
		}
	}
	return candidate
}

func toolSearchCandidateSource(meta ToolMetadata) string {
	if meta.HasMCPSource() {
		return "mcp"
	}
	if meta.Origin != "" {
		return string(meta.Origin)
	}
	return string(meta.EffectiveLoadingPolicy())
}

func firstString(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func NormalizeToolSearchRequest(req ToolSearchRequest) ToolSearchRequest {
	req.Mode = strings.TrimSpace(req.Mode)
	req.Query = strings.TrimSpace(req.Query)
	req.Intent = strings.TrimSpace(req.Intent)
	req.RiskLevel = strings.TrimSpace(req.RiskLevel)
	req.SessionType = strings.TrimSpace(req.SessionType)
	req.RuntimeMode = strings.TrimSpace(req.RuntimeMode)
	req.AgentProfile = strings.TrimSpace(req.AgentProfile)
	req.ResourceScope = strings.TrimSpace(req.ResourceScope)
	req.EvidencePreference = strings.TrimSpace(req.EvidencePreference)
	req.Ranker = strings.TrimSpace(req.Ranker)
	req.TargetRefs = normalizeToolSearchStrings(req.TargetRefs)
	req.RequiredCaps = normalizeToolSearchStrings(req.RequiredCaps)
	req.ForbiddenCaps = normalizeToolSearchStrings(req.ForbiddenCaps)
	req.EnvironmentFacts = normalizeToolSearchStrings(req.EnvironmentFacts)
	req.MCPHealth = normalizeToolSearchHealth(req.MCPHealth)
	return req
}

func normalizeToolSearchStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeToolSearchHealth(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.ToLower(strings.TrimSpace(value))
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
