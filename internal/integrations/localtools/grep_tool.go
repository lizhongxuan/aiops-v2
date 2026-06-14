package localtools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"aiops-v2/internal/tooling"
)

type grepInput struct {
	Pattern       string `json:"pattern"`
	Path          string `json:"path,omitempty"`
	MaxMatches    int    `json:"maxMatches,omitempty"`
	CaseSensitive bool   `json:"caseSensitive,omitempty"`
	Literal       bool   `json:"literal,omitempty"`
}

type grepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Preview string `json:"preview"`
}

func NewGrepTool(opts Options) tooling.Tool {
	opts = opts.normalize()
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "grep",
			Aliases:     []string{"search_text", "search_files"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Search text files under the current workspace or bounded path. Use for local logs, configs, and text artifacts; use observability MCP for centralized logs.",
			Layer:       tooling.ToolLayerCore,
			Pack:        "filesystem_search",
			AlwaysLoad:  true,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:    "search",
				ResourceTypes:     []string{"file", "log", "configuration", "text"},
				OperationKinds:    []string{"read", "search"},
				ToolPackIDs:       []string{"filesystem_search"},
				PermissionScope:   "read",
				PromptBudgetClass: "compact",
				SchemaBudgetClass: "on_demand",
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: opts.MaxOutputBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData: json.RawMessage(`{
			"type":"object",
			"properties":{
				"pattern":{"type":"string"},
				"path":{"type":"string"},
				"maxMatches":{"type":"integer","minimum":1,"maximum":200},
				"caseSensitive":{"type":"boolean"},
				"literal":{"type":"boolean"}
			},
			"required":["pattern"]
		}`),
		OutputSchemaData:    json.RawMessage(`{"type":"object"}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc:   validateGrepInput(opts),
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: executeGrep(opts),
	}
}

func validateGrepInput(opts Options) func(context.Context, json.RawMessage) error {
	return func(_ context.Context, input json.RawMessage) error {
		req, err := parseGrepInput(input)
		if err != nil {
			return err
		}
		if _, err := resolveGrepRoot(opts, req.Path); err != nil {
			return err
		}
		return nil
	}
}

func executeGrep(opts Options) func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
		req, err := parseGrepInput(input)
		if err != nil {
			return tooling.ToolResult{}, err
		}
		root, err := resolveGrepRoot(opts, req.Path)
		if err != nil {
			return tooling.ToolResult{}, err
		}
		matcher, err := buildGrepMatcher(req)
		if err != nil {
			return tooling.ToolResult{}, err
		}
		limit := req.MaxMatches
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		var matches []grepMatch
		truncated := false
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				name := entry.Name()
				if name == ".git" || name == "node_modules" || name == ".data" {
					return filepath.SkipDir
				}
				return nil
			}
			if len(matches) >= limit {
				truncated = true
				return filepath.SkipAll
			}
			fileMatches, err := grepFile(root, path, matcher, limit-len(matches))
			if err != nil {
				return nil
			}
			matches = append(matches, fileMatches...)
			if len(matches) >= limit {
				truncated = true
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil {
			return tooling.ToolResult{}, err
		}
		data, _ := json.Marshal(map[string]any{
			"schemaVersion": "aiops.grep/v1",
			"tool":          "grep",
			"pattern":       req.Pattern,
			"root":          root,
			"matches":       matches,
			"truncated":     truncated,
		})
		return tooling.ToolResult{Content: string(data)}, nil
	}
}

func parseGrepInput(input json.RawMessage) (grepInput, error) {
	var req grepInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return grepInput{}, err
		}
	}
	req.Pattern = strings.TrimSpace(req.Pattern)
	req.Path = strings.TrimSpace(req.Path)
	if req.Pattern == "" {
		return grepInput{}, fmt.Errorf("grep: pattern is required")
	}
	return req, nil
}

func resolveGrepRoot(opts Options, requested string) (string, error) {
	base := opts.normalize().WorkingDir
	if requested == "" {
		requested = "."
	}
	root := requested
	if !filepath.IsAbs(root) {
		root = filepath.Join(base, root)
	}
	root = filepath.Clean(root)
	rel, err := filepath.Rel(base, root)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("grep: path must stay under working directory")
	}
	return root, nil
}

func buildGrepMatcher(req grepInput) (func(string) bool, error) {
	pattern := req.Pattern
	if req.Literal {
		if req.CaseSensitive {
			return func(line string) bool { return strings.Contains(line, pattern) }, nil
		}
		needle := strings.ToLower(pattern)
		return func(line string) bool { return strings.Contains(strings.ToLower(line), needle) }, nil
	}
	if !req.CaseSensitive {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.MatchString, nil
}

func grepFile(root, path string, matcher func(string) bool, limit int) ([]grepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var matches []grepMatch
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if matcher(line) {
			rel, _ := filepath.Rel(root, path)
			matches = append(matches, grepMatch{File: rel, Line: lineNo, Preview: strings.TrimSpace(line)})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, scanner.Err()
}
