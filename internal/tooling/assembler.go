package tooling

import "github.com/cloudwego/eino/components/tool"

// DynamicToolProvider supplies dynamic tools that should participate in the
// same assembly pass as statically registered tools.
type DynamicToolProvider interface {
	DynamicTools() []Tool
}

// Assembler composes the base tool registry with dynamic providers so prompt,
// runtime, and agent assembly all read from one tool source of truth.
type Assembler struct {
	registry   *Registry
	providers  []DynamicToolProvider
}

// NewAssembler creates an assembler backed by a base registry plus any number
// of dynamic tool providers such as MCP registries.
func NewAssembler(registry *Registry, providers ...DynamicToolProvider) *Assembler {
	filtered := make([]DynamicToolProvider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		filtered = append(filtered, provider)
	}
	return &Assembler{
		registry:  registry,
		providers: filtered,
	}
}

// AssembleToolsWithOptions returns the visible tool set after merging the base
// registry with all dynamic providers.
func (a *Assembler) AssembleToolsWithOptions(session, mode string, opts AssembleOptions) []Tool {
	if a == nil {
		return nil
	}

	merged := opts
	merged.ExtraTools = append([]Tool(nil), opts.ExtraTools...)
	for _, provider := range a.providers {
		merged.ExtraTools = append(merged.ExtraTools, provider.DynamicTools()...)
	}

	if a.registry == nil {
		tmp := NewRegistry()
		return tmp.AssembleToolsWithOptions(session, mode, merged)
	}
	return a.registry.AssembleToolsWithOptions(session, mode, merged)
}

// AssembleToolPoolWithOptions adapts the assembled tools into Eino base tools.
func (a *Assembler) AssembleToolPoolWithOptions(session, mode string, opts AssembleOptions) []tool.BaseTool {
	return AssembleEinoToolPool(a.AssembleToolsWithOptions(session, mode, opts))
}

