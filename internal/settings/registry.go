package settings

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Source describes where runtime settings were loaded from.
type Source string

const (
	SourceUserSettings    Source = "userSettings"
	SourceProjectSettings Source = "projectSettings"
	SourceLocalSettings   Source = "localSettings"
	SourceFlagSettings    Source = "flagSettings"
	SourcePolicySettings  Source = "policySettings"
)

var precedence = []Source{
	SourceUserSettings,
	SourceProjectSettings,
	SourceLocalSettings,
	SourceFlagSettings,
	SourcePolicySettings,
}

// PolicySource describes the internal precedence layers within policySettings.
type PolicySource string

const (
	// PolicySourceRemote is the lowest-precedence remote managed policy layer.
	PolicySourceRemote PolicySource = "remote"
	// PolicySourceMachine represents HKLM/plist machine-managed policy.
	PolicySourceMachine PolicySource = "machine"
	// PolicySourceManaged represents managed-settings.json + managed-settings.d.
	PolicySourceManaged PolicySource = "managed"
	// PolicySourceUser represents HKCU user-scoped managed policy.
	PolicySourceUser PolicySource = "user"
)

var policyPrecedence = []PolicySource{
	PolicySourceRemote,
	PolicySourceMachine,
	PolicySourceManaged,
	PolicySourceUser,
}

// Precedence returns settings sources in low-to-high override order.
func Precedence() []Source {
	return append([]Source(nil), precedence...)
}

// Rank returns the precedence rank for a source. Higher ranks override lower ranks.
func Rank(source Source) int {
	for i, candidate := range precedence {
		if candidate == source {
			return i
		}
	}
	return -1
}

// PolicyPrecedence returns policySettings layers in low-to-high override order.
func PolicyPrecedence() []PolicySource {
	return append([]PolicySource(nil), policyPrecedence...)
}

// PolicyRank returns the internal precedence rank for a policy settings layer.
func PolicyRank(source PolicySource) int {
	for i, candidate := range policyPrecedence {
		if candidate == source {
			return i
		}
	}
	return -1
}

// EnabledSources normalizes and sorts sources using Claude-like precedence.
func EnabledSources(sources []Source) []Source {
	seen := make(map[Source]struct{})
	var out []Source
	for _, source := range sources {
		if Rank(source) < 0 {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}
	slices.SortFunc(out, func(a, b Source) int {
		return Rank(a) - Rank(b)
	})
	return out
}

// Entry stores a plugin- or component-contributed settings object.
type Entry struct {
	Name   string
	Values map[string]any
}

// Registry stores settings entries by name.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Entry
	order []string
}

// NewRegistry creates an empty settings registry.
func NewRegistry() *Registry {
	return &Registry{
		items: make(map[string]Entry),
	}
}

// Register stores a settings entry by name and rejects duplicates.
func (r *Registry) Register(entry Entry) error {
	entry = normalizeEntry(entry)
	if entry.Name == "" {
		return fmt.Errorf("settings: entry name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.items[entry.Name]; exists {
		return fmt.Errorf("settings: entry %q already registered", entry.Name)
	}
	r.items[entry.Name] = cloneEntry(entry)
	r.order = append(r.order, entry.Name)
	return nil
}

// RegisterBatch stores multiple settings entries atomically.
func (r *Registry) RegisterBatch(entries []Entry) error {
	for _, entry := range entries {
		if err := r.Register(entry); err != nil {
			return err
		}
	}
	return nil
}

// Get returns a cloned settings entry by name.
func (r *Registry) Get(name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.items[strings.TrimSpace(name)]
	if !ok {
		return Entry{}, false
	}
	return cloneEntry(entry), true
}

// List returns all settings entries in registration order.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Entry, 0, len(r.order))
	for _, name := range r.order {
		entry, ok := r.items[name]
		if !ok {
			continue
		}
		out = append(out, cloneEntry(entry))
	}
	return out
}

// Unregister removes a settings entry by name.
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

func normalizeEntry(entry Entry) Entry {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Values = cloneValues(entry.Values)
	return entry
}

func cloneEntry(entry Entry) Entry {
	entry.Values = cloneValues(entry.Values)
	return entry
}

func cloneValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
