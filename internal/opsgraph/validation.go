package opsgraph

import "strings"

type ValidationIssue struct {
	Code     string `json:"code"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	GraphID  string `json:"graphId,omitempty"`
	NodeID   string `json:"nodeId,omitempty"`
	EdgeID   string `json:"edgeId,omitempty"`
	Relation string `json:"relation,omitempty"`
}

func ValidateDocument(doc GraphDocument) []ValidationIssue {
	var issues []ValidationIssue
	graphIDs := map[string]bool{}
	defaultCount := 0
	for _, graph := range doc.Graphs {
		id := strings.TrimSpace(graph.ID)
		if id == "" {
			issues = append(issues, ValidationIssue{Code: "missing_graph_id", Level: "error", Message: "图谱缺少 ID"})
		}
		if graphIDs[id] {
			issues = append(issues, ValidationIssue{Code: "duplicate_graph_id", Level: "error", Message: "图谱 ID 重复", GraphID: id})
		}
		graphIDs[id] = true
		if graph.IsDefault {
			defaultCount++
		}
		issues = append(issues, ValidateGraph(graph)...)
	}
	if len(doc.Graphs) > 0 && defaultCount == 0 {
		issues = append(issues, ValidationIssue{Code: "missing_default_graph", Level: "error", Message: "需要设置一个默认图谱"})
	}
	if defaultCount > 1 {
		issues = append(issues, ValidationIssue{Code: "multiple_default_graphs", Level: "error", Message: "只能有一个默认图谱"})
	}
	return issues
}

func ValidateGraph(graph GraphRecord) []ValidationIssue {
	var issues []ValidationIssue
	nodeIDs := map[string]Node{}
	edgeIDs := map[string]bool{}
	clusterMemberCount := map[string]int{}
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			issues = append(issues, ValidationIssue{Code: "missing_node_id", Level: "error", Message: "节点缺少 ID", GraphID: graph.ID})
			continue
		}
		if _, exists := nodeIDs[id]; exists {
			issues = append(issues, ValidationIssue{Code: "duplicate_node_id", Level: "error", Message: "节点 ID 重复", GraphID: graph.ID, NodeID: id})
		}
		if strings.TrimSpace(node.Name) == "" {
			issues = append(issues, ValidationIssue{Code: "missing_node_name", Level: "error", Message: "节点缺少名称", GraphID: graph.ID, NodeID: id})
		}
		if node.Type == NodeService && strings.TrimSpace(node.Labels["owner"]) == "" {
			issues = append(issues, ValidationIssue{Code: "service_missing_owner", Level: "warning", Message: "服务建议补充 owner", GraphID: graph.ID, NodeID: id})
		}
		nodeIDs[id] = node
		if node.ParentID != "" {
			clusterMemberCount[node.ParentID]++
		}
	}
	for _, edge := range graph.Edges {
		id := strings.TrimSpace(edge.ID)
		if id == "" {
			issues = append(issues, ValidationIssue{Code: "missing_edge_id", Level: "error", Message: "关系缺少 ID", GraphID: graph.ID})
			continue
		}
		if edgeIDs[id] {
			issues = append(issues, ValidationIssue{Code: "duplicate_edge_id", Level: "error", Message: "关系 ID 重复", GraphID: graph.ID, EdgeID: id})
		}
		edgeIDs[id] = true
		if _, ok := nodeIDs[strings.TrimSpace(edge.From)]; !ok {
			issues = append(issues, ValidationIssue{Code: "missing_edge_source", Level: "error", Message: "关系源节点不存在", GraphID: graph.ID, EdgeID: id})
		}
		if _, ok := nodeIDs[strings.TrimSpace(edge.To)]; !ok {
			issues = append(issues, ValidationIssue{Code: "missing_edge_target", Level: "error", Message: "关系目标节点不存在", GraphID: graph.ID, EdgeID: id})
		}
		if edge.Type == "" {
			issues = append(issues, ValidationIssue{Code: "missing_edge_type", Level: "error", Message: "关系缺少类型", GraphID: graph.ID, EdgeID: id})
		}
		if edge.Type == RelContains {
			clusterMemberCount[edge.From]++
		}
	}
	for _, node := range graph.Nodes {
		if node.Type == NodeMiddlewareCluster && clusterMemberCount[node.ID] == 0 {
			issues = append(issues, ValidationIssue{Code: "cluster_without_instances", Level: "warning", Message: "中间件集群还没有实例", GraphID: graph.ID, NodeID: node.ID})
		}
	}
	return issues
}
