package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

const (
	defaultLimit                = 10
	defaultMCPServerBucketLimit = 8
)

var inputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mode": {"type": "string", "enum": ["search", "select", "describe"], "description": "Discovery mode. Defaults to search for backward compatibility."},
		"session_type": {"type": "string", "description": "Runtime session type used for catalog scoping, for example host or workspace"},
		"runtime_mode": {"type": "string", "description": "Runtime mode used for catalog scoping, for example chat, inspect, plan, or execute"},
		"agent_profile": {"type": "string", "description": "Agent profile used for profile-gated tools"},
		"resource_scope": {"type": "string", "description": "Optional generic resource scope for search ranking"},
		"intent": {"type": "string", "description": "Optional task intent for search ranking"},
		"evidence_preference": {"type": "string", "description": "Optional evidence preference for search ranking"},
		"target_refs": {"type": "array", "items": {"type": "string"}, "description": "Explicit user-selected targets such as service:checkout, host:db-a, @local, or @IP"},
		"required_caps": {"type": "array", "items": {"type": "string"}, "description": "Capabilities that search results must provide, for example read, inspect, execute"},
		"forbidden_caps": {"type": "array", "items": {"type": "string"}, "description": "Capabilities that search results must not require"},
		"risk_level": {"type": "string", "description": "Maximum acceptable risk level for this search, for example low, medium, high, or critical"},
		"environment_facts": {"type": "array", "items": {"type": "string"}, "description": "Known environment facts used only for ranking and traceability"},
		"query": {"type": "string", "description": "Natural language description of the deferred MCP or dynamic operational tool needed"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum number of matches to return"},
		"includeLoaded": {"type": "boolean", "description": "Whether already loaded/currently visible tools should be included in search output for diagnostics"},
		"include_unavailable": {"type": "boolean", "description": "Include unavailable MCP candidates as non-selectable search results"},
		"mcp_health": {"type": "object", "additionalProperties": {"type": "string"}, "description": "MCP server health snapshot keyed by server id"},
		"tools": {"type": "array", "items": {"type": "string"}, "description": "Tool names to select or describe"},
		"packs": {"type": "array", "items": {"type": "string"}, "description": "Tool packs to select"},
		"reason": {"type": "string", "description": "Why selected capabilities are needed for the current task"},
		"detail": {"type": "string", "enum": ["compact", "schema"], "description": "Describe detail level"}
	}
}`)

var outputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mode": {"type": "string"},
		"ranker": {"type": "string"},
		"request": {"type": "object"},
		"matches": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"kind": {"type": "string"},
					"name": {"type": "string"},
					"description": {"type": "string"},
					"domain": {"type": "string"},
					"layer": {"type": "string"},
					"pack": {"type": "string"},
					"deferred": {"type": "boolean"},
					"tools": {"type": "array", "items": {"type": "string"}},
					"mock": {"type": "boolean"},
					"riskLevel": {"type": "string"},
					"mutating": {"type": "boolean"},
					"requiresApproval": {"type": "boolean"},
					"capabilityKind": {"type": "string"},
					"resourceTypes": {"type": "array", "items": {"type": "string"}},
					"operationKinds": {"type": "array", "items": {"type": "string"}},
					"requiresSelect": {"type": "boolean"},
					"status": {"type": "string"},
					"source": {"type": "string"},
					"mcpServerId": {"type": "string"},
					"healthStatus": {"type": "string"},
					"filteredReason": {"type": "string"},
					"targetCompatibility": {"type": "string"},
					"riskDecision": {"type": "string"},
					"matchReasons": {"type": "array", "items": {"type": "string"}},
					"why": {"type": "string"},
					"selectHint": {"type": "string"}
				}
			}
		},
		"rejected": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"reason": {"type": "string"},
					"status": {"type": "string"},
					"source": {"type": "string"},
					"mcpServerId": {"type": "string"},
					"healthStatus": {"type": "string"},
					"filteredReason": {"type": "string"},
					"targetCompatibility": {"type": "string"},
					"riskDecision": {"type": "string"},
					"matchReasons": {"type": "array", "items": {"type": "string"}}
				}
			}
		},
		"selection": {"type": "object"},
		"descriptions": {"type": "array"}
	}
}`)

type searchInput struct {
	Mode               string            `json:"mode"`
	SessionType        string            `json:"session_type"`
	RuntimeMode        string            `json:"runtime_mode"`
	AgentProfile       string            `json:"agent_profile"`
	ResourceScope      string            `json:"resource_scope"`
	Intent             string            `json:"intent"`
	EvidencePreference string            `json:"evidence_preference"`
	TargetRefs         []string          `json:"target_refs"`
	RequiredCaps       []string          `json:"required_caps"`
	ForbiddenCaps      []string          `json:"forbidden_caps"`
	RiskLevel          string            `json:"risk_level"`
	EnvironmentFacts   []string          `json:"environment_facts"`
	Query              string            `json:"query"`
	Limit              int               `json:"limit"`
	IncludeLoaded      bool              `json:"includeLoaded"`
	IncludeUnavailable bool              `json:"include_unavailable"`
	MCPHealth          map[string]string `json:"mcp_health"`
	Tools              []string          `json:"tools"`
	Packs              []string          `json:"packs"`
	Reason             string            `json:"reason"`
	Detail             string            `json:"detail"`
}

