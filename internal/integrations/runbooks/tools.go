package runbooks

import (
	"context"
	"encoding/json"

	core "aiops-v2/internal/runbooks"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.runbook/v1"

func tools(service *core.Service) []tooling.Tool {
	visibility := tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"inspect", "plan", "execute"}}
	return []tooling.Tool{
		newTool("runbook.match", "Match ERP SRE runbooks by symptom, capability, service, and environment", matchSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.MatchRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "runbook.match", "status": "ok", "candidates": service.Match(req)}, nil
		}),
		newTool("runbook.start", "Create a runbook instance without executing any step", startSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.StartRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			instance, err := service.Start(req)
			if err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "runbook.start", "status": "ok", "instance": instance}, nil
		}),
		newTool("runbook.next_action", "Generate the next ActionProposal for a runbook instance; does not execute the underlying tool", nextActionSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.NextActionRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			proposal, ok, err := service.NextAction(req)
			if err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "runbook.next_action", "status": "ok", "hasAction": ok, "proposal": proposal}, nil
		}),
		newTool("runbook.observe_result", "Observe a tool result reference and advance a runbook instance", observeSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.ObserveResultRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			if err := service.ObserveResult(req); err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "runbook.observe_result", "status": "ok"}, nil
		}),
		newTool("runbook.close", "Close a runbook instance and return a structured summary", closeSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var req core.CloseRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
			summary, err := service.Close(req)
			if err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "runbook.close", "status": "ok", "summary": summary}, nil
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
			return tooling.ToolResult{Content: string(data), Display: &tooling.ToolDisplayPayload{Type: "runbook", Title: name, Data: data}}, nil
		},
	}
}

var matchSchema = json.RawMessage(`{"type":"object","properties":{"symptom":{"type":"string"},"capability":{"type":"string"},"service":{"type":"string"},"environment":{"type":"string"},"limit":{"type":"integer"}}}`)
var startSchema = json.RawMessage(`{"type":"object","properties":{"runbookId":{"type":"string"},"incidentId":{"type":"string"},"context":{"type":"object"},"evidence":{"type":"object"}},"required":["runbookId"]}`)
var nextActionSchema = json.RawMessage(`{"type":"object","properties":{"instanceId":{"type":"string"},"sessionId":{"type":"string"},"turnId":{"type":"string"}},"required":["instanceId","sessionId","turnId"]}`)
var observeSchema = json.RawMessage(`{"type":"object","properties":{"instanceId":{"type":"string"},"stepId":{"type":"string"},"toolResultRef":{"type":"string"},"evidenceRef":{"type":"string"},"evidencePatch":{"type":"object"},"failed":{"type":"boolean"},"failureReason":{"type":"string"}},"required":["instanceId","stepId"]}`)
var closeSchema = json.RawMessage(`{"type":"object","properties":{"instanceId":{"type":"string"},"reason":{"type":"string"}},"required":["instanceId"]}`)
