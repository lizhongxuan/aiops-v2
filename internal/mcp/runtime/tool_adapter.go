package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func makeTool(server mcp.ServerConfig, governance mcp.ServerGovernance, def ToolDefinition, caller func(context.Context, string, string, json.RawMessage) (ToolCallResult, error)) tooling.Tool {
	name := strings.TrimSpace(def.Name)
	meta := tooling.ToolMetadata{
		Name:           name,
		Description:    strings.TrimSpace(def.Description),
		Origin:         tooling.ToolOriginMCP,
		IsMCP:          true,
		Layer:          tooling.ToolLayerDeferred,
		Pack:           firstNonEmpty(server.ToolPack, mcpToolPackName(server.ID)),
		DeferByDefault: true,
		RiskLevel:      mcpToolRisk(def),
		Mutating:       def.Destructive,
		Triggers:       []string{"MCP tool", "dynamic tool", firstNonEmpty(server.Name, server.ID), name},
		Discovery: tooling.ToolDiscoveryMetadata{
			DiscoveryGroup: "mcp",
			DiscoveryTags:  []string{"mcp", "dynamic", firstNonEmpty(server.Name, server.ID), name},
			ResourceTypes:  []string{"mcp_tool"},
			OperationKinds: mcpToolOperationKinds(def),
			RequiresSelect: true,
		},
		MCPInfo: tooling.MCPInfo{
			ServerID:   server.ID,
			ServerName: firstNonEmpty(server.Name, server.ID),
			ToolName:   name,
			Raw:        append(json.RawMessage(nil), def.Raw...),
		},
	}
	meta = mcp.ApplyServerManifestToToolMetadata(server, meta, def.ReadOnly, def.Destructive)
	meta = mcp.MergeMCPGovernance(server, governance, meta)
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

func mcpToolRisk(def ToolDefinition) tooling.ToolRiskLevel {
	if def.ReadOnly && !def.Destructive {
		return tooling.ToolRiskLow
	}
	return tooling.ToolRiskMedium
}

func mcpToolOperationKinds(def ToolDefinition) []string {
	if def.Destructive {
		return []string{"write"}
	}
	if def.ReadOnly {
		return []string{"read"}
	}
	return []string{"execute"}
}

func mcpToolPackName(serverID string) string {
	serverID = strings.ToLower(strings.TrimSpace(serverID))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range serverID {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if b.Len() > 0 && !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	suffix := strings.Trim(b.String(), "_")
	if suffix == "" {
		suffix = "server"
	}
	return "mcp_dynamic_" + suffix
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
