package skilltools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/skills"
	"aiops-v2/internal/tooling"
)

const (
	schemaVersion        = "aiops.skill_discovery/v1"
	defaultSearchLimit   = 8
	maxSkillReadChars    = 4096
	defaultSkillReadMode = "text"
)

var searchInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"mode": {"type": "string", "enum": ["search"]},
		"query": {"type": "string"},
		"resourceUri": {"type": "string"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 20},
		"includeLoaded": {"type": "boolean"}
	},
	"required": ["query"]
}`)

var readInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"skill": {"type": "string"},
		"range": {
			"type": "object",
			"properties": {
				"offset": {"type": "integer", "minimum": 0},
				"limit": {"type": "integer", "minimum": 1, "maximum": 4096}
			}
		},
		"format": {"type": "string", "enum": ["text", "metadata"]},
		"reason": {"type": "string"}
	},
	"required": ["skill", "reason"]
}`)

type searchInput struct {
	Mode          string `json:"mode"`
	Query         string `json:"query"`
	ResourceURI   string `json:"resourceUri"`
	Limit         int    `json:"limit"`
	IncludeLoaded bool   `json:"includeLoaded"`
}

type searchOutput struct {
	SchemaVersion  string        `json:"schemaVersion"`
	Mode           string        `json:"mode"`
	SkillIndexHash string        `json:"skillIndexHash,omitempty"`
	Matches        []searchMatch `json:"matches,omitempty"`
	Dropped        []dropped     `json:"dropped,omitempty"`
}

