package workfloweditor

import (
	"context"
	"fmt"

	"aiops-v2/internal/workflowgen"
)

type WorkflowGenAdapter struct {
	Generator workflowgen.GraphGenerator
}

func (a WorkflowGenAdapter) CreateDraftFromPlan(ctx context.Context, req WorkflowDraftFromPlanRequest) (WorkflowDraftFromPlanResult, error) {
	generator := a.Generator
	if generator == nil {
		generator = workflowgen.RunnerGraphGenerator{}
	}
	graph, err := generator.GenerateGraph(ctx, workflowgen.GenerateGraphRequest{
		SessionID: req.SessionID,
		Plan:      req.Plan,
	})
	if err != nil {
		return WorkflowDraftFromPlanResult{}, fmt.Errorf("generate workflow draft: %w", err)
	}
	return WorkflowDraftFromPlanResult{Graph: graph, Revision: RevisionDigest(graph)}, nil
}
