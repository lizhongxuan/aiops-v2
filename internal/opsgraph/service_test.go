package opsgraph

import (
	"context"
	"path/filepath"
	"strings"
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

func TestGraphServiceSuffixesDefaultGraphNames(t *testing.T) {
	ctx := context.Background()
	service := NewGraphService(NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json")))

	first, err := service.CreateGraph(ctx, GraphRecord{ID: "graph.manual-1", Name: "新建图谱"})
	if err != nil {
		t.Fatalf("CreateGraph(first) error = %v", err)
	}
	second, err := service.CreateGraph(ctx, GraphRecord{ID: "graph.manual-2", Name: "新建图谱"})
	if err != nil {
		t.Fatalf("CreateGraph(second) error = %v", err)
	}
	third, err := service.CreateGraph(ctx, GraphRecord{ID: "graph.manual-3", Name: "新建图谱"})
	if err != nil {
		t.Fatalf("CreateGraph(third) error = %v", err)
	}

	if first.Name != "新建图谱" || second.Name != "新建图谱-2" || third.Name != "新建图谱-3" {
		t.Fatalf("graph names = %q, %q, %q; want suffixes", first.Name, second.Name, third.Name)
	}
}

func TestGraphServiceImportsAndExportsGraphYAML(t *testing.T) {
	ctx := context.Background()
	service := NewGraphService(NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json")))
	graph, err := service.CreateGraph(ctx, GraphRecord{ID: "graph.import", Name: "导入前", Nodes: []Node{}, Edges: []Edge{}})
	if err != nil {
		t.Fatalf("CreateGraph() error = %v", err)
	}

	imported, found, err := service.ImportGraphYAML(ctx, graph.ID, []byte(strings.TrimSpace(`
id: ignored-from-file
name: YAML 图谱
environment: prod
nodes:
  - id: service.checkout
    type: service
    name: checkout-api
    properties:
      ports: 8080/http
  - id: middleware.redis
    type: middleware
    subtype: redis
    name: checkout-redis
edges:
  - id: edge.checkout-redis
    from: service.checkout
    type: depends_on
    to: middleware.redis
viewport:
  x: 10
  y: 20
  zoom: 0.8
`)))
	if err != nil || !found {
		t.Fatalf("ImportGraphYAML() found=%v err=%v", found, err)
	}
	if imported.ID != graph.ID || imported.Name != "YAML 图谱" || len(imported.Nodes) != 2 || len(imported.Edges) != 1 {
		t.Fatalf("imported graph = %#v, want current id and YAML contents", imported)
	}

	exported, found, err := service.ExportGraphYAML(ctx, graph.ID)
	if err != nil || !found {
		t.Fatalf("ExportGraphYAML() found=%v err=%v", found, err)
	}
	exportedText := string(exported)
	for _, want := range []string{"name: YAML 图谱", "environment: prod", "id: service.checkout", "type: depends_on"} {
		if !strings.Contains(exportedText, want) {
			t.Fatalf("exported yaml = %s, want %q", exportedText, want)
		}
	}
	if strings.Contains(exportedText, "createdAt:") || strings.Contains(exportedText, "updatedAt:") {
		t.Fatalf("exported yaml = %s, should omit volatile timestamps", exportedText)
	}
}

func TestGraphServiceRejectsInvalidGraphYAML(t *testing.T) {
	ctx := context.Background()
	service := NewGraphService(NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json")))
	graph, err := service.CreateGraph(ctx, GraphRecord{ID: "graph.invalid-import", Name: "导入前", Nodes: []Node{}, Edges: []Edge{}})
	if err != nil {
		t.Fatalf("CreateGraph() error = %v", err)
	}

	if _, _, err := service.ImportGraphYAML(ctx, graph.ID, []byte("name: broken\nedges: []\n")); err == nil {
		t.Fatal("ImportGraphYAML(missing nodes) error = nil, want validation error")
	}
	if _, _, err := service.ImportGraphYAML(ctx, graph.ID, []byte(strings.TrimSpace(`
name: broken
nodes:
  - id: service.a
    type: service
    name: a
edges:
  - id: edge.missing
    from: service.a
    type: depends_on
    to: service.missing
`))); err == nil {
		t.Fatal("ImportGraphYAML(missing edge target) error = nil, want validation error")
	}
}
