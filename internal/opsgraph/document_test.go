package opsgraph

import "testing"

func TestGraphDocumentCompileStoreSupportsManualGraph(t *testing.T) {
	graph := GraphRecord{
		ID:        "graph.default",
		Name:      "默认图谱",
		IsDefault: true,
		Nodes: []Node{
			{ID: "service.order-api", Type: NodeService, Name: "order-api", Aliases: []string{"订单服务"}},
			{ID: "middleware.order-postgres", Type: NodeMiddlewareCluster, Name: "order-postgres", Properties: map[string]string{"subtype": "postgres"}},
			{ID: "middleware.order-postgres-0", Type: NodeMiddlewareInstance, Name: "order-postgres-0", ParentID: "middleware.order-postgres", Properties: map[string]string{"role": "primary"}},
			{ID: "host.erp-node-a", Type: NodeHost, Name: "erp-node-a", Container: true},
			{ID: "business.order-submit", Type: NodeBusiness, Name: "订单提交"},
			{ID: "workflow.order-restart", Type: NodeWorkflow, Name: "订单服务重启 Workflow"},
		},
		Edges: []Edge{
			{ID: "e1", From: "service.order-api", Type: RelDependsOn, To: "middleware.order-postgres"},
			{ID: "e2", From: "middleware.order-postgres", Type: RelContains, To: "middleware.order-postgres-0"},
			{ID: "e3", From: "middleware.order-postgres-0", Type: RelRunsOn, To: "host.erp-node-a"},
			{ID: "e4", From: "service.order-api", Type: RelAffects, To: "business.order-submit"},
			{ID: "e5", From: "service.order-api", Type: RelHandledBy, To: "workflow.order-restart"},
		},
	}

	store := CompileGraphStore(graph)
	matches := store.Lookup(LookupRequest{Query: "订单服务", Limit: 5})
	if len(matches) == 0 || matches[0].ID != "service.order-api" {
		t.Fatalf("Lookup() = %#v, want service.order-api", matches)
	}

	neighbors := store.Neighborhood("service.order-api", 3)
	if len(neighbors.Entities) < 6 {
		t.Fatalf("Neighborhood() entities = %d, want service, cluster, instance, host, business and workflow", len(neighbors.Entities))
	}

	impact := store.BusinessImpact("service.order-api")
	if len(impact.Capabilities) == 0 {
		t.Fatalf("BusinessImpact() = %#v, want business capability impact", impact)
	}

	runbooks := store.RelatedRunbooks("service.order-api")
	if len(runbooks) != 1 || runbooks[0].Runbook.ID != "workflow.order-restart" {
		t.Fatalf("RelatedRunbooks() = %#v, want workflow match", runbooks)
	}
}

func TestGraphSummaryCountsNodesEdgesAndValidationIssues(t *testing.T) {
	doc := GraphDocument{
		SchemaVersion: ManualGraphSchemaVersion,
		Graphs: []GraphRecord{{
			ID:        "graph.default",
			Name:      "默认图谱",
			IsDefault: true,
			Nodes: []Node{
				{ID: "service.order-api", Type: NodeService, Name: "order-api"},
				{ID: "host.erp-node-a", Type: NodeHost, Name: "erp-node-a", Container: true},
			},
			Edges: []Edge{{ID: "e1", From: "service.order-api", Type: RelRunsOn, To: "host.erp-node-a"}},
		}},
	}

	summaries := doc.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("Summaries() len = %d, want 1", len(summaries))
	}
	if summaries[0].NodeCount != 2 || summaries[0].RelationshipCount != 1 || !summaries[0].IsDefault {
		t.Fatalf("summary = %#v, want counts and default flag", summaries[0])
	}
}
