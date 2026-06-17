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

func hasIssueCode(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
