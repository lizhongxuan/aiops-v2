package outputstyle

import (
	"fmt"
	"strings"
	"sync"
)

// Definition describes an output style prompt asset.
type Definition struct {
	Name        string
	Description string
	Prompt      string
	Source      string
}

// Registry stores output styles by name.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Definition
	order []string
}

// NewRegistry creates an empty output style registry.
func NewRegistry() *Registry {
	return &Registry{
		items: make(map[string]Definition),
	}
}

// Register stores or replaces an output style definition.
func (r *Registry) Register(def Definition) error {
	def = normalizeDefinition(def)
	if def.Name == "" {
		return fmt.Errorf("outputstyle: name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[def.Name]; !exists {
		r.order = append(r.order, def.Name)
	}
	r.items[def.Name] = def
	return nil
}

// RegisterBatch stores multiple output style definitions.
func (r *Registry) RegisterBatch(defs []Definition) error {
	for _, def := range defs {
		if err := r.Register(def); err != nil {
			return err
		}
	}
	return nil
}

// Get returns an output style definition by name.
func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.items[strings.TrimSpace(name)]
	if !ok {
		return Definition{}, false
	}
	return def, true
}

// List returns all registered output style definitions in registration order.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		def, ok := r.items[name]
		if !ok {
			continue
		}
		out = append(out, def)
	}
	return out
}

// Unregister removes an output style by name.
func (r *Registry) Unregister(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.items, name)
	filtered := r.order[:0]
	for _, candidate := range r.order {
		if candidate == name {
			continue
		}
		filtered = append(filtered, candidate)
	}
	r.order = filtered
}

func normalizeDefinition(def Definition) Definition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	def.Prompt = strings.TrimSpace(def.Prompt)
	def.Source = strings.TrimSpace(def.Source)
	return def
}
