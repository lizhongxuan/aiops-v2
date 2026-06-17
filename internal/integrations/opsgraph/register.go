package opsgraph

import (
	"context"
	"fmt"

	graph "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/tooling"
)

type StoreProvider func(context.Context) (*graph.Store, error)

func RegisterBuiltins(registry *tooling.Registry, store *graph.Store) error {
	if registry == nil {
		return fmt.Errorf("opsgraph: registry is required")
	}
	if store == nil {
		return fmt.Errorf("opsgraph: store is required")
	}
	return RegisterBuiltinsWithProvider(registry, func(context.Context) (*graph.Store, error) {
		return store, nil
	})
}

func RegisterBuiltinsWithProvider(registry *tooling.Registry, provider StoreProvider) error {
	if registry == nil {
		return fmt.Errorf("opsgraph: registry is required")
	}
	if provider == nil {
		return fmt.Errorf("opsgraph: store provider is required")
	}
	for _, tool := range tools(provider) {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
