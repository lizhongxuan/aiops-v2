package skills

import (
	"strings"
	"sync"
)

// Definition describes a skill definition and its prompt asset.
type Definition struct {
	Name        string
	Description string
	Prompt      string
	Tools       []string
	Source      string
}

// Registry stores skill definitions keyed by name.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Definition
	order []string
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		items: make(map[string]Definition),
	}
}

// Register stores or replaces a definition by name.
func (r *Registry) Register(def Definition) {
	def.Name = strings.TrimSpace(def.Name)
	if def.Name == "" {
		return
	}
	def.Description = strings.TrimSpace(def.Description)
	def.Prompt = strings.TrimSpace(def.Prompt)
	def.Source = strings.TrimSpace(def.Source)
	def.Tools = append([]string(nil), def.Tools...)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[def.Name]; !exists {
		r.order = append(r.order, def.Name)
	}
	r.items[def.Name] = def
}

// RegisterBatch stores multiple definitions.
func (r *Registry) RegisterBatch(defs []Definition) {
	for _, def := range defs {
		r.Register(def)
	}
}

// Get returns a definition by name.
func (r *Registry) Get(name string) (Definition, bool) {
	name = strings.TrimSpace(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.items[name]
	if !ok {
		return Definition{}, false
	}
	def.Tools = append([]string(nil), def.Tools...)
	return def, true
}

// List returns all registered definitions in registration order.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.order))
	for _, name := range r.order {
		def, ok := r.items[name]
		if !ok {
			continue
		}
		def.Tools = append([]string(nil), def.Tools...)
		out = append(out, def)
	}
	return out
}

// PromptAssets returns the non-empty prompt texts suitable for injection.
func (r *Registry) PromptAssets() []string {
	defs := r.List()
	out := make([]string, 0, len(defs))
	for _, def := range defs {
		if prompt := strings.TrimSpace(def.Prompt); prompt != "" {
			out = append(out, prompt)
		}
	}
	return out
}