type searchOutput struct {
	Mode         string                          `json:"mode"`
	Ranker       string                          `json:"ranker,omitempty"`
	Request      *tooling.ToolSearchRequest      `json:"request,omitempty"`
	Matches      []searchMatch                   `json:"matches,omitempty"`
	Rejected     []tooling.RejectedToolCandidate `json:"rejected,omitempty"`
	Selection    *selectionPayload               `json:"selection,omitempty"`
	Descriptions []describePayload               `json:"descriptions,omitempty"`
	Error        string                          `json:"error,omitempty"`
}

type selectionPayload struct {
	LoadedTools      []string          `json:"loadedTools,omitempty"`
	LoadedPacks      []string          `json:"loadedPacks,omitempty"`
	NotLoaded        []string          `json:"notLoaded,omitempty"`
	NotLoadedReasons map[string]string `json:"notLoadedReasons,omitempty"`
	Reason           string            `json:"reason,omitempty"`
}

type describePayload struct {
	Name             string                `json:"name"`
	Description      string                `json:"description,omitempty"`
	Pack             string                `json:"pack,omitempty"`
	Layer            tooling.ToolLayer     `json:"layer,omitempty"`
	RiskLevel        tooling.ToolRiskLevel `json:"riskLevel,omitempty"`
	Mutating         bool                  `json:"mutating,omitempty"`
	RequiresApproval bool                  `json:"requiresApproval,omitempty"`
	CapabilityKind   string                `json:"capabilityKind,omitempty"`
	ResourceTypes    []string              `json:"resourceTypes,omitempty"`
	OperationKinds   []string              `json:"operationKinds,omitempty"`
	RequiresSelect   bool                  `json:"requiresSelect,omitempty"`
	Status           string                `json:"status,omitempty"`
	Source           string                `json:"source,omitempty"`
	MCPServerID      string                `json:"mcpServerId,omitempty"`
	HealthStatus     string                `json:"healthStatus,omitempty"`
	FilteredReason   string                `json:"filteredReason,omitempty"`
	InputSchema      json.RawMessage       `json:"inputSchema,omitempty"`
	OutputSchema     json.RawMessage       `json:"outputSchema,omitempty"`
}

type searchMatch struct {
	Kind                string                `json:"kind,omitempty"`
	Name                string                `json:"name"`
	Description         string                `json:"description,omitempty"`
	Domain              string                `json:"domain,omitempty"`
	Layer               tooling.ToolLayer     `json:"layer,omitempty"`
	Pack                string                `json:"pack,omitempty"`
	Deferred            bool                  `json:"deferred,omitempty"`
	Tools               []string              `json:"tools,omitempty"`
	Mock                bool                  `json:"mock,omitempty"`
	RiskLevel           tooling.ToolRiskLevel `json:"riskLevel"`
	Mutating            bool                  `json:"mutating"`
	RequiresApproval    bool                  `json:"requiresApproval"`
	CapabilityKind      string                `json:"capabilityKind,omitempty"`
	ResourceTypes       []string              `json:"resourceTypes,omitempty"`
	OperationKinds      []string              `json:"operationKinds,omitempty"`
	RequiresSelect      bool                  `json:"requiresSelect,omitempty"`
	Status              string                `json:"status,omitempty"`
	Source              string                `json:"source,omitempty"`
	MCPServerID         string                `json:"mcpServerId,omitempty"`
	HealthStatus        string                `json:"healthStatus,omitempty"`
	FilteredReason      string                `json:"filteredReason,omitempty"`
	TargetCompatibility string                `json:"targetCompatibility,omitempty"`
	RiskDecision        string                `json:"riskDecision,omitempty"`
	MatchReasons        []string              `json:"matchReasons,omitempty"`
	Why                 string                `json:"why,omitempty"`
	SelectHint          string                `json:"selectHint,omitempty"`
}

type packCandidate struct {
	name                string
	description         string
	domain              string
	tools               []string
	score               float64
	capabilityKind      string
	resourceTypes       []string
	operationKinds      []string
	status              string
	source              string
	mcpServerID         string
	healthStatus        string
	filteredReason      string
	targetCompatibility string
	riskDecision        string
	matchReasons        []string
	why                 string
}

type scoredMatch struct {
	match searchMatch
	score float64
}

type toolSearchIndexEntry struct {
	tool         tooling.Tool
	meta         tooling.ToolMetadata
	availability toolAvailabilityResult
	searchText   string
}

type toolSearchCandidateEvaluation struct {
	rejectReason        string
	scoreBoost          float64
	targetCompatibility string
	riskDecision        string
	matchReasons        []string
}

// NewToolSearchTool creates a read-only discovery tool for the current catalog.
func NewToolSearchTool(provider tooling.ToolCatalogProvider) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "tool_search",
			Description: "Search deferred MCP and dynamic operational tools with BM25 by name, description, domain, schema, and governance metadata",
			Origin:      tooling.ToolOriginMeta,
			Layer:       tooling.ToolLayerCore,
			AlwaysLoad:  true,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				HiddenFromDiscovery: true,
				CapabilityKind:      "search",
				ResourceTypes:       []string{"tool"},
				OperationKinds:      []string{"search"},
			},
		},
		Visibility:       tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:  inputSchema,
		OutputSchemaData: outputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return false
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			return executeSearch(ctx, provider, input)
		},
	}
}

