package workfloweditor

import (
	"strings"
	"testing"
)

func TestNormalizeWorkflowEditPlanRejectsSingleUserEcho(t *testing.T) {
	_, err := normalizeWorkflowEditPlan(WorkflowEditPlan{
		Items: []WorkflowEditPlanItem{{
			ID:          "item-1",
			Title:       "创建一个pg备份的工作流,使用工具pgbackrest",
			Description: "生成一个最小 Workflow patch，确认后再应用。",
		}},
	}, ProposeEditPlanRequest{WorkflowID: "wf", Message: "创建一个pg备份的工作流,使用工具pgbackrest"})
	if err == nil {
		t.Fatal("expected template plan item to be rejected")
	}
}

func TestDefaultWorkflowEditPlannerRequiresConfiguredLLMPlanner(t *testing.T) {
	_, err := DefaultWorkflowEditPlanner{}.BuildWorkflowEditPlan(testContext(), WorkflowEditPlanningRequest{
		WorkflowID: "wf",
		Message:    "你帮我随便添加一个节点",
	})
	if err == nil || !strings.Contains(err.Error(), "workflow edit planner is not configured") {
		t.Fatalf("BuildWorkflowEditPlan() error = %v, want missing planner configuration", err)
	}
}

func TestNormalizeWorkflowEditPlanRepairsMalformedModelTitle(t *testing.T) {
	normalized, err := normalizeWorkflowEditPlan(WorkflowEditPlan{
		Items: []WorkflowEditPlanItem{{
			ID:          "add-log-start-node",
			Title:       "步骤 1：在 Start 节点后添加一个名为“",
			Description: "在 Start 节点后添加一个名为“记录开始”的 Python 动作节点，用于记录工作流启动。",
			NodeLabel:   "记录开始",
		}},
	}, ProposeEditPlanRequest{WorkflowID: "wf", Message: "把 Start 后面添加一个日志节点，节点名称叫记录开始"})
	if err != nil {
		t.Fatalf("normalizeWorkflowEditPlan() error = %v", err)
	}
	item := normalized.Items[0]
	if strings.Contains(item.Title, "步骤 1") || !strings.Contains(item.Title, "记录开始") {
		t.Fatalf("normalized title = %q, want complete model description without step prefix", item.Title)
	}
	if item.NodeLabel != "记录开始" {
		t.Fatalf("node label = %q, want model-provided short node label", item.NodeLabel)
	}
}
