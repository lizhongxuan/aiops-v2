package lsp

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ServerConfig stores the registration-time configuration for an LSP server.
type ServerConfig struct {
	ID        string
	Name      string
	Command   []string
	Languages []string
	Roots     []string
	Source    string
}

// Registry tracks configured LSP servers.
type Registry struct {
	mu      sync.RWMutex
	servers map[string]ServerConfig
}

// NewRegistry creates an empty LSP registry.
func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]ServerConfig),
	}
}

// RegisterServer stores or replaces an LSP server configuration.
func (r *Registry) RegisterServer(cfg ServerConfig) error {
	cfg = normalizeServerConfig(cfg)
	if cfg.ID == "" {
		return fmt.Errorf("lsp: server id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers[cfg.ID] = cfg
	return nil
}

// GetServer returns a cloned server configuration by id.
func (r *Registry) GetServer(id string) (ServerConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cfg, ok := r.servers[strings.TrimSpace(id)]
	if !ok {
		return ServerConfig{}, false
	}
	return cloneServerConfig(cfg), true
}

// ListServers returns all registered server configs sorted by id.
func (r *Registry) ListServers() []ServerConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ServerConfig, 0, len(r.servers))
	for _, cfg := range r.servers {
		out = append(out, cloneServerConfig(cfg))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

// UnregisterServer removes an LSP server configuration by id.
func (r *Registry) UnregisterServer(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.servers, strings.TrimSpace(id))
}

func normalizeServerConfig(cfg ServerConfig) ServerConfig {
	cfg.ID = strings.TrimSpace(cfg.ID)
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	cfg.Source = strings.TrimSpace(cfg.Source)
	cfg.Command = trimSlice(cfg.Command)
	cfg.Languages = trimSlice(cfg.Languages)
	cfg.Roots = trimSlice(cfg.Roots)
	return cfg
}

func cloneServerConfig(cfg ServerConfig) ServerConfig {
	cfg.Command = append([]string(nil), cfg.Command...)
	cfg.Languages = append([]string(nil), cfg.Languages...)
	cfg.Roots = append([]string(nil), cfg.Roots...)
	return cfg
}

func trimSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
