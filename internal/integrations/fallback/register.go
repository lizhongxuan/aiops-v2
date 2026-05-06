package fallback

import (
	"fmt"

	core "aiops-v2/internal/fallback"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, service *core.Service) error {
	if registry == nil {
		return fmt.Errorf("fallback: registry is required")
	}
	if service == nil {
		return fmt.Errorf("fallback: service is required")
	}
	for _, tool := range tools(service) {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