func executeSearch(ctx context.Context, provider tooling.ToolCatalogProvider, input json.RawMessage) (tooling.ToolResult, error) {
	if provider == nil {
		return tooling.ToolResult{}, fmt.Errorf("tool_search: catalog provider is required")
	}

	var req searchInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return tooling.ToolResult{}, fmt.Errorf("tool_search: invalid input: %w", err)
		}
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Mode = normalizeMode(req.Mode)
	req.SessionType = normalizeSessionType(req.SessionType)
	req.RuntimeMode = normalizeRuntimeMode(req.RuntimeMode)
	req.AgentProfile = strings.TrimSpace(req.AgentProfile)
	req.ResourceScope = strings.TrimSpace(req.ResourceScope)
	req.Intent = strings.TrimSpace(req.Intent)
	req.EvidencePreference = strings.TrimSpace(req.EvidencePreference)
	req.TargetRefs = trimmedStrings(req.TargetRefs)
	req.RequiredCaps = trimmedStrings(req.RequiredCaps)
	req.ForbiddenCaps = trimmedStrings(req.ForbiddenCaps)
	req.RiskLevel = strings.ToLower(strings.TrimSpace(req.RiskLevel))
	req.EnvironmentFacts = trimmedStrings(req.EnvironmentFacts)
	req.MCPHealth = mergeProviderMCPHealth(provider, req.MCPHealth)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Detail = strings.TrimSpace(req.Detail)
	switch req.Mode {
	case "search":
		if req.Query == "" {
			return tooling.ToolResult{}, fmt.Errorf("tool_search: query is required")
		}
	case "select":
		if len(trimmedStrings(req.Tools)) == 0 && len(trimmedStrings(req.Packs)) == 0 {
			return tooling.ToolResult{}, fmt.Errorf("tool_search: select requires tools or packs")
		}
		if req.Reason == "" {
			return tooling.ToolResult{}, fmt.Errorf("tool_search: select requires reason")
		}
	case "describe":
		if len(trimmedStrings(req.Tools)) == 0 {
			return tooling.ToolResult{}, fmt.Errorf("tool_search: describe requires tools")
		}
	}
	useDefaultLimit := req.Limit <= 0 || req.Limit > 20
	limit := req.Limit
	if useDefaultLimit {
		limit = defaultLimit
	}

	catalog := provider.AssembleToolsWithOptions(req.SessionType, req.RuntimeMode, tooling.AssembleOptions{
		Profile:                req.AgentProfile,
		IncludeDeferredCatalog: true,
		MCPHealthSnapshot:      req.MCPHealth,
	})
	if execCtx, ok := tooling.ToolExecutionContextFrom(ctx); ok {
		catalog = tooling.FilterToolsByPackMetadata(catalog, execCtx.Metadata)
	}
	switch req.Mode {
	case "select":
		return emitOutput(selectTools(catalog, req))
	case "describe":
		return emitOutput(describeTools(catalog, req))
	}

	terms := searchTerms(req.Query)
	request := toolSearchV3Request(req, limit)
	entries := buildToolSearchIndexEntries(catalog, req)
	bm25Scores := scoreToolSearchEntries(entries, req.Query)
	scored := make([]scoredMatch, 0)
	rejected := make([]tooling.RejectedToolCandidate, 0)
	packs := map[string]*packCandidate{}
	for index, entry := range entries {
		meta := entry.meta
		score := combinedToolSearchScore(meta, bm25Scores[index], terms)
		if meta.Mock {
			score -= 0.25
		}
		evaluation := evaluateToolSearchCandidate(meta, entry.searchText, score, req)
		score += evaluation.scoreBoost
		if score <= 0 {
			continue
		}
		if evaluation.rejectReason != "" {
			rejected = append(rejected, rejectedToolCandidate(meta, entry.availability, evaluation))
			continue
		}
		if !req.IncludeUnavailable && !entry.availability.selectable {
			rejected = append(rejected, rejectedToolCandidate(meta, entry.availability, evaluation))
			continue
		}
		if isDeferredPackTool(meta) {
			accumulatePackCandidate(packs, meta, entry.availability, evaluation, score)
			continue
		}
		gov := meta.EffectiveGovernance(0)
		discovery := meta.EffectiveDiscovery()
		availability := entry.availability
		requiresSelect := tooling.ToolRequiresSelect(meta)
		scored = append(scored, scoredMatch{
			score: score,
			match: searchMatch{
				Kind:                "tool",
				Name:                meta.Name,
				Description:         meta.Description,
				Domain:              meta.Domain,
				Layer:               meta.Layer,
				Pack:                meta.Pack,
				Mock:                meta.Mock,
				RiskLevel:           gov.RiskLevel,
				Mutating:            gov.Mutating,
				RequiresApproval:    gov.RequiresApproval,
				CapabilityKind:      discovery.CapabilityKind,
				ResourceTypes:       discovery.ResourceTypes,
				OperationKinds:      discovery.OperationKinds,
				RequiresSelect:      requiresSelect,
				Status:              availability.status,
				Source:              availability.source,
				MCPServerID:         availability.mcpServerID,
				HealthStatus:        availability.healthStatus,
				FilteredReason:      availability.filteredReason,
				TargetCompatibility: evaluation.targetCompatibility,
				RiskDecision:        evaluation.riskDecision,
				MatchReasons:        evaluation.matchReasons,
				Why:                 searchWhy(score, availability),
				SelectHint:          selectHint(requiresSelect),
			},
		})
	}
	for _, pack := range packs {
		if pack.score <= 0 {
			continue
		}
		sort.Strings(pack.tools)
		scored = append(scored, scoredMatch{
			score: pack.score,
			match: searchMatch{
				Kind:                "pack",
				Name:                pack.name,
				Description:         pack.description,
				Domain:              pack.domain,
				Layer:               tooling.ToolLayerDeferred,
				Pack:                pack.name,
				Deferred:            true,
				Tools:               pack.tools,
				RiskLevel:           tooling.ToolRiskLow,
				CapabilityKind:      pack.capabilityKind,
				ResourceTypes:       pack.resourceTypes,
				OperationKinds:      pack.operationKinds,
				RequiresSelect:      true,
				Status:              firstNonEmpty(pack.status, "deferred"),
				Source:              firstNonEmpty(pack.source, string(tooling.ToolLoadingPolicyDeferred)),
				MCPServerID:         pack.mcpServerID,
				HealthStatus:        pack.healthStatus,
				FilteredReason:      pack.filteredReason,
				TargetCompatibility: pack.targetCompatibility,
				RiskDecision:        pack.riskDecision,
				MatchReasons:        pack.matchReasons,
				Why:                 firstNonEmpty(pack.why, "matched_deferred_pack"),
				SelectHint:          selectHint(true),
			},
		})
	}
	scored = filterScopeMismatchedDeferredMatches(scored, terms)

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].match.Name < scored[j].match.Name
	})
	if useDefaultLimit {
		scored = limitDefaultMCPServerBuckets(scored)
	}
	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := searchOutput{
		Mode:     "search",
		Ranker:   "bm25",
		Request:  &request,
		Matches:  make([]searchMatch, 0, len(scored)),
		Rejected: rejected,
	}
	for _, item := range scored {
		out.Matches = append(out.Matches, item.match)
	}
	return emitOutput(out)
}

