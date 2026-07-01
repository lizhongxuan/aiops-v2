package opsgraph

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const manualGraphDefaultName = "新建图谱"

type GraphService struct {
	repo Repository
	now  func() time.Time
}

func NewGraphService(repo Repository) *GraphService {
	return &GraphService{repo: repo, now: time.Now}
}

func (s *GraphService) EnsureDefaultGraph(ctx context.Context) (GraphRecord, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphRecord{}, err
	}
	if graph, ok := doc.DefaultGraph(); ok {
		return graph, nil
	}
	now := s.now().UTC()
	graph := GraphRecord{ID: "graph.default", Name: "默认图谱", IsDefault: true, CreatedAt: now, UpdatedAt: now}
	doc.SchemaVersion = ManualGraphSchemaVersion
	doc.Graphs = append(doc.Graphs, graph)
	if err := s.repo.Save(ctx, doc); err != nil {
		return GraphRecord{}, err
	}
	return graph, nil
}

func (s *GraphService) ListGraphs(ctx context.Context) ([]GraphSummary, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return nil, err
	}
	return doc.Summaries(), nil
}

func (s *GraphService) GetGraph(ctx context.Context, graphID string) (GraphRecord, bool, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphRecord{}, false, err
	}
	if strings.TrimSpace(graphID) == "" {
		graph, ok := doc.DefaultGraph()
		return graph, ok, nil
	}
	graph, ok := doc.GraphByID(graphID)
	return graph, ok, nil
}

func (s *GraphService) CreateGraph(ctx context.Context, graph GraphRecord) (GraphRecord, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphRecord{}, err
	}
	graph.ID = strings.TrimSpace(graph.ID)
	graph.Name = strings.TrimSpace(graph.Name)
	if graph.Name == "" {
		return GraphRecord{}, errors.New("opsgraph: graph name is required")
	}
	if isManualDefaultGraphName(graph.Name) && graphNameExists(doc.Graphs, graph.Name) {
		graph.Name = uniqueDefaultGraphName(doc.Graphs, manualGraphDefaultName)
	}
	if graph.ID == "" {
		graph.ID = graphIDFromName(graph.Name)
	}
	for i := range doc.Graphs {
		if doc.Graphs[i].ID == graph.ID {
			return GraphRecord{}, fmt.Errorf("opsgraph: graph %s already exists", graph.ID)
		}
		if graph.IsDefault {
			doc.Graphs[i].IsDefault = false
		}
	}
	if len(doc.Graphs) == 0 {
		graph.IsDefault = true
	}
	now := s.now().UTC()
	graph.CreatedAt = now
	graph.UpdatedAt = now
	doc.SchemaVersion = ManualGraphSchemaVersion
	doc.Graphs = append(doc.Graphs, graph)
	return graph, s.repo.Save(ctx, doc)
}

func (s *GraphService) ExportGraphYAML(ctx context.Context, graphID string) ([]byte, bool, error) {
	graph, found, err := s.GetGraph(ctx, graphID)
	if err != nil || !found {
		return nil, found, err
	}
	clean := graphForYAML(graph)
	raw, err := yaml.Marshal(clean)
	if err != nil {
		return nil, true, err
	}
	return raw, true, nil
}

