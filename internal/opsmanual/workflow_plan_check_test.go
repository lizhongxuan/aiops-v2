package opsmanual

import (
	"context"
	"testing"
)

func TestWorkflowPlanCheckerFunc(t *testing.T) {
	checker := WorkflowPlanCheckerFunc(func(ctx context.Context, req WorkflowPlanCheckRequest) (WorkflowPlanCheckResult, error) {
		if req.WorkflowID != "wf-redis" {
			t.Fatalf("workflow id = %q", req.WorkflowID)
		}
		return WorkflowPlanCheckResult{
			Status:         "passed",
			WorkflowDigest: "sha256:redis",
			TargetHosts:    []string{"redis-01"},
			ActionsUsed:    []string{"script.shell"},
			RiskLevel:      "medium",
			Summary:        "将对 redis-01 执行 1 个步骤",
		}, nil
	})
	result, err := checker.CheckWorkflowPlan(context.Background(), WorkflowPlanCheckRequest{
		WorkflowID: "wf-redis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkflowDigest != "sha256:redis" || result.TargetHosts[0] != "redis-01" {
		t.Fatalf("result = %#v", result)
	}
}
