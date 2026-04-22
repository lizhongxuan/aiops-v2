package coroot

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(mcpRegistry *mcp.Registry, endpoint string) error {
	if mcpRegistry == nil {
		return fmt.Errorf("coroot: mcp registry is required")
	}

	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{
		ID:        "coroot",
		Name:      "coroot",
		Transport: "http",
		Command:   []string{endpoint},
		Source:    "builtin",
	}); err != nil {
		return err
	}

	return mcpRegistry.OnServerConnected("coroot", corootTools())
}

func corootTools() []tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"inspect", "plan", "execute"},
	}

	return []tooling.Tool{
		newCorootTool("coroot.list_services", "List all monitored services from Coroot with their health status", listServicesSchema, visibility),
		newCorootTool("coroot.service_metrics", "Get detailed metrics for a specific service (CPU, memory, latency, error rate)", serviceMetricsSchema, visibility),
		newCorootTool("coroot.rca_report", "Generate root cause analysis report for a service incident", rcaReportSchema, visibility),
		newCorootTool("coroot.service_topology", "Get service dependency topology map showing upstream and downstream services", serviceTopologySchema, visibility),
		newCorootTool("coroot.alert_rules", "List and manage alert rules configured in Coroot", alertRulesSchema, visibility),
		newCorootTool("coroot.incident_timeline", "Get chronological timeline of events for a specific incident", incidentTimelineSchema, visibility),
		newCorootTool("coroot.slo_status", "Get current SLO compliance status for services", sloStatusSchema, visibility),
	}
}

func newCorootTool(name, description string, schema json.RawMessage, visibility tooling.Visibility) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
		},
		Visibility:      visibility,
		InputSchemaData: schema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{
				Content: `{"status":"ok","message":"coroot tool executed"}`,
				Display: &tooling.ToolDisplayPayload{Type: "coroot", Title: name},
			}, nil
		},
	}
}

var listServicesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"namespace": {"type": "string", "description": "Filter by namespace"},
		"status": {"type": "string", "enum": ["healthy", "warning", "critical"], "description": "Filter by health status"}
	}
}`)

var serviceMetricsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"service": {"type": "string", "description": "Service name"},
		"timeRange": {"type": "string", "description": "Time range (e.g. 1h, 24h, 7d)"},
		"metrics": {"type": "array", "items": {"type": "string"}, "description": "Metric names to retrieve"}
	},
	"required": ["service"]
}`)

var rcaReportSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"service": {"type": "string", "description": "Service name"},
		"incidentId": {"type": "string", "description": "Incident ID for targeted RCA"},
		"timeRange": {"type": "string", "description": "Time range for analysis"}
	},
	"required": ["service"]
}`)

var serviceTopologySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"service": {"type": "string", "description": "Center service for topology view"},
		"depth": {"type": "integer", "description": "Depth of dependency traversal", "default": 2}
	},
	"required": ["service"]
}`)

var alertRulesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"service": {"type": "string", "description": "Filter by service name"},
		"severity": {"type": "string", "enum": ["info", "warning", "critical"], "description": "Filter by severity"}
	}
}`)

var incidentTimelineSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"incidentId": {"type": "string", "description": "Incident ID"},
		"service": {"type": "string", "description": "Service name"}
	},
	"required": ["incidentId"]
}`)

var sloStatusSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"service": {"type": "string", "description": "Filter by service name"},
		"sloName": {"type": "string", "description": "Filter by SLO name"}
	}
}`)
