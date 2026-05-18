package mcpresources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.mcp_resource/v1"

func NewListTool(registry *mcp.Registry) tooling.Tool {
	return newTool("list_mcp_resources", "List readable MCP resources exposed by connected servers", listSchema, func(_ context.Context, input json.RawMessage) (any, error) {
		var req struct {
			Server string `json:"server,omitempty"`
		}
		if len(input) > 0 {
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, err
			}
		}
		resources := registry.ListResources(req.Server)
		return envelope("list_mcp_resources", map[string]any{
			"server":    strings.TrimSpace(req.Server),
			"resources": resources,
		}), nil
	})
}

func NewReadTool(registry *mcp.Registry) tooling.Tool {
	return newTool("read_mcp_resource", "Read the content of an MCP resource by server and URI", readSchema, func(ctx context.Context, input json.RawMessage) (any, error) {
		var req struct {
			Server string `json:"server"`
			URI    string `json:"uri"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		content, ok, err := registry.ReadResource(ctx, req.Server, req.URI)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("mcp resource %q not found on server %q", strings.TrimSpace(req.URI), strings.TrimSpace(req.Server))
		}
		return envelope("read_mcp_resource", map[string]any{
			"server":      content.ServerID,
			"uri":         content.URI,
			"contentType": content.MimeType,
			"text":        content.Text,
			"blob":        content.Blob,
			"digest":      content.Digest,
			"bytes":       content.Bytes,
			"refs": []map[string]any{{
				"kind":        "mcp_resource",
				"uri":         content.URI,
				"title":       content.URI,
				"contentType": content.MimeType,
				"digest":      content.Digest,
				"bytes":       content.Bytes,
			}},
		}), nil
	})
}

func newTool(name, description string, schema json.RawMessage, execute func(context.Context, json.RawMessage) (any, error)) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           name,
			Origin:         tooling.ToolOriginBuiltin,
			Description:    description,
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "mcp_resource",
			DeferByDefault: true,
			Triggers:       []string{"MCP resource", "resource URI", "mcp://"},
			RiskLevel:      tooling.ToolRiskLow,
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:     schema,
		OutputSchemaData:    outputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
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
				Display: &tooling.ToolDisplayPayload{
					Type:  "mcp_resource",
					Title: name,
					Data:  data,
				},
			}, nil
		},
	}
}

func envelope(tool string, data map[string]any) map[string]any {
	return map[string]any{
		"schemaVersion": schemaVersion,
		"tool":          tool,
		"status":        "ok",
		"data":          data,
	}
}

var listSchema = json.RawMessage(`{"type":"object","properties":{"server":{"type":"string","description":"Optional MCP server id"}}}`)

var readSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"server":{"type":"string","description":"MCP server id"},
		"uri":{"type":"string","description":"Resource URI"}
	},
	"required":["server","uri"]
}`)

var outputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"data":{"type":"object"}
	},
	"required":["schemaVersion","tool","status"]
}`)
