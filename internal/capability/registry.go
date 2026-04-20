package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Type aliases to avoid circular imports with runtimekernel.
// ---------------------------------------------------------------------------

// SessionType mirrors runtimekernel.SessionType (type alias, not new type).
type SessionType = string

// Mode mirrors runtimekernel.Mode (type alias, not new type).
type Mode = string

// ---------------------------------------------------------------------------
// Kind identifies the six canonical capability categories in V2.
// ---------------------------------------------------------------------------

// Kind identifies the six canonical capability categories.
type Kind string

const (
	KindTool      Kind = "tool"
	KindSkill     Kind = "skill"
	KindMCPTool   Kind = "mcp_tool"
	KindUISurface Kind = "ui_surface"
	KindModeRule  Kind = "mode_rule"
	KindWorkspace Kind = "workspace"
)

var allKinds = []Kind{
	KindTool,
	KindSkill,
	KindMCPTool,
	KindUISurface,
	KindModeRule,
	KindWorkspace,
}

// AllKinds returns the six canonical capability kinds.
func AllKinds() []Kind {
	out := make([]Kind, len(allKinds))
	copy(out, allKinds)
	return out
}

// IsValid reports whether the value is one of the six canonical kinds.
func (k Kind) IsValid() bool {
	switch k {
	case KindTool, KindSkill, KindMCPTool, KindUISurface, KindModeRule, KindWorkspace:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Visibility defines where a capability is visible.
// ---------------------------------------------------------------------------

// Visibility defines the session types and modes where a capability is visible.
type Visibility struct {
	SessionTypes []SessionType `json:"sessionTypes"`
	Modes        []Mode        `json:"modes"`
}

// ---------------------------------------------------------------------------
// Entry is an atomic capability item in the registry.
// ---------------------------------------------------------------------------

// Entry represents a single registered capability in the registry.
type Entry struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        Kind       `json:"kind"`
	Description string     `json:"description"`
	Visibility  Visibility `json:"visibility"`

	// Tool is non-nil only for KindTool entries. It holds the UnifiedTool implementation.
	Tool ToolRuntime `json:"-"`
}

// Validate checks that the entry has required fields and valid kind.
func (e Entry) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("entry id is required")
	}
	if e.Name == "" {
		return fmt.Errorf("entry name is required")
	}
	if !e.Kind.IsValid() {
		return fmt.Errorf("invalid kind %q", e.Kind)
	}
	// tool:* kind requires a non-nil Tool implementing UnifiedTool contract
	if e.Kind == KindTool && e.Tool == nil {
		return fmt.Errorf("tool kind entry %q requires a non-nil ToolRuntime", e.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// UnifiedTool contract interface (ToolRuntime)
// ---------------------------------------------------------------------------

// ToolRuntime is the UnifiedTool contract interface that all tool:* capabilities
// must implement. It defines the standard methods for tool discovery, permission
// checking, safety classification, and display output.
type ToolRuntime interface {
	// Description returns a human-readable description of the tool.
	Description() string

	// CheckPermissions verifies that the current context has permission to invoke this tool.
	CheckPermissions(ctx context.Context) error

	// IsReadOnly reports whether the tool only reads state without mutation.
	IsReadOnly() bool

	// IsDestructive reports whether the tool can cause irreversible changes.
	IsDestructive() bool

	// IsConcurrencySafe reports whether the tool can be safely invoked concurrently.
	IsConcurrencySafe() bool

	// Display returns the structured UI output payload for this tool's result.
	Display() ToolDisplayPayload

	// InputSchema returns the JSON Schema describing the tool's input parameters.
	InputSchema() json.RawMessage

	// Execute runs the tool with the given arguments and returns a result.
	Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

// ---------------------------------------------------------------------------
// ToolDisplayPayload — structured UI output for tool results.
// ---------------------------------------------------------------------------

// ToolDisplayPayload is the structured UI output data for tool execution results.
type ToolDisplayPayload struct {
	Type    string          `json:"type"`
	Title   string          `json:"title,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	CardRef string          `json:"cardRef,omitempty"`
}

// ---------------------------------------------------------------------------
// ToolResult — the result of a tool execution.
// ---------------------------------------------------------------------------

// ToolResult represents the outcome of a tool execution.
type ToolResult struct {
	ToolCallID string              `json:"toolCallId"`
	Content    string              `json:"content"`
	Display    *ToolDisplayPayload `json:"display,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// EinoToolAdapter — adapts UnifiedTool to Eino's tool.InvokableTool interface.
// This allows tools to be directly passed to adk.ChatModelAgent's ToolsConfig.
// ---------------------------------------------------------------------------

// EinoToolAdapter adapts a UnifiedTool (ToolRuntime) to the Eino framework's
// tool.InvokableTool interface. It implements both tool.BaseTool (Info) and
// tool.InvokableTool (InvokableRun), making it usable with adk.ChatModelAgent.
type EinoToolAdapter struct {
	tool     ToolRuntime
	entry    Entry
	registry *Registry
	toolInfo *schema.ToolInfo // cached
}

// NewEinoToolAdapter creates a new adapter for the given tool and entry.
func NewEinoToolAdapter(t ToolRuntime, entry Entry, registry *Registry) *EinoToolAdapter {
	a := &EinoToolAdapter{
		tool:     t,
		entry:    entry,
		registry: registry,
	}
	a.toolInfo = a.buildToolInfo()
	return a
}

// Info implements tool.BaseTool — returns the tool's metadata for ChatModel discovery.
func (a *EinoToolAdapter) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return a.toolInfo, nil
}

// InvokableRun implements tool.InvokableTool — executes the tool with JSON args
// and returns a string result. This is called by Eino's ToolsNode during the
// ChatModelAgent ReAct loop.
func (a *EinoToolAdapter) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	result, err := a.tool.Execute(ctx, json.RawMessage(args))
	if err != nil {
		return "", fmt.Errorf("tool %q execution failed: %w", a.entry.Name, err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("tool %q returned error: %s", a.entry.Name, result.Error)
	}
	return result.Content, nil
}

// ToEinoTool returns the cached *schema.ToolInfo (for backward compatibility).
func (a *EinoToolAdapter) ToEinoTool() *schema.ToolInfo {
	return a.toolInfo
}

// Execute invokes the underlying tool and returns a ToolResult (internal use).
func (a *EinoToolAdapter) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
	return a.tool.Execute(ctx, args)
}

// buildToolInfo constructs the *schema.ToolInfo from the ToolRuntime.
func (a *EinoToolAdapter) buildToolInfo() *schema.ToolInfo {
	inputSchema := a.tool.InputSchema()

	var paramsOneOf *schema.ParamsOneOf
	if len(inputSchema) > 0 {
		var js jsonschema.Schema
		if err := json.Unmarshal(inputSchema, &js); err == nil {
			paramsOneOf = schema.NewParamsOneOfByJSONSchema(&js)
		}
	}

	return &schema.ToolInfo{
		Name:        a.entry.Name,
		Desc:        a.tool.Description(),
		ParamsOneOf: paramsOneOf,
	}
}

// Compile-time checks that EinoToolAdapter satisfies Eino tool interfaces.
var (
	_ tool.BaseTool      = (*EinoToolAdapter)(nil)
	_ tool.InvokableTool = (*EinoToolAdapter)(nil)
)

// ---------------------------------------------------------------------------
// Registry — the central capability registry.
// ---------------------------------------------------------------------------

// Registry is the central capability registry that manages all six kinds of capabilities.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry // keyed by Entry.ID
	tools   *tooling.Registry
}

// NewRegistry creates a new empty capability registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]Entry),
		tools:   tooling.NewRegistry(),
	}
}

