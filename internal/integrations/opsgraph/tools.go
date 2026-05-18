package opsgraph

import (
	"context"
	"encoding/json"

	graph "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.opsgraph/v1"

type lookupInput struct {
	Query string             `json:"query"`
	Types []graph.EntityType `json:"types,omitempty"`
	Limit int                `json:"limit,omitempty"`
}

type entityInput struct {
	EntityID string `json:"entityId"`
	Query    string `json:"query,omitempty"`
	Depth    int    `json:"depth,omitempty"`
}

func tools(store *graph.Store) []tooling.Tool {
	visibility := tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}}
	return []tooling.Tool{
		newTool("opsgraph.lookup", "Look up ERP modules, business capabilities, services, data stores, tenants, and runbooks by symptom or name", lookupSchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var in lookupInput
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, err
			}
			return map[string]any{"schemaVersion": schemaVersion, "tool": "opsgraph.lookup", "status": "ok", "matches": store.Lookup(graph.LookupRequest{Query: in.Query, Types: in.Types, Limit: in.Limit})}, nil
		}),
		newTool("opsgraph.neighborhood", "Return the 1-2 hop ERP operations graph neighborhood for an entity", entitySchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var in entityInput
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, err
			}
			id := firstNonEmpty(in.EntityID, in.Query)
			return map[string]any{"schemaVersion": schemaVersion, "tool": "opsgraph.neighborhood", "status": "ok", "neighborhood": store.Neighborhood(id, in.Depth)}, nil
		}),
		newTool("opsgraph.business_impact", "Summarize ERP business impact for an entity using graph relationships", entitySchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var in entityInput
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, err
			}
			id := firstNonEmpty(in.EntityID, in.Query)
			return map[string]any{"schemaVersion": schemaVersion, "tool": "opsgraph.business_impact", "status": "ok", "impact": store.BusinessImpact(id)}, nil
		}),
		newTool("opsgraph.related_runbooks", "Return candidate runbooks for an ERP graph entity with match reasons", entitySchema, visibility, func(ctx context.Context, input json.RawMessage) (any, error) {
			var in entityInput
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, err
			}
			id := firstNonEmpty(in.EntityID, in.Query)
			return map[string]any{"schemaVersion": schemaVersion, "tool": "opsgraph.related_runbooks", "status": "ok", "runbooks": store.RelatedRunbooks(id)}, nil
		}),
	}
}

func newTool(name, description string, schema json.RawMessage, visibility tooling.Visibility, execute func(context.Context, json.RawMessage) (any, error)) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           name,
			Origin:         tooling.ToolOriginBuiltin,
			Description:    description,
			Domain:         "opsgraph",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "opsgraph",
			DeferByDefault: true,
			Triggers:       []string{"业务影响", "依赖关系", "服务图谱", "runbook", "impact", "dependency", "graph"},
			RiskLevel:      tooling.ToolRiskLow,
		},
		Visibility:       visibility,
		InputSchemaData:  schema,
		OutputSchemaData: outputSchema,
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
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{Type: "opsgraph", Title: name, Data: data},
			}, nil
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var lookupSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"query": {"type": "string", "description": "Symptom, service name, business capability, ERP module, tenant, or runbook name"},
		"types": {"type": "array", "items": {"type": "string"}, "description": "Optional entity type filters"},
		"limit": {"type": "integer", "description": "Maximum number of matches"}
	},
	"required": ["query"]
}`)

var entitySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"entityId": {"type": "string", "description": "OpsGraph entity id"},
		"query": {"type": "string", "description": "Fallback lookup query when entityId is unknown"},
		"depth": {"type": "integer", "description": "Neighborhood traversal depth, defaults to 1"}
	}
}`)

var outputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"source":{"type":"string"},
		"mock":{"type":"boolean"},
		"evidenceRefs":{"type":"array","items":{"type":"string"}}
	},
	"required":["schemaVersion","tool","status"]
}`)
