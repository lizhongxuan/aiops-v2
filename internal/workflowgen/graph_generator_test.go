package workflowgen

import (
	"context"
	"strings"
	"testing"

	"aiops-v2/internal/opsmanual"
	"runner/workflow/visual"
)

func TestGraphGeneratorBuildsValidRunnerGraph(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上8点抓取AI新闻，提取三条关键内容直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	generator := RunnerGraphGenerator{}
	graph, err := generator.GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-1",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}
	if graph.Version != visual.GraphVersion {
		t.Fatalf("graph version = %q, want %q", graph.Version, visual.GraphVersion)
	}
	if graph.Workflow.Name == "" {
		t.Fatal("workflow name is empty")
	}
	if len(graph.Nodes) < 4 {
		t.Fatalf("nodes len = %d, want start, action nodes, end", len(graph.Nodes))
	}
	if err := visual.ValidateGraph(graph); err != nil {
		t.Fatalf("generated graph is invalid: %v", err)
	}
	for _, node := range graph.Nodes {
		if node.Type == visual.NodeTypeAction && node.Step != nil && node.Step.Action == "script.python" {
			if node.Step.Args["script"] == "" {
				t.Fatalf("python node %q missing script", node.ID)
			}
			if len(node.Outputs) == 0 {
				t.Fatalf("python node %q missing output schema", node.ID)
			}
		}
	}
}

func TestPlanBuilderSelectsKubernetesSecurityTemplate(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上9点自动抓取Kubernetes安全公告和云原生漏洞新闻，提取三条需要关注的风险，直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if plan.Title != "Kubernetes 安全风险摘要工作流" {
		t.Fatalf("Title = %q, want Kubernetes 安全风险摘要工作流", plan.Title)
	}
	if plan.Trigger.Schedule != "0 9 * * *" {
		t.Fatalf("Schedule = %q, want 0 9 * * *", plan.Trigger.Schedule)
	}
	if len(plan.Nodes) < 2 {
		t.Fatalf("nodes len = %d, want at least 2", len(plan.Nodes))
	}
	if plan.Nodes[0].ID != "search-kubernetes-security" || plan.Nodes[1].ID != "extract-security-risks" {
		t.Fatalf("node ids = %q/%q, want kubernetes security template", plan.Nodes[0].ID, plan.Nodes[1].ID)
	}
	if plan.Outputs[0].Description != "在工作流输出中直接返回三条需要关注的安全风险。" {
		t.Fatalf("output description = %q", plan.Outputs[0].Description)
	}
}

func TestPlanBuilderSelectsOpsIncidentTemplate(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天下午6点自动抓取数据库和中间件故障案例新闻，提取三条可复盘的运维经验，直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}

	if plan.Title != "数据库与中间件故障复盘工作流" {
		t.Fatalf("Title = %q, want 数据库与中间件故障复盘工作流", plan.Title)
	}
	if plan.Trigger.Schedule != "0 18 * * *" {
		t.Fatalf("Schedule = %q, want 0 18 * * *", plan.Trigger.Schedule)
	}
	if len(plan.Nodes) < 2 {
		t.Fatalf("nodes len = %d, want at least 2", len(plan.Nodes))
	}
	if plan.Nodes[0].ID != "search-ops-incidents" || plan.Nodes[1].ID != "extract-ops-lessons" {
		t.Fatalf("node ids = %q/%q, want ops incident template", plan.Nodes[0].ID, plan.Nodes[1].ID)
	}
	if plan.Outputs[0].Description != "在工作流输出中直接返回三条可复盘的运维经验。" {
		t.Fatalf("output description = %q", plan.Outputs[0].Description)
	}
}

func TestGraphGeneratorUsesReadableWorkflowNameForKnownChineseTitles(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天下午6点自动抓取数据库和中间件故障案例新闻，提取三条可复盘的运维经验，直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	generator := RunnerGraphGenerator{}
	graph, err := generator.GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-3",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}

	if graph.Workflow.Name != "db-middleware-incident-review-wfgen-3" {
		t.Fatalf("workflow name = %q, want db-middleware-incident-review-wfgen-3", graph.Workflow.Name)
	}
}

