package agents

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
)

// DefinitionSource identifies where an agent definition came from.
type DefinitionSource string

const (
	SourceBuiltin         DefinitionSource = "built-in"
	SourcePlugin          DefinitionSource = "plugin"
	SourceUserSettings    DefinitionSource = "userSettings"
	SourceProjectSettings DefinitionSource = "projectSettings"
	SourceFlagSettings    DefinitionSource = "flagSettings"
	SourcePolicySettings  DefinitionSource = "policySettings"
)

var sourcePrecedence = []DefinitionSource{
	SourceBuiltin,
	SourcePlugin,
	SourceUserSettings,
	SourceProjectSettings,
	SourceFlagSettings,
	SourcePolicySettings,
}

// AgentTool is the orchestration-facing view derived from an agent definition.
// It is intentionally separate from runtime tooling.Tool implementations.
type AgentTool struct {
	Kind          string
	Name          string
	Description   string
	Prompt        string
	Discovery     AgentDiscoveryMetadata
	Budget        AgentBudgetMetadata
	Tools         []string
	Model         string
	Hooks         []string
	MCPServers    []string
	MaxIterations int
}

// Definition describes a registered agent template.
type Definition struct {
	Kind          string
	Name          string
	Source        string
	Description   string
	Prompt        string
	Discovery     AgentDiscoveryMetadata
	Budget        AgentBudgetMetadata
	Tools         []string
	Model         string
	Hooks         []string
	MCPServers    []string
	MaxIterations int
}

// ToAgentTool projects the orchestration-facing fields needed by AgentTool-style dispatch.
func (d Definition) ToAgentTool() AgentTool {
	return AgentTool{
		Kind:          d.Kind,
		Name:          d.Name,
		Description:   d.Description,
		Prompt:        d.Prompt,
		Discovery:     cloneDiscoveryMetadata(d.Discovery),
		Budget:        normalizeBudgetMetadata(d.Budget),
		Tools:         append([]string(nil), d.Tools...),
		Model:         d.Model,
		Hooks:         append([]string(nil), d.Hooks...),
		MCPServers:    append([]string(nil), d.MCPServers...),
		MaxIterations: d.MaxIterations,
	}
}

// Validate checks whether the definition has the minimum required fields.
func (d Definition) Validate() error {
	if d.Kind == "" {
		return fmt.Errorf("agent definition kind is required")
	}
	if d.Name == "" {
		return fmt.Errorf("agent definition name is required")
	}
	if _, err := normalizeSource(d.Source); err != nil {
		return err
	}
	if d.MaxIterations < 0 {
		return fmt.Errorf("max iterations must be non-negative, got %d", d.MaxIterations)
	}
	if d.Budget.MaxConcurrent < 0 {
		return fmt.Errorf("max concurrent must be non-negative, got %d", d.Budget.MaxConcurrent)
	}
	switch d.Budget.CostClass {
	case "", "low", "medium", "high":
	default:
		return fmt.Errorf("unknown cost class %q", d.Budget.CostClass)
	}
	return nil
}

type registryRecord struct {
	def Definition
}

// Registry manages agent definitions by name and kind with source precedence.
type Registry struct {
	mu         sync.RWMutex
	governance *settings.Governance
	byName     map[string]registryRecord
	byKind     map[string]map[DefinitionSource]registryRecord
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]registryRecord),
		byKind: make(map[string]map[DefinitionSource]registryRecord),
	}
}

// SetGovernance attaches a live governance snapshot source to the registry.
func (r *Registry) SetGovernance(governance *settings.Governance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governance = governance
}

// Register adds a definition to the registry.
func (r *Registry) Register(def Definition) error {
	if err := def.Validate(); err != nil {
		return err
	}
	source, err := normalizeSource(def.Source)
	if err != nil {
		return err
	}
	def.Source = string(source)
	if err := r.validateDefinition(def); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byName[def.Name]; ok {
		return fmt.Errorf("agent definition name %q already registered", def.Name)
	}
	bySource := r.byKind[def.Kind]
	if bySource == nil {
		bySource = make(map[DefinitionSource]registryRecord)
		r.byKind[def.Kind] = bySource
	}
	if _, ok := bySource[source]; ok {
		return fmt.Errorf("agent definition kind %q already registered for source %q", def.Kind, source)
	}

	r.byName[def.Name] = registryRecord{def: cloneDefinition(def)}
	bySource[source] = registryRecord{def: cloneDefinition(def)}
	return nil
}

