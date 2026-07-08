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

func TestWorkflowEditPlannerPromptAllowsSmallEdits(t *testing.T) {
	prompt := workflowEditPlannerSystemPrompt()
	if strings.Contains(prompt, "计划必须是 4 到 8 个步骤") {
		t.Fatalf("planner prompt still forces every request into a large fixed plan")
	}
	if !strings.Contains(prompt, "简单节点变更") {
		t.Fatalf("planner prompt should instruct the model to keep simple node edits small")
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

func TestParseWorkflowEditPlanPreservesModelGeneratedStepDetails(t *testing.T) {
	plan, err := parseWorkflowEditPlanModelContent(`{
	  "items": [{
	    "id": "verify",
	    "title": "添加验证步骤",
		    "description": "检查上游输出",
		    "goal": "LLM 目标",
		    "environment": "LLM 环境",
		    "nodeLabel": "验证输出",
		    "nodeType": "action",
		    "nodeAction": "script.python",
		    "scriptSummary": "LLM 脚本摘要",
	    "validationSummary": "LLM 校验",
	    "inputVariables": [{"name":"memory_usage","type":"number","required":true}],
	    "outputVariables": [{"name":"validation_result","type":"object"}],
	    "script": "# llm script"
	  }]
	}`)
	if err != nil {
		t.Fatalf("parseWorkflowEditPlanModelContent() error = %v", err)
	}
	normalized, err := normalizeWorkflowEditPlan(plan, ProposeEditPlanRequest{WorkflowID: "wf", Message: "添加验证步骤"})
	if err != nil {
		t.Fatalf("normalizeWorkflowEditPlan() error = %v", err)
	}
	item := normalized.Items[0]
	if item.Goal != "LLM 目标" || item.Environment != "LLM 环境" || item.NodeLabel != "验证输出" || item.NodeType != "action" || item.NodeAction != "script.python" || item.ScriptSummary != "LLM 脚本摘要" || item.ValidationSummary != "LLM 校验" || item.Script != "# llm script" {
		t.Fatalf("item details = %#v, want model-generated details preserved", item)
	}
	if len(item.InputVariables) != 1 || item.InputVariables[0].Name != "memory_usage" || !item.InputVariables[0].Required {
		t.Fatalf("input variables = %#v, want model-generated input variable", item.InputVariables)
	}
	if len(item.OutputVariables) != 1 || item.OutputVariables[0].Name != "validation_result" {
		t.Fatalf("output variables = %#v, want model-generated output variable", item.OutputVariables)
	}
}
