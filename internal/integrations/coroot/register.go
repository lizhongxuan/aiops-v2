package coroot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
)

func RegisterBuiltins(mcpRegistry *mcp.Registry, endpoint string) error {
	cfg := ClientConfigFromEnv(endpoint)
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}
	return RegisterBuiltinsWithClientProvider(mcpRegistry, ClientProviderFunc(func(context.Context) (*Client, error) {
		return client, nil
	}), client.BaseURL())
}

func RegisterBuiltinsWithClientProvider(mcpRegistry *mcp.Registry, provider ClientProvider, displayEndpoint string) error {
	if mcpRegistry == nil {
		return fmt.Errorf("coroot: mcp registry is required")
	}
	if provider == nil {
		return fmt.Errorf("coroot: client provider is required")
	}
	command := strings.TrimSpace(displayEndpoint)
	if command == "" {
		command = "configured-from-coroot-page"
	}
	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{
		ID:        "coroot",
		Name:      "coroot",
		Transport: "http",
		Command:   []string{command},
		Source:    "builtin",
	}); err != nil {
		return err
	}

	return mcpRegistry.OnServerConnected("coroot", corootToolsWithClientProvider(provider))
}

var listServicesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"namespace": {"type": "string", "description": "Filter by namespace"},
		"status": {"type": "string", "enum": ["healthy", "warning", "critical"], "description": "Filter by health status"}
	}
}`)

var serviceMetricsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name"},
		"timeRange": {"type": "string", "description": "Time range (e.g. 1h, 24h, 7d)"},
		"metrics": {"type": "array", "items": {"type": "string"}, "description": "Optional normalized metric summary names to retrieve; native Coroot chart/chart_group widgets are returned for all reports"}
	},
	"required": ["service"]
}`)

var rcaReportSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name"},
		"incidentId": {"type": "string", "description": "Incident ID for targeted RCA"},
		"timeRange": {"type": "string", "description": "Time range for analysis"}
	},
	"required": ["service"]
}`)

var serviceTopologySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Center service for topology view"},
		"depth": {"type": "integer", "description": "Depth of dependency traversal", "default": 2}
	},
	"required": ["service"]
}`)

var alertRulesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Filter by service name"},
		"severity": {"type": "string", "enum": ["info", "warning", "critical"], "description": "Filter by severity"}
	}
}`)

var incidentTimelineSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"incidentId": {"type": "string", "description": "Incident ID"},
		"service": {"type": "string", "description": "Service name"}
	},
	"required": ["incidentId"]
}`)

var sloStatusSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Filter by service name"},
		"sloName": {"type": "string", "description": "Filter by SLO name"}
	}
}`)
