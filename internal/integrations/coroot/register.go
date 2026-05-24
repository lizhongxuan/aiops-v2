package coroot

import (
	"encoding/json"
)

var emptyCorootSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

var projectOnlySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."}
	}
}`)

var healthCheckSchema = emptyCorootSchema

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
	}
}`)

var collectRCAContextSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
			"service": {"type": "string", "description": "Service name or Coroot application id to analyze. Provide service or incidentId; if both are omitted the tool returns a validation error."},
			"incidentId": {"type": "string", "description": "Optional incident id when the RCA starts from a Coroot incident. Provide service or incidentId; if both are omitted the tool returns a validation error."},
			"timeRange": {"type": "string", "description": "Analysis window such as 30m, 1h, 24h, or 7d. Defaults to 1h when no incident/from/to is provided."},
			"from": {"type": "string", "description": "Optional analysis window start as RFC3339 or Coroot-compatible relative/absolute time."},
			"to": {"type": "string", "description": "Optional analysis window end as RFC3339 or Coroot-compatible relative/absolute time."},
			"fromTimestamp": {"type": "integer", "description": "Optional analysis window start timestamp in seconds or milliseconds."},
			"toTimestamp": {"type": "integer", "description": "Optional analysis window end timestamp in seconds or milliseconds."},
			"depth": {"type": "integer", "minimum": 1, "maximum": 4, "description": "Dependency traversal depth. Defaults to 3 for RCA and is bounded by 4."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum recent incidents to summarize. Defaults to 20."},
			"includeRca": {"type": "boolean", "description": "When true, include Coroot RCA as optional reference evidence. Defaults to false because aiops performs its own RCA."}
		}
	}`)

var rcaReportSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name. Provide service or incidentId; if both are omitted the tool returns a validation error."},
		"incidentId": {"type": "string", "description": "Incident ID for targeted RCA. Provide service or incidentId; if both are omitted the tool returns a validation error."},
		"timeRange": {"type": "string", "description": "Time range for analysis"}
	}
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

var overviewSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"query": {"type": "string", "description": "Optional Coroot overview search/filter query"},
		"service": {"type": "string", "description": "Optional service/application filter used by deployment summaries"}
	}
}`)

var applicationLogsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name or Coroot application id"},
		"appId": {"type": "string", "description": "Explicit Coroot application id. If omitted, service is resolved through Coroot applications overview."},
		"fromTimestamp": {"type": "integer", "description": "Optional start timestamp in seconds or milliseconds"},
		"toTimestamp": {"type": "integer", "description": "Optional end timestamp in seconds or milliseconds"},
		"query": {"type": "string", "description": "Optional log search query"},
		"severity": {"type": "string", "description": "Optional severity/level filter"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum summarized log entries. Defaults to 10."}
	}
}`)

var applicationTracesSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name or Coroot application id"},
		"appId": {"type": "string", "description": "Explicit Coroot application id. If omitted, service is resolved through Coroot applications overview."},
		"fromTimestamp": {"type": "integer", "description": "Optional start timestamp in seconds or milliseconds"},
		"toTimestamp": {"type": "integer", "description": "Optional end timestamp in seconds or milliseconds"},
		"traceId": {"type": "string", "description": "Optional trace id to retrieve a targeted trace"},
		"query": {"type": "string", "description": "Optional trace search query"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum slow spans summarized. Defaults to 10."}
	}
}`)

var applicationProfilingSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name or Coroot application id"},
		"appId": {"type": "string", "description": "Explicit Coroot application id. If omitted, service is resolved through Coroot applications overview."},
		"fromTimestamp": {"type": "integer", "description": "Optional start timestamp in seconds or milliseconds"},
		"toTimestamp": {"type": "integer", "description": "Optional end timestamp in seconds or milliseconds"},
		"query": {"type": "string", "description": "Optional profiling search query"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum profile/instance names summarized. Defaults to 10."}
	}
}`)

var getNodeSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"nodeId": {"type": "string", "description": "Coroot node id"}
	},
	"required": ["nodeId"]
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

var incidentsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Optional application/service name or Coroot application id to filter incidents"},
		"query": {"type": "string", "description": "Optional free-text filter over incident id, application id/name, description, RCA status, and root cause"},
		"status": {"type": "string", "enum": ["open", "critical", "warning", "resolved"], "description": "Optional incident state/severity filter. For recent/latest incident lists, omit this field unless the user explicitly asks for open, resolved, critical, or warning incidents."},
		"severity": {"type": "string", "enum": ["critical", "warning"], "description": "Optional Coroot severity filter. For recent/latest incident lists, omit this field unless the user explicitly asks for a severity."},
		"applicationCategory": {"type": "string", "enum": ["application", "monitoring"], "description": "Optional Coroot application category filter. For recent/latest incident lists, omit this field unless the user explicitly asks for application or monitoring incidents."},
		"showResolved": {"type": "boolean", "description": "When false, resolved incidents are filtered out. Omit to include the latest incidents returned by Coroot."},
		"limit": {"type": "integer", "minimum": 1, "maximum": 200, "description": "Maximum incidents to request from Coroot. Defaults to 50."}
	}
}`)

var sloStatusSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Filter by service name"},
		"sloName": {"type": "string", "description": "Filter by SLO name"}
	}
}`)

var dashboardSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"dashboardId": {"type": "string", "description": "Coroot dashboard id"}
	},
	"required": ["dashboardId"]
}`)

var panelDataSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"dashboardId": {"type": "string", "description": "Coroot dashboard id"},
		"panelId": {"type": "string", "description": "Panel id inside the dashboard"},
		"from": {"type": "string", "description": "Optional start time, for example ISO-8601 or relative time"},
		"to": {"type": "string", "description": "Optional end time, for example ISO-8601 or now"},
		"limit": {"type": "integer", "minimum": 1, "maximum": 50, "description": "Maximum chart summaries returned. Defaults to 10."}
	},
	"required": ["dashboardId", "panelId"]
}`)

var integrationSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"integrationType": {"type": "string", "description": "Integration type, for example prometheus, slack, pagerduty, webhook, clickhouse, aws"}
	},
	"required": ["integrationType"]
}`)

var inspectionConfigSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"project": {"type": "string", "description": "Optional Coroot project id. Omit this field to use configured Coroot project; do not send default as a placeholder."},
		"service": {"type": "string", "description": "Service name or Coroot application id"},
		"appId": {"type": "string", "description": "Explicit Coroot application id. If omitted, service is resolved through Coroot applications overview."},
		"inspectionType": {"type": "string", "description": "Inspection type, for example cpu, memory, slo_availability, or slo_latency"}
	},
	"required": ["inspectionType"]
}`)