func TestGraphGeneratorScriptsMatchWorkflowTopic(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天下午6点自动抓取数据库和中间件故障案例新闻，提取三条可复盘的运维经验，直接返回给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	graph, err := (RunnerGraphGenerator{}).GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-topic-script",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}

	var searchScript string
	for _, node := range graph.Nodes {
		if node.ID == "search-ops-incidents" && node.Step != nil {
			searchScript, _ = node.Step.Args["script"].(string)
		}
	}
	if searchScript == "" {
		t.Fatal("search-ops-incidents script is empty")
	}
	if !strings.Contains(searchScript, "PostgreSQL 连接池耗尽复盘") || !strings.Contains(searchScript, `"node_id": "search-ops-incidents"`) {
		t.Fatalf("ops incident script does not match ops topic:\n%s", searchScript)
	}
	if strings.Contains(searchScript, "AI 基础设施投资持续增长") || strings.Contains(searchScript, `"node_id": "search-news"`) {
		t.Fatalf("ops incident script reused AI news fixture:\n%s", searchScript)
	}
}

func TestRunnerGraphGeneratorPreservesResourcePlanVarsAndStageMetadata(t *testing.T) {
	frame := opsmanual.OperationFrame{
		Target: opsmanual.OperationTarget{Type: "postgresql", Name: "pg-cluster"},
		Roles: []opsmanual.OperationResourceRole{
			{ID: "host-a", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-a"},
			{ID: "host-b", Kind: opsmanual.ResourceRoleDataNode, ResourceRef: "host-b"},
		},
		RiskPreference: opsmanual.OperationRiskPreference{StillRequiresApproval: true},
		RawText:        "让主机A和主机B形成PG集群",
	}
	plan, err := (ResourcePlanBuilder{}).BuildResourcePlan(context.Background(), BuildResourcePlanRequest{
		Requirement:    frame.RawText,
		OperationFrame: frame,
	})
	if err != nil {
		t.Fatalf("BuildResourcePlan() error = %v", err)
	}

	graph, err := (RunnerGraphGenerator{}).GenerateGraph(context.Background(), GenerateGraphRequest{
		SessionID: "wfgen-resource-1",
		Plan:      *plan,
	})
	if err != nil {
		t.Fatalf("GenerateGraph() error = %v", err)
	}

	if graph.Workflow.Vars["workflow_generation_session_id"] != "wfgen-resource-1" {
		t.Fatalf("workflow_generation_session_id var = %#v", graph.Workflow.Vars["workflow_generation_session_id"])
	}
	if graph.Workflow.Vars["review_status"] != string(ReviewStatusPendingReview) {
		t.Fatalf("review_status var = %#v, want pending_review", graph.Workflow.Vars["review_status"])
	}
	if graph.Workflow.Vars["resource_kind"] != "postgresql" {
		t.Fatalf("resource_kind var = %#v, want postgresql", graph.Workflow.Vars["resource_kind"])
	}

	preflight := graphNodeByStage(t, graph.Nodes, "preflight")
	if preflight.UI["read_only"] != true {
		t.Fatalf("preflight UI = %#v, want read_only=true", preflight.UI)
	}
	verify := graphNodeByStage(t, graph.Nodes, "verify")
	if verify.UI["read_only"] != true {
		t.Fatalf("verify UI = %#v, want read_only=true", verify.UI)
	}
	execute := graphNodeByStage(t, graph.Nodes, "execute")
	if execute.UI["requires_approval"] != true {
		t.Fatalf("execute UI = %#v, want requires_approval=true", execute.UI)
	}
}

func graphNodeByStage(t *testing.T, nodes []visual.Node, stage string) visual.Node {
	t.Helper()
	for _, node := range nodes {
		if node.UI == nil {
			continue
		}
		if got, _ := node.UI["stage"].(string); got == stage {
			return node
		}
	}
	t.Fatalf("stage %q not found in graph nodes: %#v", stage, nodes)
	return visual.Node{}
}
