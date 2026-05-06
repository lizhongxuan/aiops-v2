package k8s

import (
	"fmt"

	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, opts Options) error {
	if registry == nil {
		return fmt.Errorf("k8s: registry is required")
	}
	for _, tool := range tools(opts) {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
