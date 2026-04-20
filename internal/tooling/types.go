package tooling

import (
	"context"
	"encoding/json"
)

// ToolOrigin describes where a tool came from.
type ToolOrigin string

const (
	ToolOriginBuiltin ToolOrigin = "builtin"
	ToolOriginMCP     ToolOrigin = "mcp"
	ToolOriginMeta    ToolOrigin = "meta"
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

// LegacyToolRuntime is a structural compatibility interface for old tool runtimes.
// Capability code can bridge its existing ToolRuntime into this shape without
// importing the capability package here.
type LegacyToolRuntime interface {
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, input json.RawMessage) (ToolResult, error)
	IsEnabled(ctx ToolContext) bool
	CheckPermissions(ctx context.Context) error
	IsReadOnly() bool
	IsDestructive() bool
	IsConcurrencySafe() bool
	Display() ToolDisplayPayload
}