func filterScopeMismatchedDeferredMatches(scored []scoredMatch, terms []string) []scoredMatch {
	if len(scored) == 0 {
		return scored
	}
	termSet := termSetFromTerms(terms)
	scopes := []struct {
		queryTerms []string
		resources  []string
	}{
		{
			queryTerms: []string{"host", "hosts", "server", "servers", "node", "nodes", "system", "主机", "机器", "服务器", "节点", "系统"},
			resources:  []string{"host", "hosts", "server", "servers", "node", "nodes", "system", "主机", "机器", "服务器", "节点", "系统"},
		},
		{
			queryTerms: []string{"service", "services", "application", "applications", "app", "apps", "服务", "应用"},
			resources:  []string{"service", "services", "application", "applications", "app", "apps", "服务", "应用"},
		},
	}
	for _, scope := range scopes {
		if !termSetHasAny(termSet, scope.queryTerms...) || !hasDirectToolForResourceScope(scored, scope.resources) {
			continue
		}
		filtered := scored[:0]
		for _, item := range scored {
			if item.match.Kind != "pack" {
				filtered = append(filtered, item)
				continue
			}
			if item.match.Domain != "" && termSetHasAny(termSet, item.match.Domain) {
				filtered = append(filtered, item)
				continue
			}
			if searchMatchHasAnyResource(item.match, scope.resources) {
				filtered = append(filtered, item)
			}
		}
		scored = filtered
	}
	return scored
}

func hasDirectToolForResourceScope(scored []scoredMatch, resources []string) bool {
	for _, item := range scored {
		if item.match.Kind == "tool" && !item.match.Deferred && searchMatchHasAnyResource(item.match, resources) {
			return true
		}
	}
	return false
}

func searchMatchHasAnyResource(match searchMatch, values []string) bool {
	wants := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			wants[value] = struct{}{}
		}
	}
	for _, value := range match.ResourceTypes {
		if _, ok := wants[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func limitDefaultMCPServerBuckets(scored []scoredMatch) []scoredMatch {
	if len(scored) == 0 {
		return scored
	}
	counts := map[string]int{}
	filtered := make([]scoredMatch, 0, len(scored))
	for _, item := range scored {
		serverID := strings.TrimSpace(item.match.MCPServerID)
		if serverID == "" {
			filtered = append(filtered, item)
			continue
		}
		if counts[serverID] >= defaultMCPServerBucketLimit {
			continue
		}
		counts[serverID]++
		filtered = append(filtered, item)
	}
	return filtered
}

func emitOutput(out searchOutput) (tooling.ToolResult, error) {
	content, err := json.Marshal(out)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "tool_search",
			Title: "Tool search",
			Data:  content,
		},
	}, nil
}

func selectTools(catalog []tooling.Tool, req searchInput) searchOutput {
	toolsByName := make(map[string]tooling.Tool)
	packs := make(map[string][]string)
	for _, tool := range catalog {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if tooling.ToolHiddenFromDiscovery(meta) {
			continue
		}
		toolsByName[meta.Name] = tool
		for _, alias := range meta.Aliases {
			if strings.TrimSpace(alias) != "" {
				toolsByName[alias] = tool
			}
		}
		if meta.Pack != "" {
			packs[meta.Pack] = append(packs[meta.Pack], meta.Name)
		}
	}
	selection := &selectionPayload{Reason: req.Reason}
	for _, pack := range trimmedStrings(req.Packs) {
		if _, ok := packs[pack]; ok {
			if packSelectable(catalog, pack, req) {
				selection.LoadedPacks = append(selection.LoadedPacks, pack)
			} else {
				selection.NotLoaded = append(selection.NotLoaded, pack)
				selection.NotLoadedReasons = addNotLoadedReason(selection.NotLoadedReasons, pack, "pack_unavailable")
			}
		} else {
			selection.NotLoaded = append(selection.NotLoaded, pack)
		}
	}
	for _, name := range trimmedStrings(req.Tools) {
		tool, ok := toolsByName[name]
		if !ok {
			selection.NotLoaded = append(selection.NotLoaded, name)
			continue
		}
		meta := tool.Metadata()
		availability := toolAvailability(meta, req)
		if !availability.selectable {
			selection.NotLoaded = append(selection.NotLoaded, name)
			selection.NotLoadedReasons = addNotLoadedReason(selection.NotLoadedReasons, name, availability.filteredReason)
			continue
		}
		selection.LoadedTools = append(selection.LoadedTools, meta.Name)
	}
	sort.Strings(selection.LoadedPacks)
	sort.Strings(selection.LoadedTools)
	sort.Strings(selection.NotLoaded)
	return searchOutput{Mode: "select", Selection: selection}
}