type searchMatch struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	WhenToUse        string   `json:"whenToUse,omitempty"`
	ResourceTypes    []string `json:"resourceTypes,omitempty"`
	TaskIntents      []string `json:"taskIntents,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	RequiresRead     bool     `json:"requiresRead"`
	RequiredForMatch bool     `json:"requiredForMatch,omitempty"`
}

type dropped struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type readInput struct {
	Skill  string    `json:"skill"`
	Range  readRange `json:"range"`
	Format string    `json:"format"`
	Reason string    `json:"reason"`
}

type readRange struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type readOutput struct {
	SchemaVersion string           `json:"schemaVersion"`
	Skill         string           `json:"skill"`
	Description   string           `json:"description,omitempty"`
	WhenToUse     string           `json:"whenToUse,omitempty"`
	Body          string           `json:"body,omitempty"`
	MetadataOnly  bool             `json:"metadataOnly,omitempty"`
	Range         readRange        `json:"range"`
	Hash          string           `json:"hash,omitempty"`
	LoadedSkills  []loadedSkillRef `json:"loadedSkills,omitempty"`
}

type loadedSkillRef struct {
	Name         string    `json:"name"`
	Source       string    `json:"source"`
	Reason       string    `json:"reason,omitempty"`
	Range        readRange `json:"range"`
	Hash         string    `json:"hash,omitempty"`
	RiskCeiling  string    `json:"riskCeiling,omitempty"`
	AllowedTools []string  `json:"allowedTools,omitempty"`
	DeniedTools  []string  `json:"deniedTools,omitempty"`
}

// NewSkillSearchTool creates the compact skill discovery tool.
func NewSkillSearchTool(registry *skills.Registry) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "skill_search",
			Description: "Search available skills by compact metadata before reading a skill body",
			Origin:      tooling.ToolOriginMeta,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:      "search",
				ResourceTypes:       []string{"skill"},
				OperationKinds:      []string{"search"},
				HiddenFromDiscovery: true,
			},
		},
		Visibility:       tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:  searchInputSchema,
		OutputSchemaData: json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:     func(json.RawMessage) bool { return true },
		DestructiveFunc:  func(json.RawMessage) bool { return false },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			_ = ctx
			return executeSearch(registry, input)
		},
	}
}

// NewSkillReadTool creates the bounded skill body reader.
func NewSkillReadTool(registry *skills.Registry) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "skill_read",
			Description: "Read one selected skill body with a bounded range and a reason",
			Origin:      tooling.ToolOriginMeta,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:      "read",
				ResourceTypes:       []string{"skill"},
				OperationKinds:      []string{"read"},
				HiddenFromDiscovery: true,
			},
		},
		Visibility:       tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:  readInputSchema,
		OutputSchemaData: json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:     func(json.RawMessage) bool { return true },
		DestructiveFunc:  func(json.RawMessage) bool { return false },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			_ = ctx
			return executeRead(registry, input)
		},
	}
}

func executeSearch(registry *skills.Registry, input json.RawMessage) (tooling.ToolResult, error) {
	if registry == nil {
		return tooling.ToolResult{}, fmt.Errorf("skill_search: registry is required")
	}
	var req searchInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return tooling.ToolResult{}, fmt.Errorf("skill_search: invalid input: %w", err)
		}
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return tooling.ToolResult{}, fmt.Errorf("skill_search: query is required")
	}
	limit := req.Limit
	if limit <= 0 || limit > 20 {
		limit = defaultSearchLimit
	}
	index := skills.BuildSkillIndex(registry.List(), skills.SkillIndexOptions{
		Query:       req.Query,
		ResourceURI: req.ResourceURI,
		MaxChars:    skills.MaxSkillIndexChars,
	})
	if len(index.Entries) > limit {
		index.Entries = index.Entries[:limit]
	}
	out := searchOutput{
		SchemaVersion:  schemaVersion,
		Mode:           "search",
		SkillIndexHash: index.Hash,
		Matches:        make([]searchMatch, 0, len(index.Entries)),
	}
	for _, entry := range index.Entries {
		out.Matches = append(out.Matches, searchMatch{
			Name:             entry.Name,
			Description:      entry.Description,
			WhenToUse:        entry.WhenToUse,
			ResourceTypes:    append([]string(nil), entry.ResourceTypes...),
			TaskIntents:      append([]string(nil), entry.TaskIntents...),
			Risk:             entry.Risk,
			RequiresRead:     true,
			RequiredForMatch: entry.RequiredForMatch,
		})
	}
	for _, entry := range index.Dropped {
		out.Dropped = append(out.Dropped, dropped{Name: entry.Name, Reason: entry.Reason})
	}
	return emitSearchOutput(out)
}

func executeRead(registry *skills.Registry, input json.RawMessage) (tooling.ToolResult, error) {
	if registry == nil {
		return tooling.ToolResult{}, fmt.Errorf("skill_read: registry is required")
	}
	var req readInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return tooling.ToolResult{}, fmt.Errorf("skill_read: invalid input: %w", err)
		}
	}
	req.Skill = strings.TrimSpace(req.Skill)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Format = strings.TrimSpace(req.Format)
	if req.Skill == "" {
		return tooling.ToolResult{}, fmt.Errorf("skill_read: skill is required")
	}
	if req.Reason == "" {
		return tooling.ToolResult{}, fmt.Errorf("skill_read: reason is required")
	}
	def, ok := registry.Get(req.Skill)
	if !ok {
		return tooling.ToolResult{}, fmt.Errorf("skill_read: skill %q not found", req.Skill)
	}
	if req.Format == "" {
		req.Format = defaultSkillReadMode
	}
	if req.Format != "text" && req.Format != "metadata" {
		return tooling.ToolResult{}, fmt.Errorf("skill_read: unsupported format %q", req.Format)
	}
	req.Range = normalizeReadRange(req.Range, len(def.Prompt))
	body := boundedString(def.Prompt, req.Range)
	hash := hashSkillBody(def.Name, def.Prompt)
	out := readOutput{
		SchemaVersion: schemaVersion,
		Skill:         def.Name,
		Description:   def.Description,
		WhenToUse:     def.Discovery.WhenToUse,
		Range:         req.Range,
		Hash:          hash,
	}
	if req.Format == "metadata" {
		out.MetadataOnly = true
	} else {
		out.Body = body
	}
	out.LoadedSkills = []loadedSkillRef{{
		Name:         def.Name,
		Source:       "skill_read",
		Reason:       req.Reason,
		Range:        req.Range,
		Hash:         hash,
		RiskCeiling:  def.Governance.Risk,
		AllowedTools: append([]string(nil), def.Governance.AllowedTools...),
		DeniedTools:  append([]string(nil), def.Governance.DeniedTools...),
	}}
	return emitReadOutput(out)
}

func normalizeReadRange(r readRange, bodyLen int) readRange {
	if r.Offset < 0 {
		r.Offset = 0
	}
	if r.Offset > bodyLen {
		r.Offset = bodyLen
	}
	if r.Limit <= 0 || r.Limit > maxSkillReadChars {
		r.Limit = maxSkillReadChars
	}
	if r.Offset+r.Limit > bodyLen {
		r.Limit = bodyLen - r.Offset
	}
	if r.Limit < 0 {
		r.Limit = 0
	}
	return r
}

func boundedString(value string, r readRange) string {
	if r.Offset >= len(value) || r.Limit <= 0 {
		return ""
	}
	end := r.Offset + r.Limit
	if end > len(value) {
		end = len(value)
	}
	return value[r.Offset:end]
}

func emitSearchOutput(out searchOutput) (tooling.ToolResult, error) {
	content, err := json.Marshal(out)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "skill_search",
			Title: "Skill search",
			Data:  content,
		},
	}, nil
}

func emitReadOutput(out readOutput) (tooling.ToolResult, error) {
	content, err := json.Marshal(out)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(content),
		Display: &tooling.ToolDisplayPayload{
			Type:  "skill_read",
			Title: "Skill read",
			Data:  content,
		},
	}, nil
}

func hashSkillBody(name, body string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(name) + "\n" + body))
	return "sha256:" + hex.EncodeToString(sum[:])
}
