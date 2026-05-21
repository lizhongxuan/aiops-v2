package opsmanuals

import (
	"context"
	"encoding/json"
	"fmt"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

const runOpsManualPreflightToolName = "run_ops_manual_preflight"

func newRunOpsManualPreflightTool(service *core.Service, cache *turnSearchContextCache) tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"chat", "plan", "execute"},
	}
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           runOpsManualPreflightToolName,
			Aliases:        []string{"ops_manual.preflight"},
			Origin:         tooling.ToolOriginBuiltin,
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "ops_manual_flow",
			DeferByDefault: true,
			Description:    "Run read-only Node 0 preflight for a selected ops manual. Checks parameter readiness, environment compatibility, permission gaps, and probe evidence without executing the workflow.",
			RiskLevel:      tooling.ToolRiskLow,
		},
		Visibility:      visibility,
		InputSchemaData: runOpsManualPreflightInputSchema,
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
			req, err := decodeRunOpsManualPreflightInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if cache != nil {
				req = cache.enrichPreflightRequest(ctx, req)
			}
			result, err := service.RunPreflight(req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			modelContent, err := json.Marshal(runOpsManualPreflightModelResult(result, req))
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(modelContent),
				Display: &tooling.ToolDisplayPayload{
					Type:  "ops_manual_preflight_result",
					Title: runOpsManualPreflightToolName,
					Data:  data,
				},
			}, nil
		},
	}
}

type runOpsManualPreflightModelPayload struct {
	core.PreflightResult
	Instructions []string `json:"instructions,omitempty"`
}

func runOpsManualPreflightModelResult(result core.PreflightResult, req core.PreflightRequest) runOpsManualPreflightModelPayload {
	payload := runOpsManualPreflightModelPayload{PreflightResult: result}
	if result.Status == core.PreflightStatusPassed && preflightRequestIsStatusCheck(req) {
		payload.Instructions = []string{
			"Status/health check preflight passed. Stop tool use now; do not run host, shell, Docker, Kubernetes, Coroot, or other probes unless evidence is failed, stale, or explicitly insufficient.",
			"Final answer format: output only 1-3 bullets total; no introductory sentence, no headings, and no separate evidence section.",
			"Each bullet must combine conclusion and compact evidence; include that no change was executed in one bullet.",
		}
		return payload
	}
	switch result.Status {
	case core.PreflightStatusPassed:
		payload.Instructions = []string{"Preflight passed. Do not execute the workflow until the user confirms execution or completes required approval."}
	case core.PreflightStatusBlocked, core.PreflightStatusFailed:
		payload.Instructions = []string{"Preflight did not pass. State the concrete blocker briefly and use the manual fallback path instead of executing workflow."}
	}
	return payload
}

func preflightRequestIsStatusCheck(req core.PreflightRequest) bool {
	action := req.OperationFrame.Operation.Action
	if action == "" {
		action = req.OperationFrame.Intent
	}
	return action == "status_check" || action == "health_check"
}

func decodeRunOpsManualPreflightInput(input json.RawMessage) (core.PreflightRequest, error) {
	var req core.PreflightRequest
	if len(input) == 0 {
		return req, fmt.Errorf("invalid run_ops_manual_preflight input: empty input")
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return req, fmt.Errorf("invalid run_ops_manual_preflight input: %w", err)
	}
	return req, nil
}

var runOpsManualPreflightInputSchema = json.RawMessage(`{
  "type": "object",
  "description": "Read-only Node 0 preflight. For a direct_execute search_ops_manuals result, copy the returned operation_frame and extracted parameters into this input. Do not call with only manual_id/workflow_id.",
  "required": ["manual_id", "operation_frame", "parameters"],
  "properties": {
    "manual_id": {
      "type": "string",
      "minLength": 1,
      "description": "The selected ops manual id from search_ops_manuals, for example manual-pg-backup-ubuntu."
    },
    "workflow_id": {
      "type": "string",
      "description": "The workflow_id bound to the selected ops manual."
    },
    "operation_frame": {
      "type": "object",
      "description": "The operation_frame returned by search_ops_manuals. Preserve target, operation, environment, evidence, required_params, and raw_text.",
      "additionalProperties": true
    },
    "parameters": {
      "type": "object",
      "description": "Extracted parameters required by the selected manual, such as target_instance, namespace, pod_name, backup_path, ssh_access, pg_isready, metrics, or symptom.",
      "additionalProperties": true
    },
    "triggered_by": {
      "type": "string",
      "description": "The trigger surface, for example chat."
    },
    "requested_by": {
      "type": "string",
      "description": "The requester, for example user."
    }
  },
  "additionalProperties": false
}`)
