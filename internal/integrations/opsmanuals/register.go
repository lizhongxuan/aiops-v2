package opsmanuals

import (
	"fmt"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, service *core.Service) error {
	if registry == nil {
		return fmt.Errorf("opsmanuals: registry is required")
	}
	if service == nil {
		return fmt.Errorf("opsmanuals: service is required")
	}
	return registry.Register(newSearchOpsManualsTool(service))
}
