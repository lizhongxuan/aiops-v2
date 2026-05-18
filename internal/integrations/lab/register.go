package lab

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
		return fmt.Errorf("lab: mcp registry is required")
	}

	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{
		ID:        "lab",
		Name:      "lab",
		Transport: "local",
		Command:   []string{"lab"},
		Source:    "builtin",
	}); err != nil {
		return err
	}

	if !devMode(opts...) {
		return nil
	}
	return mcpRegistry.OnServerConnected("lab", labTools())
}

func devMode(opts ...Options) bool {
	if len(opts) == 0 {
		return false
	}
	return opts[0].Mode == "dev"
}

func labTools() []tooling.Tool {
	planAndExecute := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"plan", "execute"},
	}
	executeOnly := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"execute"},
	}

	return []tooling.Tool{
		newLabTool("lab.create_environment", "Create a new sandbox lab environment with specified configuration", createEnvSchema, planAndExecute, false),
		newLabTool("lab.start_environment", "Start a previously created lab environment", startEnvSchema, planAndExecute, false),
		newLabTool("lab.inject_fault", "Inject a fault condition into a running lab environment for chaos testing", injectFaultSchema, executeOnly, true),
		newLabTool("lab.reset_environment", "Reset a lab environment to its initial clean state", resetEnvSchema, planAndExecute, true),
	}
}

func newLabTool(name, description string, schema json.RawMessage, visibility tooling.Visibility, destructive bool) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
		},
		Visibility:      visibility,
		InputSchemaData: schema,
		DestructiveFunc: func(json.RawMessage) bool {
			return destructive
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Content: `{"status":"ok","message":"lab tool executed"}`,
				Display: &tooling.ToolDisplayPayload{Type: "lab", Title: name},
			}, nil
		},
	}
}

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
