package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func makeTool(server mcp.ServerConfig, def ToolDefinition, caller func(context.Context, string, string, json.RawMessage) (ToolCallResult, error)) tooling.Tool {
	name := strings.TrimSpace(def.Name)
	meta := tooling.ToolMetadata{
		Name:        name,
		Description: strings.TrimSpace(def.Description),
		Origin:      tooling.ToolOriginMCP,
		IsMCP:       true,
		MCPInfo: tooling.MCPInfo{
			ServerID:   server.ID,
			ServerName: firstNonEmpty(server.Name, server.ID),
			ToolName:   name,
			Raw:        append(json.RawMessage(nil), def.Raw...),
		},
	}
	return &tooling.StaticTool{
		Meta:             meta,
		InputSchemaData:  append(json.RawMessage(nil), def.InputSchema...),
		OutputSchemaData: append(json.RawMessage(nil), def.OutputSchema...),
		ReadOnlyFunc: func(json.RawMessage) bool {
			return def.ReadOnly
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return def.Destructive
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return def.ConcurrencySafe
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			if caller == nil {
				return tooling.ToolResult{}, fmt.Errorf("mcp runtime caller is not configured")
			}
			result, err := caller(ctx, server.ID, name, input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: result.Content,
				Error:   result.Error,
			}, nil
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
