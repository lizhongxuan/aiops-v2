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

const defaultLimit = 10

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
		"query": {"type": "string", "description": "Natural language description of the operational tool needed"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum number of matches to return"},
		"includeLoaded": {"type": "boolean", "description": "Whether already selected tools should be included in search output"},
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
					"why": {"type": "string"},
					"selectHint": {"type": "string"}
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
	Mode         string            `json:"mode"`
	Matches      []searchMatch     `json:"matches,omitempty"`
	Selection    *selectionPayload `json:"selection,omitempty"`
	Descriptions []describePayload `json:"descriptions,omitempty"`
	Error        string            `json:"error,omitempty"`
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
	Kind             string                `json:"kind,omitempty"`
	Name             string                `json:"name"`
	Description      string                `json:"description,omitempty"`
	Domain           string                `json:"domain,omitempty"`
	Layer            tooling.ToolLayer     `json:"layer,omitempty"`
	Pack             string                `json:"pack,omitempty"`
	Deferred         bool                  `json:"deferred,omitempty"`
	Tools            []string              `json:"tools,omitempty"`
	Mock             bool                  `json:"mock,omitempty"`
	RiskLevel        tooling.ToolRiskLevel `json:"riskLevel"`
	Mutating         bool                  `json:"mutating"`
	RequiresApproval bool                  `json:"requiresApproval"`
	CapabilityKind   string                `json:"capabilityKind,omitempty"`
	ResourceTypes    []string              `json:"resourceTypes,omitempty"`
	OperationKinds   []string              `json:"operationKinds,omitempty"`
	RequiresSelect   bool                  `json:"requiresSelect,omitempty"`
	Status           string                `json:"status,omitempty"`
	Source           string                `json:"source,omitempty"`
	MCPServerID      string                `json:"mcpServerId,omitempty"`
	HealthStatus     string                `json:"healthStatus,omitempty"`
	FilteredReason   string                `json:"filteredReason,omitempty"`
	Why              string                `json:"why,omitempty"`
	SelectHint       string                `json:"selectHint,omitempty"`
}

type packCandidate struct {
	name           string
	description    string
	domain         string
	tools          []string
	score          int
	capabilityKind string
	resourceTypes  []string
	operationKinds []string
	status         string
	source         string
	mcpServerID    string
	healthStatus   string
	filteredReason string
	why            string
}

type scoredMatch struct {
	match searchMatch
	score int
}

// NewToolSearchTool creates a read-only discovery tool for the current catalog.
func NewToolSearchTool(provider tooling.ToolCatalogProvider) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "tool_search",
			Description: "Search available operational tools by name, description, domain, and governance metadata",
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

func executeSearch(_ context.Context, provider tooling.ToolCatalogProvider, input json.RawMessage) (tooling.ToolResult, error) {
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
	req.MCPHealth = mergeRegistryMCPHealth(req.MCPHealth)
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
	limit := req.Limit
	if limit <= 0 || limit > 20 {
		limit = defaultLimit
	}

	catalog := provider.AssembleToolsWithOptions(req.SessionType, req.RuntimeMode, tooling.AssembleOptions{
		Profile:                req.AgentProfile,
		IncludeDeferredCatalog: true,
		MCPHealthSnapshot:      req.MCPHealth,
	})
	switch req.Mode {
	case "select":
		return emitOutput(selectTools(catalog, req))
	case "describe":
		return emitOutput(describeTools(catalog, req))
	}

	terms := searchTerms(req.Query)
	scored := make([]scoredMatch, 0)
	packs := map[string]*packCandidate{}
	for _, candidate := range catalog {
		meta := candidate.Metadata()
		if tooling.ToolHiddenFromDiscovery(meta) {
			continue
		}
		if isDeferredPackTool(meta) {
			accumulatePackCandidate(packs, meta, terms, req)
			continue
		}
		score := scoreTool(meta, terms)
		if score == 0 {
			continue
		}
		if meta.Mock {
			score--
		}
		gov := meta.EffectiveGovernance(0)
		discovery := meta.EffectiveDiscovery()
		availability := toolAvailability(meta, req)
		if !req.IncludeUnavailable && !availability.selectable {
			continue
		}
		requiresSelect := tooling.ToolRequiresSelect(meta)
		scored = append(scored, scoredMatch{
			score: score,
			match: searchMatch{
				Kind:             "tool",
				Name:             meta.Name,
				Description:      meta.Description,
				Domain:           meta.Domain,
				Layer:            meta.Layer,
				Pack:             meta.Pack,
				Mock:             meta.Mock,
				RiskLevel:        gov.RiskLevel,
				Mutating:         gov.Mutating,
				RequiresApproval: gov.RequiresApproval,
				CapabilityKind:   discovery.CapabilityKind,
				ResourceTypes:    discovery.ResourceTypes,
				OperationKinds:   discovery.OperationKinds,
				RequiresSelect:   requiresSelect,
				Status:           availability.status,
				Source:           availability.source,
				MCPServerID:      availability.mcpServerID,
				HealthStatus:     availability.healthStatus,
				FilteredReason:   availability.filteredReason,
				Why:              searchWhy(score, availability),
				SelectHint:       selectHint(requiresSelect),
			},
		})
	}
	for _, pack := range packs {
		if pack.score == 0 {
			continue
		}
		sort.Strings(pack.tools)
		scored = append(scored, scoredMatch{
			score: pack.score,
			match: searchMatch{
				Kind:           "pack",
				Name:           pack.name,
				Description:    pack.description,
				Domain:         pack.domain,
				Layer:          tooling.ToolLayerDeferred,
				Pack:           pack.name,
				Deferred:       true,
				Tools:          pack.tools,
				RiskLevel:      tooling.ToolRiskLow,
				CapabilityKind: pack.capabilityKind,
				ResourceTypes:  pack.resourceTypes,
				OperationKinds: pack.operationKinds,
				RequiresSelect: true,
				Status:         firstNonEmpty(pack.status, "deferred"),
				Source:         firstNonEmpty(pack.source, string(tooling.ToolLoadingPolicyDeferred)),
				MCPServerID:    pack.mcpServerID,
				HealthStatus:   pack.healthStatus,
				FilteredReason: pack.filteredReason,
				Why:            firstNonEmpty(pack.why, "matched_deferred_pack"),
				SelectHint:     selectHint(true),
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
	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := searchOutput{Mode: "search", Matches: make([]searchMatch, 0, len(scored))}
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

func mergeRegistryMCPHealth(explicit map[string]string) map[string]string {
	out := make(map[string]string, len(explicit))
	for serverID, status := range explicit {
		serverID = strings.TrimSpace(serverID)
		status = strings.ToLower(strings.TrimSpace(status))
		if serverID != "" && status != "" {
			out[serverID] = status
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

func searchWhy(score int, availability toolAvailabilityResult) string {
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

func accumulatePackCandidate(packs map[string]*packCandidate, meta tooling.ToolMetadata, terms []string, req searchInput) {
	availability := toolAvailability(meta, req)
	if !req.IncludeUnavailable && !availability.selectable {
		return
	}
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
	score := scoreTool(tooling.ToolMetadata{
		Name:        strings.Join([]string{meta.Pack, meta.Name}, " "),
		Description: meta.Description,
		SearchHint:  meta.SearchHint,
		Domain:      meta.Domain,
		Aliases:     meta.Aliases,
		Triggers:    meta.Triggers,
		Discovery:   meta.Discovery,
	}, terms)
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
