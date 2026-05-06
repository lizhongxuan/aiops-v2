package coroot

import (
	"encoding/json"
	"fmt"

	"aiops-v2/internal/mcp"
)

func RegisterBuiltins(mcpRegistry *mcp.Registry, endpoint string) error {
	if mcpRegistry == nil {
		return fmt.Errorf("coroot: mcp registry is required")
	}
	cfg := ClientConfigFromEnv(endpoint)
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}

	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{
		ID:        "coroot",
		Name:      "coroot",
		Transport: "http",
		Command:   []string{client.BaseURL()},
		Source:    "builtin",
	}); err != nil {
		return err
	}

	return mcpRegistry.OnServerConnected("coroot", corootToolsWithClient(client))
}

var listServicesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"namespace": {"type": "string", "description": "Filter by namespace"},
		"status": {"type": "string", "enum": ["healthy", "warning", "critical"], "description": "Filter by health status"}
	}
}`)

var serviceMetricsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"service": {"type": "string", "description": "Service name"},
		"timeRange": {"type": "string", "description": "Time range (e.g. 1h, 24h, 7d)"},
		"metrics": {"type": "array", "items": {"type": "string"}, "description": "Metric names to retrieve"}
	},
	"required": ["service"]
}`)

var rcaReportSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"service": {"type": "string", "description": "Service name"},
		"incidentId": {"type": "string", "description": "Incident ID for targeted RCA"},
		"timeRange": {"type": "string", "description": "Time range for analysis"}
	},
	"required": ["service"]
}`)

var serviceTopologySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"service": {"type": "string", "description": "Center service for topology view"},
		"depth": {"type": "integer", "description": "Depth of dependency traversal", "default": 2}
	},
	"required": ["service"]
}`)

var alertRulesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"service": {"type": "string", "description": "Filter by service name"},
		"severity": {"type": "string", "enum": ["info", "warning", "critical"], "description": "Filter by severity"}
	}
}`)

var incidentTimelineSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"incidentId": {"type": "string", "description": "Incident ID"},
		"service": {"type": "string", "description": "Service name"}
	},
	"required": ["incidentId"]
}`)

var sloStatusSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Coroot project id, defaults to AIOPS_COROOT_PROJECT or default"},
		"service": {"type": "string", "description": "Filter by service name"},
		"sloName": {"type": "string", "description": "Filter by SLO name"}
	}
}`)