// Register adds a capability entry to the registry.
// For tool:* kind, the entry must have a non-nil Tool implementing the UnifiedTool contract.
func (r *Registry) Register(entry Entry) error {
	if err := entry.Validate(); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[entry.ID] = entry
	r.rebuildToolRegistryLocked()
	return nil
}

// Unregister removes a capability entry from the registry by ID.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, id)
	r.rebuildToolRegistryLocked()
}

// RegisterBatch registers multiple entries atomically. If any entry fails validation,
// none are registered.
func (r *Registry) RegisterBatch(entries []Entry) error {
	for i, e := range entries {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("register batch entry[%d]: %w", i, err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range entries {
		r.entries[e.ID] = e
	}
	r.rebuildToolRegistryLocked()
	return nil
}

// Get returns a capability entry by ID, or false if not found.
func (r *Registry) Get(id string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	return e, ok
}

// VisibleCapabilities returns entries visible for the given session type and mode.
func (r *Registry) VisibleCapabilities(session SessionType, mode Mode) []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Entry
	for _, e := range r.entries {
		if isVisible(e, session, mode) {
			result = append(result, e)
		}
	}
	return result
}

// SkillPromptAssets returns compatibility prompt fragments derived from visible skill entries.
func (r *Registry) SkillPromptAssets(session SessionType, mode Mode) []string {
	return r.promptAssetsForKind(session, mode, KindSkill, "Skill available")
}

// MCPPromptAssets returns compatibility prompt fragments derived from visible MCP entries.
func (r *Registry) MCPPromptAssets(session SessionType, mode Mode) []string {
	return r.promptAssetsForKind(session, mode, KindMCPTool, "MCP tool available")
}

// AssembleTools returns the unified tool set visible for the given session and mode.
func (r *Registry) AssembleTools(session SessionType, mode Mode) []tooling.Tool {
	return r.tools.AssembleTools(string(session), string(mode))
}

// AssembleToolPool merges built-in tools with MCP tools for the given session/mode.
// Built-in tools take priority on name conflicts (per claude code/tools.ts pattern).
// Returns []tool.BaseTool for direct use with adk.ChatModelAgent's ToolsConfig.
func (r *Registry) AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool {
	return r.tools.AssembleToolPool(string(session), string(mode))
}

// AssembleToolInfoPool is a convenience method that returns []*schema.ToolInfo
// for use with model.ChatModel.BindTools (metadata only, no execution).
func (r *Registry) AssembleToolInfoPool(session SessionType, mode Mode) []*schema.ToolInfo {
	tools := r.AssembleToolPool(session, mode)
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err == nil && info != nil {
			infos = append(infos, info)
		}
	}
	return infos
}

