package workfloweditor

import (
	"fmt"

	"aiops-v2/internal/mcp"
	edit "aiops-v2/internal/workfloweditor"
)

func Register(registry *mcp.Registry, service *edit.Service) error {
	if registry == nil {
		return fmt.Errorf("workfloweditor: mcp registry is required")
	}
	if service == nil {
		service = edit.NewService(nil)
	}
	return registry.RegisterRunnerCapability(Tools(service))
}
