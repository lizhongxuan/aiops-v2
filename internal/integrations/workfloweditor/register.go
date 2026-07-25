package workfloweditor

import (
	"fmt"

	"aiops-v2/internal/mcp"
	"runner/server/service"
)

func Register(registry *mcp.Registry, app *service.WorkflowEditApplication, resources *WorkflowResourcesV2) error {
	if registry == nil {
		return fmt.Errorf("workfloweditor: mcp registry is required")
	}
	if app == nil || resources == nil {
		return fmt.Errorf("workfloweditor: workflow edit application and resources are required")
	}
	tools := append(WorkflowReadToolsV2(app, resources), WorkflowCompilePlanToolV2(app))
	tools = append(tools, WorkflowMutationToolsV2(app)...)
	return registry.RegisterRunnerCapability(tools)
}
