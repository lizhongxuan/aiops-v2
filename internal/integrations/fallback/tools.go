package fallback

import (
	"context"
	"encoding/json"

	core "aiops-v2/internal/fallback"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.fallback/v1"

func tools(service *core.Service) []tooling.Tool {
	visibility := tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"inspect", "plan", "execute"}}
	return []tooling.Tool{
		newTool("fallback.plan_exec", "Generate governed ActionProposal objects when no suitable runbook covers the incident", planExecSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.PlanExecRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			result, err := service.PlanExec(req)
			if err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "fallback.plan_exec", "status": "ok", "plan": result.Plan}, nil
		}),
		newTool("fallback.observe_result", "Record a fallback action result reference and verification note", observeSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.ObserveResultRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			if err := service.ObserveResult(req); err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "fallback.observe_result", "status": "ok"}, nil
		}),
	}
}

func newTool(name, description string, schema json.RawMessage, visibility tooling.Visibility, execute func(context.Context, json.RawMessage) (any, error)) tooling.Tool {
	return &tooling.StaticTool{
		Meta:            tooling.ToolMetadata{Name: name, Origin: tooling.ToolOriginBuiltin, Description: description, RiskLevel: tooling.ToolRiskLow},
		Visibility:      visibility,
		InputSchemaData: schema,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return false
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			payload, err := execute(ctx, input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "fallback", Title: name, Data: data}}, nil
		},
	}
}

var planExecSchema = json.RawMessage(`{"type":"object","properties":{"sessionId":{"type":"string"},"turnId":{"type":"string"},"incidentId":{"type":"string"},"goal":{"type":"string"},"whyNoRunbook":{"type":"string"},"evidenceRefs":{"type":"array","items":{"type":"string"}},"actions":{"type":"array","items":{"type":"object"}},"runbookMatches":{"type":"array","items":{"type":"object"}}},"required":["sessionId","turnId","incidentId","actions"]}`)
var observeSchema = json.RawMessage(`{"type":"object","properties":{"planId":{"type":"string"},"actionToken":{"type":"string"},"toolResultRef":{"type":"string"},"evidenceRef":{"type":"string"},"failed":{"type":"boolean"},"reason":{"type":"string"}},"required":["planId"]}`)
