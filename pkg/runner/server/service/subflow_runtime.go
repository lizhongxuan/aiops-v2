package service

import (
	"context"
	"fmt"
	"strings"

	"runner/executor"
	"runner/workflow"
)

type workflowSubflowRuntime struct {
	workflowSvc *WorkflowService
}

func newWorkflowSubflowRuntime(workflowSvc *WorkflowService) executor.SubflowRuntime {
	if workflowSvc == nil {
		return nil
	}
	return &workflowSubflowRuntime{workflowSvc: workflowSvc}
}

func (r *workflowSubflowRuntime) LoadSubflow(ctx context.Context, parent workflow.Workflow, _ workflow.GraphNodeSpec, request executor.SubflowRequest) (workflow.Workflow, error) {
	if r == nil || r.workflowSvc == nil {
		return workflow.Workflow{}, fmt.Errorf("%w: workflow service is not configured", ErrUnavailable)
	}
	name := strings.TrimSpace(request.WorkflowName)
	if name == "" {
		return workflow.Workflow{}, fmt.Errorf("%w: subflow workflow name is required", ErrInvalid)
	}
	if name == strings.TrimSpace(parent.Name) {
		return workflow.Workflow{}, fmt.Errorf("%w: subflow cannot call parent workflow %q", ErrInvalid, name)
	}
	record, err := r.workflowSvc.Get(ctx, name)
	if err != nil {
		return workflow.Workflow{}, err
	}
	child, err := workflow.Load(record.RawYAML)
	if err != nil {
		return workflow.Workflow{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := child.Validate(); err != nil {
		return workflow.Workflow{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return child, nil
}
