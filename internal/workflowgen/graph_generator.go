package workflowgen

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"runner/workflow"
	"runner/workflow/visual"
)

type GenerateGraphRequest struct {
	SessionID string                 `json:"session_id,omitempty"`
	Plan      WorkflowGenerationPlan `json:"plan"`
}

type GraphGenerator interface {
	GenerateGraph(ctx context.Context, req GenerateGraphRequest) (visual.Graph, error)
}

type RunnerGraphGenerator struct{}

func (g RunnerGraphGenerator) GenerateGraph(_ context.Context, req GenerateGraphRequest) (visual.Graph, error) {
	if len(req.Plan.Nodes) == 0 {
		return visual.Graph{}, fmt.Errorf("plan nodes are required")
	}
	name := workflowName(req.Plan.Title, req.SessionID)
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        name,
			Description: req.Plan.Title,
			Inventory: workflow.Inventory{
				Groups: map[string]workflow.Group{"all": {Hosts: []string{"local"}}},
				Hosts:  map[string]workflow.Host{"local": {Address: "127.0.0.1", Vars: map[string]any{"capabilities": []any{"script.python"}}}},
				Vars:   map[string]any{},
			},
			Vars: map[string]any{
				"workflow_generation_session_id": req.SessionID,
				"validation_provider":            string(req.Plan.ValidationStrategy.Provider),
				"validation_scenario":            req.Plan.ValidationStrategy.Scenario,
			},
			Plan: workflow.Plan{Mode: "serial", Strategy: "plan_first"},
		},
		Layout: visual.Layout{
			Direction: "TB",
			Viewport:  visual.Viewport{X: 0, Y: 0, Zoom: 1},
		},
		UI: map[string]any{
			"source": "aiops_workflow_generation",
			"title":  req.Plan.Title,
		},
	}

	graph.Nodes = append(graph.Nodes, visual.Node{
		ID:       "start",
		Type:     visual.NodeTypeStart,
		Position: visual.Position{X: 320, Y: 80},
		Label:    "开始",
	})

	previousID := "start"
	steps := make([]workflow.Step, 0, len(req.Plan.Nodes))
	for i, planNode := range req.Plan.Nodes {
		nodeID := sanitizeID(planNode.ID)
		if nodeID == "" {
			nodeID = fmt.Sprintf("node-%d", i+1)
		}
		step := workflow.Step{
			ID:      nodeID,
			Name:    nodeID,
			Targets: []string{"local"},
			Action:  firstNonEmpty(planNode.Action, "script.python"),
			Args: map[string]any{
				"script":           scriptForPlanNode(planNode),
				"max_output_bytes": 32768,
			},
			Timeout: "2m",
		}
		steps = append(steps, step)
		graph.Nodes = append(graph.Nodes, visual.Node{
			ID:       nodeID,
			Type:     visual.NodeTypeAction,
			Position: visual.Position{X: 320, Y: float64(220 + i*150)},
			StepName: step.Name,
			StepID:   step.ID,
			Step:     &step,
			Label:    firstNonEmpty(planNode.Title, nodeID),
			Inputs:   ioToVisualInputs(planNode.Inputs),
			Outputs:  ioToVisualOutputs(planNode.Outputs),
			UI: map[string]any{
				"kind":        string(planNode.Kind),
				"description": planNode.Description,
			},
		})
		graph.Edges = append(graph.Edges, visual.Edge{
			ID:     fmt.Sprintf("%s-to-%s", previousID, nodeID),
			Source: previousID,
			Target: nodeID,
			Kind:   visual.EdgeKindNext,
		})
		previousID = nodeID
	}
	graph.Workflow.Steps = steps

	graph.Nodes = append(graph.Nodes, visual.Node{
		ID:       "end",
		Type:     visual.NodeTypeEnd,
		Position: visual.Position{X: 320, Y: float64(220 + len(req.Plan.Nodes)*150)},
		Label:    "结束",
	})
	graph.Edges = append(graph.Edges, visual.Edge{
		ID:     fmt.Sprintf("%s-to-end", previousID),
		Source: previousID,
		Target: "end",
		Kind:   visual.EdgeKindNext,
	})

	graph.Workflow.XRunnerGraph = &workflow.GraphSpec{
		Version: string(visual.GraphVersion),
		Nodes:   workflowGraphNodes(graph.Nodes),
		Edges:   workflowGraphEdges(graph.Edges),
		UI:      graph.UI,
	}
	if err := visual.ValidateGraph(graph); err != nil {
		return visual.Graph{}, err
	}
	return graph, nil
}

