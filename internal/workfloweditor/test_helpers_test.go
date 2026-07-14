package workfloweditor

import (
	"context"

	"runner/workflow"
	"runner/workflow/visual"
)

type staticWorkflowEditPlanner struct {
	plan WorkflowEditPlan
	err  error
}

func (p staticWorkflowEditPlanner) BuildWorkflowEditPlan(_ context.Context, req WorkflowEditPlanningRequest) (WorkflowEditPlan, error) {
	if p.err != nil {
		return WorkflowEditPlan{}, p.err
	}
	plan := p.plan
	if plan.Message == "" {
		plan.Message = req.Message
	}
	if plan.WorkflowID == "" {
		plan.WorkflowID = req.WorkflowID
	}
	if len(plan.Items) == 0 {
		plan.Items = []WorkflowEditPlanItem{{
			ID:          "item-test",
			Title:       "模型生成的测试步骤",
			Description: "由测试 planner 模拟 LLM 返回，不使用默认模板。",
			Status:      "pending",
		}}
	}
	return plan, nil
}

func workflowEditorTestGraph() visual.Graph {
	collectStep := workflow.Step{
		ID:      "collect",
		Name:    "collect",
		Targets: []string{"local"},
		Action:  "script.python",
		Args:    map[string]any{"script": "print('collect')"},
	}
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "redis-memory",
			Description: "Redis memory pressure workflow",
			Steps:       []workflow.Step{collectStep},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart, Label: "Start"},
			{
				ID:     "collect",
				Type:   visual.NodeTypeAction,
				Label:  "Collect metrics",
				StepID: "collect",
				Step:   &collectStep,
				Outputs: []visual.OutputParamSpec{{
					Key: "memory_usage",
				}},
			},
			{ID: "end", Type: visual.NodeTypeEnd, Label: "End"},
		},
		Edges: []visual.Edge{
			{ID: "start-collect", Source: "start", Target: "collect", Kind: visual.EdgeKindNext},
			{ID: "collect-end", Source: "collect", Target: "end", Kind: visual.EdgeKindNext},
		},
		UI: map[string]any{"title": "Redis Memory"},
	}
}

func newWorkflowEditorTestService() (*Service, *MemoryWorkflowStore, WorkflowRecord) {
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "redis-memory", Graph: workflowEditorTestGraph()})
	return NewService(store, WithEditPlanner(staticWorkflowEditPlanner{})), store, record
}
