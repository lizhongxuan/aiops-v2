package tooling

import (
	"fmt"
	"sort"
	"sync"

	"github.com/cloudwego/eino/components/tool"
)

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
		currentRank := originRank(meta.Origin)
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
		currentRank := originRank(meta.Origin)
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
	r.mu.RLock()
	defer r.mu.RUnlock()

	best := make(map[string]Tool)
	ranks := make(map[string]int)

	for i := len(r.records) - 1; i >= 0; i-- {
		t := r.records[i].tool
		meta := t.Metadata()
		ctx := ToolContext{SessionType: session, Mode: mode, Metadata: meta}
		if !t.IsEnabled(ctx) {
			continue
		}
		name := meta.Name
		currentRank := originRank(meta.Origin)
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

// AssembleToolPool returns Eino tools for the visible unified tools.
func (r *Registry) AssembleToolPool(session, mode string) []tool.BaseTool {
	return AssembleEinoToolPool(r.AssembleTools(session, mode))
}

func originRank(origin ToolOrigin) int {
	switch origin {
	case ToolOriginBuiltin:
		return 2
	case ToolOriginMCP:
		return 1
	case ToolOriginMeta:
		return 0
	default:
		return -1
	}
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