func scriptForPlanNode(node WorkflowPlanNode) string {
	topic := configString(node.Config, "topic")
	fixture := workflowScriptFixtureForTopic(topic)
	nodeID := sanitizeID(node.ID)
	if nodeID == "" {
		nodeID = "generated-node"
	}
	switch node.Kind {
	case NodeKindSearch:
		outputID := firstWorkflowIOID(node.Outputs, fixture.ItemsOutputID, "items")
		return fmt.Sprintf(`import json
from datetime import datetime, timezone

items = %s
envelope = {
    "schema_version": "aiops.node_result/v1",
    "node_id": %q,
    "node_type": "script.python",
    "status": "success",
    "finished_at": datetime.now(timezone.utc).isoformat(),
    "outputs": {%q: items},
    "metrics": {"count": len(items)},
}
print("AIOPS_NODE_RESULT_BEGIN")
print(json.dumps(envelope, ensure_ascii=False))
print("AIOPS_NODE_RESULT_END")`, pythonLiteral(fixture.Items), nodeID, outputID)
	case NodeKindTransform:
		outputID := firstWorkflowIOID(node.Outputs, "key_news")
		return fmt.Sprintf(`import json
from datetime import datetime, timezone

source_items = %s
selected_items = source_items[:3]
envelope = {
    "schema_version": "aiops.node_result/v1",
    "node_id": %q,
    "node_type": "script.python",
    "status": "success",
    "finished_at": datetime.now(timezone.utc).isoformat(),
    "outputs": {%q: selected_items},
    "metrics": {"count": len(selected_items)},
}
print("AIOPS_NODE_RESULT_BEGIN")
print(json.dumps(envelope, ensure_ascii=False))
print("AIOPS_NODE_RESULT_END")`, pythonLiteral(fixture.KeyItems), nodeID, outputID)
	default:
		target := "return"
		if node.Config != nil {
			if raw, ok := node.Config["target"].(string); ok && raw != "" {
				target = raw
			}
		}
		return fmt.Sprintf(`import json
from datetime import datetime, timezone

key_items = %s
delivery_result = {"target": %q, "sent": %s, "items": key_items}
envelope = {
    "schema_version": "aiops.node_result/v1",
    "node_id": %q,
    "node_type": "script.python",
    "status": "success",
    "finished_at": datetime.now(timezone.utc).isoformat(),
    "outputs": {"delivery_result": delivery_result},
    "metrics": {"count": len(key_items)},
}
print("AIOPS_NODE_RESULT_BEGIN")
print(json.dumps(envelope, ensure_ascii=False))
print("AIOPS_NODE_RESULT_END")`, pythonLiteral(fixture.KeyItems), target, pyBool(target != "return"), nodeID)
	}
}

type workflowScriptFixture struct {
	ItemsOutputID string
	Items         []map[string]string
	KeyItems      []map[string]string
}

func workflowScriptFixtureForTopic(topic string) workflowScriptFixture {
	switch strings.TrimSpace(topic) {
	case "kubernetes-security":
		items := []map[string]string{
			{"title": "Kubernetes ingress-nginx 高危漏洞公告", "url": "https://example.invalid/k8s-ingress", "summary": "ingress 控制面组件存在可被利用的配置注入风险。", "impact": "暴露公网入口的集群", "priority": "高"},
			{"title": "容器镜像基础包 CVE 批量修复窗口", "url": "https://example.invalid/image-cve", "summary": "多个基础镜像发布安全更新，需要重建业务镜像。", "impact": "使用受影响基础镜像的工作负载", "priority": "中"},
			{"title": "Kubernetes API Server 审计规则加固建议", "url": "https://example.invalid/audit", "summary": "建议补充高风险资源变更审计与告警。", "impact": "生产集群管控面", "priority": "中"},
		}
		return workflowScriptFixture{ItemsOutputID: "security_items", Items: items, KeyItems: items}
	case "ops-incident":
		items := []map[string]string{
			{"title": "PostgreSQL 连接池耗尽复盘", "url": "https://example.invalid/postgres-pool", "summary": "突增慢查询占满连接池，导致业务接口超时。", "trigger": "连接数接近上限且慢查询堆积", "remediation": "临时扩容连接池并终止异常慢查询", "prevention": "为核心 SQL 建立基线与连接池水位告警"},
			{"title": "Redis 主从切换后读写不一致复盘", "url": "https://example.invalid/redis-failover", "summary": "故障切换期间客户端未刷新拓扑，部分请求打到旧主节点。", "trigger": "Sentinel failover 后错误率上升", "remediation": "重启受影响客户端并刷新连接配置", "prevention": "接入连接拓扑变更检测与重试退避"},
			{"title": "Kafka 消费积压导致告警风暴复盘", "url": "https://example.invalid/kafka-lag", "summary": "下游处理耗时变长，消费组 lag 快速扩大。", "trigger": "consumer lag 持续增长且处理耗时升高", "remediation": "临时扩容 consumer 并暂停低优先级 topic", "prevention": "按 topic 建立 lag、吞吐和处理耗时联合告警"},
		}
		return workflowScriptFixture{ItemsOutputID: "incident_items", Items: items, KeyItems: items}
	default:
		items := []map[string]string{
			{"title": "AI 基础设施投资持续增长", "url": "https://example.invalid/ai-infra", "summary": "云厂商继续扩大 AI 算力与平台投入。"},
			{"title": "企业开始用智能体自动化运营流程", "url": "https://example.invalid/agents", "summary": "更多企业把 AI Agent 接入日常运营流程。"},
			{"title": "模型成本优化成为应用落地重点", "url": "https://example.invalid/cost", "summary": "推理成本、缓存和小模型协同成为部署重点。"},
		}
		return workflowScriptFixture{ItemsOutputID: "news_items", Items: items, KeyItems: items}
	}
}

