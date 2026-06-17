package opsgraph

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGraphServiceCreatesDefaultGraphNodesEdgesAndLayout(t *testing.T) {
	ctx := context.Background()
	service := NewGraphService(NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json")))

	graph, err := service.EnsureDefaultGraph(ctx)
	if err != nil {
		t.Fatalf("EnsureDefaultGraph() error = %v", err)
	}
	if graph.ID == "" || !graph.IsDefault {
		t.Fatalf("default graph = %#v, want default id", graph)
	}

	serviceNode, err := service.CreateNode(ctx, graph.ID, Node{ID: "service.order-api", Type: NodeService, Name: "order-api"})
	if err != nil {
		t.Fatalf("CreateNode(service) error = %v", err)
	}
	hostNode, err := service.CreateNode(ctx, graph.ID, Node{ID: "host.erp-node-a", Type: NodeHost, Name: "erp-node-a", Container: true})
	if err != nil {
		t.Fatalf("CreateNode(host) error = %v", err)
	}
	if _, err := service.CreateEdge(ctx, graph.ID, Edge{ID: "e1", From: serviceNode.ID, Type: RelRunsOn, To: hostNode.ID}); err != nil {
		t.Fatalf("CreateEdge() error = %v", err)
	}
	if err := service.SaveLayout(ctx, graph.ID, []Node{{ID: serviceNode.ID, Position: &Position{X: 10, Y: 20}}}, &Viewport{X: 1, Y: 2, Zoom: 0.8}); err != nil {
		t.Fatalf("SaveLayout() error = %v", err)
	}
	neighborhood, found, err := service.Neighborhood(ctx, graph.ID, serviceNode.ID, 1)
	if err != nil || !found {
		t.Fatalf("Neighborhood() found=%v err=%v", found, err)
	}
	if len(neighborhood.Entities) != 2 {
		t.Fatalf("entities = %d, want service and host", len(neighborhood.Entities))
	}
	loaded, found, err := service.GetGraph(ctx, graph.ID)
	if err != nil || !found {
		t.Fatalf("GetGraph() found=%v err=%v", found, err)
	}
	if loaded.Viewport == nil || loaded.Viewport.Zoom != 0.8 {
		t.Fatalf("viewport = %#v, want saved viewport", loaded.Viewport)
	}
}

func TestGraphServiceRejectsDeleteReferencedNodeWithoutCascade(t *testing.T) {
	ctx := context.Background()
	service := NewGraphService(NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json")))
	graph, _ := service.EnsureDefaultGraph(ctx)
	_, _ = service.CreateNode(ctx, graph.ID, Node{ID: "service.order-api", Type: NodeService, Name: "order-api"})
	_, _ = service.CreateNode(ctx, graph.ID, Node{ID: "host.erp-node-a", Type: NodeHost, Name: "erp-node-a"})
	_, _ = service.CreateEdge(ctx, graph.ID, Edge{ID: "e1", From: "service.order-api", Type: RelRunsOn, To: "host.erp-node-a"})

	err := service.DeleteNode(ctx, graph.ID, "host.erp-node-a", false)
	if err == nil {
		t.Fatal("DeleteNode(cascade=false) error = nil, want reference error")
	}
	if err := service.DeleteNode(ctx, graph.ID, "host.erp-node-a", true); err != nil {
		t.Fatalf("DeleteNode(cascade=true) error = %v", err)
	}
}
