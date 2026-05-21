package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

var (
	defaultRegistryMu sync.RWMutex
	defaultRegistry   *Registry
)

// ServerConfig stores the registration-time configuration for an MCP server.
type ServerConfig struct {
	ID        string
	Name      string
	Transport string
	Command   []string
	Disabled  bool
	Source    string
}

type ServerState string

const (
	ServerStateDisconnected ServerState = "disconnected"
	ServerStateConnecting   ServerState = "connecting"
	ServerStateConnected    ServerState = "connected"
	ServerStateFailed       ServerState = "failed"
	ServerStateStale        ServerState = "stale"
)

type ServerStatus struct {
	State        ServerState `json:"state,omitempty"`
	LastError    string      `json:"lastError,omitempty"`
	RefreshToken string      `json:"refreshToken,omitempty"`
}

// Resource describes an MCP resource exposed by a server.
type Resource struct {
	ServerID    string          `json:"serverId,omitempty"`
	URI         string          `json:"uri"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

// ResourceContent is the readable payload for an MCP resource.
type ResourceContent struct {
	ServerID string `json:"serverId,omitempty"`
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     []byte `json:"blob,omitempty"`
	Digest   string `json:"digest,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
}

// Registry tracks MCP server configuration and dynamically connected tools.
type Registry struct {
	mu          sync.RWMutex
	governance  *settings.Governance
	serverCfgs  map[string]ServerConfig
	serverTools map[string][]tooling.Tool
	serverState map[string]bool
	statuses    map[string]ServerStatus
	resources   map[string][]Resource
	contents    map[string]map[string]ResourceContent
}

// NewRegistry creates an empty MCP server registry.
func NewRegistry() *Registry {
	r := &Registry{
		serverCfgs:  make(map[string]ServerConfig),
		serverTools: make(map[string][]tooling.Tool),
		serverState: make(map[string]bool),
		statuses:    make(map[string]ServerStatus),
		resources:   make(map[string][]Resource),
		contents:    make(map[string]map[string]ResourceContent),
	}
	setDefaultRegistry(r)
	return r
}

// DefaultRegistry returns the most recently created registry, if any.
func DefaultRegistry() *Registry {
	defaultRegistryMu.RLock()
	defer defaultRegistryMu.RUnlock()
	return defaultRegistry
}

func setDefaultRegistry(r *Registry) {
	defaultRegistryMu.Lock()
	defer defaultRegistryMu.Unlock()
	defaultRegistry = r
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
	if _, ok := r.statuses[cfg.ID]; !ok {
		r.statuses[cfg.ID] = ServerStatus{State: ServerStateDisconnected}
	}
	return nil
}

// SetServerDisabled marks a server as disabled or enabled without removing its config.
func (r *Registry) SetServerDisabled(serverID string, disabled bool) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if disabled {
		r.serverState[serverID] = true
		return
	}
	delete(r.serverState, serverID)
}

// IsServerDisabled reports whether a server is currently disabled.
func (r *Registry) IsServerDisabled(serverID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.serverState[strings.TrimSpace(serverID)]
}

func (r *Registry) SetServerStatus(serverID string, status ServerStatus) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return
	}
	if status.State == "" {
		status.State = ServerStateDisconnected
	}
	status.LastError = strings.TrimSpace(status.LastError)
	status.RefreshToken = strings.TrimSpace(status.RefreshToken)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses[serverID] = status
}

func (r *Registry) GetServerStatus(serverID string) (ServerStatus, bool) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return ServerStatus{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	status, ok := r.statuses[serverID]
	if !ok {
		return ServerStatus{}, false
	}
	return status, true
}

// GetServer returns a cloned server configuration by id.
func (r *Registry) GetServer(id string) (ServerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.serverCfgs[strings.TrimSpace(id)]
	if !ok {
		return ServerConfig{}, false
	}
	cfg.Disabled = r.serverState[strings.TrimSpace(id)]
	return cloneServerConfig(cfg), true
}

