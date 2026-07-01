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
	"time"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

var (
	defaultRegistryMu sync.RWMutex
	defaultRegistry   *Registry
)

// ServerConfig stores the registration-time configuration for an MCP server.
type ServerConfig struct {
	ID                   string                    `json:"id,omitempty"`
	Name                 string                    `json:"name,omitempty"`
	Transport            string                    `json:"transport,omitempty"`
	Command              []string                  `json:"command,omitempty"`
	Disabled             bool                      `json:"disabled,omitempty"`
	Source               string                    `json:"source,omitempty"`
	TenantScope          TenantScope               `json:"tenantScope,omitempty"`
	UserScope            UserScope                 `json:"userScope,omitempty"`
	Profiles             []string                  `json:"profiles,omitempty"`
	CapabilityDomain     string                    `json:"capabilityDomain,omitempty"`
	ResourceTypes        []string                  `json:"resourceTypes,omitempty"`
	OperationKinds       []string                  `json:"operationKinds,omitempty"`
	DefaultLoadingPolicy tooling.ToolLoadingPolicy `json:"defaultLoadingPolicy,omitempty"`
	RiskLevel            tooling.ToolRiskLevel     `json:"riskLevel,omitempty"`
	HealthCheckType      string                    `json:"healthCheckType,omitempty"`
	OwnerSource          string                    `json:"ownerSource,omitempty"`
	ToolPack             string                    `json:"toolPack,omitempty"`
	RequiresHealthyMCP   bool                      `json:"requiresHealthyMcp,omitempty"`
	PermissionScope      string                    `json:"permissionScope,omitempty"`
	PromptBudgetClass    string                    `json:"promptBudgetClass,omitempty"`
	SchemaBudgetClass    string                    `json:"schemaBudgetClass,omitempty"`
	DiscoveryTags        []string                  `json:"discoveryTags,omitempty"`
}

type TenantScope struct {
	TenantIDs []string `json:"tenantIds,omitempty"`
}

type UserScope struct {
	UserIDs []string `json:"userIds,omitempty"`
}

type DynamicToolOptions struct {
	TenantID string
	UserID   string
	Profile  string
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
	mu           sync.RWMutex
	governance   *settings.Governance
	serverCfgs   map[string]ServerConfig
	serverTools  map[string][]tooling.Tool
	serverState  map[string]bool
	statuses     map[string]ServerStatus
	health       *HealthRegistry
	resources    map[string][]Resource
	contents     map[string]map[string]ResourceContent
	instructions map[string]ServerInstruction
}

// NewRegistry creates an empty MCP server registry.
func NewRegistry() *Registry {
	r := &Registry{
		serverCfgs:   make(map[string]ServerConfig),
		serverTools:  make(map[string][]tooling.Tool),
		serverState:  make(map[string]bool),
		statuses:     make(map[string]ServerStatus),
		health:       NewHealthRegistry(DefaultHealthTTL),
		resources:    make(map[string][]Resource),
		contents:     make(map[string]map[string]ResourceContent),
		instructions: make(map[string]ServerInstruction),
	}
	setDefaultRegistry(r)
	return r
}

func (r *Registry) SetServerInstructions(serverID, text string) {
	serverID = strings.TrimSpace(serverID)
	text = strings.TrimSpace(text)
	if serverID == "" || text == "" {
		return
	}
	redacted := redactInstructionText(text)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instructions[serverID] = ServerInstruction{
		ServerID:  serverID,
		Text:      redacted,
		Hash:      serverInstructionHash(serverID, redacted),
		UpdatedAt: time.Now(),
	}
}

func (r *Registry) ListServerInstructions() []ServerInstruction {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerInstruction, 0, len(r.instructions))
	for _, instruction := range r.instructions {
		out = append(out, instruction)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerID < out[j].ServerID })
	return out
}

