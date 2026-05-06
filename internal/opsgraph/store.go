package opsgraph

import (
	"sort"
	"strings"
)

type Store struct {
	entities map[string]Entity
	outgoing map[string][]Relationship
	incoming map[string][]Relationship
}

func NewStore(entities []Entity, relationships []Relationship) *Store {
	s := &Store{
		entities: map[string]Entity{},
		outgoing: map[string][]Relationship{},
		incoming: map[string][]Relationship{},
	}
	for _, entity := range entities {
		entity.ID = strings.TrimSpace(entity.ID)
		if entity.ID == "" {
			continue
		}
		s.entities[entity.ID] = entity
	}
	for _, rel := range relationships {
		if rel.From == "" || rel.To == "" || rel.Type == "" {
			continue
		}
		s.outgoing[rel.From] = append(s.outgoing[rel.From], rel)
		s.incoming[rel.To] = append(s.incoming[rel.To], rel)
	}
	return s
}

func (s *Store) Entity(id string) (Entity, bool) {
	if s == nil {
		return Entity{}, false
	}
	entity, ok := s.entities[strings.TrimSpace(id)]
	return entity, ok
}

func (s *Store) Lookup(req LookupRequest) []Entity {
	if s == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	allowed := map[EntityType]bool{}
	for _, typ := range req.Types {
		allowed[typ] = true
	}
	type scored struct {
		entity Entity
		score  int
	}
	var matches []scored
	for _, entity := range s.entities {
		if len(allowed) > 0 && !allowed[entity.Type] {
			continue
		}
		score := entityScore(entity, query)
		if score == 0 {
			continue
		}
		matches = append(matches, scored{entity: entity, score: score})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].entity.ID < matches[j].entity.ID
		}
		return matches[i].score > matches[j].score
	})
	limit := req.Limit
	if limit <= 0 || limit > len(matches) {
		limit = len(matches)
	}
	out := make([]Entity, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, matches[i].entity)
	}
	return out
}

func (s *Store) Neighborhood(id string, depth int) Neighborhood {
	if depth <= 0 {
		depth = 1
	}
	root, ok := s.resolveEntity(id)
	if !ok {
		return Neighborhood{Depth: depth}
	}
	dist := map[string]int{root.ID: 0}
	queue := []string{root.ID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if dist[current] >= depth {
			continue
		}
		for _, rel := range s.adjacent(current) {
			next := rel.To
			if next == current {
				next = rel.From
			}
			if _, seen := dist[next]; seen {
				continue
			}
			dist[next] = dist[current] + 1
			queue = append(queue, next)
		}
	}
	var entities []Entity
	for entityID := range dist {
		if entity, ok := s.entities[entityID]; ok {
			entities = append(entities, entity)
		}
	}
	sortEntities(entities)
	var relationships []Relationship
	for entityID := range dist {
		for _, rel := range s.outgoing[entityID] {
			if _, ok := dist[rel.To]; ok {
				relationships = append(relationships, rel)
			}
		}
	}
	sortRelationships(relationships)
	return Neighborhood{Root: root, Depth: depth, Entities: entities, Relationships: relationships}
}

func (s *Store) BusinessImpact(id string) BusinessImpact {
	root, ok := s.resolveEntity(id)
	if !ok {
		return BusinessImpact{}
	}
	neighbors := s.Neighborhood(root.ID, 3)
	impact := BusinessImpact{Entity: root}
	for _, entity := range neighbors.Entities {
		switch entity.Type {
		case EntityERPModule:
			impact.Modules = append(impact.Modules, entity)
		case EntityBusinessCapability:
			impact.Capabilities = append(impact.Capabilities, entity)
		case EntityTenant:
			impact.Tenants = append(impact.Tenants, entity)
		case EntityService:
			impact.Services = append(impact.Services, entity)
		}
	}
	sortEntities(impact.Modules)
	sortEntities(impact.Capabilities)
	sortEntities(impact.Tenants)
	sortEntities(impact.Services)
	impact.Summary = impactSummary(impact)
	return impact
}

func (s *Store) RelatedRunbooks(id string) []RunbookMatch {
	root, ok := s.resolveEntity(id)
	if !ok {
		return nil
	}
	neighbors := s.Neighborhood(root.ID, 3)
	selected := map[string]bool{}
	for _, entity := range neighbors.Entities {
		selected[entity.ID] = true
	}
	seen := map[string]bool{}
	var out []RunbookMatch
	for entityID := range selected {
		for _, rel := range s.outgoing[entityID] {
			if rel.Type != RelHandledBy {
				continue
			}
			runbook, ok := s.entities[rel.To]
			if !ok || runbook.Type != EntityRunbook || seen[runbook.ID] {
				continue
			}
			seen[runbook.ID] = true
			out = append(out, RunbookMatch{Runbook: runbook, Reason: firstNonEmpty(rel.Reason, "matched by graph handled_by relation")})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Runbook.ID < out[j].Runbook.ID })
	return out
}

func (s *Store) resolveEntity(idOrQuery string) (Entity, bool) {
	if entity, ok := s.Entity(idOrQuery); ok {
		return entity, true
	}
	matches := s.Lookup(LookupRequest{Query: idOrQuery, Limit: 1})
	if len(matches) == 0 {
		return Entity{}, false
	}
	return matches[0], true
}

func (s *Store) adjacent(id string) []Relationship {
	out := append([]Relationship(nil), s.outgoing[id]...)
	out = append(out, s.incoming[id]...)
	return out
}

func entityScore(entity Entity, query string) int {
	if query == "" {
		return 1
	}
	fields := []string{entity.ID, entity.Name, entity.Description}
	fields = append(fields, entity.Aliases...)
	fields = append(fields, entity.Tags...)
	for _, field := range fields {
		value := strings.ToLower(strings.TrimSpace(field))
		switch {
		case value == query:
			return 100
		case strings.Contains(value, query):
			return 50
		}
	}
	return 0
}

func impactSummary(impact BusinessImpact) string {
	parts := []string{}
	if len(impact.Modules) > 0 {
		parts = append(parts, "modules="+entityNames(impact.Modules))
	}
	if len(impact.Capabilities) > 0 {
		parts = append(parts, "capabilities="+entityNames(impact.Capabilities))
	}
	if len(impact.Tenants) > 0 {
		parts = append(parts, "tenants="+entityNames(impact.Tenants))
	}
	if len(parts) == 0 {
		return "no business impact found in graph"
	}
	return strings.Join(parts, "; ")
}

func entityNames(entities []Entity) string {
	names := make([]string, 0, len(entities))
	for _, entity := range entities {
		names = append(names, firstNonEmpty(entity.Name, entity.ID))
	}
	return strings.Join(names, ", ")
}

func sortEntities(entities []Entity) {
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })
}

func sortRelationships(relationships []Relationship) {
	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].From == relationships[j].From {
			if relationships[i].Type == relationships[j].Type {
				return relationships[i].To < relationships[j].To
			}
			return relationships[i].Type < relationships[j].Type
		}
		return relationships[i].From < relationships[j].From
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
