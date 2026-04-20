package capability

import (
	"fmt"
	"sync"
)

// ---------------------------------------------------------------------------
// Extension interface – defines how non-core capabilities (Coroot, Lab,
// Generator) mount into the system. Extensions can ONLY access the system
// through Capability Registry (for registering tools) and Projection (for
// emitting events). They CANNOT reverse-drive RuntimeKernel, PromptCompiler,
// or PolicyEngine design decisions (Req 10.1, 10.5).
// ---------------------------------------------------------------------------

// Extension defines the extension mount interface for non-core capabilities.
// Extensions register their capabilities via the Capability Registry and
// interact with the system only through the Registry and Projection model.
type Extension interface {
	// Name returns the unique name of this extension.
	Name() string

	// Register registers the extension's capabilities into the given Registry.
	Register(registry *Registry) error

	// Unregister removes the extension's capabilities from the given Registry.
	Unregister(registry *Registry) error
}

// ---------------------------------------------------------------------------
// ExtensionManager – manages extension lifecycle and enforces isolation.
// ---------------------------------------------------------------------------

// ExtensionManager manages the lifecycle of extensions, ensuring they only
// interact with the system through the Capability Registry.
type ExtensionManager struct {
	mu         sync.RWMutex
	registry   *Registry
	extensions map[string]Extension
}

// NewExtensionManager creates a new ExtensionManager backed by the given registry.
func NewExtensionManager(registry *Registry) *ExtensionManager {
	return &ExtensionManager{
		registry:   registry,
		extensions: make(map[string]Extension),
	}
}

// Register mounts an extension, calling its Register method to add capabilities
// to the registry. Returns an error if the extension is already registered or
// if registration fails.
func (m *ExtensionManager) Register(ext Extension) error {
	if ext == nil {
		return fmt.Errorf("extension: cannot register nil extension")
	}
	name := ext.Name()
	if name == "" {
		return fmt.Errorf("extension: name is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.extensions[name]; exists {
		return fmt.Errorf("extension: %q is already registered", name)
	}

	if err := ext.Register(m.registry); err != nil {
		return fmt.Errorf("extension: %q registration failed: %w", name, err)
	}

	m.extensions[name] = ext
	return nil
}

// Unregister unmounts an extension, calling its Unregister method to remove
// capabilities from the registry.
func (m *ExtensionManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ext, exists := m.extensions[name]
	if !exists {
		return fmt.Errorf("extension: %q is not registered", name)
	}

	if err := ext.Unregister(m.registry); err != nil {
		return fmt.Errorf("extension: %q unregistration failed: %w", name, err)
	}

	delete(m.extensions, name)
	return nil
}

// Get returns a registered extension by name, or nil if not found.
func (m *ExtensionManager) Get(name string) Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.extensions[name]
}

// List returns the names of all registered extensions.
func (m *ExtensionManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.extensions))
	for name := range m.extensions {
		names = append(names, name)
	}
	return names
}