func (r *Registry) ServerInstructionDelta(state *MCPInstructionSessionState) []MCPInstructionDelta {
	r.mu.RLock()
	instructions := make([]ServerInstruction, 0, len(r.instructions))
	for _, instruction := range r.instructions {
		instructions = append(instructions, instruction)
	}
	disabled := make(map[string]bool, len(r.serverState))
	for serverID, value := range r.serverState {
		disabled[serverID] = value
	}
	r.mu.RUnlock()
	sort.Slice(instructions, func(i, j int) bool { return instructions[i].ServerID < instructions[j].ServerID })
	return buildInstructionDelta(instructions, disabled, state)
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
	cfg.CapabilityDomain = normalizeManifestToken(cfg.CapabilityDomain)
	cfg.ResourceTypes = normalizeManifestList(cfg.ResourceTypes)
	cfg.OperationKinds = normalizeManifestList(cfg.OperationKinds)
	cfg.DefaultLoadingPolicy = normalizeMCPToolLoadingPolicy(cfg.DefaultLoadingPolicy)
	cfg.RiskLevel = normalizeMCPRisk(cfg.RiskLevel)
	cfg.HealthCheckType = normalizeManifestToken(cfg.HealthCheckType)
	cfg.OwnerSource = strings.TrimSpace(cfg.OwnerSource)
	cfg.ToolPack = normalizeManifestToken(cfg.ToolPack)
	cfg.PermissionScope = normalizeManifestToken(cfg.PermissionScope)
	cfg.PromptBudgetClass = normalizeManifestToken(cfg.PromptBudgetClass)
	cfg.SchemaBudgetClass = normalizeManifestToken(cfg.SchemaBudgetClass)
	cfg.DiscoveryTags = normalizeManifestList(cfg.DiscoveryTags)
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
		if r.health != nil {
			r.health.Set(HealthSnapshot{ServerID: serverID, Status: HealthDisabled, LastCheckedAt: time.Now(), TTLSeconds: int(DefaultHealthTTL.Seconds())})
		}
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

func (r *Registry) SetServerHealthSnapshot(snapshot HealthSnapshot) {
	if r == nil || r.health == nil {
		return
	}
	r.health.Set(snapshot)
}

func (r *Registry) GetServerHealthSnapshot(serverID string) (HealthSnapshot, bool) {
	if r == nil || r.health == nil {
		return HealthSnapshot{}, false
	}
	return r.health.Snapshot(serverID)
}

func (r *Registry) ListServerHealthSnapshots() []HealthSnapshot {
	if r == nil || r.health == nil {
		return nil
	}
	return r.health.List()
}

// ToolHealthSnapshots exposes MCP health to the unified tool catalog provider
// without forcing callers to know about MCP internals.
func (r *Registry) ToolHealthSnapshots() map[string]string {
	snapshots := r.ListServerHealthSnapshots()
	if len(snapshots) == 0 {
		return nil
	}
	out := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		serverID := strings.TrimSpace(snapshot.ServerID)
		status := strings.ToLower(strings.TrimSpace(string(snapshot.Status)))
		if serverID != "" && status != "" {
			out[serverID] = status
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *Registry) RefreshServerHealth(ctx context.Context, serverID string, force bool, probe HealthProbe) HealthSnapshot {
	if r == nil || r.health == nil {
		return HealthSnapshot{}
	}
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return HealthSnapshot{}
	}
	r.mu.RLock()
	cfg, ok := r.serverCfgs[serverID]
	disabled := r.serverState[serverID]
	r.mu.RUnlock()
	if !ok {
		cfg = ServerConfig{ID: serverID, Name: serverID}
	}
	return r.health.Refresh(ctx, cfg, disabled, force, probe)
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

	r.mu.RLock()
	cfg, ok := r.serverCfgs[serverID]
	r.mu.RUnlock()
	if !ok {
		cfg = ServerConfig{ID: serverID, Name: serverID}
	}

	normalized := make([]tooling.Tool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		normalized = append(normalized, normalizeServerTool(cfg, t))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.serverTools[serverID] = normalized
	status := r.statuses[serverID]
	status.State = ServerStateConnected
	status.LastError = ""
	status.RefreshToken = r.dynamicToolRefreshTokenLocked()
	r.statuses[serverID] = status
	if r.health != nil {
		r.health.Set(HealthSnapshot{
			ServerID:      serverID,
			Status:        HealthHealthy,
			LastCheckedAt: time.Now(),
			LastSuccessAt: time.Now(),
			TTLSeconds:    int(DefaultHealthTTL.Seconds()),
			Capabilities:  []string{"tools"},
		})
	}
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
	if r.health != nil {
		r.health.Set(HealthSnapshot{ServerID: serverID, Status: HealthUnknown, LastCheckedAt: time.Now(), TTLSeconds: int(DefaultHealthTTL.Seconds())})
	}
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
	if r.health != nil {
		r.health.Set(HealthSnapshot{ServerID: serverID, Status: HealthUnknown, LastCheckedAt: time.Now(), TTLSeconds: int(DefaultHealthTTL.Seconds())})
	}
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
	return r.DynamicToolsWithOptions(DynamicToolOptions{})
}

// DynamicToolsWithOptions returns connected tools visible to one scoped request.
func (r *Registry) DynamicToolsWithOptions(opts DynamicToolOptions) []tooling.Tool {
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
		if !serverConfigVisibleToOptions(r.serverCfgs[serverID], opts) {
			continue
		}
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Metadata().Name < tools[j].Metadata().Name
		})
		out = append(out, tools...)
	}
	return out
}

func (r *Registry) DynamicToolsForScope(scope tooling.DynamicToolScope) []tooling.Tool {
	return r.DynamicToolsWithOptions(DynamicToolOptions{
		TenantID: scope.TenantID,
		UserID:   scope.UserID,
		Profile:  scope.Profile,
	})
}

