package generator

import (
	"context"
	"encoding/json"

	"aiops-v2/internal/capability"
)

// ---------------------------------------------------------------------------
// GeneratorExtension registers auto-generation capabilities supporting a
// 4-step flow: generate → lint → preview → publish-draft (Req 10.4).
// This enables automated creation of Skill/Card/Bundle drafts.
// ---------------------------------------------------------------------------

// GeneratorExtension implements capability.Extension for auto-generation.
type GeneratorExtension struct{}

// NewGeneratorExtension creates a new GeneratorExtension.
func NewGeneratorExtension() *GeneratorExtension {
	return &GeneratorExtension{}
}

// Name returns the extension name.
func (e *GeneratorExtension) Name() string { return "generator" }

// generatorToolIDs lists the tool IDs registered by this extension.
var generatorToolIDs = []string{
	"generator/generate",
	"generator/lint",
	"generator/preview",
	"generator/publish_draft",
}

// Register registers the 4-step generation flow tools into the Capability Registry.
func (e *GeneratorExtension) Register(registry *capability.Registry) error {
	entries := []capability.Entry{
		{
			ID:          "generator/generate",
			Name:        "generator.generate",
			Kind:        capability.KindMCPTool,
			Description: "Generate a Skill/Card/Bundle draft from a specification or template",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &generatorTool{
				name:        "generator.generate",
				description: "Generate a Skill/Card/Bundle draft from a specification or template",
				readOnly:    false,
				step:        StepGenerate,
				schema:      generateSchema,
			},
		},
		{
			ID:          "generator/lint",
			Name:        "generator.lint",
			Kind:        capability.KindMCPTool,
			Description: "Lint and validate a generated draft for correctness and best practices",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &generatorTool{
				name:        "generator.lint",
				description: "Lint and validate a generated draft for correctness and best practices",
				readOnly:    true,
				step:        StepLint,
				schema:      lintSchema,
			},
		},
		{
			ID:          "generator/preview",
			Name:        "generator.preview",
			Kind:        capability.KindMCPTool,
			Description: "Preview a generated draft before publishing, showing rendered output",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &generatorTool{
				name:        "generator.preview",
				description: "Preview a generated draft before publishing, showing rendered output",
				readOnly:    true,
				step:        StepPreview,
				schema:      previewSchema,
			},
		},
		{
			ID:          "generator/publish_draft",
			Name:        "generator.publish_draft",
			Kind:        capability.KindMCPTool,
			Description: "Publish a validated draft to the system as a new Skill/Card/Bundle",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"execute"},
			},
			Tool: &generatorTool{
				name:        "generator.publish_draft",
				description: "Publish a validated draft to the system as a new Skill/Card/Bundle",
				readOnly:    false,
				step:        StepPublishDraft,
				schema:      publishDraftSchema,
			},
		},
	}

	return registry.RegisterBatch(entries)
}

// Unregister removes all Generator tools from the registry.
func (e *GeneratorExtension) Unregister(registry *capability.Registry) error {
	for _, id := range generatorToolIDs {
		registry.Unregister(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// GeneratorStep represents the 4-step generation flow.
// ---------------------------------------------------------------------------

// GeneratorStep identifies which step in the 4-step flow a tool represents.
type GeneratorStep string

const (
	StepGenerate     GeneratorStep = "generate"
	StepLint         GeneratorStep = "lint"
	StepPreview      GeneratorStep = "preview"
	StepPublishDraft GeneratorStep = "publish_draft"
)

// ---------------------------------------------------------------------------
// generatorTool implements capability.ToolRuntime for Generator tools.
// ---------------------------------------------------------------------------

type generatorTool struct {
	name        string
	description string
	readOnly    bool
	step        GeneratorStep
	schema      json.RawMessage
}

func (t *generatorTool) Description() string                        { return t.description }
func (t *generatorTool) CheckPermissions(_ context.Context) error   { return nil }
func (t *generatorTool) IsReadOnly() bool                           { return t.readOnly }
func (t *generatorTool) IsDestructive() bool                        { return false }
func (t *generatorTool) IsConcurrencySafe() bool                    { return false }
func (t *generatorTool) InputSchema() json.RawMessage               { return t.schema }
func (t *generatorTool) Display() capability.ToolDisplayPayload {
	return capability.ToolDisplayPayload{Type: "generator", Title: t.name}
}

func (t *generatorTool) Execute(_ context.Context, args json.RawMessage) (capability.ToolResult, error) {
	// Placeholder: actual implementation would perform generation steps.
	return capability.ToolResult{
		Content: `{"status":"ok","step":"` + string(t.step) + `","message":"generator step executed"}`,
	}, nil
}

// ---------------------------------------------------------------------------
// JSON Schemas for each tool's input parameters.
// ---------------------------------------------------------------------------

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

// Compile-time check that GeneratorExtension implements Extension.
var _ capability.Extension = (*GeneratorExtension)(nil)
