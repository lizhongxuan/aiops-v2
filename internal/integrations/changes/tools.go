package changes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.changes/v1"

type changeInput struct {
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
	Window      string `json:"window,omitempty"`
}

type Deployment struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Version     string `json:"version"`
	Actor       string `json:"actor"`
	StartedAt   string `json:"startedAt"`
	Status      string `json:"status"`
}

type ConfigChange struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Key         string `json:"key"`
	Actor       string `json:"actor"`
	ChangedAt   string `json:"changedAt"`
	Summary     string `json:"summary"`
}

func tools() []tooling.Tool {
	visibility := tooling.Visibility{SessionTypes: []string{"workspace", "host"}, Modes: []string{"inspect", "execute"}}
	return []tooling.Tool{
		newChangesTool("changes.recent_deployments", "Read recent deployments for ERP services", visibility, func(in changeInput) any {
			return map[string]any{
				"schemaVersion": schemaVersion,
				"tool":          "changes.recent_deployments",
				"status":        "ok",
				"service":       strings.TrimSpace(in.Service),
				"deployments": []Deployment{{
					ID: "deploy-20260504-1", Service: firstNonEmpty(in.Service, "order-api"), Environment: firstNonEmpty(in.Environment, "prod"), Version: "2026.05.04-1", Actor: "ci", StartedAt: "2026-05-04T09:12:00Z", Status: "completed",
				}},
			}
		}),
		newChangesTool("changes.recent_config_changes", "Read recent ERP configuration changes", visibility, func(in changeInput) any {
			return map[string]any{
				"schemaVersion": schemaVersion,
				"tool":          "changes.recent_config_changes",
				"status":        "ok",
				"service":       strings.TrimSpace(in.Service),
				"changes": []ConfigChange{{
					ID: "cfg-20260504-1", Service: firstNonEmpty(in.Service, "order-api"), Environment: firstNonEmpty(in.Environment, "prod"), Key: "db.maxConnections", Actor: "ops", ChangedAt: "2026-05-04T08:47:00Z", Summary: "raised connection pool limit",
				}},
			}
		}),
	}
}

func newChangesTool(name, description string, visibility tooling.Visibility, build func(changeInput) any) tooling.Tool {
	return &tooling.StaticTool{
		Meta:                tooling.ToolMetadata{Name: name, Origin: tooling.ToolOriginBuiltin, Description: description},
		Visibility:          visibility,
		InputSchemaData:     json.RawMessage(`{"type":"object","properties":{"service":{"type":"string"},"environment":{"type":"string"},"window":{"type":"string"}}}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var in changeInput
			if len(input) > 0 {
				if err := json.Unmarshal(input, &in); err != nil {
					return tooling.ToolResult{}, fmt.Errorf("invalid changes input: %w", err)
				}
			}
			data, _ := json.Marshal(build(in))
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "changes", Title: name, Data: data}}, nil
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
