package erp

import (
	"fmt"

	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry) error {
	if registry == nil {
		return fmt.Errorf("erp: registry is required")
	}
	for _, tool := range tools() {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
