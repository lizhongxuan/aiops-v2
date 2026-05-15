package opsmanuals

import (
	"context"
	"encoding/json"
	"fmt"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

const searchOpsManualsToolName = "search_ops_manuals"

func newSearchOpsManualsTool(service *core.Service) tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"inspect", "plan", "execute"},
	}
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        searchOpsManualsToolName,
			Aliases:     []string{"ops_manual.search"},
			Origin:      tooling.ToolOriginBuiltin,
			Description: "Search verified ops manuals before complex or high-risk operations such as service restart, configuration change, database backup/restore/migration, or cluster change. Pass the user's original task text verbatim whenever available; do not summarize away target_instance, execution_surface, symptoms, metrics, risk, or negations such as no restart/no write. Return an auditable decision: direct_execute, need_info, adapt, reference_only, or no_match; follow that decision and never treat need_info, adapt, or reference_only as directly executable.",
			RiskLevel:   tooling.ToolRiskLow,
		},
		Visibility:      visibility,
		InputSchemaData: searchOpsManualsInputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return false
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			req, err := decodeSearchOpsManualsInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			result, err := service.SearchOpsManuals(req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{
					Type:  "ops_manual_search_result",
					Title: searchOpsManualsToolName,
					Data:  data,
				},
			}, nil
		},
	}
}

func decodeSearchOpsManualsInput(input json.RawMessage) (core.SearchOpsManualsRequest, error) {
	var req core.SearchOpsManualsRequest
	if len(input) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return req, fmt.Errorf("invalid search_ops_manuals input: %w", err)
	}
	return req, nil
}

var searchOpsManualsInputSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "text": {
      "type": "string"
    },
    "metadata": {
      "type": "object",
      "additionalProperties": true
    },
    "operation_frame": {
      "type": "object",
      "additionalProperties": true
    },
    "limit": {
      "type": "integer",
      "minimum": 1
    }
  },
  "additionalProperties": false
}`)