func packSelectable(catalog []tooling.Tool, pack string, req searchInput) bool {
	hasTool := false
	for _, tool := range catalog {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if meta.Pack != pack {
			continue
		}
		hasTool = true
		if toolAvailability(meta, req).selectable {
			return true
		}
	}
	return !hasTool
}

func addNotLoadedReason(reasons map[string]string, name, reason string) map[string]string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "not_selectable"
	}
	if reasons == nil {
		reasons = map[string]string{}
	}
	reasons[name] = reason
	return reasons
}

func describeTools(catalog []tooling.Tool, req searchInput) searchOutput {
	want := make(map[string]bool)
	for _, name := range trimmedStrings(req.Tools) {
		want[name] = true
	}
	out := searchOutput{Mode: "describe"}
	detail := req.Detail
	if detail == "" {
		detail = "compact"
	}
	for _, tool := range catalog {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if !want[meta.Name] {
			matchedAlias := false
			for _, alias := range meta.Aliases {
				if want[alias] {
					matchedAlias = true
					break
				}
			}
			if !matchedAlias {
				continue
			}
		}
		gov := meta.EffectiveGovernance(0)
		discovery := meta.EffectiveDiscovery()
		availability := toolAvailability(meta, req)
		desc := describePayload{
			Name:             meta.Name,
			Description:      meta.Description,
			Pack:             meta.Pack,
			Layer:            meta.Layer,
			RiskLevel:        gov.RiskLevel,
			Mutating:         gov.Mutating,
			RequiresApproval: gov.RequiresApproval,
			CapabilityKind:   discovery.CapabilityKind,
			ResourceTypes:    discovery.ResourceTypes,
			OperationKinds:   discovery.OperationKinds,
			RequiresSelect:   tooling.ToolRequiresSelect(meta),
			Status:           availability.status,
			Source:           availability.source,
			MCPServerID:      availability.mcpServerID,
			HealthStatus:     availability.healthStatus,
			FilteredReason:   availability.filteredReason,
		}
		if detail == "schema" && !desc.RequiresSelect && !gov.RequiresApproval {
			desc.InputSchema = tool.InputSchema()
			desc.OutputSchema = tool.OutputSchema()
		}
		out.Descriptions = append(out.Descriptions, desc)
	}
	sort.Slice(out.Descriptions, func(i, j int) bool {
		return out.Descriptions[i].Name < out.Descriptions[j].Name
	})
	return out
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "search":
		return "search"
	case "select", "describe":
		return mode
	default:
		return "search"
	}
}

func normalizeSessionType(sessionType string) string {
	sessionType = strings.ToLower(strings.TrimSpace(sessionType))
	switch sessionType {
	case "workspace":
		return "workspace"
	default:
		return "host"
	}
}

func normalizeRuntimeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "inspect", "plan", "execute":
		return mode
	default:
		return "chat"
	}
}

type toolAvailabilityResult struct {
	status         string
	source         string
	mcpServerID    string
	healthStatus   string
	filteredReason string
	selectable     bool
}

func toolAvailability(meta tooling.ToolMetadata, req searchInput) toolAvailabilityResult {
	discovery := meta.EffectiveDiscovery()
	source := string(discovery.LoadingPolicy)
	if source == "" {
		source = string(meta.Layer)
	}
	if source == "" {
		source = string(tooling.ToolLoadingPolicyCore)
	}
	out := toolAvailabilityResult{
		status:     "ready",
		source:     source,
		selectable: true,
	}
	if tooling.ToolRequiresSelect(meta) {
		out.status = "deferred"
	}
	if meta.HasMCPSource() {
		out.source = string(tooling.ToolLoadingPolicyMCP)
		out.mcpServerID = discovery.MCPServerID
		out.healthStatus = strings.ToLower(strings.TrimSpace(req.MCPHealth[out.mcpServerID]))
		if out.healthStatus == "" {
			out.healthStatus = "unknown"
		}
		switch out.healthStatus {
		case "disabled":
			out.status = "disabled"
			out.filteredReason = "mcp_disabled"
			out.selectable = false
		case "unavailable":
			out.status = "unavailable"
			out.filteredReason = "mcp_unavailable"
			out.selectable = false
		case "degraded":
			out.status = "unavailable"
			out.filteredReason = "mcp_degraded"
			out.selectable = false
		}
	}
	return out
}

