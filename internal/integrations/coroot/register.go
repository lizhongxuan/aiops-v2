package coroot

import (
	"encoding/json"
)

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
