package lab

import (
	"context"
	"encoding/json"

	"aiops-v2/internal/capability"
)

// ---------------------------------------------------------------------------
// LabExtension registers sandbox environment management capabilities
// including environment creation, start, fault injection, and reset (Req 10.3).
// ---------------------------------------------------------------------------

// LabExtension implements capability.Extension for Lab sandbox management.
type LabExtension struct{}

// NewLabExtension creates a new LabExtension.
func NewLabExtension() *LabExtension {
	return &LabExtension{}
}

// Name returns the extension name.
func (e *LabExtension) Name() string { return "lab" }

// labToolIDs lists the tool IDs registered by this extension.
var labToolIDs = []string{
	"lab/create_environment",
	"lab/start_environment",
	"lab/inject_fault",
	"lab/reset_environment",
}

// Register registers sandbox management capabilities into the Capability Registry.
func (e *LabExtension) Register(registry *capability.Registry) error {
	entries := []capability.Entry{
		{
			ID:          "lab/create_environment",
			Name:        "lab.create_environment",
			Kind:        capability.KindMCPTool,
			Description: "Create a new sandbox lab environment with specified configuration",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &labTool{
				name:        "lab.create_environment",
				description: "Create a new sandbox lab environment with specified configuration",
				readOnly:    false,
				destructive: false,
				schema:      createEnvSchema,
			},
		},
		{
			ID:          "lab/start_environment",
			Name:        "lab.start_environment",
			Kind:        capability.KindMCPTool,
			Description: "Start a previously created lab environment",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &labTool{
				name:        "lab.start_environment",
				description: "Start a previously created lab environment",
				readOnly:    false,
				destructive: false,
				schema:      startEnvSchema,
			},
		},
		{
			ID:          "lab/inject_fault",
			Name:        "lab.inject_fault",
			Kind:        capability.KindMCPTool,
			Description: "Inject a fault condition into a running lab environment for chaos testing",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"execute"},
			},
			Tool: &labTool{
				name:        "lab.inject_fault",
				description: "Inject a fault condition into a running lab environment for chaos testing",
				readOnly:    false,
				destructive: true,
				schema:      injectFaultSchema,
			},
		},
		{
			ID:          "lab/reset_environment",
			Name:        "lab.reset_environment",
			Kind:        capability.KindMCPTool,
			Description: "Reset a lab environment to its initial clean state",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"plan", "execute"},
			},
			Tool: &labTool{
				name:        "lab.reset_environment",
				description: "Reset a lab environment to its initial clean state",
				readOnly:    false,
				destructive: true,
				schema:      resetEnvSchema,
			},
		},
	}

	return registry.RegisterBatch(entries)
}

// Unregister removes all Lab tools from the registry.
func (e *LabExtension) Unregister(registry *capability.Registry) error {
	for _, id := range labToolIDs {
		registry.Unregister(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// labTool implements capability.ToolRuntime for Lab sandbox tools.
// ---------------------------------------------------------------------------

type labTool struct {
	name        string
	description string
	readOnly    bool
	destructive bool
	schema      json.RawMessage
}

func (t *labTool) Description() string                        { return t.description }
func (t *labTool) CheckPermissions(_ context.Context) error   { return nil }
func (t *labTool) IsReadOnly() bool                           { return t.readOnly }
func (t *labTool) IsDestructive() bool                        { return t.destructive }
func (t *labTool) IsConcurrencySafe() bool                    { return false }
func (t *labTool) InputSchema() json.RawMessage               { return t.schema }
func (t *labTool) Display() capability.ToolDisplayPayload {
	return capability.ToolDisplayPayload{Type: "lab", Title: t.name}
}

func (t *labTool) Execute(_ context.Context, args json.RawMessage) (capability.ToolResult, error) {
	// Placeholder: actual implementation would manage sandbox environments.
	return capability.ToolResult{
		Content: `{"status":"ok","message":"lab tool executed"}`,
	}, nil
}

// ---------------------------------------------------------------------------
// JSON Schemas for each tool's input parameters.
// ---------------------------------------------------------------------------

var createEnvSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {"type": "string", "description": "Environment name"},
		"template": {"type": "string", "description": "Template to use (e.g. kubernetes, docker-compose)"},
		"config": {"type": "object", "description": "Environment-specific configuration"}
	},
	"required": ["name", "template"]
}`)

var startEnvSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"environmentId": {"type": "string", "description": "Environment ID to start"}
	},
	"required": ["environmentId"]
}`)

var injectFaultSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"environmentId": {"type": "string", "description": "Target environment ID"},
		"faultType": {"type": "string", "enum": ["network_delay", "cpu_stress", "memory_pressure", "disk_full", "process_kill", "network_partition"], "description": "Type of fault to inject"},
		"target": {"type": "string", "description": "Target service or container"},
		"duration": {"type": "string", "description": "Fault duration (e.g. 30s, 5m)"},
		"intensity": {"type": "number", "description": "Fault intensity (0.0-1.0)"}
	},
	"required": ["environmentId", "faultType", "target"]
}`)

var resetEnvSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"environmentId": {"type": "string", "description": "Environment ID to reset"}
	},
	"required": ["environmentId"]
}`)

// Compile-time check that LabExtension implements Extension.
var _ capability.Extension = (*LabExtension)(nil)
