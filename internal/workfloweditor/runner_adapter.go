package workfloweditor

import (
	"context"
	"fmt"

	runnerapp "runner/server/app"
	"runner/server/service"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

type RunnerWorkflowFacade interface {
	GetGraph(context.Context, edit.WorkflowKey) (runnerapp.WorkflowGraphSnapshot, error)
	GetHashes(context.Context, edit.WorkflowKey) (edit.Revision, error)
	ListActions(context.Context, edit.WorkflowKey, service.ActionCatalogFilter) ([]service.ActionSpec, error)
	Validate(context.Context, edit.WorkflowKey, visual.Graph) (*service.VisualWorkflowValidationResult, error)
	Compile(context.Context, edit.WorkflowKey, visual.Graph) (*service.CompiledVisualWorkflow, error)
}

type RunnerAdapter struct {
	facade RunnerWorkflowFacade
}

func NewRunnerAdapter(facade RunnerWorkflowFacade) (*RunnerAdapter, error) {
	if facade == nil {
		return nil, fmt.Errorf("runner workflow facade is required")
	}
	return &RunnerAdapter{facade: facade}, nil
}

func (adapter *RunnerAdapter) GetGraph(ctx context.Context, key edit.WorkflowKey) (runnerapp.WorkflowGraphSnapshot, error) {
	if err := validateRunnerWorkflowKey(adapter, key); err != nil {
		return runnerapp.WorkflowGraphSnapshot{}, err
	}
	return adapter.facade.GetGraph(ctx, key)
}

func (adapter *RunnerAdapter) GetHashes(ctx context.Context, key edit.WorkflowKey) (edit.Revision, error) {
	if err := validateRunnerWorkflowKey(adapter, key); err != nil {
		return edit.Revision{}, err
	}
	return adapter.facade.GetHashes(ctx, key)
}

func (adapter *RunnerAdapter) ListActions(ctx context.Context, key edit.WorkflowKey, filter service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	if err := validateRunnerWorkflowKey(adapter, key); err != nil {
		return nil, err
	}
	return adapter.facade.ListActions(ctx, key, filter)
}

func (adapter *RunnerAdapter) Validate(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	if err := validateRunnerWorkflowKey(adapter, key); err != nil {
		return nil, err
	}
	return adapter.facade.Validate(ctx, key, graph)
}

func (adapter *RunnerAdapter) Compile(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.CompiledVisualWorkflow, error) {
	if err := validateRunnerWorkflowKey(adapter, key); err != nil {
		return nil, err
	}
	return adapter.facade.Compile(ctx, key, graph)
}

func validateRunnerWorkflowKey(adapter *RunnerAdapter, key edit.WorkflowKey) error {
	if adapter == nil || adapter.facade == nil {
		return fmt.Errorf("runner adapter is not configured")
	}
	return key.Validate()
}
