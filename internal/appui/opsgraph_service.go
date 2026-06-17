package appui

import (
	"context"
	"path/filepath"
	"strings"

	"aiops-v2/internal/opsgraph"
)

type OpsGraphEntityView = opsgraph.Entity
type OpsGraphRelationshipView = opsgraph.Relationship
type OpsGraphLookupCommand = opsgraph.LookupRequest
type OpsGraphBusinessImpactView = opsgraph.BusinessImpact

type OpsGraphNeighborhoodView struct {
	Entity        opsgraph.Entity         `json:"entity"`
	Depth         int                     `json:"depth"`
	Neighbors     []map[string]any        `json:"neighbors"`
	Entities      []opsgraph.Entity       `json:"entities"`
	Relationships []opsgraph.Relationship `json:"relationships"`
}

type OpsGraphService interface {
	ListGraphs(ctx context.Context) ([]opsgraph.GraphSummary, error)
	CreateGraph(ctx context.Context, graph opsgraph.GraphRecord) (opsgraph.GraphRecord, error)
	GetGraph(ctx context.Context, graphID string) (opsgraph.GraphRecord, bool, error)
	UpdateGraph(ctx context.Context, graphID string, graph opsgraph.GraphRecord) (opsgraph.GraphRecord, bool, error)
	ExportGraphYAML(ctx context.Context, graphID string) ([]byte, bool, error)
	ImportGraphYAML(ctx context.Context, graphID string, raw []byte) (opsgraph.GraphRecord, bool, error)
	DuplicateGraph(ctx context.Context, graphID string) (opsgraph.GraphRecord, bool, error)
	DeleteGraph(ctx context.Context, graphID string) (bool, error)
	CreateNode(ctx context.Context, graphID string, node opsgraph.Node) (opsgraph.Node, error)
	UpdateNode(ctx context.Context, graphID, nodeID string, node opsgraph.Node) (opsgraph.Node, bool, error)
	DeleteNode(ctx context.Context, graphID, nodeID string, cascade bool) (bool, error)
	CreateRelationship(ctx context.Context, graphID string, edge opsgraph.Edge) (opsgraph.Edge, error)
	UpdateRelationship(ctx context.Context, graphID, edgeID string, edge opsgraph.Edge) (opsgraph.Edge, bool, error)
	DeleteRelationship(ctx context.Context, graphID, edgeID string) (bool, error)
	SaveLayout(ctx context.Context, graphID string, nodes []opsgraph.Node, viewport *opsgraph.Viewport) error
	Validate(ctx context.Context, graphID string) ([]opsgraph.ValidationIssue, bool, error)
	Lookup(ctx context.Context, graphID string, cmd OpsGraphLookupCommand) ([]OpsGraphEntityView, error)
	Neighborhood(ctx context.Context, graphID, entityID string, depth int) (OpsGraphNeighborhoodView, bool, error)
	BusinessImpact(ctx context.Context, graphID, entityID string) (OpsGraphBusinessImpactView, bool, error)
}

type defaultOpsGraphService struct {
	graphs *opsgraph.GraphService
}

func NewOpsGraphService(storagePath string) OpsGraphService {
	if strings.TrimSpace(storagePath) == "" {
		storagePath = projectRelativePath(filepath.Join("data", "opsgraph", "manual.graph.json"))
	} else if !filepath.IsAbs(storagePath) {
		storagePath = projectRelativePath(storagePath)
	}
	return &defaultOpsGraphService{graphs: opsgraph.NewGraphService(opsgraph.NewFileRepository(storagePath))}
}

func (s *defaultOpsGraphService) ListGraphs(ctx context.Context) ([]opsgraph.GraphSummary, error) {
	if _, err := s.graphs.EnsureDefaultGraph(ctx); err != nil {
		return nil, err
	}
	return s.graphs.ListGraphs(ctx)
}

func (s *defaultOpsGraphService) CreateGraph(ctx context.Context, graph opsgraph.GraphRecord) (opsgraph.GraphRecord, error) {
	return s.graphs.CreateGraph(ctx, graph)
}

func (s *defaultOpsGraphService) GetGraph(ctx context.Context, graphID string) (opsgraph.GraphRecord, bool, error) {
	if strings.TrimSpace(graphID) == "" {
		if _, err := s.graphs.EnsureDefaultGraph(ctx); err != nil {
			return opsgraph.GraphRecord{}, false, err
		}
	}
	return s.graphs.GetGraph(ctx, graphID)
}

func (s *defaultOpsGraphService) UpdateGraph(ctx context.Context, graphID string, graph opsgraph.GraphRecord) (opsgraph.GraphRecord, bool, error) {
	return s.graphs.UpdateGraph(ctx, graphID, graph)
}

func (s *defaultOpsGraphService) ExportGraphYAML(ctx context.Context, graphID string) ([]byte, bool, error) {
	return s.graphs.ExportGraphYAML(ctx, graphID)
}

func (s *defaultOpsGraphService) ImportGraphYAML(ctx context.Context, graphID string, raw []byte) (opsgraph.GraphRecord, bool, error) {
	return s.graphs.ImportGraphYAML(ctx, graphID, raw)
}