// ListServers returns all registered server configs sorted by id.
func (r *Registry) ListServers() []ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ServerConfig, 0, len(r.serverCfgs))
	for _, cfg := range r.serverCfgs {
		cfg.Disabled = r.serverState[cfg.ID]
		out = append(out, cloneServerConfig(cfg))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// OnServerResources replaces the resource metadata set for a server.
func (r *Registry) OnServerResources(serverID string, resources []Resource) error {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return fmt.Errorf("mcp: server id is required")
	}
	normalized := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		resource.URI = strings.TrimSpace(resource.URI)
		if resource.URI == "" {
			continue
		}
		resource.ServerID = serverID
		resource.Name = strings.TrimSpace(resource.Name)
		resource.Description = strings.TrimSpace(resource.Description)
		resource.MimeType = strings.TrimSpace(resource.MimeType)
		resource.Raw = append(json.RawMessage(nil), resource.Raw...)
		normalized = append(normalized, resource)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].URI < normalized[j].URI
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[serverID] = normalized
	return nil
}

// SetResourceContent registers local readable content for a resource.
func (r *Registry) SetResourceContent(serverID, uri string, content ResourceContent) error {
	serverID = strings.TrimSpace(serverID)
	uri = strings.TrimSpace(uri)
	if serverID == "" {
		return fmt.Errorf("mcp: server id is required")
	}
	if uri == "" {
		return fmt.Errorf("mcp: resource uri is required")
	}
	content.ServerID = serverID
	content.URI = uri
	content.MimeType = strings.TrimSpace(content.MimeType)
	content.Blob = append([]byte(nil), content.Blob...)
	content.Bytes = int64(len(content.Text) + len(content.Blob))
	if content.Digest == "" {
		content.Digest = resourceContentDigest(content)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.contents[serverID] == nil {
		r.contents[serverID] = make(map[string]ResourceContent)
	}
	r.contents[serverID][uri] = content
	return nil
}

// ListResources returns resource metadata for one server, or all enabled servers when serverID is empty.
func (r *Registry) ListResources(serverID string) []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()

	serverID = strings.TrimSpace(serverID)
	var out []Resource
	if serverID != "" {
		if r.serverState[serverID] {
			return nil
		}
		for _, resource := range r.resources[serverID] {
			out = append(out, cloneResource(resource))
		}
		return out
	}

	serverIDs := make([]string, 0, len(r.resources))
	for id := range r.resources {
		serverIDs = append(serverIDs, id)
	}
	sort.Strings(serverIDs)
	for _, id := range serverIDs {
		if r.serverState[id] {
			continue
		}
		for _, resource := range r.resources[id] {
			out = append(out, cloneResource(resource))
		}
	}
	return out
}

// ReadResource returns locally registered resource content.
func (r *Registry) ReadResource(_ context.Context, serverID, uri string) (ResourceContent, bool, error) {
	serverID = strings.TrimSpace(serverID)
	uri = strings.TrimSpace(uri)
	if serverID == "" {
		return ResourceContent{}, false, fmt.Errorf("mcp: server id is required")
	}
	if uri == "" {
		return ResourceContent{}, false, fmt.Errorf("mcp: resource uri is required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.serverState[serverID] {
		return ResourceContent{}, false, nil
	}
	content, ok := r.contents[serverID][uri]
	if !ok {
		return ResourceContent{}, false, nil
	}
	return cloneResourceContent(content), true, nil
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
	status := r.statuses[serverID]
	status.State = ServerStateConnected
	status.LastError = ""
	status.RefreshToken = r.dynamicToolRefreshTokenLocked()
	r.statuses[serverID] = status
	return nil
}

// OnServerDisconnected removes all dynamically connected tools for a server.
func (r *Registry) OnServerDisconnected(serverID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	serverID = strings.TrimSpace(serverID)
	delete(r.serverTools, serverID)
	delete(r.resources, serverID)
	delete(r.contents, serverID)
	status := r.statuses[serverID]
	status.State = ServerStateDisconnected
	status.RefreshToken = r.dynamicToolRefreshTokenLocked()
	r.statuses[serverID] = status
}

// UnregisterServer removes a server config and any connected tools.
func (r *Registry) UnregisterServer(serverID string) {
	serverID = strings.TrimSpace(serverID)

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.serverCfgs, serverID)
	delete(r.serverTools, serverID)
	delete(r.resources, serverID)
	delete(r.contents, serverID)
	delete(r.statuses, serverID)
}

