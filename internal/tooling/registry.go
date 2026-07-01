package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

// AssembleOptions controls per-call assembly behavior for the unified tool registry.
type AssembleOptions struct {
	ExtraTools               []Tool
	EnabledPacks             []string
	EnabledTools             []string
	Profile                  string
	TenantID                 string
	UserID                   string
	RuntimeCapabilities      []string
	ContextArtifactAvailable bool
	MCPHealthSnapshot        map[string]string
	IncludeDebug             bool
	IncludeDeferredCatalog   bool
	MetadataTransform        func(ToolMetadata) ToolMetadata
	Filter                   func(tool Tool, ctx ToolContext, meta ToolMetadata) bool
}

type registeredTool struct {
	tool Tool
}

// Registry manages unified tools and resolves builtin-vs-MCP priority.
type Registry struct {
	mu      sync.RWMutex
	records []registeredTool
}

// NewRegistry creates an empty tooling registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds or replaces a tool record.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("tool: cannot register nil tool")
	}
	meta := t.Metadata()
	if meta.Name == "" {
		return fmt.Errorf("tool: metadata name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, registeredTool{tool: t})
	return nil
}

// Get returns the highest-priority tool matching the provided name or alias.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		best Tool
		ok   bool
		rank = -1
	)

	for i := len(r.records) - 1; i >= 0; i-- {
		t := r.records[i].tool
		meta := t.Metadata()
		if !matchesName(meta, name) {
			continue
		}
		currentRank := selectionRank(meta)
		if !ok || currentRank > rank {
			best = t
			ok = true
			rank = currentRank
		}
	}

	return best, ok
}

// List returns the selected tool for each canonical name, applying origin priority.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	best := make(map[string]Tool)
	ranks := make(map[string]int)

	for i := len(r.records) - 1; i >= 0; i-- {
		t := r.records[i].tool
		meta := t.Metadata()
		name := meta.Name
		currentRank := selectionRank(meta)
		if prevRank, ok := ranks[name]; !ok || currentRank > prevRank {
			best[name] = t
			ranks[name] = currentRank
		}
	}

	names := make([]string, 0, len(best))
	for name := range best {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, best[name])
	}
	return out
}

// Unregister removes all records matching the provided name or alias.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.records[:0]
	for _, rec := range r.records {
		if matchesName(rec.tool.Metadata(), name) {
			continue
		}
		filtered = append(filtered, rec)
	}
	r.records = filtered
}

// AssembleTools returns the visible tools for a session/mode, preferring builtin over MCP on conflicts.
func (r *Registry) AssembleTools(session, mode string) []Tool {
	return r.AssembleToolsWithOptions(session, mode, AssembleOptions{})
}

// AssembleToolsWithOptions returns the visible tools for a session/mode after
// applying per-call transforms, filters, and extra dynamic tool sources.
func (r *Registry) AssembleToolsWithOptions(session, mode string, opts AssembleOptions) []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	best := make(map[string]Tool)
	ranks := make(map[string]int)

	candidates := make([]Tool, 0, len(r.records)+len(opts.ExtraTools))
	for _, rec := range r.records {
		candidates = append(candidates, rec.tool)
	}
	candidates = append(candidates, opts.ExtraTools...)

	for i := len(candidates) - 1; i >= 0; i-- {
		t := candidates[i]
		if t == nil {
			continue
		}
		meta := t.Metadata()
		if opts.MetadataTransform != nil {
			meta = opts.MetadataTransform(meta)
			t = metadataOverrideTool{base: t, meta: meta}
		}
		if meta.Name == "" {
			continue
		}
		ctx := ToolContext{SessionType: session, Mode: mode, Metadata: meta}
		if opts.Filter != nil && !opts.Filter(t, ctx, meta) {
			continue
		}
		if !t.IsEnabled(ctx) {
			continue
		}
		if !isVisibleForAssembleOptions(meta, opts) {
			continue
		}
		name := meta.Name
		currentRank := selectionRank(meta)
		if prevRank, ok := ranks[name]; !ok || currentRank > prevRank {
			best[name] = t
			ranks[name] = currentRank
		}
	}

	names := make([]string, 0, len(best))
	for name := range best {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, best[name])
	}
	return out
}

func isVisibleForAssembleOptions(meta ToolMetadata, opts AssembleOptions) bool {
	if !profileAllowsTool(meta, opts.Profile) {
		return false
	}
	if meta.AlwaysLoad {
		return true
	}
	layer := meta.Layer
	if layer == "" {
		if meta.HasMCPSource() {
			layer = ToolLayerMCP
		} else if meta.ShouldDefer {
			layer = ToolLayerDeferred
		} else {
			layer = ToolLayerCore
		}
	}
	switch layer {
	case ToolLayerCore:
		return true
	case ToolLayerDeferred:
		if opts.IncludeDeferredCatalog {
			return true
		}
		if toolEnabled(meta, opts.EnabledTools) {
			return true
		}
		if meta.DeferByDefault || meta.Pack != "" {
			return packEnabledForMeta(meta, opts.EnabledPacks)
		}
		return true
	case ToolLayerProfile:
		if toolEnabled(meta, opts.EnabledTools) {
			return true
		}
		if packEnabledForMeta(meta, opts.EnabledPacks) {
			return true
		}
		return opts.Profile != ""
	case ToolLayerMCP:
		if opts.IncludeDeferredCatalog {
			return true
		}
		if toolEnabled(meta, opts.EnabledTools) {
			return true
		}
		return packEnabledForMeta(meta, opts.EnabledPacks)
	case ToolLayerInternal:
		return false
	case ToolLayerDebug:
		return opts.IncludeDebug || opts.Profile == "debug"
	case ToolLayerMutation:
		if meta.Pack != "" {
			return packEnabledForMeta(meta, opts.EnabledPacks)
		}
		return packEnabled(string(ToolLayerMutation), opts.EnabledPacks)
	case ToolLayerConditional:
		if runtimeCapabilityAllowsTool(meta, opts.RuntimeCapabilities) {
			return true
		}
		if toolEnabled(meta, opts.EnabledTools) {
			return true
		}
		return packEnabledForMeta(meta, opts.EnabledPacks)
	default:
		return true
	}
}

