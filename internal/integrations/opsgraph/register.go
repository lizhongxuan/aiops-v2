package opsgraph

import (
	"fmt"

	graph "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, store *graph.Store) error {
	if registry == nil {
		return fmt.Errorf("opsgraph: registry is required")
	}
	if store == nil {
		return fmt.Errorf("opsgraph: store is required")
	}
	for _, tool := range tools(store) {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