func (s *GraphService) ImportGraphYAML(ctx context.Context, graphID string, raw []byte) (GraphRecord, bool, error) {
	current, found, err := s.GetGraph(ctx, graphID)
	if err != nil || !found {
		return GraphRecord{}, found, err
	}
	var probe struct {
		Nodes *[]Node `yaml:"nodes"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return GraphRecord{}, true, fmt.Errorf("opsgraph: invalid yaml: %w", err)
	}
	if probe.Nodes == nil {
		return GraphRecord{}, true, errors.New("opsgraph: yaml must include nodes")
	}
	var incoming GraphRecord
	if err := yaml.Unmarshal(raw, &incoming); err != nil {
		return GraphRecord{}, true, fmt.Errorf("opsgraph: invalid yaml: %w", err)
	}
	incoming.ID = current.ID
	if strings.TrimSpace(incoming.Name) == "" {
		incoming.Name = current.Name
	}
	if incoming.Nodes == nil {
		incoming.Nodes = []Node{}
	}
	if incoming.Edges == nil {
		incoming.Edges = []Edge{}
	}
	if err := validateImportedGraph(incoming); err != nil {
		return GraphRecord{}, true, err
	}
	updated, found, err := s.UpdateGraph(ctx, current.ID, incoming)
	if err != nil || !found {
		return GraphRecord{}, found, err
	}
	return updated, true, nil
}

func (s *GraphService) UpdateGraph(ctx context.Context, graphID string, next GraphRecord) (GraphRecord, bool, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphRecord{}, false, err
	}
	for i := range doc.Graphs {
		if doc.Graphs[i].ID != graphID {
			continue
		}
		updated := doc.Graphs[i]
		if strings.TrimSpace(next.Name) != "" {
			updated.Name = strings.TrimSpace(next.Name)
		}
		updated.Description = strings.TrimSpace(next.Description)
		updated.Environment = strings.TrimSpace(next.Environment)
		if next.Nodes != nil {
			updated.Nodes = next.Nodes
		}
		if next.Edges != nil {
			updated.Edges = next.Edges
		}
		if next.Viewport != nil {
			updated.Viewport = next.Viewport
		}
		if next.IsDefault {
			for j := range doc.Graphs {
				doc.Graphs[j].IsDefault = false
			}
			updated.IsDefault = true
		}
		if !updated.IsDefault && countDefaultGraphs(doc.Graphs) == 0 {
			updated.IsDefault = true
		}
		updated.UpdatedAt = s.now().UTC()
		doc.Graphs[i] = updated
		return updated, true, s.repo.Save(ctx, doc)
	}
	return GraphRecord{}, false, nil
}

func (s *GraphService) DuplicateGraph(ctx context.Context, graphID string) (GraphRecord, bool, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphRecord{}, false, err
	}
	sourceIndex := -1
	for i := range doc.Graphs {
		if doc.Graphs[i].ID == graphID {
			sourceIndex = i
			break
		}
	}
	if sourceIndex < 0 {
		return GraphRecord{}, false, nil
	}
	existing := map[string]bool{}
	for _, graph := range doc.Graphs {
		existing[graph.ID] = true
	}
	copyGraph := doc.Graphs[sourceIndex]
	copyGraph.ID = uniqueGraphID(copyGraph.ID+".copy", existing)
	copyGraph.Name = firstNonEmpty(copyGraph.Name, copyGraph.ID) + " 副本"
	copyGraph.IsDefault = false
	now := s.now().UTC()
	copyGraph.CreatedAt = now
	copyGraph.UpdatedAt = now
	doc.Graphs = append(doc.Graphs, copyGraph)
	return copyGraph, true, s.repo.Save(ctx, doc)
}

func (s *GraphService) DeleteGraph(ctx context.Context, graphID string) (bool, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return false, err
	}
	removedDefault := false
	kept := make([]GraphRecord, 0, len(doc.Graphs))
	found := false
	for _, graph := range doc.Graphs {
		if graph.ID == graphID {
			found = true
			removedDefault = graph.IsDefault
			continue
		}
		kept = append(kept, graph)
	}
	if !found {
		return false, nil
	}
	if removedDefault && len(kept) > 0 {
		kept[0].IsDefault = true
	}
	doc.Graphs = kept
	return true, s.repo.Save(ctx, doc)
}

func (s *GraphService) CreateNode(ctx context.Context, graphID string, node Node) (Node, error) {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return Node{}, err
	}
	node.ID = strings.TrimSpace(node.ID)
	node.Name = strings.TrimSpace(node.Name)
	if node.ID == "" || node.Name == "" || node.Type == "" {
		return Node{}, errors.New("opsgraph: node id, name and type are required")
	}
	for _, existing := range graph.Nodes {
		if existing.ID == node.ID {
			return Node{}, fmt.Errorf("opsgraph: node %s already exists", node.ID)
		}
	}
	now := s.now().UTC()
	node.CreatedAt = now
	node.UpdatedAt = now
	graph.Nodes = append(graph.Nodes, node)
	graph.UpdatedAt = now
	doc.Graphs[index] = graph
	return node, s.repo.Save(ctx, doc)
}

func (s *GraphService) UpdateNode(ctx context.Context, graphID, nodeID string, next Node) (Node, bool, error) {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return Node{}, false, err
	}
	for i := range graph.Nodes {
		if graph.Nodes[i].ID != nodeID {
			continue
		}
		updated := next
		updated.ID = nodeID
		updated.CreatedAt = graph.Nodes[i].CreatedAt
		updated.UpdatedAt = s.now().UTC()
		if strings.TrimSpace(updated.Name) == "" {
			updated.Name = graph.Nodes[i].Name
		}
		if updated.Type == "" {
			updated.Type = graph.Nodes[i].Type
		}
		graph.Nodes[i] = updated
		graph.UpdatedAt = updated.UpdatedAt
		doc.Graphs[index] = graph
		return updated, true, s.repo.Save(ctx, doc)
	}
	return Node{}, false, nil
}

func (s *GraphService) CreateEdge(ctx context.Context, graphID string, edge Edge) (Edge, error) {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return Edge{}, err
	}
	edge.ID = strings.TrimSpace(edge.ID)
	if edge.ID == "" || edge.From == "" || edge.To == "" || edge.Type == "" {
		return Edge{}, errors.New("opsgraph: edge id, from, to and type are required")
	}
	nodeIDs := map[string]bool{}
	for _, node := range graph.Nodes {
		nodeIDs[node.ID] = true
	}
	if !nodeIDs[edge.From] || !nodeIDs[edge.To] {
		return Edge{}, errors.New("opsgraph: edge endpoint is missing")
	}
	for _, existing := range graph.Edges {
		if existing.ID == edge.ID {
			return Edge{}, fmt.Errorf("opsgraph: edge %s already exists", edge.ID)
		}
	}
	now := s.now().UTC()
	edge.CreatedAt = now
	edge.UpdatedAt = now
	graph.Edges = append(graph.Edges, edge)
	graph.UpdatedAt = now
	doc.Graphs[index] = graph
	return edge, s.repo.Save(ctx, doc)
}

func (s *GraphService) CreateRelationship(ctx context.Context, graphID string, edge Edge) (Edge, error) {
	return s.CreateEdge(ctx, graphID, edge)
}

func (s *GraphService) UpdateRelationship(ctx context.Context, graphID, edgeID string, next Edge) (Edge, bool, error) {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return Edge{}, false, err
	}
	nodeIDs := map[string]bool{}
	for _, node := range graph.Nodes {
		nodeIDs[node.ID] = true
	}
	for i := range graph.Edges {
		if graph.Edges[i].ID != edgeID {
			continue
		}
		updated := next
		updated.ID = edgeID
		if strings.TrimSpace(updated.From) == "" {
			updated.From = graph.Edges[i].From
		}
		if strings.TrimSpace(updated.To) == "" {
			updated.To = graph.Edges[i].To
		}
		if updated.Type == "" {
			updated.Type = graph.Edges[i].Type
		}
		if !nodeIDs[updated.From] || !nodeIDs[updated.To] {
			return Edge{}, false, errors.New("opsgraph: edge endpoint is missing")
		}
		updated.CreatedAt = graph.Edges[i].CreatedAt
		updated.UpdatedAt = s.now().UTC()
		graph.Edges[i] = updated
		graph.UpdatedAt = updated.UpdatedAt
		doc.Graphs[index] = graph
		return updated, true, s.repo.Save(ctx, doc)
	}
	return Edge{}, false, nil
}

func (s *GraphService) DeleteRelationship(ctx context.Context, graphID, edgeID string) (bool, error) {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return false, err
	}
	kept := make([]Edge, 0, len(graph.Edges))
	found := false
	for _, edge := range graph.Edges {
		if edge.ID == edgeID {
			found = true
			continue
		}
		kept = append(kept, edge)
	}
	if !found {
		return false, nil
	}
	graph.Edges = kept
	graph.UpdatedAt = s.now().UTC()
	doc.Graphs[index] = graph
	return true, s.repo.Save(ctx, doc)
}

func (s *GraphService) DeleteNode(ctx context.Context, graphID, nodeID string, cascade bool) error {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return err
	}
	var keptEdges []Edge
	for _, edge := range graph.Edges {
		if edge.From == nodeID || edge.To == nodeID {
			if !cascade {
				return fmt.Errorf("opsgraph: node %s is referenced by edge %s", nodeID, edge.ID)
			}
			continue
		}
		keptEdges = append(keptEdges, edge)
	}
	var keptNodes []Node
	for _, node := range graph.Nodes {
		if node.ID != nodeID {
			keptNodes = append(keptNodes, node)
		}
	}
	graph.Nodes = keptNodes
	graph.Edges = keptEdges
	graph.UpdatedAt = s.now().UTC()
	doc.Graphs[index] = graph
	return s.repo.Save(ctx, doc)
}

func (s *GraphService) SaveLayout(ctx context.Context, graphID string, nodes []Node, viewport *Viewport) error {
	doc, graph, index, err := s.loadGraphForWrite(ctx, graphID)
	if err != nil {
		return err
	}
	layout := map[string]Node{}
	for _, node := range nodes {
		layout[node.ID] = node
	}
	for i := range graph.Nodes {
		if next, ok := layout[graph.Nodes[i].ID]; ok {
			graph.Nodes[i].Position = next.Position
			graph.Nodes[i].Collapsed = next.Collapsed
		}
	}
	graph.Viewport = viewport
	graph.UpdatedAt = s.now().UTC()
	doc.Graphs[index] = graph
	return s.repo.Save(ctx, doc)
}

func (s *GraphService) Neighborhood(ctx context.Context, graphID, entityID string, depth int) (Neighborhood, bool, error) {
	graph, ok, err := s.GetGraph(ctx, graphID)
	if err != nil || !ok {
		return Neighborhood{}, false, err
	}
	store := CompileGraphStore(graph)
	out := store.Neighborhood(entityID, depth)
	return out, strings.TrimSpace(out.Root.ID) != "", nil
}

func (s *GraphService) Lookup(ctx context.Context, graphID string, req LookupRequest) ([]Entity, error) {
	graph, ok, err := s.GetGraph(ctx, graphID)
	if err != nil || !ok {
		return nil, err
	}
	return CompileGraphStore(graph).Lookup(req), nil
}

func (s *GraphService) BusinessImpact(ctx context.Context, graphID, entityID string) (BusinessImpact, bool, error) {
	graph, ok, err := s.GetGraph(ctx, graphID)
	if err != nil || !ok {
		return BusinessImpact{}, false, err
	}
	impact := CompileGraphStore(graph).BusinessImpact(entityID)
	return impact, strings.TrimSpace(impact.Entity.ID) != "", nil
}

func (s *GraphService) Validate(ctx context.Context, graphID string) ([]ValidationIssue, bool, error) {
	graph, ok, err := s.GetGraph(ctx, graphID)
	if err != nil || !ok {
		return nil, ok, err
	}
	return ValidateGraph(graph), true, nil
}

func (s *GraphService) loadGraphForWrite(ctx context.Context, graphID string) (GraphDocument, GraphRecord, int, error) {
	doc, err := s.repo.Load(ctx)
	if err != nil {
		return GraphDocument{}, GraphRecord{}, -1, err
	}
	for i, graph := range doc.Graphs {
		if graph.ID == graphID || (graphID == "" && graph.IsDefault) {
			return doc, graph, i, nil
		}
	}
	return GraphDocument{}, GraphRecord{}, -1, fmt.Errorf("opsgraph: graph %s not found", graphID)
}

func graphIDFromName(name string) string {
	id := strings.ToLower(strings.TrimSpace(name))
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "/", "-")
	if id == "" {
		return "graph.default"
	}
	if strings.HasPrefix(id, "graph.") {
		return id
	}
	return "graph." + id
}

func countDefaultGraphs(graphs []GraphRecord) int {
	count := 0
	for _, graph := range graphs {
		if graph.IsDefault {
			count++
		}
	}
	return count
}

func graphNameExists(graphs []GraphRecord, name string) bool {
	name = strings.TrimSpace(name)
	for _, graph := range graphs {
		if strings.TrimSpace(graph.Name) == name {
			return true
		}
	}
	return false
}

func isManualDefaultGraphName(name string) bool {
	name = strings.TrimSpace(name)
	if name == manualGraphDefaultName {
		return true
	}
	if !strings.HasPrefix(name, manualGraphDefaultName+"-") {
		return false
	}
	suffix := strings.TrimPrefix(name, manualGraphDefaultName+"-")
	number, err := strconv.Atoi(suffix)
	return err == nil && number > 1 && strconv.Itoa(number) == suffix
}

func uniqueGraphID(base string, existing map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "graph.copy"
	}
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !existing[candidate] {
			return candidate
		}
	}
}

func uniqueDefaultGraphName(graphs []GraphRecord, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = manualGraphDefaultName
	}
	maxSuffix := 0
	prefix := base + "-"
	for _, graph := range graphs {
		name := strings.TrimSpace(graph.Name)
		if name == base {
			if maxSuffix < 1 {
				maxSuffix = 1
			}
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
		if err == nil && suffix > maxSuffix {
			maxSuffix = suffix
		}
	}
	if maxSuffix == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, maxSuffix+1)
}

func graphForYAML(graph GraphRecord) GraphRecord {
	graph.CreatedAt = time.Time{}
	graph.UpdatedAt = time.Time{}
	if graph.Nodes == nil {
		graph.Nodes = []Node{}
	}
	if graph.Edges == nil {
		graph.Edges = []Edge{}
	}
	for i := range graph.Nodes {
		graph.Nodes[i].CreatedAt = time.Time{}
		graph.Nodes[i].UpdatedAt = time.Time{}
	}
	for i := range graph.Edges {
		graph.Edges[i].CreatedAt = time.Time{}
		graph.Edges[i].UpdatedAt = time.Time{}
	}
	return graph
}

func validateImportedGraph(graph GraphRecord) error {
	issues := ValidateGraph(graph)
	var codes []string
	for _, issue := range issues {
		if issue.Level == "error" {
			codes = append(codes, issue.Code)
		}
	}
	if len(codes) == 0 {
		return nil
	}
	return fmt.Errorf("opsgraph: invalid yaml graph: %s", strings.Join(codes, ", "))
}
