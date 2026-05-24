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
		"query": {"type": "string", "description": "Natural language description of the operational tool needed"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 20, "description": "Maximum number of matches to return"}
	},
	"required": ["query"]
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
					"requiresApproval": {"type": "boolean"}
				}
			}
		}
	}
}`)

type searchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type searchOutput struct {
	Matches []searchMatch `json:"matches"`
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
	if req.Query == "" {
		return tooling.ToolResult{}, fmt.Errorf("tool_search: query is required")
	}
	limit := req.Limit
	if limit <= 0 || limit > 20 {
		limit = defaultLimit
	}

	terms := searchTerms(req.Query)
	scored := make([]scoredMatch, 0)
	packs := map[string]*packCandidate{}
	for _, candidate := range provider.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{IncludeDeferredCatalog: true}) {
		meta := candidate.Metadata()
		if shouldOmit(meta.Name) {
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
				Kind:        "pack",
				Name:        pack.name,
				Description: pack.description,
				Domain:      pack.domain,
				Layer:       tooling.ToolLayerDeferred,
				Pack:        pack.name,
				Deferred:    true,
				Tools:       pack.tools,
				RiskLevel:   tooling.ToolRiskLow,
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

	out := searchOutput{Matches: make([]searchMatch, 0, len(scored))}
	for _, item := range scored {
		out.Matches = append(out.Matches, item.match)
	}
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
	haystack := strings.ToLower(strings.Join([]string{
		meta.Name,
		meta.Description,
		meta.SearchHint,
		meta.Domain,
		strings.Join(meta.Aliases, " "),
	}, " "))

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
	}, terms)
	if score > pack.score {
		pack.score = score
	}
}

func shouldOmit(name string) bool {
	if name == "tool_search" || name == "update_plan" {
		return true
	}
	for _, prefix := range []string{"k8s.", "changes.", "runbook.", "fallback.", "erp."} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
