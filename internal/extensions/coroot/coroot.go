package coroot

import (
	"context"
	"encoding/json"

	"aiops-v2/internal/capability"
)

// ---------------------------------------------------------------------------
// CorootExtension registers 7 MCP dynamic tools for Coroot observability
// integration. These tools provide service discovery, metrics querying,
// root cause analysis, and alerting capabilities (Req 10.2).
// ---------------------------------------------------------------------------

// CorootExtension implements capability.Extension for Coroot integration.
type CorootExtension struct {
	// endpoint is the Coroot API base URL.
	endpoint string
}

// NewCorootExtension creates a new CorootExtension with the given endpoint.
func NewCorootExtension(endpoint string) *CorootExtension {
	return &CorootExtension{endpoint: endpoint}
}

// Name returns the extension name.
func (e *CorootExtension) Name() string { return "coroot" }

// toolIDs lists the 7 MCP tool IDs registered by this extension.
var toolIDs = []string{
	"coroot/list_services",
	"coroot/service_metrics",
	"coroot/rca_report",
	"coroot/service_topology",
	"coroot/alert_rules",
	"coroot/incident_timeline",
	"coroot/slo_status",
}

// Register registers 7 MCP dynamic tools into the Capability Registry.
func (e *CorootExtension) Register(registry *capability.Registry) error {
	entries := []capability.Entry{
		{
			ID:          "coroot/list_services",
			Name:        "coroot.list_services",
			Kind:        capability.KindMCPTool,
			Description: "List all monitored services from Coroot with their health status",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.list_services",
				description: "List all monitored services from Coroot with their health status",
				readOnly:    true,
				schema:      listServicesSchema,
			},
		},
		{
			ID:          "coroot/service_metrics",
			Name:        "coroot.service_metrics",
			Kind:        capability.KindMCPTool,
			Description: "Get detailed metrics for a specific service (CPU, memory, latency, error rate)",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.service_metrics",
				description: "Get detailed metrics for a specific service (CPU, memory, latency, error rate)",
				readOnly:    true,
				schema:      serviceMetricsSchema,
			},
		},
		{
			ID:          "coroot/rca_report",
			Name:        "coroot.rca_report",
			Kind:        capability.KindMCPTool,
			Description: "Generate root cause analysis report for a service incident",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.rca_report",
				description: "Generate root cause analysis report for a service incident",
				readOnly:    true,
				schema:      rcaReportSchema,
			},
		},
		{
			ID:          "coroot/service_topology",
			Name:        "coroot.service_topology",
			Kind:        capability.KindMCPTool,
			Description: "Get service dependency topology map showing upstream and downstream services",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.service_topology",
				description: "Get service dependency topology map showing upstream and downstream services",
				readOnly:    true,
				schema:      serviceTopologySchema,
			},
		},
		{
			ID:          "coroot/alert_rules",
			Name:        "coroot.alert_rules",
			Kind:        capability.KindMCPTool,
			Description: "List and manage alert rules configured in Coroot",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.alert_rules",
				description: "List and manage alert rules configured in Coroot",
				readOnly:    true,
				schema:      alertRulesSchema,
			},
		},
		{
			ID:          "coroot/incident_timeline",
			Name:        "coroot.incident_timeline",
			Kind:        capability.KindMCPTool,
			Description: "Get chronological timeline of events for a specific incident",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.incident_timeline",
				description: "Get chronological timeline of events for a specific incident",
				readOnly:    true,
				schema:      incidentTimelineSchema,
			},
		},
		{
			ID:          "coroot/slo_status",
			Name:        "coroot.slo_status",
			Kind:        capability.KindMCPTool,
			Description: "Get current SLO compliance status for services",
			Visibility: capability.Visibility{
				SessionTypes: []string{"host", "workspace"},
				Modes:        []string{"inspect", "plan", "execute"},
			},
			Tool: &corootTool{
				name:        "coroot.slo_status",
				description: "Get current SLO compliance status for services",
				readOnly:    true,
				schema:      sloStatusSchema,
			},
		},
	}

	return registry.RegisterBatch(entries)
}

// Unregister removes all Coroot tools from the registry.
func (e *CorootExtension) Unregister(registry *capability.Registry) error {
	for _, id := range toolIDs {
		registry.Unregister(id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// corootTool implements capability.ToolRuntime for Coroot MCP tools.
// ---------------------------------------------------------------------------

type corootTool struct {
	name        string
	description string
	readOnly    bool
	schema      json.RawMessage
}

func (t *corootTool) Description() string                        { return t.description }
func (t *corootTool) CheckPermissions(_ context.Context) error   { return nil }
func (t *corootTool) IsReadOnly() bool                           { return t.readOnly }
func (t *corootTool) IsDestructive() bool                        { return false }
func (t *corootTool) IsConcurrencySafe() bool                    { return true }
func (t *corootTool) InputSchema() json.RawMessage               { return t.schema }
func (t *corootTool) Display() capability.ToolDisplayPayload {
	return capability.ToolDisplayPayload{Type: "coroot", Title: t.name}
}

func (t *corootTool) Execute(_ context.Context, args json.RawMessage) (capability.ToolResult, error) {
	// Placeholder: actual implementation would call Coroot API.
	return capability.ToolResult{
		Content: `{"status":"ok","message":"coroot tool executed"}`,
	}, nil
}

// ---------------------------------------------------------------------------
// JSON Schemas for each tool's input parameters.
// ---------------------------------------------------------------------------

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

// Compile-time check that CorootExtension implements Extension.
var _ capability.Extension = (*CorootExtension)(nil)