// RegisterBatch adds all definitions atomically.
func (r *Registry) RegisterBatch(defs []Definition) error {
	if len(defs) == 0 {
		return nil
	}

	temp := make([]Definition, len(defs))
	copy(temp, defs)
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()
	var snapshot settings.GovernanceSnapshot
	if governance != nil {
		snapshot = governance.Snapshot()
	}
	for _, def := range temp {
		if err := def.Validate(); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	nextByName := make(map[string]registryRecord, len(r.byName)+len(temp))
	nextByKind := make(map[string]map[DefinitionSource]registryRecord, len(r.byKind)+len(temp))
	for name, rec := range r.byName {
		nextByName[name] = registryRecord{def: cloneDefinition(rec.def)}
	}
	for kind, bySource := range r.byKind {
		nextByKind[kind] = make(map[DefinitionSource]registryRecord, len(bySource))
		for source, rec := range bySource {
			nextByKind[kind][source] = registryRecord{def: cloneDefinition(rec.def)}
		}
	}

	for _, def := range temp {
		source, err := normalizeSource(def.Source)
		if err != nil {
			return err
		}
		def.Source = string(source)
		if governance != nil && !snapshot.AllowsSource(settings.SurfaceAgents, def.Source) {
			return fmt.Errorf("agent definition %q blocked by strictPluginOnlyCustomization for agents", def.Name)
		}
		if _, ok := nextByName[def.Name]; ok {
			return fmt.Errorf("agent definition name %q already registered", def.Name)
		}
		bySource := nextByKind[def.Kind]
		if bySource == nil {
			bySource = make(map[DefinitionSource]registryRecord)
			nextByKind[def.Kind] = bySource
		}
		if _, ok := bySource[source]; ok {
			return fmt.Errorf("agent definition kind %q already registered for source %q", def.Kind, source)
		}
		nextByName[def.Name] = registryRecord{def: cloneDefinition(def)}
		bySource[source] = registryRecord{def: cloneDefinition(def)}
	}

	r.byName = nextByName
	r.byKind = nextByKind
	return nil
}

// Get returns a definition by name, falling back to the active definition for a kind.
func (r *Registry) Get(key string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if rec, ok := r.byName[key]; ok {
		return cloneDefinition(rec.def), true
	}
	if bySource, ok := r.byKind[key]; ok {
		for i := len(sourcePrecedence) - 1; i >= 0; i-- {
			source := sourcePrecedence[i]
			if rec, ok := bySource[source]; ok {
				return cloneDefinition(rec.def), true
			}
		}
	}
	return Definition{}, false
}

// List returns all registered definitions sorted by kind, source precedence, then name.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.byName))
	for _, rec := range r.byName {
		out = append(out, cloneDefinition(rec.def))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if sourcePriority(out[i].Source) == sourcePriority(out[j].Source) {
				return out[i].Name < out[j].Name
			}
			return sourcePriority(out[i].Source) < sourcePriority(out[j].Source)
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// Unregister removes a definition by name.
func (r *Registry) Unregister(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.byName[name]
	if !ok {
		return
	}
	delete(r.byName, name)

	source, err := normalizeSource(rec.def.Source)
	if err != nil {
		return
	}
	bySource := r.byKind[rec.def.Kind]
	delete(bySource, source)
	if len(bySource) == 0 {
		delete(r.byKind, rec.def.Kind)
	}
}

func normalizeSource(source string) (DefinitionSource, error) {
	if source == "" {
		return SourceBuiltin, nil
	}
	switch DefinitionSource(source) {
	case SourceBuiltin, "builtin":
		return SourceBuiltin, nil
	case SourcePlugin:
		return SourcePlugin, nil
	case SourceUserSettings:
		return SourceUserSettings, nil
	case SourceProjectSettings:
		return SourceProjectSettings, nil
	case SourceFlagSettings:
		return SourceFlagSettings, nil
	case SourcePolicySettings:
		return SourcePolicySettings, nil
	default:
		return "", fmt.Errorf("unknown agent definition source %q", source)
	}
}

func sourcePriority(source string) int {
	normalized, err := normalizeSource(source)
	if err != nil {
		return len(sourcePrecedence)
	}
	for i, candidate := range sourcePrecedence {
		if candidate == normalized {
			return i
		}
	}
	return len(sourcePrecedence)
}

func cloneDefinition(def Definition) Definition {
	def.Discovery = cloneDiscoveryMetadata(def.Discovery)
	def.Tools = append([]string(nil), def.Tools...)
	def.Hooks = append([]string(nil), def.Hooks...)
	def.MCPServers = append([]string(nil), def.MCPServers...)
	return def
}

func (r *Registry) validateDefinition(def Definition) error {
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()

	if governance == nil {
		return nil
	}
	if governance.Snapshot().AllowsSource(settings.SurfaceAgents, def.Source) {
		return nil
	}
	return fmt.Errorf("agent definition %q blocked by strictPluginOnlyCustomization for agents", def.Name)
}
