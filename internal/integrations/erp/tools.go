package erp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.erp/v1"

type erpInput struct {
	Capability  string `json:"capability,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func tools() []tooling.Tool {
	visibility := tooling.Visibility{SessionTypes: []string{"workspace", "host"}, Modes: []string{"inspect", "execute"}}
	return []tooling.Tool{
		newERPTool("erp.business_metric", "Read ERP business metrics using a fixed schema", visibility, func(in erpInput) any {
			return map[string]any{
				"schemaVersion": schemaVersion,
				"tool":          "erp.business_metric",
				"status":        "ok",
				"capability":    strings.TrimSpace(in.Capability),
				"service":       strings.TrimSpace(in.Service),
				"metrics":       sampleMetrics(in.Capability, in.Service),
			}
		}),
		newERPTool("erp.tenant_impact", "Read affected ERP tenants and critical processes", visibility, func(in erpInput) any {
			tenants := sampleTenantImpact(in.Capability)
			return map[string]any{
				"schemaVersion":   schemaVersion,
				"tool":            "erp.tenant_impact",
				"status":          "ok",
				"affectedCount":   len(tenants),
				"businessProcess": strings.TrimSpace(in.Capability),
				"tenants":         tenants,
			}
		}),
		newERPTool("erp.job_status", "Read ERP job status, queue depth, and recent failures", visibility, func(in erpInput) any {
			return map[string]any{
				"schemaVersion": schemaVersion,
				"tool":          "erp.job_status",
				"status":        "ok",
				"service":       strings.TrimSpace(in.Service),
				"jobs":          sampleJobStatus(in.Service),
			}
		}),
	}
}

func newERPTool(name, description string, visibility tooling.Visibility, build func(erpInput) any) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Origin:      tooling.ToolOriginBuiltin,
			Description: description,
		},
		Visibility: visibility,
		InputSchemaData: json.RawMessage(`{
			"type":"object",
			"properties":{
				"capability":{"type":"string"},
				"service":{"type":"string"},
				"environment":{"type":"string"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var in erpInput
			if len(input) > 0 {
				if err := json.Unmarshal(input, &in); err != nil {
					return tooling.ToolResult{}, fmt.Errorf("invalid ERP input: %w", err)
				}
			}
			data, _ := json.Marshal(build(in))
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "erp", Title: name, Data: data}}, nil
		},
	}
}
