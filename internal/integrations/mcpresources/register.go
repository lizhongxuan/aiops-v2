package mcpresources

import (
	"fmt"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, resources *mcp.Registry) error {
	return RegisterBuiltinsWithOptions(registry, resources, ReadToolOptions{})
}

func RegisterBuiltinsWithOptions(registry *tooling.Registry, resources *mcp.Registry, opts ReadToolOptions) error {
	if registry == nil {
		return fmt.Errorf("mcpresources: tooling registry is required")
	}
	if resources == nil {
		return fmt.Errorf("mcpresources: mcp registry is required")
	}
	for _, tool := range []tooling.Tool{
		NewListTool(resources),
		NewReadToolWithOptions(resources, opts),
	} {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
