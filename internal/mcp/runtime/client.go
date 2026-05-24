package runtime

import (
	"context"
	"encoding/json"

	"aiops-v2/internal/mcp"
)

type ToolDefinition struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	InputSchema     json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema    json.RawMessage `json:"outputSchema,omitempty"`
	ReadOnly        bool            `json:"readOnly,omitempty"`
	Destructive     bool            `json:"destructive,omitempty"`
	ConcurrencySafe bool            `json:"concurrencySafe,omitempty"`
	Raw             json.RawMessage `json:"raw,omitempty"`
}

type ToolCallResult struct {
	Content string          `json:"content,omitempty"`
	Error   string          `json:"error,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}

type Client interface {
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	CallTool(ctx context.Context, name string, input json.RawMessage) (ToolCallResult, error)
	ListResources(ctx context.Context) ([]mcp.Resource, error)
	ReadResource(ctx context.Context, uri string) (mcp.ResourceContent, error)
	Close(ctx context.Context) error
}

type ClientFactory interface {
	NewClient(ctx context.Context, cfg mcp.ServerConfig) (Client, error)
}

type ClientFactoryFunc func(ctx context.Context, cfg mcp.ServerConfig) (Client, error)

func (f ClientFactoryFunc) NewClient(ctx context.Context, cfg mcp.ServerConfig) (Client, error) {
	return f(ctx, cfg)
}