func (r *Registry) rebuildToolRegistryLocked() {
	r.tools = tooling.NewRegistry()
	for _, entry := range r.entries {
		r.syncToolLocked(entry)
	}
}

func (r *Registry) syncToolLocked(entry Entry) {
	if entry.Tool == nil || (entry.Kind != KindTool && entry.Kind != KindMCPTool) {
		return
	}

	meta := tooling.ToolMetadata{
		Name:        entry.Name,
		Description: entry.Description,
		Origin:      originForEntry(entry),
		IsMCP:       entry.Kind == KindMCPTool,
	}

	_ = r.tools.Register(tooling.NewLegacyToolAdapter(legacyToolRuntimeBridge{
		runtime:    entry.Tool,
		visibility: entry.Visibility,
	}, meta))
}

type legacyToolRuntimeBridge struct {
	runtime    ToolRuntime
	visibility Visibility
}

func (b legacyToolRuntimeBridge) Description() string { return b.runtime.Description() }

func (b legacyToolRuntimeBridge) InputSchema() json.RawMessage { return b.runtime.InputSchema() }

func (b legacyToolRuntimeBridge) Execute(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
	res, err := b.runtime.Execute(ctx, input)
	return tooling.ToolResult{
		ToolCallID: res.ToolCallID,
		Content:    res.Content,
		Display:    convertDisplayPayload(res.Display),
		Error:      res.Error,
	}, err
}