func configString(config map[string]any, key string) string {
	if config == nil {
		return ""
	}
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func firstWorkflowIOID(items []WorkflowIO, fallback ...string) string {
	for _, item := range items {
		if strings.TrimSpace(item.ID) != "" {
			return sanitizeID(item.ID)
		}
	}
	return firstNonEmpty(fallback...)
}

func pythonLiteral(value any) string {
	data, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		return "[]"
	}
	return string(data)
}

func pyBool(value bool) string {
	if value {
		return "True"
	}
	return "False"
}

func ioToVisualInputs(input []WorkflowIO) []visual.InputParamSpec {
	if len(input) == 0 {
		return nil
	}
	output := make([]visual.InputParamSpec, 0, len(input))
	for _, item := range input {
		output = append(output, visual.InputParamSpec{
			Key:         sanitizeID(item.ID),
			Type:        firstNonEmpty(item.Type, "any"),
			Label:       item.ID,
			Description: item.Description,
			Required:    item.Required,
		})
	}
	return output
}

func ioToVisualOutputs(input []WorkflowIO) []visual.OutputParamSpec {
	if len(input) == 0 {
		return nil
	}
	output := make([]visual.OutputParamSpec, 0, len(input))
	for _, item := range input {
		output = append(output, visual.OutputParamSpec{
			Key:         sanitizeID(item.ID),
			Type:        firstNonEmpty(item.Type, "any"),
			Label:       item.ID,
			Description: item.Description,
			Required:    item.Required,
			ExtractSource: visual.ExtractSource{
				Type: "jsonpath",
				Path: "$.outputs." + sanitizeID(item.ID),
			},
		})
	}
	return output
}

func workflowName(title, sessionID string) string {
	base := workflowTitleSlug(title)
	if base == "" {
		base = sanitizeID(title)
	}
	if base == "" {
		base = "workflow"
	}
	if sessionID != "" {
		return sanitizeID(base + "-" + sessionID)
	}
	return base
}

func workflowTitleSlug(title string) string {
	normalized := strings.ToLower(strings.TrimSpace(title))
	switch {
	case strings.Contains(normalized, "kubernetes") && strings.Contains(title, "安全"):
		return "kubernetes-security-risk"
	case strings.Contains(title, "数据库") && strings.Contains(title, "中间件"):
		return "db-middleware-incident-review"
	case strings.Contains(normalized, "ai") && strings.Contains(title, "新闻"):
		return "ai-news-summary"
	default:
		return ""
	}
}

var invalidIDChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func sanitizeID(input string) string {
	value := strings.Trim(strings.ToLower(input), " _-")
	value = invalidIDChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return ""
	}
	return value
}

func workflowGraphNodes(nodes []visual.Node) []workflow.GraphNodeSpec {
	output := make([]workflow.GraphNodeSpec, 0, len(nodes))
	for _, node := range nodes {
		spec := workflow.GraphNodeSpec{
			ID:       node.ID,
			Type:     string(node.Type),
			Position: workflow.GraphPosition{X: node.Position.X, Y: node.Position.Y},
			StepName: node.StepName,
			StepID:   node.StepID,
			Label:    node.Label,
			UI:       node.UI,
		}
		output = append(output, spec)
	}
	return output
}

func workflowGraphEdges(edges []visual.Edge) []workflow.GraphEdgeSpec {
	output := make([]workflow.GraphEdgeSpec, 0, len(edges))
	for _, edge := range edges {
		output = append(output, workflow.GraphEdgeSpec{
			ID:         edge.ID,
			Source:     edge.Source,
			SourcePort: edge.SourcePort,
			Target:     edge.Target,
			TargetPort: edge.TargetPort,
			Kind:       string(edge.Kind),
			Condition:  edge.Condition,
			UI:         edge.UI,
		})
	}
	return output
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
