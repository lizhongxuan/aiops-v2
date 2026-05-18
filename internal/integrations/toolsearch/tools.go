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
					"name": {"type": "string"},
					"description": {"type": "string"},
					"domain": {"type": "string"},
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
	Name             string                `json:"name"`
	Description      string                `json:"description,omitempty"`
	Domain           string                `json:"domain,omitempty"`
	Mock             bool                  `json:"mock,omitempty"`
	RiskLevel        tooling.ToolRiskLevel `json:"riskLevel"`
	Mutating         bool                  `json:"mutating"`
	RequiresApproval bool                  `json:"requiresApproval"`
}

type scoredMatch struct {
	match searchMatch
	score int
}

// NewToolSearchTool creates a read-only discovery tool for the current registry.
func NewToolSearchTool(registry *tooling.Registry) tooling.Tool {
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
			return executeSearch(ctx, registry, input)
		},
	}
}

func executeSearch(_ context.Context, registry *tooling.Registry, input json.RawMessage) (tooling.ToolResult, error) {
	if registry == nil {
		return tooling.ToolResult{}, fmt.Errorf("tool_search: registry is required")
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
	for _, candidate := range registry.AssembleTools("host", "inspect") {
		meta := candidate.Metadata()
		if shouldOmit(meta.Name) {
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
				Name:             meta.Name,
				Description:      meta.Description,
				Domain:           meta.Domain,
				Mock:             meta.Mock,
				RiskLevel:        gov.RiskLevel,
				Mutating:         gov.Mutating,
				RequiresApproval: gov.RequiresApproval,
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

func shouldOmit(name string) bool {
	if name == "tool_search" || name == "update_plan" {
		return true
	}
	for _, prefix := range []string{"runbook.", "fallback.", "erp."} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
