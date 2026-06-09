package skilltools

import (
	"fmt"

	"aiops-v2/internal/skills"
	"aiops-v2/internal/tooling"
)

// RegisterBuiltins registers the compact skill discovery and bounded skill read tools.
func RegisterBuiltins(registry *tooling.Registry, skillRegistry *skills.Registry) error {
	if registry == nil {
		return fmt.Errorf("skilltools: tooling registry is required")
	}
	if skillRegistry == nil {
		return fmt.Errorf("skilltools: skill registry is required")
	}
	if err := registry.Register(NewSkillSearchTool(skillRegistry)); err != nil {
		return err
	}
	return registry.Register(NewSkillReadTool(skillRegistry))
}