func runtimeCapabilityAllowsTool(meta ToolMetadata, capabilities []string) bool {
	if len(capabilities) == 0 {
		return false
	}
	discovery := meta.EffectiveDiscovery()
	candidates := []string{
		meta.Name,
		meta.Pack,
		discovery.CapabilityKind,
		string(discovery.LoadingPolicy),
	}
	candidates = append(candidates, discovery.ToolPackIDs...)
	candidates = append(candidates, discovery.DiscoveryTags...)
	for _, capability := range capabilities {
		normalizedCapability := normalizeDiscoveryToken(capability)
		if normalizedCapability == "" {
			continue
		}
		for _, candidate := range candidates {
			if normalizeDiscoveryToken(candidate) == normalizedCapability {
				return true
			}
		}
	}
	return false
}

func toolEnabled(meta ToolMetadata, enabled []string) bool {
	for _, candidate := range enabled {
		if matchesName(meta, candidate) {
			return true
		}
	}
	return false
}

func profileAllowsTool(meta ToolMetadata, profile string) bool {
	profile = strings.TrimSpace(profile)
	if len(meta.Profiles) == 0 {
		return true
	}
	for _, candidate := range meta.Profiles {
		if strings.TrimSpace(candidate) == profile {
			return true
		}
	}
	return false
}

func packEnabled(pack string, enabled []string) bool {
	if pack == "" {
		return false
	}
	normalizedPack := normalizeDiscoveryToken(pack)
	for _, candidate := range enabled {
		if normalizeDiscoveryToken(candidate) == normalizedPack {
			return true
		}
	}
	return false
}

func packEnabledForMeta(meta ToolMetadata, enabled []string) bool {
	for _, pack := range meta.EffectiveDiscovery().ToolPackIDs {
		if packEnabled(pack, enabled) {
			return true
		}
	}
	return false
}

// AssembleToolPool returns Eino tools for the visible unified tools.
func (r *Registry) AssembleToolPool(session, mode string) []tool.BaseTool {
	return r.AssembleToolPoolWithOptions(session, mode, AssembleOptions{})
}

// AssembleToolPoolWithOptions returns Eino tools for the visible unified tools.
func (r *Registry) AssembleToolPoolWithOptions(session, mode string, opts AssembleOptions) []tool.BaseTool {
	return AssembleEinoToolPool(r.AssembleToolsWithOptions(session, mode, opts))
}

// CompileContextWithMetadata assembles prompt tools while applying turn-level
// metadata such as explicit user opt-outs.
func (r *Registry) CompileContextWithMetadata(session, mode string, metadata map[string]string) []Tool {
	return r.AssembleToolsWithOptions(session, mode, AssembleOptionsForTurnMetadata(metadata))
}

// AssembleToolPoolWithMetadata adapts the metadata-filtered visible tools into
// Eino base tools for the same turn.
func (r *Registry) AssembleToolPoolWithMetadata(session, mode string, metadata map[string]string) []tool.BaseTool {
	return AssembleEinoToolPool(r.CompileContextWithMetadata(session, mode, metadata))
}

func selectionRank(meta ToolMetadata) int {
	if meta.HasMCPSource() {
		return 0
	}
	return 1
}

func matchesName(meta ToolMetadata, name string) bool {
	if meta.Name == name {
		return true
	}
	for _, alias := range meta.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

type metadataOverrideTool struct {
	base Tool
	meta ToolMetadata
}

func (t metadataOverrideTool) Metadata() ToolMetadata { return t.meta }

func (t metadataOverrideTool) InputSchema() json.RawMessage { return t.base.InputSchema() }

func (t metadataOverrideTool) OutputSchema() json.RawMessage { return t.base.OutputSchema() }

func (t metadataOverrideTool) Description(input json.RawMessage, ctx DescribeContext) string {
	ctx.Metadata = t.meta
	if desc := t.base.Description(input, ctx); desc != "" {
		return desc
	}
	return t.meta.Description
}

func (t metadataOverrideTool) Prompt(ctx PromptContext) string {
	ctx.Metadata = t.meta
	if prompt := t.base.Prompt(ctx); prompt != "" {
		return prompt
	}
	return t.meta.Description
}

func (t metadataOverrideTool) IsEnabled(ctx ToolContext) bool {
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

func (t metadataOverrideTool) CheckPermissions(ctx context.Context, input json.RawMessage) PermissionDecision {
	return t.base.CheckPermissions(ctx, input)
}

func (t metadataOverrideTool) Execute(ctx context.Context, input json.RawMessage) (ToolResult, error) {
	return t.base.Execute(ctx, input)
}
