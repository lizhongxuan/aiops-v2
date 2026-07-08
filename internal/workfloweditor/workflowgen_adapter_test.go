package workfloweditor

import (
	"testing"

	"aiops-v2/internal/workflowgen"
)

func TestWorkflowGenAdapterCreatesDraftGraphFromPlan(t *testing.T) {
	adapter := WorkflowGenAdapter{}
	result, err := adapter.CreateDraftFromPlan(testContext(), WorkflowDraftFromPlanRequest{
		SessionID: "session",
		Plan: workflowgen.WorkflowGenerationPlan{
			Version: 1,
			Title:   "Redis Memory Check",
			Trigger: workflowgen.WorkflowTrigger{Type: workflowgen.TriggerTypeManual},
			Nodes: []workflowgen.WorkflowPlanNode{{
				ID:     "collect",
				Kind:   workflowgen.NodeKindSearch,
				Title:  "Collect",
				Action: "script.python",
			}},
			Outputs: []workflowgen.WorkflowOutput{{
				ID:     "summary",
				Target: workflowgen.OutputTargetReturn,
			}},
			ValidationStrategy: workflowgen.ValidationStrategy{Enabled: false, Provider: workflowgen.ValidationProviderNone},
		},
	})
	if err != nil {
		t.Fatalf("CreateDraftFromPlan() error = %v", err)
	}
	if result.Revision == "" || len(result.Graph.Nodes) == 0 || result.Graph.Workflow.Name == "" {
		t.Fatalf("result = %#v, want graph and revision", result)
	}
}
