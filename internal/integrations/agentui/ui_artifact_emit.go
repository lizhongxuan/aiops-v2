package agentui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/tooling"
)

const (
	artifactSchemaVersion = "aiops.agent_ui_artifact/v1"
	toolName              = "aiops.ui_artifact_emit"
	rcaReportType         = "rca_report"
	rcaReportSchema       = "aiops.rca_report/v1"
	maxInlineDataBytes    = 256 * 1024
)

type artifactEmitInput struct {
	Type            string           `json:"type"`
	Title           string           `json:"title,omitempty"`
	TitleZh         string           `json:"titleZh,omitempty"`
	Summary         string           `json:"summary,omitempty"`
	SummaryZh       string           `json:"summaryZh,omitempty"`
	Status          string           `json:"status,omitempty"`
	Severity        string           `json:"severity,omitempty"`
	Source          string           `json:"source,omitempty"`
	CaseID          string           `json:"caseId,omitempty"`
	EvidenceRef     string           `json:"evidenceRef,omitempty"`
	PromptTraceID   string           `json:"promptTraceId,omitempty"`
	PermissionScope string           `json:"permissionScope,omitempty"`
	RedactionStatus string           `json:"redactionStatus,omitempty"`
	InlineData      map[string]any   `json:"inlineData,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	Actions         []map[string]any `json:"actions,omitempty"`
}

func NewUIArtifactEmitTool() tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        toolName,
			Description: "Emit a controlled read-only Agent-to-UI artifact. P0 supports rca_report artifacts.",
			Domain:      "agentui",
			Origin:      tooling.ToolOriginBuiltin,
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:     inputSchema,
		OutputSchemaData:    outputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			var req artifactEmitInput
			if err := json.Unmarshal(input, &req); err != nil {
				return fmt.Errorf("agentui: invalid artifact input: %w", err)
			}
			return validateArtifact(req)
		},
		ExecuteFunc: executeEmit,
	}
}

func executeEmit(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
	var req artifactEmitInput
	if err := json.Unmarshal(input, &req); err != nil {
		return tooling.ToolResult{}, fmt.Errorf("agentui: invalid artifact input: %w", err)
	}
	if err := validateArtifact(req); err != nil {
		return tooling.ToolResult{}, err
	}
	payload := normalizeArtifact(req)
	data, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	if len(data) > maxInlineDataBytes {
		return tooling.ToolResult{}, fmt.Errorf("agentui: artifact payload exceeds %d bytes", maxInlineDataBytes)
	}
	return tooling.ToolResult{
		Content: string(data),
		Display: &tooling.ToolDisplayPayload{
			Type:  rcaReportType,
			Title: firstNonEmpty(req.Title, req.TitleZh, "Root cause analysis"),
			Data:  data,
		},
	}, nil
}

func validateArtifact(req artifactEmitInput) error {
	if strings.TrimSpace(req.Type) != rcaReportType {
		return fmt.Errorf("agentui: unsupported artifact type %q", req.Type)
	}
	if strings.TrimSpace(req.PermissionScope) != "" && strings.TrimSpace(req.PermissionScope) != "read" {
		return fmt.Errorf("agentui: permissionScope must be read")
	}
	schema, _ := req.InlineData["schemaVersion"].(string)
	if strings.TrimSpace(schema) != rcaReportSchema {
		return fmt.Errorf("agentui: inlineData.schemaVersion must be %s", rcaReportSchema)
	}
	if key, ok := containsForbiddenKey(map[string]any{"inlineData": req.InlineData, "metadata": req.Metadata, "actions": req.Actions}); ok {
		return fmt.Errorf("agentui: forbidden artifact key %q", key)
	}
	status := firstNonEmpty(req.Status, stringValue(req.InlineData["status"]), "ok")
	if (status == "ok" || status == "partial") && !hasRCAEvidence(req.InlineData) {
		return fmt.Errorf("agentui: rca_report status %q requires evidenceRefs or rawRefs", status)
	}
	return validateArtifactActions(req.Actions)
}

func normalizeArtifact(req artifactEmitInput) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"schemaVersion":   artifactSchemaVersion,
		"type":            rcaReportType,
		"title":           strings.TrimSpace(req.Title),
		"titleZh":         firstNonEmpty(req.TitleZh, "根因分析"),
		"summary":         strings.TrimSpace(req.Summary),
		"summaryZh":       strings.TrimSpace(req.SummaryZh),
		"status":          firstNonEmpty(req.Status, stringValue(req.InlineData["status"]), "ok"),
		"severity":        firstNonEmpty(req.Severity, "info"),
		"source":          firstNonEmpty(req.Source, "aiops"),
		"caseId":          strings.TrimSpace(req.CaseID),
		"evidenceRef":     strings.TrimSpace(req.EvidenceRef),
		"promptTraceId":   strings.TrimSpace(req.PromptTraceID),
		"permissionScope": "read",
		"redactionStatus": firstNonEmpty(req.RedactionStatus, "redacted"),
		"inlineData":      req.InlineData,
		"metadata":        req.Metadata,
		"actions":         req.Actions,
		"createdAt":       now,
		"updatedAt":       now,
	}
}

func containsForbiddenKey(value any) (string, bool) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			normalized := strings.ToLower(strings.TrimSpace(key))
			switch normalized {
			case "html", "script", "iframe", "innerhtml", "dangerouslysetinnerhtml":
				return key, true
			}
			if found, ok := containsForbiddenKey(child); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range v {
			if found, ok := containsForbiddenKey(child); ok {
				return found, true
			}
		}
	case []map[string]any:
		for _, child := range v {
			if found, ok := containsForbiddenKey(child); ok {
				return found, true
			}
		}
	}
	return "", false
}

func hasRCAEvidence(inline map[string]any) bool {
	return len(stringSlice(inline["evidenceRefs"])) > 0 || len(anySlice(inline["rawRefs"])) > 0
}

func validateArtifactActions(actions []map[string]any) error {
	for idx, action := range actions {
		if strings.TrimSpace(stringValue(action["href"])) != "" {
			return fmt.Errorf("agentui: direct action href is not allowed")
		}
		if boolValue(action["mutation"]) {
			return fmt.Errorf("agentui: mutating artifact actions are not allowed")
		}
		target, _ := action["target"].(map[string]any)
		kind := strings.TrimSpace(stringValue(target["kind"]))
		id := strings.TrimSpace(stringValue(target["id"]))
		switch kind {
		case "case", "evidence", "prompt_trace":
			if id == "" {
				return fmt.Errorf("agentui: action %d target id is required", idx)
			}
		default:
			return fmt.Errorf("agentui: action %d target kind must be case, evidence, or prompt_trace", idx)
		}
	}
	return nil
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s := strings.TrimSpace(stringValue(item)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func anySlice(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return items
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func boolValue(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

var inputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"type":{"type":"string","enum":["rca_report"]},
		"title":{"type":"string"},
		"titleZh":{"type":"string"},
		"summary":{"type":"string"},
		"summaryZh":{"type":"string"},
		"status":{"type":"string","enum":["ok","partial","inconclusive","error"]},
		"severity":{"type":"string","enum":["info","low","medium","high","critical"]},
		"source":{"type":"string"},
		"caseId":{"type":"string"},
		"evidenceRef":{"type":"string"},
		"promptTraceId":{"type":"string"},
		"permissionScope":{"type":"string","enum":["read"]},
		"redactionStatus":{"type":"string","enum":["none","partial","restricted","redacted"]},
		"inlineData":{"type":"object"},
		"metadata":{"type":"object"},
		"actions":{
			"type":"array",
			"items":{
				"type":"object",
				"properties":{
					"id":{"type":"string"},
					"label":{"type":"string"},
					"target":{
						"type":"object",
						"properties":{
							"kind":{"type":"string","enum":["case","evidence","prompt_trace"]},
							"id":{"type":"string"},
							"caseId":{"type":"string"}
						},
						"required":["kind","id"]
					},
					"mutation":{"const":false}
				},
				"required":["target"]
			}
		}
	},
	"required":["type","inlineData"]
}`)

var outputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"type":{"type":"string"},
		"status":{"type":"string"},
		"severity":{"type":"string"},
		"source":{"type":"string"},
		"inlineData":{"type":"object"}
	},
	"required":["schemaVersion","type","status"]
}`)
