package opsgraph

import "testing"

func TestValidateGraphRejectsDuplicateNodeBrokenEdgeAndClusterWithoutInstances(t *testing.T) {
	graph := GraphRecord{
		ID:        "graph.default",
		Name:      "默认图谱",
		IsDefault: true,
		Nodes: []Node{
			{ID: "service.order-api", Type: NodeService, Name: "order-api"},
			{ID: "service.order-api", Type: NodeService, Name: "order-api duplicate"},
			{ID: "middleware.pg", Type: NodeMiddlewareCluster, Name: "pg"},
		},
		Edges: []Edge{
			{ID: "e1", From: "service.order-api", Type: RelDependsOn, To: "missing.node"},
			{ID: "e1", From: "service.order-api", Type: RelDependsOn, To: "middleware.pg"},
		},
	}

	issues := ValidateGraph(graph)
	wantCodes := []string{"duplicate_node_id", "duplicate_edge_id", "missing_edge_target", "cluster_without_instances"}
	for _, code := range wantCodes {
		if !hasIssueCode(issues, code) {
			t.Fatalf("ValidateGraph() issues = %#v, missing %s", issues, code)
		}
	}
}

func TestValidateDocumentRequiresOneDefaultGraph(t *testing.T) {
	doc := GraphDocument{
		SchemaVersion: ManualGraphSchemaVersion,
		Graphs: []GraphRecord{
			{ID: "graph.one", Name: "one", IsDefault: true},
			{ID: "graph.two", Name: "two", IsDefault: true},
		},
	}

	issues := ValidateDocument(doc)
	if !hasIssueCode(issues, "multiple_default_graphs") {
		t.Fatalf("ValidateDocument() = %#v, want multiple_default_graphs", issues)
	}
}

func TestValidateGraphAllowsServiceTopologyRelationships(t *testing.T) {
	graph := GraphRecord{
		ID:   "graph.default",
		Name: "服务拓扑",
		Nodes: []Node{
			{ID: "service.order-api", Type: NodeService, Name: "order-api", Properties: map[string]string{"owner": "platform-sre"}},
			{ID: "middleware.redis", Type: NodeMiddleware, Subtype: "redis", Name: "redis-cache"},
			{ID: "external.sms", Type: NodeExternal, Name: "sms-provider"},
		},
		Edges: []Edge{
			{ID: "edge.calls", From: "service.order-api", Type: RelCalls, To: "external.sms"},
			{ID: "edge.depends", From: "service.order-api", Type: RelDependsOn, To: "middleware.redis"},
			{ID: "edge.publishes", From: "service.order-api", Type: RelPublishes, To: "middleware.redis"},
			{ID: "edge.consumes", From: "service.order-api", Type: RelConsumes, To: "middleware.redis"},
			{ID: "edge.proxies", From: "service.order-api", Type: RelProxiesTo, To: "external.sms"},
		},
	}

	if issues := ValidateGraph(graph); len(errorIssues(issues)) != 0 {
		t.Fatalf("ValidateGraph() errors = %#v, want none", issues)
	}
}

func TestValidateGraphDefaultsMiddlewareSubtypeWarningFree(t *testing.T) {
	graph := GraphRecord{
		ID:    "graph.default",
		Name:  "服务拓扑",
		Nodes: []Node{{ID: "middleware.generic", Type: NodeMiddleware, Name: "generic-middleware"}},
		Edges: []Edge{},
	}

	if issues := ValidateGraph(graph); len(errorIssues(issues)) != 0 {
		t.Fatalf("ValidateGraph() errors = %#v, want none", issues)
	}
}

func errorIssues(issues []ValidationIssue) []ValidationIssue {
	var out []ValidationIssue
	for _, issue := range issues {
		if issue.Level == "error" {
			out = append(out, issue)
		}
	}
	return out
}

func hasIssueCode(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
