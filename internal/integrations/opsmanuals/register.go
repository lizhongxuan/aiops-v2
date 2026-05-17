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
	cache := newTurnSearchContextCache()
	if err := registry.Register(newSearchOpsManualsTool(service, cache)); err != nil {
		return err
	}
	if err := registry.Register(newResolveOpsManualParamsTool(service, cache)); err != nil {
		return err
	}
	return registry.Register(newRunOpsManualPreflightTool(service, cache))
}
