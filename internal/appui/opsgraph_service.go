package appui

import (
	"context"
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
	Lookup(ctx context.Context, cmd OpsGraphLookupCommand) ([]OpsGraphEntityView, error)
	Neighborhood(ctx context.Context, entityID string, depth int) (OpsGraphNeighborhoodView, bool, error)
	BusinessImpact(ctx context.Context, entityID string) (OpsGraphBusinessImpactView, bool, error)
}

type defaultOpsGraphService struct {
	store *opsgraph.Store
}

func NewOpsGraphService(seedPath string) OpsGraphService {
	if strings.TrimSpace(seedPath) == "" {
		seedPath = "data/opsgraph/erp.seed.yaml"
	}
	store, err := opsgraph.LoadSeedFile(projectRelativePath(seedPath))
	if err != nil {
		store = opsgraph.NewStore(nil, nil)
	}
	return &defaultOpsGraphService{store: store}
}

func (s *defaultOpsGraphService) Lookup(_ context.Context, cmd OpsGraphLookupCommand) ([]OpsGraphEntityView, error) {
	return s.store.Lookup(cmd), nil
}

func (s *defaultOpsGraphService) Neighborhood(_ context.Context, entityID string, depth int) (OpsGraphNeighborhoodView, bool, error) {
	neighborhood := s.store.Neighborhood(entityID, depth)
	if strings.TrimSpace(neighborhood.Root.ID) == "" {
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

func (s *defaultOpsGraphService) BusinessImpact(_ context.Context, entityID string) (OpsGraphBusinessImpactView, bool, error) {
	impact := s.store.BusinessImpact(entityID)
	if strings.TrimSpace(impact.Entity.ID) == "" {
		return OpsGraphBusinessImpactView{}, false, nil
	}
	return impact, true, nil
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