func (b legacyToolRuntimeBridge) IsEnabled(ctx tooling.ToolContext) bool {
	if len(b.visibility.SessionTypes) == 0 && len(b.visibility.Modes) == 0 {
		return true
	}

	sessionOK := len(b.visibility.SessionTypes) == 0
	for _, st := range b.visibility.SessionTypes {
		if st == SessionType(ctx.SessionType) {
			sessionOK = true
			break
		}
	}

	modeOK := len(b.visibility.Modes) == 0
	for _, m := range b.visibility.Modes {
		if m == Mode(ctx.Mode) {
			modeOK = true
			break
		}
	}

	return sessionOK && modeOK
}

func (b legacyToolRuntimeBridge) CheckPermissions(ctx context.Context) error {
	return b.runtime.CheckPermissions(ctx)
}

func (b legacyToolRuntimeBridge) IsReadOnly() bool { return b.runtime.IsReadOnly() }

func (b legacyToolRuntimeBridge) IsDestructive() bool { return b.runtime.IsDestructive() }

func (b legacyToolRuntimeBridge) IsConcurrencySafe() bool { return b.runtime.IsConcurrencySafe() }

func (b legacyToolRuntimeBridge) Display() tooling.ToolDisplayPayload {
	display := b.runtime.Display()
	return tooling.ToolDisplayPayload{
		Type:    display.Type,
		Title:   display.Title,
		Data:    display.Data,
		CardRef: display.CardRef,
	}
}

func convertDisplayPayload(display *ToolDisplayPayload) *tooling.ToolDisplayPayload {
	if display == nil {
		return nil
	}
	return &tooling.ToolDisplayPayload{
		Type:    display.Type,
		Title:   display.Title,
		Data:    display.Data,
		CardRef: display.CardRef,
	}
}

func originForEntry(entry Entry) tooling.ToolOrigin {
	switch entry.Kind {
	case KindTool:
		return tooling.ToolOriginBuiltin
	case KindMCPTool:
		return tooling.ToolOriginMCP
	default:
		return tooling.ToolOriginMeta
	}
}

func (r *Registry) promptAssetsForKind(session SessionType, mode Mode, kind Kind, prefix string) []string {
	visible := r.VisibleCapabilities(session, mode)
	filtered := make([]Entry, 0, len(visible))
	for _, entry := range visible {
		if entry.Kind == kind {
			filtered = append(filtered, entry)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Name == filtered[j].Name {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Name < filtered[j].Name
	})

	assets := make([]string, 0, len(filtered))
	for _, entry := range filtered {
		name := strings.TrimSpace(entry.Name)
		desc := strings.TrimSpace(entry.Description)
		switch {
		case name == "" && desc == "":
			continue
		case desc == "":
			assets = append(assets, fmt.Sprintf("%s: %s", prefix, name))
		case name == "":
			assets = append(assets, fmt.Sprintf("%s: %s", prefix, desc))
		default:
			assets = append(assets, fmt.Sprintf("%s: %s - %s", prefix, name, desc))
		}
	}
	return assets
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isVisible checks if an entry is visible for the given session type and mode.
func isVisible(e Entry, session SessionType, mode Mode) bool {
	// If no visibility constraints are set, the entry is visible everywhere.
	if len(e.Visibility.SessionTypes) == 0 && len(e.Visibility.Modes) == 0 {
		return true
	}

	sessionOK := len(e.Visibility.SessionTypes) == 0
	for _, st := range e.Visibility.SessionTypes {
		if st == session {
			sessionOK = true
			break
		}
	}

	modeOK := len(e.Visibility.Modes) == 0
	for _, m := range e.Visibility.Modes {
		if m == mode {
			modeOK = true
			break
		}
	}

	return sessionOK && modeOK
}
