package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/tooling"
)

const defaultLimit = 10

var inputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mode": {"type": "string", "enum": ["search", "select", "describe"], "description": "Discovery mode. Defaults to search for backward compatibility."},
		"query": {"type": "string", "description": "Natural language description of the operational tool needed"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum number of matches to return"},
		"includeLoaded": {"type": "boolean", "description": "Whether already selected tools should be included in search output"},
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
					"selectHint": {"type": "string"}
				}
			}
		},
		"selection": {"type": "object"},
		"descriptions": {"type": "array"}
	}
}`)

type searchInput struct {
	Mode          string   `json:"mode"`
	Query         string   `json:"query"`
	Limit         int      `json:"limit"`
	IncludeLoaded bool     `json:"includeLoaded"`
	Tools         []string `json:"tools"`
	Packs         []string `json:"packs"`
	Reason        string   `json:"reason"`
	Detail        string   `json:"detail"`
}

type searchOutput struct {
	Mode         string            `json:"mode"`
	Matches      []searchMatch     `json:"matches,omitempty"`
	Selection    *selectionPayload `json:"selection,omitempty"`
	Descriptions []describePayload `json:"descriptions,omitempty"`
	Error        string            `json:"error,omitempty"`
}

type selectionPayload struct {
	LoadedTools []string `json:"loadedTools,omitempty"`
	LoadedPacks []string `json:"loadedPacks,omitempty"`
	NotLoaded   []string `json:"notLoaded,omitempty"`
	Reason      string   `json:"reason,omitempty"`
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
	SelectHint       string                `json:"selectHint,omitempty"`
}

type packCandidate struct {
	name        string
	description string
	domain      string
	tools       []string
	score       int
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

	catalog := provider.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{IncludeDeferredCatalog: true})
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
			accumulatePackCandidate(packs, meta, terms)
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
				RequiresSelect: true,
				SelectHint:     selectHint(true),
			},
		})
	}

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
			selection.LoadedPacks = append(selection.LoadedPacks, pack)
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
		selection.LoadedTools = append(selection.LoadedTools, meta.Name)
	}
	sort.Strings(selection.LoadedPacks)
	sort.Strings(selection.LoadedTools)
	sort.Strings(selection.NotLoaded)
	return searchOutput{Mode: "select", Selection: selection}
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
	return score
}

func isDeferredPackTool(meta tooling.ToolMetadata) bool {
	return meta.Pack != "" && (meta.Layer == tooling.ToolLayerDeferred || meta.DeferByDefault)
}

func accumulatePackCandidate(packs map[string]*packCandidate, meta tooling.ToolMetadata, terms []string) {
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
	}
}

func selectHint(requiresSelect bool) string {
	if requiresSelect {
		return "call tool_search with mode=select before using this tool or pack"
	}
	return ""
}
