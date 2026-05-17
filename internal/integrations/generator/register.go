package generator

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

type Options struct {
	Mode string
}

func RegisterBuiltins(mcpRegistry *mcp.Registry, opts ...Options) error {
	if mcpRegistry == nil {
		return fmt.Errorf("generator: mcp registry is required")
	}

	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{
		ID:        "generator",
		Name:      "generator",
		Transport: "local",
		Command:   []string{"generator"},
		Source:    "builtin",
	}); err != nil {
		return err
	}

	if !devMode(opts...) {
		return nil
	}
	return mcpRegistry.OnServerConnected("generator", generatorTools())
}

func devMode(opts ...Options) bool {
	if len(opts) == 0 {
		return false
	}
	return opts[0].Mode == "dev"
}

func generatorTools() []tooling.Tool {
	planAndExecute := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"plan", "execute"},
	}
	executeOnly := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"execute"},
	}

	return []tooling.Tool{
		newGeneratorTool("generator.generate", "Generate a Skill/Card/Bundle draft from a specification or template", generateSchema, planAndExecute, false, "generate"),
		newGeneratorTool("generator.lint", "Lint and validate a generated draft for correctness and best practices", lintSchema, planAndExecute, true, "lint"),
		newGeneratorTool("generator.preview", "Preview a generated draft before publishing, showing rendered output", previewSchema, planAndExecute, true, "preview"),
		newGeneratorTool("generator.publish_draft", "Publish a validated draft to the system as a new Skill/Card/Bundle", publishDraftSchema, executeOnly, false, "publish_draft"),
	}
}

func newGeneratorTool(name, description string, schema json.RawMessage, visibility tooling.Visibility, readOnly bool, step string) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
		},
		Visibility:      visibility,
		InputSchemaData: schema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return readOnly
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Content: `{"status":"ok","step":"` + step + `","message":"generator step executed"}`,
				Display: &tooling.ToolDisplayPayload{Type: "generator", Title: name},
			}, nil
		},
	}
}

var generateSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"type": {"type": "string", "enum": ["skill", "card", "bundle"], "description": "Type of artifact to generate"},
		"name": {"type": "string", "description": "Name for the generated artifact"},
		"spec": {"type": "object", "description": "Specification or template parameters"},
		"baseTemplate": {"type": "string", "description": "Optional base template to extend"}
	},
	"required": ["type", "name"]
}`)

var lintSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"draftId": {"type": "string", "description": "Draft ID to lint"},
		"rules": {"type": "array", "items": {"type": "string"}, "description": "Specific lint rules to apply"}
	},
	"required": ["draftId"]
}`)

var previewSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"draftId": {"type": "string", "description": "Draft ID to preview"},
		"format": {"type": "string", "enum": ["html", "json", "markdown"], "description": "Preview output format"}
	},
	"required": ["draftId"]
}`)

var publishDraftSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"draftId": {"type": "string", "description": "Draft ID to publish"},
		"version": {"type": "string", "description": "Version tag for the published artifact"},
		"description": {"type": "string", "description": "Publish description/changelog"}
	},
	"required": ["draftId"]
}`)