func mergeProviderMCPHealth(provider tooling.ToolCatalogProvider, explicit map[string]string) map[string]string {
	out := make(map[string]string, len(explicit))
	for serverID, status := range explicit {
		serverID = strings.TrimSpace(serverID)
		status = strings.ToLower(strings.TrimSpace(status))
		if serverID != "" && status != "" {
			out[serverID] = status
		}
	}
	if healthProvider, ok := provider.(interface {
		ToolHealthSnapshots() map[string]string
	}); ok {
		for serverID, status := range healthProvider.ToolHealthSnapshots() {
			serverID = strings.TrimSpace(serverID)
			status = strings.ToLower(strings.TrimSpace(status))
			if serverID == "" || status == "" {
				continue
			}
			if _, exists := out[serverID]; !exists {
				out[serverID] = status
			}
		}
	}
	registry := mcp.DefaultRegistry()
	if registry == nil {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	for _, snapshot := range registry.ListServerHealthSnapshots() {
		serverID := strings.TrimSpace(snapshot.ServerID)
		if serverID == "" {
			continue
		}
		if _, exists := out[serverID]; exists {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(string(snapshot.Status)))
		if status != "" {
			out[serverID] = status
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func searchWhy(score float64, availability toolAvailabilityResult) string {
	if availability.filteredReason != "" {
		return availability.filteredReason
	}
	if score > 0 {
		return "matched_query"
	}
	return "catalog_candidate"
}

func trimmedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func buildToolSearchIndexEntries(catalog []tooling.Tool, req searchInput) []toolSearchIndexEntry {
	entries := make([]toolSearchIndexEntry, 0, len(catalog))
	for _, candidate := range catalog {
		if candidate == nil {
			continue
		}
		meta := candidate.Metadata()
		if tooling.ToolHiddenFromDiscovery(meta) {
			continue
		}
		if !req.IncludeLoaded && !toolSearchDeferredCandidate(meta) {
			continue
		}
		availability := toolAvailability(meta, req)
		entries = append(entries, toolSearchIndexEntry{
			tool:         candidate,
			meta:         meta,
			availability: availability,
			searchText:   toolSearchIndexText(candidate, meta),
		})
	}
	return entries
}

func toolSearchDeferredCandidate(meta tooling.ToolMetadata) bool {
	if tooling.ToolRequiresSelect(meta) || meta.HasMCPSource() || meta.DeferByDefault {
		return true
	}
	discovery := meta.EffectiveDiscovery()
	switch discovery.LoadingPolicy {
	case tooling.ToolLoadingPolicyDeferred, tooling.ToolLoadingPolicyMCP, tooling.ToolLoadingPolicyProfile, tooling.ToolLoadingPolicyConditional:
		return true
	}
	return strings.TrimSpace(meta.Pack) != ""
}

func toolSearchV3Request(req searchInput, limit int) tooling.ToolSearchRequest {
	return tooling.NormalizeToolSearchRequest(tooling.ToolSearchRequest{
		Mode:               "search",
		Query:              req.Query,
		Intent:             req.Intent,
		SessionType:        req.SessionType,
		RuntimeMode:        req.RuntimeMode,
		AgentProfile:       req.AgentProfile,
		ResourceScope:      req.ResourceScope,
		EvidencePreference: req.EvidencePreference,
		TargetRefs:         req.TargetRefs,
		RequiredCaps:       req.RequiredCaps,
		ForbiddenCaps:      req.ForbiddenCaps,
		RiskLevel:          req.RiskLevel,
		Limit:              limit,
		IncludeUnavailable: req.IncludeUnavailable,
		MCPHealth:          req.MCPHealth,
		EnvironmentFacts:   req.EnvironmentFacts,
		Ranker:             "bm25",
	})
}

func rejectedToolCandidate(meta tooling.ToolMetadata, availability toolAvailabilityResult, evaluation toolSearchCandidateEvaluation) tooling.RejectedToolCandidate {
	reason := strings.TrimSpace(availability.filteredReason)
	if strings.TrimSpace(evaluation.rejectReason) != "" {
		reason = strings.TrimSpace(evaluation.rejectReason)
	}
	if reason == "" {
		reason = "not_selectable"
	}
	return tooling.RejectedToolCandidate{
		Name:                strings.TrimSpace(meta.Name),
		Reason:              reason,
		Status:              strings.TrimSpace(availability.status),
		Source:              strings.TrimSpace(availability.source),
		MCPServerID:         strings.TrimSpace(availability.mcpServerID),
		HealthStatus:        strings.TrimSpace(availability.healthStatus),
		FilteredReason:      strings.TrimSpace(availability.filteredReason),
		TargetCompatibility: strings.TrimSpace(evaluation.targetCompatibility),
		RiskDecision:        strings.TrimSpace(evaluation.riskDecision),
		MatchReasons:        append([]string(nil), evaluation.matchReasons...),
	}
}

func scoreToolSearchEntries(entries []toolSearchIndexEntry, query string) map[int]float64 {
	if len(entries) == 0 {
		return nil
	}
	documents := make([]tooling.BM25Document, 0, len(entries))
	for index, entry := range entries {
		documents = append(documents, tooling.BM25Document{ID: index, Text: entry.searchText})
	}
	index := tooling.NewBM25Index(documents)
	results := index.Search(query, len(entries))
	scores := make(map[int]float64, len(results))
	for _, result := range results {
		scores[result.ID] = result.Score
	}
	return scores
}

func combinedToolSearchScore(meta tooling.ToolMetadata, bm25Score float64, terms []string) float64 {
	score := bm25Score * 10
	alignment := scoreDiscoveryAlignment(meta, terms)
	score += float64(alignment)
	if score == 0 && lexicalTermMatchScore(tooling.ToolDiscoverySearchText(meta), terms) > 0 {
		score = 0.1
	}
	return score
}

func evaluateToolSearchCandidate(meta tooling.ToolMetadata, searchText string, baseScore float64, req searchInput) toolSearchCandidateEvaluation {
	discovery := meta.EffectiveDiscovery()
	evaluation := toolSearchCandidateEvaluation{
		targetCompatibility: targetCompatibilityForSearch(discovery, req.TargetRefs),
		riskDecision:        riskDecisionForSearch(meta, req.RiskLevel),
	}
	if baseScore > 0 {
		evaluation.matchReasons = append(evaluation.matchReasons, "bm25")
	}
	switch evaluation.targetCompatibility {
	case "matched":
		evaluation.matchReasons = append(evaluation.matchReasons, "target_compatible", "explicit_target")
		evaluation.scoreBoost += 2
	case "incompatible":
		evaluation.rejectReason = "target_incompatible"
	}
	capabilityDecision := capabilityDecisionForSearch(discovery, req.RequiredCaps, req.ForbiddenCaps)
	switch capabilityDecision {
	case "matched":
		evaluation.matchReasons = append(evaluation.matchReasons, "capability_match")
		evaluation.scoreBoost++
	case "forbidden":
		if evaluation.rejectReason == "" {
			evaluation.rejectReason = "forbidden_capability"
		}
	case "missing_required":
		if evaluation.rejectReason == "" {
			evaluation.rejectReason = "capability_mismatch"
		}
	}
	if lexicalTermMatchScore(searchText, searchTerms(req.Intent)) > 0 {
		evaluation.matchReasons = append(evaluation.matchReasons, "intent_match")
		evaluation.scoreBoost++
	}
	switch evaluation.riskDecision {
	case "allowed":
		if req.RiskLevel != "" {
			evaluation.matchReasons = append(evaluation.matchReasons, "risk_allowed")
			evaluation.scoreBoost += 0.5
		}
	case "exceeds_request":
		if evaluation.rejectReason == "" {
			evaluation.rejectReason = "risk_exceeds_request"
		}
	}
	if environmentFactMatchScore(searchText, req.EnvironmentFacts) > 0 {
		evaluation.matchReasons = append(evaluation.matchReasons, "environment_fact_match")
		evaluation.scoreBoost++
	}
	evaluation.matchReasons = uniqueNonEmptyStrings(evaluation.matchReasons)
	return evaluation
}

func targetCompatibilityForSearch(discovery tooling.ToolDiscoveryMetadata, targetRefs []string) string {
	targetTypes := targetTypesFromRefs(targetRefs)
	if len(targetTypes) == 0 {
		return "unspecified"
	}
	resourceTypes := append([]string{}, discovery.ResourceTypes...)
	resourceTypes = append(resourceTypes, discovery.TargetKinds...)
	if len(resourceTypes) == 0 {
		return "unknown"
	}
	for _, resource := range resourceTypes {
		resource = strings.ToLower(strings.TrimSpace(resource))
		if _, ok := targetTypes[resource]; ok {
			return "matched"
		}
	}
	return "incompatible"
}

func targetTypesFromRefs(targetRefs []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, ref := range targetRefs {
		ref = strings.ToLower(strings.TrimSpace(ref))
		if ref == "" {
			continue
		}
		if strings.HasPrefix(ref, "@") {
			out["host"] = struct{}{}
			continue
		}
		if index := strings.Index(ref, ":"); index > 0 {
			if kind := strings.TrimSpace(ref[:index]); kind != "" {
				out[kind] = struct{}{}
			}
			continue
		}
		if strings.Count(ref, ".") == 3 {
			out["host"] = struct{}{}
			continue
		}
	}
	return out
}

func capabilityDecisionForSearch(discovery tooling.ToolDiscoveryMetadata, requiredCaps, forbiddenCaps []string) string {
	caps := capabilitySetForSearch(discovery)
	for _, cap := range forbiddenCaps {
		if _, ok := caps[strings.ToLower(strings.TrimSpace(cap))]; ok {
			return "forbidden"
		}
	}
	if len(requiredCaps) == 0 {
		return ""
	}
	for _, cap := range requiredCaps {
		if _, ok := caps[strings.ToLower(strings.TrimSpace(cap))]; !ok {
			return "missing_required"
		}
	}
	return "matched"
}

func capabilitySetForSearch(discovery tooling.ToolDiscoveryMetadata) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range append([]string{discovery.CapabilityKind}, discovery.OperationKinds...) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	for _, value := range discovery.Capabilities {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func riskDecisionForSearch(meta tooling.ToolMetadata, maxRisk string) string {
	maxRisk = strings.ToLower(strings.TrimSpace(maxRisk))
	if maxRisk == "" {
		return ""
	}
	if riskRank(string(meta.EffectiveGovernance(0).RiskLevel)) > riskRank(maxRisk) {
		return "exceeds_request"
	}
	return "allowed"
}

func riskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 1
	case "medium", "":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 2
	}
}

func environmentFactMatchScore(searchText string, facts []string) int {
	if strings.TrimSpace(searchText) == "" || len(facts) == 0 {
		return 0
	}
	terms := searchTerms(strings.Join(facts, " "))
	return lexicalTermMatchScore(searchText, terms)
}

func lexicalTermMatchScore(text string, terms []string) int {
	if strings.TrimSpace(text) == "" || len(terms) == 0 {
		return 0
	}
	text = strings.ToLower(text)
	score := 0
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(text, term) {
			score++
		}
	}
	return score
}

func uniqueNonEmptyStrings(values []string) []string {
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
	return out
}

func toolSearchIndexText(tool tooling.Tool, meta tooling.ToolMetadata) string {
	parts := []string{tooling.ToolDiscoverySearchText(meta)}
	parts = append(parts, schemaPropertySearchText(tool.InputSchema())...)
	return strings.Join(parts, " ")
}

func schemaPropertySearchText(schema json.RawMessage) []string {
	if len(schema) == 0 {
		return nil
	}
	var payload struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &payload); err != nil || len(payload.Properties) == 0 {
		return nil
	}
	out := make([]string, 0, len(payload.Properties)*2)
	for key := range payload.Properties {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out = append(out, key)
		if expanded := strings.ReplaceAll(key, "_", " "); expanded != key {
			out = append(out, expanded)
		}
	}
	sort.Strings(out)
	return out
}

func searchTerms(query string) []string {
	parts := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(parts))
	seen := make(map[string]bool)
	for _, part := range parts {
		part = strings.Trim(part, ".,;:!?()[]{}\"'")
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		terms = append(terms, part)
	}
	return terms
}

