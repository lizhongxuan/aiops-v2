package agents

import (
	"fmt"
	"sort"
	"sync"
)

// Definition describes a registered agent template.
type Definition struct {
	Kind          string
	Name          string
	Description   string
	Prompt        string
	Tools         []string
	Model         string
	Hooks         []string
	MCPServers    []string
	MaxIterations int

	// Compat fields preserve data when converting from agentmgr definitions.
	CapabilityKinds []string
	CapabilityHosts []string
}

// Validate checks whether the definition has the minimum required fields.
func (d Definition) Validate() error {
	if d.Kind == "" {
		return fmt.Errorf("agent definition kind is required")
	}
	if d.Name == "" {
		return fmt.Errorf("agent definition name is required")
	}
	if d.MaxIterations < 0 {
		return fmt.Errorf("max iterations must be non-negative, got %d", d.MaxIterations)
	}
	return nil
}

type registryRecord struct {
	def Definition
}

// Registry manages agent definitions by name and kind.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]registryRecord
	byKind map[string]string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byName: make(map[string]registryRecord),
		byKind: make(map[string]string),
	}
}

// Register adds a definition to the registry.
func (r *Registry) Register(def Definition) error {
	if err := def.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byName[def.Name]; ok {
		return fmt.Errorf("agent definition name %q already registered", def.Name)
	}
	if existingName, ok := r.byKind[def.Kind]; ok {
		return fmt.Errorf("agent definition kind %q already registered by %q", def.Kind, existingName)
	}

	r.byName[def.Name] = registryRecord{def: cloneDefinition(def)}
	r.byKind[def.Kind] = def.Name
	return nil
}

// RegisterBatch adds all definitions atomically.
func (r *Registry) RegisterBatch(defs []Definition) error {
	if len(defs) == 0 {
		return nil
	}

	temp := make([]Definition, len(defs))
	copy(temp, defs)
	for _, def := range temp {
		if err := def.Validate(); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	nextByName := make(map[string]registryRecord, len(r.byName)+len(temp))
	nextByKind := make(map[string]string, len(r.byKind)+len(temp))
	for name, rec := range r.byName {
		nextByName[name] = registryRecord{def: cloneDefinition(rec.def)}
	}
	for kind, name := range r.byKind {
		nextByKind[kind] = name
	}

	for _, def := range temp {
		if _, ok := nextByName[def.Name]; ok {
			return fmt.Errorf("agent definition name %q already registered", def.Name)
		}
		if existingName, ok := nextByKind[def.Kind]; ok {
			return fmt.Errorf("agent definition kind %q already registered by %q", def.Kind, existingName)
		}
		nextByName[def.Name] = registryRecord{def: cloneDefinition(def)}
		nextByKind[def.Kind] = def.Name
	}

	r.byName = nextByName
	r.byKind = nextByKind
	return nil
}

// Get returns a definition by name, falling back to kind for compatibility.
func (r *Registry) Get(key string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if rec, ok := r.byName[key]; ok {
		return cloneDefinition(rec.def), true
	}
	if name, ok := r.byKind[key]; ok {
		rec, ok := r.byName[name]
		if !ok {
			return Definition{}, false
		}
		return cloneDefinition(rec.def), true
	}
	return Definition{}, false
}

// List returns all registered definitions sorted by name then kind.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.byName))
	for _, rec := range r.byName {
		out = append(out, cloneDefinition(rec.def))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func cloneDefinition(def Definition) Definition {
	def.Tools = append([]string(nil), def.Tools...)
	def.Hooks = append([]string(nil), def.Hooks...)
	def.MCPServers = append([]string(nil), def.MCPServers...)
	def.CapabilityKinds = append([]string(nil), def.CapabilityKinds...)
	def.CapabilityHosts = append([]string(nil), def.CapabilityHosts...)
	return def
}