func (s *defaultOpsGraphService) DuplicateGraph(ctx context.Context, graphID string) (opsgraph.GraphRecord, bool, error) {
	return s.graphs.DuplicateGraph(ctx, graphID)
}

func (s *defaultOpsGraphService) DeleteGraph(ctx context.Context, graphID string) (bool, error) {
	return s.graphs.DeleteGraph(ctx, graphID)
}

func (s *defaultOpsGraphService) CreateNode(ctx context.Context, graphID string, node opsgraph.Node) (opsgraph.Node, error) {
	return s.graphs.CreateNode(ctx, graphID, node)
}

func (s *defaultOpsGraphService) UpdateNode(ctx context.Context, graphID, nodeID string, node opsgraph.Node) (opsgraph.Node, bool, error) {
	return s.graphs.UpdateNode(ctx, graphID, nodeID, node)
}

func (s *defaultOpsGraphService) DeleteNode(ctx context.Context, graphID, nodeID string, cascade bool) (bool, error) {
	graph, found, err := s.graphs.GetGraph(ctx, graphID)
	if err != nil || !found {
		return false, err
	}
	nodeFound := false
	for _, node := range graph.Nodes {
		if node.ID == nodeID {
			nodeFound = true
			break
		}
	}
	if !nodeFound {
		return false, nil
	}
	return true, s.graphs.DeleteNode(ctx, graphID, nodeID, cascade)
}

func (s *defaultOpsGraphService) CreateRelationship(ctx context.Context, graphID string, edge opsgraph.Edge) (opsgraph.Edge, error) {
	return s.graphs.CreateRelationship(ctx, graphID, edge)
}

func (s *defaultOpsGraphService) UpdateRelationship(ctx context.Context, graphID, edgeID string, edge opsgraph.Edge) (opsgraph.Edge, bool, error) {
	return s.graphs.UpdateRelationship(ctx, graphID, edgeID, edge)
}

func (s *defaultOpsGraphService) DeleteRelationship(ctx context.Context, graphID, edgeID string) (bool, error) {
	return s.graphs.DeleteRelationship(ctx, graphID, edgeID)
}

func (s *defaultOpsGraphService) SaveLayout(ctx context.Context, graphID string, nodes []opsgraph.Node, viewport *opsgraph.Viewport) error {
	return s.graphs.SaveLayout(ctx, graphID, nodes, viewport)
}

func (s *defaultOpsGraphService) Validate(ctx context.Context, graphID string) ([]opsgraph.ValidationIssue, bool, error) {
	return s.graphs.Validate(ctx, graphID)
}

func (s *defaultOpsGraphService) Lookup(ctx context.Context, graphID string, cmd OpsGraphLookupCommand) ([]OpsGraphEntityView, error) {
	if strings.TrimSpace(graphID) == "" {
		if _, err := s.graphs.EnsureDefaultGraph(ctx); err != nil {
			return nil, err
		}
	}
	return s.graphs.Lookup(ctx, graphID, cmd)
}

func (s *defaultOpsGraphService) Neighborhood(ctx context.Context, graphID, entityID string, depth int) (OpsGraphNeighborhoodView, bool, error) {
	if strings.TrimSpace(graphID) == "" {
		if _, err := s.graphs.EnsureDefaultGraph(ctx); err != nil {
			return OpsGraphNeighborhoodView{}, false, err
		}
	}
	neighborhood, found, err := s.graphs.Neighborhood(ctx, graphID, entityID, depth)
	if err != nil {
		return OpsGraphNeighborhoodView{Depth: depth}, false, err
	}
	if !found {
		return OpsGraphNeighborhoodView{Depth: depth}, false, nil
	}
	return OpsGraphNeighborhoodView{
		Entity:        neighborhood.Root,
		Depth:         neighborhood.Depth,
		Neighbors:     flattenOpsGraphNeighbors(neighborhood),
		Entities:      neighborhood.Entities,
		Relationships: neighborhood.Relationships,
	}, true, nil
}

func (s *defaultOpsGraphService) BusinessImpact(ctx context.Context, graphID, entityID string) (OpsGraphBusinessImpactView, bool, error) {
	if strings.TrimSpace(graphID) == "" {
		if _, err := s.graphs.EnsureDefaultGraph(ctx); err != nil {
			return OpsGraphBusinessImpactView{}, false, err
		}
	}
	return s.graphs.BusinessImpact(ctx, graphID, entityID)
}

func flattenOpsGraphNeighbors(neighborhood opsgraph.Neighborhood) []map[string]any {
	out := make([]map[string]any, 0, len(neighborhood.Entities))
	for _, entity := range neighborhood.Entities {
		if entity.ID == neighborhood.Root.ID {
			continue
		}
		out = append(out, map[string]any{
			"id":       entity.ID,
			"name":     entity.Name,
			"type":     entity.Type,
			"relation": relationBetween(neighborhood.Root.ID, entity.ID, neighborhood.Relationships),
			"status":   firstNonEmptyString(entity.Attributes["status"], "known"),
		})
	}
	return out
}

func relationBetween(rootID, entityID string, relationships []opsgraph.Relationship) string {
	for _, rel := range relationships {
		if rel.From == rootID && rel.To == entityID {
			return string(rel.Type)
		}
		if rel.To == rootID && rel.From == entityID {
			return string(rel.Type)
		}
	}
	return "neighbor"
}
