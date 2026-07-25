package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
)

const (
	MCPProtocolVersion20250326 = "2025-03-26"
	MCPProtocolVersion20250618 = "2025-06-18"
)

var supportedMCPProtocolVersions = []string{MCPProtocolVersion20250326, MCPProtocolVersion20250618}

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
	Content           string            `json:"content,omitempty"`
	ContentBlocks     []json.RawMessage `json:"-"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
	Error             string            `json:"error,omitempty"`
	Raw               json.RawMessage   `json:"-"`
}

func (r *ToolCallResult) UnmarshalJSON(data []byte) error {
	var wire struct {
		Content           json.RawMessage `json:"content"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
		Error             string          `json:"error"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	next := ToolCallResult{
		StructuredContent: append(json.RawMessage(nil), wire.StructuredContent...),
		IsError:           wire.IsError,
		Error:             wire.Error,
		Raw:               append(json.RawMessage(nil), data...),
	}
	content := bytes.TrimSpace(wire.Content)
	if len(content) > 0 && !bytes.Equal(content, []byte("null")) {
		switch content[0] {
		case '"':
			if err := json.Unmarshal(content, &next.Content); err != nil {
				return fmt.Errorf("decode MCP legacy content: %w", err)
			}
		case '[':
			if err := json.Unmarshal(content, &next.ContentBlocks); err != nil {
				return fmt.Errorf("decode MCP content blocks: %w", err)
			}
		default:
			return fmt.Errorf("decode MCP result: content must be a string or array")
		}
	}
	*r = next
	return nil
}

func (r ToolCallResult) MarshalJSON() ([]byte, error) {
	if len(r.Raw) > 0 && json.Valid(r.Raw) {
		return append([]byte(nil), r.Raw...), nil
	}
	var content any
	if len(r.ContentBlocks) > 0 {
		content = r.ContentBlocks
	} else if r.Content != "" {
		content = r.Content
	}
	return json.Marshal(struct {
		Content           any             `json:"content,omitempty"`
		StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
		IsError           bool            `json:"isError,omitempty"`
		Error             string          `json:"error,omitempty"`
	}{content, r.StructuredContent, r.IsError, r.Error})
}

type ImplementationInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeRequest struct {
	SupportedProtocolVersions []string
	Capabilities              json.RawMessage
	ClientInfo                ImplementationInfo
}

type ClientSession struct {
	ProtocolVersion string
	Capabilities    json.RawMessage
	ServerInfo      json.RawMessage
	Instructions    string
	SessionID       string
	Legacy          bool
}

type LifecycleClient interface {
	Initialize(context.Context, InitializeRequest) (ClientSession, error)
	NotifyInitialized(context.Context) error
}

func SupportedMCPProtocolVersions() []string {
	return append([]string(nil), supportedMCPProtocolVersions...)
}

func DefaultInitializeRequest() InitializeRequest {
	return InitializeRequest{
		SupportedProtocolVersions: SupportedMCPProtocolVersions(),
		Capabilities:              json.RawMessage(`{}`),
		ClientInfo:                ImplementationInfo{Name: "aiops-v2", Version: "dev"},
	}
}

type mcpRequestTokenContextKey struct{}

func WithMCPRequestToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, mcpRequestTokenContextKey{}, strings.TrimSpace(token))
}

func MCPRequestTokenFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	token, _ := ctx.Value(mcpRequestTokenContextKey{}).(string)
	token = strings.TrimSpace(token)
	return token, token != ""
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
