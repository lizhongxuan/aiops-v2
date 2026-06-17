package appui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/plugins"
	"aiops-v2/internal/store"
)

type defaultCapabilityService struct {
	skills      SkillCatalogRepository
	mcps        AgentMCPCatalogRepository
	pluginSpecs []plugins.Spec
}

func NewCapabilityService(skills SkillCatalogRepository, mcps AgentMCPCatalogRepository, pluginSpecs []plugins.Spec) CapabilityService {
	return &defaultCapabilityService{
		skills:      skills,
		mcps:        mcps,
		pluginSpecs: cloneAgentProfilePluginSpecs(pluginSpecs),
	}
}

func (s *defaultCapabilityService) ListRecords(ctx context.Context, req CapabilityListRequest) (CapabilityListResponse, error) {
	profileCatalog := &defaultAgentProfileService{
		repo:        capabilityCatalogRepo{skills: s.skills, mcps: s.mcps},
		pluginSpecs: cloneAgentProfilePluginSpecs(s.pluginSpecs),
	}

	skills, err := profileCatalog.ListSkillCatalog(ctx)
	if err != nil {
		return CapabilityListResponse{}, err
	}
	mcps, err := profileCatalog.ListMcpCatalog(ctx)
	if err != nil {
		return CapabilityListResponse{}, err
	}

	items := make([]CapabilityRecord, 0, len(skills)+len(mcps)+len(s.pluginSpecs))
	for _, item := range skills {
		items = append(items, capabilityRecordFromSkillCatalogItem(item))
	}
	for _, item := range mcps {
		items = append(items, capabilityRecordFromMcpCatalogItem(item))
	}
	for _, spec := range s.pluginSpecs {
		if record := capabilityRecordFromPluginSpec(spec); record.ID != "" {
			items = append(items, record)
		}
	}

	items = filterCapabilityRecords(items, req)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].ID < items[j].ID
	})
	return CapabilityListResponse{Items: items}, nil
}

type capabilityCatalogRepo struct {
	skills SkillCatalogRepository
	mcps   AgentMCPCatalogRepository
}

func (r capabilityCatalogRepo) GetSkillCatalog() ([]store.SkillCatalogEntry, error) {
	if r.skills == nil {
		return nil, nil
	}
	return r.skills.GetSkillCatalog()
}

func (r capabilityCatalogRepo) SaveSkillCatalog([]store.SkillCatalogEntry) error {
	return fmt.Errorf("capability catalog is read-only")
}

func (r capabilityCatalogRepo) GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error) {
	if r.mcps == nil {
		return nil, nil
	}
	return r.mcps.GetAgentMCPCatalog()
}

func (r capabilityCatalogRepo) SaveAgentMCPCatalog([]store.AgentMCPCatalogEntry) error {
	return fmt.Errorf("capability catalog is read-only")
}

func (r capabilityCatalogRepo) GetAgentProfiles() ([]store.AgentProfileRecord, error) {
	return nil, nil
}

func (r capabilityCatalogRepo) SaveAgentProfiles([]store.AgentProfileRecord) error {
	return fmt.Errorf("capability catalog is read-only")
}

func (s *defaultCapabilityService) Search(ctx context.Context, req CapabilityListRequest) (CapabilityListResponse, error) {
	return s.ListRecords(ctx, req)
}

func (s *defaultCapabilityService) Resolve(ctx context.Context, id string) (CapabilityRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return CapabilityRecord{}, fmt.Errorf("capability id is required")
	}
	result, err := s.ListRecords(ctx, CapabilityListRequest{})
	if err != nil {
		return CapabilityRecord{}, err
	}
	for _, item := range result.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return CapabilityRecord{}, fmt.Errorf("capability %q not found", id)
}

func (s *defaultCapabilityService) Pin(ctx context.Context, id string) (CapabilityRecord, error) {
	return s.Resolve(ctx, id)
}

func (s *defaultCapabilityService) Unpin(ctx context.Context, id string) (CapabilityRecord, error) {
	return s.Resolve(ctx, id)
}

func filterCapabilityRecords(items []CapabilityRecord, req CapabilityListRequest) []CapabilityRecord {
	query := strings.ToLower(strings.TrimSpace(req.Query))
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	category := strings.ToLower(strings.TrimSpace(req.Category))
	out := make([]CapabilityRecord, 0, len(items))
	for _, item := range items {
		if kind != "" && strings.ToLower(item.Kind) != kind {
			continue
		}
		if category != "" && strings.ToLower(item.Category) != category {
			continue
		}
		if query != "" && !capabilityRecordMatchesQuery(item, query) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func capabilityRecordMatchesQuery(item CapabilityRecord, query string) bool {
	haystack := []string{item.ID, item.Kind, item.Category, item.Name, item.Description, item.Source, item.SourceScope, item.Status, item.Risk}
	haystack = append(haystack, item.Tags...)
	for _, value := range haystack {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}
