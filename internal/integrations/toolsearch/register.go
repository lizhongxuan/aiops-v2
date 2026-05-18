package toolsearch

import (
	"fmt"

	"aiops-v2/internal/tooling"
)

// RegisterBuiltins registers the read-only tool discovery tool.
func RegisterBuiltins(registry *tooling.Registry) error {
	if registry == nil {
		return fmt.Errorf("toolsearch: registry is required")
	}
	return registry.Register(NewToolSearchTool(registry))
}
