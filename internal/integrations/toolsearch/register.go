package toolsearch

import (
	"fmt"

	"aiops-v2/internal/tooling"
)

// RegisterBuiltins registers the read-only tool discovery tool.
func RegisterBuiltins(registry *tooling.Registry, providers ...tooling.ToolCatalogProvider) error {
	if registry == nil {
		return fmt.Errorf("toolsearch: registry is required")
	}
	var provider tooling.ToolCatalogProvider = registry
	if len(providers) > 0 && providers[0] != nil {
		provider = providers[0]
	}
	return registry.Register(NewToolSearchTool(provider))
}