func serverConfigVisibleToOptions(cfg ServerConfig, opts DynamicToolOptions) bool {
	opts.TenantID = strings.TrimSpace(opts.TenantID)
	opts.UserID = strings.TrimSpace(opts.UserID)
	opts.Profile = strings.TrimSpace(opts.Profile)
	hasRequestScope := opts.TenantID != "" || opts.UserID != "" || opts.Profile != ""
	hasConfigScope := len(cfg.TenantScope.TenantIDs) > 0 || len(cfg.UserScope.UserIDs) > 0 || len(cfg.Profiles) > 0
	if !hasRequestScope {
		return true
	}
	if !hasConfigScope {
		return false
	}
	if len(cfg.TenantScope.TenantIDs) > 0 && !containsTrimmed(cfg.TenantScope.TenantIDs, opts.TenantID) {
		return false
	}
	if len(cfg.UserScope.UserIDs) > 0 && !containsTrimmed(cfg.UserScope.UserIDs, opts.UserID) {
		return false
	}
	if len(cfg.Profiles) > 0 && !containsTrimmed(cfg.Profiles, opts.Profile) {
		return false
	}
	return true
}

func containsTrimmed(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
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

func normalizeServerTool(server ServerConfig, t tooling.Tool) tooling.Tool {
	server.ID = strings.TrimSpace(server.ID)
	if server.ID == "" {
		server.ID = strings.TrimSpace(server.Name)
	}
	if server.Name == "" {
		server.Name = server.ID
	}
	meta := t.Metadata()
	if meta.Name == "" {
		meta.Name = strings.TrimSpace(meta.MCPInfo.ToolName)
	}
	if meta.Description == "" {
		meta.Description = strings.TrimSpace(t.Description(nil, tooling.DescribeContext{Metadata: meta}))
	}
	meta.Origin = tooling.ToolOriginMCP
	meta.IsMCP = true
	meta.Layer = tooling.ToolLayerDeferred
	meta.DeferByDefault = true
	if meta.Pack == "" {
		meta.Pack = firstNonEmptyString(server.ToolPack, dynamicMCPToolPack(server.ID))
	}
	if meta.MCPInfo.ServerID == "" {
		meta.MCPInfo.ServerID = server.ID
	}
	if meta.MCPInfo.ServerName == "" {
		meta.MCPInfo.ServerName = firstNonEmptyString(server.Name, server.ID)
	}
	if meta.MCPInfo.ToolName == "" {
		meta.MCPInfo.ToolName = meta.Name
	}
	readOnly := t.IsReadOnly(nil)
	destructive := t.IsDestructive(nil)
	if meta.RiskLevel == "" && readOnly && !destructive {
		meta.RiskLevel = tooling.ToolRiskLow
	}
	if destructive {
		meta.Mutating = true
	}
	meta = ApplyServerManifestToToolMetadata(server, meta, readOnly, destructive)
	if meta.AlwaysLoad && !mcpAlwaysLoadAllowed(meta, readOnly, destructive) {
		meta.AlwaysLoad = false
	}
	meta.Triggers = appendUniqueMCPMetadata(meta.Triggers, "MCP tool", "dynamic tool", meta.MCPInfo.ServerName, meta.MCPInfo.ToolName)
	meta.Discovery = normalizeMCPDiscovery(server, meta.Discovery, meta, readOnly, destructive)
	return metadataOverrideTool{base: t, meta: meta}
}

func mcpAlwaysLoadAllowed(meta tooling.ToolMetadata, readOnly, destructive bool) bool {
	return readOnly && !destructive && !meta.Mutating && !meta.RequiresApproval && meta.RiskLevel.Normalize() == tooling.ToolRiskLow
}

func ApplyServerManifestToToolMetadata(server ServerConfig, meta tooling.ToolMetadata, readOnly, destructive bool) tooling.ToolMetadata {
	server.ID = strings.TrimSpace(server.ID)
	server.Name = strings.TrimSpace(server.Name)
	if meta.MCPInfo.ServerID == "" {
		meta.MCPInfo.ServerID = server.ID
	}
	if meta.MCPInfo.ServerName == "" {
		meta.MCPInfo.ServerName = firstNonEmptyString(server.Name, server.ID)
	}
	if meta.MCPInfo.ToolName == "" {
		meta.MCPInfo.ToolName = meta.Name
	}
	if meta.Pack == "" && strings.TrimSpace(server.ToolPack) != "" {
		meta.Pack = normalizeManifestToken(server.ToolPack)
	}
	if meta.RiskLevel == "" && server.RiskLevel != "" {
		meta.RiskLevel = normalizeMCPRisk(server.RiskLevel)
	}
	meta.Discovery = normalizeMCPDiscovery(server, meta.Discovery, meta, readOnly, destructive)
	return meta
}

func normalizeMCPDiscovery(server ServerConfig, discovery tooling.ToolDiscoveryMetadata, meta tooling.ToolMetadata, readOnly, destructive bool) tooling.ToolDiscoveryMetadata {
	if discovery.DiscoveryGroup == "" {
		discovery.DiscoveryGroup = firstNonEmptyString(normalizeManifestToken(server.CapabilityDomain), "mcp")
	}
	discovery.DiscoveryTags = appendUniqueMCPMetadata(discovery.DiscoveryTags, "mcp", "dynamic", normalizeManifestToken(server.CapabilityDomain), meta.MCPInfo.ServerName, meta.MCPInfo.ToolName)
	discovery.DiscoveryTags = appendUniqueMCPMetadata(discovery.DiscoveryTags, server.DiscoveryTags...)
	if len(discovery.ResourceTypes) == 0 {
		if len(server.ResourceTypes) > 0 {
			discovery.ResourceTypes = append([]string(nil), server.ResourceTypes...)
		} else {
			discovery.ResourceTypes = []string{"mcp_tool"}
		}
	}
	if len(discovery.OperationKinds) == 0 {
		if len(server.OperationKinds) > 0 {
			discovery.OperationKinds = append([]string(nil), server.OperationKinds...)
		} else {
			switch {
			case destructive || meta.Mutating:
				discovery.OperationKinds = []string{"write"}
			case readOnly:
				discovery.OperationKinds = []string{"read"}
			default:
				discovery.OperationKinds = []string{"execute"}
			}
		}
	}
	if discovery.LoadingPolicy == "" && server.DefaultLoadingPolicy != "" {
		discovery.LoadingPolicy = normalizeMCPToolLoadingPolicy(server.DefaultLoadingPolicy)
	}
	if discovery.MCPServerID == "" {
		discovery.MCPServerID = server.ID
	}
	if server.RequiresHealthyMCP {
		discovery.RequiresHealthyMCP = true
	}
	if discovery.PermissionScope == "" {
		discovery.PermissionScope = normalizeManifestToken(server.PermissionScope)
	}
	if discovery.PromptBudgetClass == "" {
		discovery.PromptBudgetClass = normalizeManifestToken(server.PromptBudgetClass)
	}
	if discovery.SchemaBudgetClass == "" {
		discovery.SchemaBudgetClass = normalizeManifestToken(server.SchemaBudgetClass)
	}
	discovery.RequiresSelect = true
	return discovery
}

func dynamicMCPToolPack(serverID string) string {
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

func appendUniqueMCPMetadata(values []string, extras ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(extras))
	out := make([]string, 0, len(values)+len(extras))
	for _, value := range append(append([]string(nil), values...), extras...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeManifestList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeManifestToken(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func normalizeManifestToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeMCPToolLoadingPolicy(policy tooling.ToolLoadingPolicy) tooling.ToolLoadingPolicy {
	switch tooling.ToolLoadingPolicy(normalizeManifestToken(string(policy))) {
	case tooling.ToolLoadingPolicyCore:
		return tooling.ToolLoadingPolicyCore
	case tooling.ToolLoadingPolicyDeferred:
		return tooling.ToolLoadingPolicyDeferred
	case tooling.ToolLoadingPolicyProfile:
		return tooling.ToolLoadingPolicyProfile
	case tooling.ToolLoadingPolicyMCP:
		return tooling.ToolLoadingPolicyMCP
	case tooling.ToolLoadingPolicyInternal:
		return tooling.ToolLoadingPolicyInternal
	case tooling.ToolLoadingPolicyConditional:
		return tooling.ToolLoadingPolicyConditional
	default:
		return ""
	}
}

func normalizeMCPRisk(risk tooling.ToolRiskLevel) tooling.ToolRiskLevel {
	switch tooling.ToolRiskLevel(normalizeManifestToken(string(risk))) {
	case tooling.ToolRiskLow:
		return tooling.ToolRiskLow
	case tooling.ToolRiskMedium:
		return tooling.ToolRiskMedium
	case tooling.ToolRiskHigh:
		return tooling.ToolRiskHigh
	case tooling.ToolRiskCritical:
		return tooling.ToolRiskCritical
	default:
		return ""
	}
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Command = append([]string(nil), cfg.Command...)
	cfg.TenantScope.TenantIDs = append([]string(nil), cfg.TenantScope.TenantIDs...)
	cfg.UserScope.UserIDs = append([]string(nil), cfg.UserScope.UserIDs...)
	cfg.Profiles = append([]string(nil), cfg.Profiles...)
	cfg.ResourceTypes = append([]string(nil), cfg.ResourceTypes...)
	cfg.OperationKinds = append([]string(nil), cfg.OperationKinds...)
	cfg.DiscoveryTags = append([]string(nil), cfg.DiscoveryTags...)
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
