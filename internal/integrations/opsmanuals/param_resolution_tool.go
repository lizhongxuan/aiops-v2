package opsmanuals

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

const resolveOpsManualParamsToolName = "resolve_ops_manual_params"

func newResolveOpsManualParamsTool(service *core.Service, cache *turnSearchContextCache) tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"chat", "plan", "execute"},
	}
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           resolveOpsManualParamsToolName,
			Aliases:        []string{"ops_manual.resolve_params"},
			Origin:         tooling.ToolOriginBuiltin,
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "ops_manual_flow",
			DeferByDefault: true,
			Description:    "Resolve parameters for a matched ops manual using chat/session context and safe read-only resolvers. Returns resolved parameters for preflight or compact dynamic form fields.",
			RiskLevel:      tooling.ToolRiskLow,
		},
		Visibility:      visibility,
		InputSchemaData: resolveOpsManualParamsInputSchema,
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
			req, err := decodeResolveOpsManualParamsInput(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if cache != nil {
				req = cache.enrichParamResolutionRequest(ctx, req)
			}
			if strings.TrimSpace(req.ManualID) == "" {
				return missingManualIDToolResult()
			}
			result, err := service.ResolveOpsManualParams(req)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if cache != nil {
				cache.rememberParamResolution(ctx, result)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			modelContent, err := json.Marshal(resolveOpsManualParamsModelResult(result))
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(modelContent),
				Display: &tooling.ToolDisplayPayload{
					Type:  "ops_manual_param_resolution",
					Title: resolveOpsManualParamsToolName,
					Data:  data,
				},
			}, nil
		},
	}
}

func missingManualIDToolResult() (tooling.ToolResult, error) {
	payload := resolveOpsManualParamsModelPayload{
		Status:     string(core.ParamResolutionUnresolved),
		NextAction: "refine_search",
		Instructions: []string{
			"manual_id is missing, so parameters cannot be resolved.",
			"Do not call resolve_ops_manual_params again until search_ops_manuals returns a matched manual.",
			"If the user request already states object/action/target/environment/evidence/risk, call search_ops_manuals again with an explicit operation_frame.",
			"If object or action is genuinely missing, ask only the smallest missing question.",
		},
	}
	modelContent, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	return tooling.ToolResult{
		Content: string(modelContent),
	}, nil
}

type resolveOpsManualParamsModelPayload struct {
	Status         string            `json:"status"`
	ManualID       string            `json:"manual_id,omitempty"`
	WorkflowID     string            `json:"workflow_id,omitempty"`
	ResolvedParams map[string]any    `json:"resolved_params,omitempty"`
	Fields         []modelParamField `json:"fields,omitempty"`
	Blockers       []string          `json:"blockers,omitempty"`
	NextAction     string            `json:"next_action,omitempty"`
	Instructions   []string          `json:"instructions,omitempty"`
}

type modelParamField struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Type       string `json:"type,omitempty"`
	UIControl  string `json:"ui_control,omitempty"`
	Candidates int    `json:"candidates,omitempty"`
}

func resolveOpsManualParamsModelResult(result core.ParamResolutionResult) resolveOpsManualParamsModelPayload {
	payload := resolveOpsManualParamsModelPayload{
		Status:     string(result.Status),
		ManualID:   result.ManualID,
		WorkflowID: result.WorkflowID,
		NextAction: result.NextAction,
	}
	if len(result.ResolvedParams) > 0 {
		payload.ResolvedParams = map[string]any{}
		for _, param := range result.ResolvedParams {
			payload.ResolvedParams[param.ID] = param.Value
		}
	}
	for _, field := range result.Fields {
		payload.Fields = append(payload.Fields, modelParamField{
			ID:         field.ID,
			Label:      field.Label,
			Type:       field.Type,
			UIControl:  field.UIControl,
			Candidates: len(field.Candidates),
		})
	}
	for _, missing := range result.MissingParams {
		if reason := strings.TrimSpace(missing.Reason); reason != "" && reason != "no candidate" {
			payload.Blockers = append(payload.Blockers, reason)
		}
	}
	switch result.Status {
	case core.ParamResolutionResolved:
		payload.Instructions = []string{
			"Parameters are resolved. Next call run_ops_manual_preflight with manual_id, workflow_id, operation_frame, and resolved params.",
			"Keep prose short and do not duplicate the Agent-to-UI card.",
		}
	case core.ParamResolutionAmbiguous, core.ParamResolutionNeedUserInput:
		payload.Instructions = []string{
			"Stop tool use now and wait for the user to submit the structured Agent-to-UI form.",
			"Do not ask fixed fields in prose. The Agent-to-UI card will replace the bottom composer using the fields schema.",
			"Tell the user only that one or more specific parameters need confirmation.",
			"When blockers are present, state the blocker briefly, such as no target resource discovered by read-only discovery.",
			"Do not run host commands, Coroot probes, ordinary shell checks, preflight, or workflow execution while the form is pending.",
		}
	default:
		payload.Instructions = []string{"State the concrete blocker briefly; do not execute workflow or preflight."}
	}
	return payload
}

func decodeResolveOpsManualParamsInput(input json.RawMessage) (core.ResolveOpsManualParamsRequest, error) {
	var req core.ResolveOpsManualParamsRequest
	if len(input) == 0 {
		return req, fmt.Errorf("invalid resolve_ops_manual_params input: empty input")
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return req, fmt.Errorf("invalid resolve_ops_manual_params input: %w", err)
	}
	return req, nil
}

var resolveOpsManualParamsInputSchema = json.RawMessage(`{
  "type": "object",
  "description": "Resolve ops manual parameters after search_ops_manuals returns need_info or before preflight when parameters may be incomplete. This tool is read-only and must run before asking the user manual parameters.",
  "properties": {
    "request_text": {
      "type": "string",
      "description": "The user's original operations request."
    },
    "manual_id": {
      "type": "string",
      "description": "The matched ops manual id from search_ops_manuals."
    },
    "workflow_id": {
      "type": "string",
      "description": "The workflow id bound to the selected manual."
    },
    "operation_frame": {
      "type": "object",
      "description": "Structured semantics to preserve from search_ops_manuals. When the user named a concrete instance/service/pod/container/host/resource, pass it as target.name; keep selected/current host in target_scope.hosts.",
      "additionalProperties": true
    },
    "known_params": {
      "type": "object",
      "description": "Known parameter values from the user or prior Agent-to-UI form. Use target_instance for an explicit user-provided resource, backup_path for an explicit path, and never place guessed sensitive values here.",
      "additionalProperties": true
    },
    "metadata": {
      "type": "object",
      "additionalProperties": true
    }
  },
  "additionalProperties": false
}`)
