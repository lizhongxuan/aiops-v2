package tooling

import (
	"context"
	"encoding/json"
)

// ToolOrigin describes where a tool came from.
// It is retained as a compatibility/display field and should not be treated as
// the primary runtime classification axis for tool selection.
type ToolOrigin string

const (
	ToolOriginBuiltin ToolOrigin = "builtin"
	ToolOriginMCP     ToolOrigin = "mcp"
	ToolOriginMeta    ToolOrigin = "meta"
)

// ToolSource captures Claude-like source traits without relying on Origin as the primary axis.
type ToolSource string

const (
	ToolSourceBuiltin ToolSource = "builtin"
	ToolSourceMCP     ToolSource = "mcp"
	ToolSourceLSP     ToolSource = "lsp"
	ToolSourceMeta    ToolSource = "meta"
)

// MCPInfo stores MCP-specific metadata for tools that originated from an MCP server.
type MCPInfo struct {
	ServerID   string          `json:"serverId,omitempty"`
	ServerName string          `json:"serverName,omitempty"`
	ToolName   string          `json:"toolName,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// ToolMetadata captures the registry-facing metadata for a tool.
type ToolMetadata struct {
	Name        string     `json:"name"`
	Aliases     []string   `json:"aliases,omitempty"`
	Description string     `json:"description,omitempty"`
	Origin      ToolOrigin `json:"origin,omitempty"`
	SearchHint  string     `json:"searchHint,omitempty"`
	ShouldDefer bool       `json:"shouldDefer,omitempty"`
	AlwaysLoad  bool       `json:"alwaysLoad,omitempty"`
	IsMCP       bool       `json:"isMCP,omitempty"`
	IsLSP       bool       `json:"isLSP,omitempty"`
	MCPInfo     MCPInfo    `json:"mcpInfo,omitempty"`
}

// HasMCPSource reports whether the metadata identifies an MCP-backed tool.
// This traits-first predicate is the preferred runtime check over Origin.
func (m ToolMetadata) HasMCPSource() bool {
	if m.IsMCP {
		return true
	}
	return m.MCPInfo.ServerID != "" || m.MCPInfo.ServerName != "" || m.MCPInfo.ToolName != ""
}

// HasSource reports whether the metadata matches the requested source trait.
func (m ToolMetadata) HasSource(source ToolSource) bool {
	switch source {
	case ToolSourceMCP:
		return m.HasMCPSource()
	case ToolSourceLSP:
		return m.IsLSP
	case ToolSourceMeta:
		return m.Origin == ToolOriginMeta
	case ToolSourceBuiltin:
		return !m.HasMCPSource() && !m.IsLSP && m.Origin != ToolOriginMeta
	default:
		return false
	}
}

// ToolDisplayPayload is a simple structured payload for human-facing tool output.
type ToolDisplayPayload struct {
	Type    string          `json:"type"`
	Title   string          `json:"title,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	CardRef string          `json:"cardRef,omitempty"`
}

// ToolResult is the output of a tool invocation.
type ToolResult struct {
	ToolCallID string              `json:"toolCallId,omitempty"`
	Content    string              `json:"content,omitempty"`
	Display    *ToolDisplayPayload `json:"display,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// PermissionAction classifies the outcome of a permission check.
type PermissionAction string

const (
	PermissionActionAllow        PermissionAction = "allow"
	PermissionActionDeny         PermissionAction = "deny"
	PermissionActionNeedApproval PermissionAction = "need_approval"
	PermissionActionNeedEvidence PermissionAction = "need_evidence"
)

// PermissionDecision is the result returned by a permission check.
type PermissionDecision struct {
	Action PermissionAction `json:"action"`
	Reason string           `json:"reason,omitempty"`
}

// DescribeContext carries contextual data for Description generation.
type DescribeContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// PromptContext carries contextual data for Prompt generation.
type PromptContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// ToolContext carries visibility/runtime context for IsEnabled.
type ToolContext struct {
	Context     context.Context
	SessionType string
	Mode        string
	Metadata    ToolMetadata
}

// Tool is the unified tool abstraction used by the new tooling registry.
type Tool interface {
	Metadata() ToolMetadata
	InputSchema() json.RawMessage
	OutputSchema() json.RawMessage
	Description(input json.RawMessage, ctx DescribeContext) string
	Prompt(ctx PromptContext) string
	IsEnabled(ctx ToolContext) bool
	IsReadOnly(input json.RawMessage) bool
	IsDestructive(input json.RawMessage) bool
	IsConcurrencySafe(input json.RawMessage) bool
	ValidateInput(ctx context.Context, input json.RawMessage) error
	CheckPermissions(ctx context.Context, input json.RawMessage) PermissionDecision
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
}