// ListServerTools returns the connected tools for one server.
func (r *Registry) ListServerTools(serverID string) []tooling.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.serverState[strings.TrimSpace(serverID)] {
		return nil
	}
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
		if r.serverState[serverID] {
			continue
		}
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Metadata().Name < tools[j].Metadata().Name
		})
		out = append(out, tools...)
	}
	return out
}

// DynamicToolRefreshToken returns a stable token that changes when the
// registered server/tool surface changes. It is safe to use as an iteration-
// level refresh fingerprint for runtimekernel.
func (r *Registry) DynamicToolRefreshToken() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dynamicToolRefreshTokenLocked()
}

func (r *Registry) dynamicToolRefreshTokenLocked() string {
	h := sha256.New()

	serverIDs := make([]string, 0, len(r.serverCfgs))
	for serverID := range r.serverCfgs {
		serverIDs = append(serverIDs, serverID)
	}
	sort.Strings(serverIDs)
	for _, serverID := range serverIDs {
		cfg := r.serverCfgs[serverID]
		cfg.Disabled = r.serverState[serverID]
		writeRegistryFingerprintPart(h, "server", serverFingerprint(cfg))
		if status, ok := r.statuses[serverID]; ok {
			writeRegistryFingerprintPart(h, "status", string(status.State))
			writeRegistryFingerprintPart(h, "status-error", status.LastError)
		}
	}

	serverToolIDs := make([]string, 0, len(r.serverTools))
	for serverID := range r.serverTools {
		serverToolIDs = append(serverToolIDs, serverID)
	}
	sort.Strings(serverToolIDs)
	for _, serverID := range serverToolIDs {
		if r.serverState[serverID] {
			continue
		}
		tools := append([]tooling.Tool(nil), r.serverTools[serverID]...)
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Metadata().Name < tools[j].Metadata().Name
		})
		for _, tool := range tools {
			writeRegistryFingerprintPart(h, "tool", serverToolFingerprint(serverID, tool))
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

func serverFingerprint(cfg ServerConfig) string {
	data, err := json.Marshal(cfg)
	if err != nil {
		return cfg.ID
	}
	return string(data)
}

func serverToolFingerprint(serverID string, t tooling.Tool) string {
	meta := t.Metadata()
	meta.Aliases = append([]string(nil), meta.Aliases...)
	sort.Strings(meta.Aliases)

	payload := struct {
		ServerID        string               `json:"serverId"`
		Metadata        tooling.ToolMetadata `json:"metadata"`
		InputSchema     string               `json:"inputSchema,omitempty"`
		OutputSchema    string               `json:"outputSchema,omitempty"`
		Description     string               `json:"description,omitempty"`
		Prompt          string               `json:"prompt,omitempty"`
		Enabled         bool                 `json:"enabled"`
		ReadOnly        bool                 `json:"readOnly"`
		Destructive     bool                 `json:"destructive"`
		ConcurrencySafe bool                 `json:"concurrencySafe"`
	}{
		ServerID:        serverID,
		Metadata:        meta,
		InputSchema:     strings.TrimSpace(string(t.InputSchema())),
		OutputSchema:    strings.TrimSpace(string(t.OutputSchema())),
		Description:     strings.TrimSpace(t.Description(nil, tooling.DescribeContext{Metadata: meta})),
		Prompt:          strings.TrimSpace(t.Prompt(tooling.PromptContext{Metadata: meta})),
		Enabled:         t.IsEnabled(tooling.ToolContext{Metadata: meta}),
		ReadOnly:        t.IsReadOnly(nil),
		Destructive:     t.IsDestructive(nil),
		ConcurrencySafe: t.IsConcurrencySafe(nil),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return meta.Name
	}
	return string(data)
}

func writeRegistryFingerprintPart(h interface{ Write([]byte) (int, error) }, kind, value string) {
	_, _ = h.Write([]byte(kind))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
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

func cloneResource(resource Resource) Resource {
	resource.Raw = append(json.RawMessage(nil), resource.Raw...)
	return resource
}

func cloneResourceContent(content ResourceContent) ResourceContent {
	content.Blob = append([]byte(nil), content.Blob...)
	return content
}

func resourceContentDigest(content ResourceContent) string {
	h := sha256.New()
	_, _ = h.Write([]byte(content.ServerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(content.URI))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(content.Text))
	_, _ = h.Write(content.Blob)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
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
