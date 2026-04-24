package modules

import (
	"strings"
	"sync"
)

type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

func (r *Registry) Register(action string, module Module) {
	if module == nil {
		// "module is nil"
		return
	}
	key := strings.TrimSpace(action)
	if key == "" {
		//"action is required"
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules[key] = module
	return
}

func (r *Registry) Get(action string) (Module, bool) {
	key := strings.TrimSpace(action)
	r.mu.RLock()
	defer r.mu.RUnlock()
	module, ok := r.modules[key]
	return module, ok
}