func scoreTool(meta tooling.ToolMetadata, terms []string) int {
	haystack := tooling.ToolDiscoverySearchText(meta)

	score := 0
	for _, term := range terms {
		if strings.Contains(haystack, term) {
			score += 2
		}
		if strings.Contains(strings.ToLower(meta.Name), term) {
			score++
		}
	}
	score += scoreDiscoveryAlignment(meta, terms)
	return score
}

func scoreDiscoveryAlignment(meta tooling.ToolMetadata, terms []string) int {
	if len(terms) == 0 {
		return 0
	}
	termSet := termSetFromTerms(terms)
	d := meta.EffectiveDiscovery()
	score := 0
	for _, value := range d.ResourceTypes {
		if _, ok := termSet[strings.ToLower(strings.TrimSpace(value))]; ok {
			score += 4
		}
	}
	for _, value := range d.OperationKinds {
		if _, ok := termSet[strings.ToLower(strings.TrimSpace(value))]; ok {
			score += 2
		}
	}
	if _, ok := termSet[strings.ToLower(strings.TrimSpace(d.CapabilityKind))]; ok {
		score += 2
	}
	for _, value := range d.DiscoveryTags {
		if _, ok := termSet[strings.ToLower(strings.TrimSpace(value))]; ok {
			score++
		}
	}
	score += scoreResourceScopeAlignment(d, termSet)
	return score
}

