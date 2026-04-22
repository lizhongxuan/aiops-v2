package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

// ServerConfig stores the registration-time configuration for an MCP server.
type ServerConfig struct {
	ID        string
	Name      string
	Transport string
	Command   []string
	Source    string
}

// Registry tracks MCP server configuration and dynamically connected tools.
type Registry struct {
	mu          sync.RWMutex
	governance  *settings.Governance
	serverCfgs  map[string]ServerConfig
	serverTools map[string][]tooling.Tool
}

// NewRegistry creates an empty MCP server registry.
func NewRegistry() *Registry {
	return &Registry{
		serverCfgs:  make(map[string]ServerConfig),
		serverTools: make(map[string][]tooling.Tool),
	}
}

// SetGovernance attaches a live governance snapshot source to the registry.
func (r *Registry) SetGovernance(governance *settings.Governance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governance = governance
}

// RegisterServer stores or replaces an MCP server configuration.
func (r *Registry) RegisterServer(cfg ServerConfig) error {
	cfg.ID = strings.TrimSpace(cfg.ID)
	if cfg.ID == "" {
		return fmt.Errorf("mcp: server id is required")
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	cfg.Transport = strings.TrimSpace(cfg.Transport)
	cfg.Command = append([]string(nil), cfg.Command...)
	cfg.Source = normalizeServerSource(cfg.Source)
	if err := r.validateServerConfig(cfg); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.serverCfgs[cfg.ID] = cfg
	return nil
}

// GetServer returns a cloned server configuration by id.
func (r *Registry) GetServer(id string) (ServerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.serverCfgs[strings.TrimSpace(id)]
	if !ok {
		return ServerConfig{}, false
	}
	return cloneServerConfig(cfg), true
}

// ListServers returns all registered server configs sorted by id.
func (r *Registry) ListServers() []ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ServerConfig, 0, len(r.serverCfgs))
	for _, cfg := range r.serverCfgs {
		out = append(out, cloneServerConfig(cfg))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// OnServerConnected replaces the connected tool set for a server.
func (r *Registry) OnServerConnected(serverID string, tools []tooling.Tool) error {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("mcp: server id is required")
	}

	normalized := make([]tooling.Tool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		normalized = append(normalized, normalizeServerTool(serverID, t))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.serverTools[serverID] = normalized
	return nil
}

// OnServerDisconnected removes all dynamically connected tools for a server.
func (r *Registry) OnServerDisconnected(serverID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.serverTools, strings.TrimSpace(serverID))
}

// UnregisterServer removes a server config and any connected tools.
func (r *Registry) UnregisterServer(serverID string) {
	serverID = strings.TrimSpace(serverID)

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.serverCfgs, serverID)
	delete(r.serverTools, serverID)
}

// ListServerTools returns the connected tools for one server.
func (r *Registry) ListServerTools(serverID string) []tooling.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools, ok := r.serverTools[strings.TrimSpace(serverID)]
	if !ok {
		return nil
	}
	return append([]tooling.Tool(nil), tools...)
}

// DynamicTools returns all connected tools across servers in stable order.
func (r *Registry) DynamicTools() []tooling.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serverIDs := make([]string, 0, len(r.serverTools))
	for serverID := range r.serverTools {
		serverIDs = append(serverIDs, serverID)
	}
	sort.Strings(serverIDs)

	var out []tooling.Tool
	for _, serverID := range serverIDs {
		tools := append([]tooling.Tool(nil), r.serverTools[serverID]...)
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Metadata().Name < tools[j].Metadata().Name
		})
		out = append(out, tools...)
	}
	return out
}

type metadataOverrideTool struct {
	base tooling.Tool
	meta tooling.ToolMetadata
}

func (t metadataOverrideTool) Metadata() tooling.ToolMetadata { return t.meta }

func (t metadataOverrideTool) InputSchema() json.RawMessage { return t.base.InputSchema() }

func (t metadataOverrideTool) OutputSchema() json.RawMessage { return t.base.OutputSchema() }

func (t metadataOverrideTool) Description(input json.RawMessage, ctx tooling.DescribeContext) string {
	ctx.Metadata = t.meta
	if desc := strings.TrimSpace(t.base.Description(input, ctx)); desc != "" {
		return desc
	}
	return t.meta.Description
}

func (t metadataOverrideTool) Prompt(ctx tooling.PromptContext) string {
	ctx.Metadata = t.meta
	if prompt := strings.TrimSpace(t.base.Prompt(ctx)); prompt != "" {
		return prompt
	}
	return t.meta.Description
}

func (t metadataOverrideTool) IsEnabled(ctx tooling.ToolContext) bool {
	ctx.Metadata = t.meta
	return t.base.IsEnabled(ctx)
}

func (t metadataOverrideTool) IsReadOnly(input json.RawMessage) bool {
	return t.base.IsReadOnly(input)
}

func (t metadataOverrideTool) IsDestructive(input json.RawMessage) bool {
	return t.base.IsDestructive(input)
}

func (t metadataOverrideTool) IsConcurrencySafe(input json.RawMessage) bool {
	return t.base.IsConcurrencySafe(input)
}

func (t metadataOverrideTool) ValidateInput(ctx context.Context, input json.RawMessage) error {
	return t.base.ValidateInput(ctx, input)
}

func (t metadataOverrideTool) CheckPermissions(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
	return t.base.CheckPermissions(ctx, input)
}

func (t metadataOverrideTool) Execute(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
	return t.base.Execute(ctx, input)
}

func normalizeServerTool(serverID string, t tooling.Tool) tooling.Tool {
	meta := t.Metadata()
	if meta.Name == "" {
		meta.Name = strings.TrimSpace(meta.MCPInfo.ToolName)
	}
	if meta.Description == "" {
		meta.Description = strings.TrimSpace(t.Description(nil, tooling.DescribeContext{Metadata: meta}))
	}
	meta.Origin = tooling.ToolOriginMCP
	meta.IsMCP = true
	if meta.MCPInfo.ServerID == "" {
		meta.MCPInfo.ServerID = serverID
	}
	if meta.MCPInfo.ServerName == "" {
		meta.MCPInfo.ServerName = serverID
	}
	if meta.MCPInfo.ToolName == "" {
		meta.MCPInfo.ToolName = meta.Name
	}
	return metadataOverrideTool{base: t, meta: meta}
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Command = append([]string(nil), cfg.Command...)
	return cfg
}

func normalizeServerSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "builtin"
	}
	return source
}

func (r *Registry) validateServerConfig(cfg ServerConfig) error {
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()

	if governance == nil {
		return nil
	}
	snapshot := governance.Snapshot()
	if !snapshot.AllowsSource(settings.SurfaceMCP, cfg.Source) {
		return fmt.Errorf("mcp: server %q blocked by strictPluginOnlyCustomization for mcp", cfg.ID)
	}
	if !snapshot.AllowsMCPServer(cfg.Source, cfg.ID) {
		return fmt.Errorf("mcp: server %q blocked by allowedMcpServers policy", cfg.ID)
	}
	return nil
}
