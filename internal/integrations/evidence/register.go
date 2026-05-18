package evidence

import (
	"fmt"

	core "aiops-v2/internal/evidence"
	"aiops-v2/internal/tooling"
)

func RegisterBuiltins(registry *tooling.Registry, service *core.Service) error {
	if registry == nil {
		return fmt.Errorf("evidence: registry is required")
	}
	if service == nil {
		return fmt.Errorf("evidence: service is required")
	}
	for _, tool := range []tooling.Tool{
		NewRecordTool(service),
		NewGetTool(service),
		NewLinkIncidentTool(service),
	} {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}
	return nil
}