func termSetFromTerms(terms []string) map[string]struct{} {
	termSet := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" {
			termSet[term] = struct{}{}
		}
	}
	return termSet
}

func scoreResourceScopeAlignment(d tooling.ToolDiscoveryMetadata, termSet map[string]struct{}) int {
	score := 0
	if termSetHasAny(termSet, "host", "hosts", "server", "servers", "node", "nodes", "system", "主机", "机器", "服务器", "节点", "系统") &&
		discoveryHasAnyResource(d, "host", "hosts", "server", "servers", "node", "nodes", "system", "主机", "机器", "服务器", "节点", "系统") {
		score += 8
	}
	if termSetHasAny(termSet, "service", "services", "application", "applications", "app", "apps", "服务", "应用") &&
		discoveryHasAnyResource(d, "service", "services", "application", "applications", "app", "apps", "服务", "应用") {
		score += 8
	}
	return score
}

func termSetHasAny(termSet map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, ok := termSet[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func discoveryHasAnyResource(d tooling.ToolDiscoveryMetadata, values ...string) bool {
	wants := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			wants[value] = struct{}{}
		}
	}
	for _, value := range d.ResourceTypes {
		if _, ok := wants[strings.ToLower(strings.TrimSpace(value))]; ok {
			return true
		}
	}
	return false
}

func isDeferredPackTool(meta tooling.ToolMetadata) bool {
	return meta.Pack != "" && (meta.Layer == tooling.ToolLayerDeferred || meta.DeferByDefault)
}

func accumulatePackCandidate(packs map[string]*packCandidate, meta tooling.ToolMetadata, availability toolAvailabilityResult, evaluation toolSearchCandidateEvaluation, score float64) {
	pack := packs[meta.Pack]
	if pack == nil {
		pack = &packCandidate{name: meta.Pack, domain: meta.Domain}
		packs[meta.Pack] = pack
	}
	pack.tools = append(pack.tools, meta.Name)
	if pack.description == "" {
		pack.description = meta.Description
	}
	if pack.domain == "" {
		pack.domain = meta.Domain
	}
	if score > pack.score {
		pack.score = score
		d := meta.EffectiveDiscovery()
		pack.capabilityKind = d.CapabilityKind
		pack.resourceTypes = d.ResourceTypes
		pack.operationKinds = d.OperationKinds
		pack.status = availability.status
		pack.source = availability.source
		pack.mcpServerID = availability.mcpServerID
		pack.healthStatus = availability.healthStatus
		pack.filteredReason = availability.filteredReason
		pack.targetCompatibility = evaluation.targetCompatibility
		pack.riskDecision = evaluation.riskDecision
		pack.matchReasons = append([]string(nil), evaluation.matchReasons...)
		pack.why = searchWhy(score, availability)
	}
}

func selectHint(requiresSelect bool) string {
	if requiresSelect {
		return "call tool_search with mode=select before using this tool or pack"
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
