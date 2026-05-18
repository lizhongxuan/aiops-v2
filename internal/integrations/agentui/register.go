package agentui

import (
	"fmt"

	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry) error {
	if registry == nil {
		return fmt.Errorf("agentui: registry is required")
	}
	return registry.Register(NewUIArtifactEmitTool())
}
