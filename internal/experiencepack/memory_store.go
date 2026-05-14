package experiencepack

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

type MemoryStore struct {
	mu       sync.RWMutex
	assets   map[AssetID]any
	manifest map[string][]ExperiencePackManifest
	skills   map[string][]SkillAsset
	genes    map[string][]GEPGene
	capsules map[string][]GEPCapsule
	events   map[string][]EvolutionEvent
	memory   map[string][]MemoryGraphEvent
	avoid    map[string][]AvoidCue
	runner   map[string][]RunnerBinding
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		assets:   map[AssetID]any{},
		manifest: map[string][]ExperiencePackManifest{},
		skills:   map[string][]SkillAsset{},
		genes:    map[string][]GEPGene{},
		capsules: map[string][]GEPCapsule{},
		events:   map[string][]EvolutionEvent{},
		memory:   map[string][]MemoryGraphEvent{},
		avoid:    map[string][]AvoidCue{},
		runner:   map[string][]RunnerBinding{},
	}
}

func (s *MemoryStore) AppendManifest(ctx context.Context, manifest ExperiencePackManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return s.append(ctx, manifest.AssetID, manifest, func() {
		s.manifest[manifest.ID] = append(s.manifest[manifest.ID], manifest)
	})
}

func (s *MemoryStore) GetManifest(ctx context.Context, packID string, version string) (ExperiencePackManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.manifest[packID]
	if len(items) == 0 {
		return ExperiencePackManifest{}, ErrNotFound
	}
	if version != "" {
		for _, item := range items {
			if fmt.Sprint(item.Metadata["version"]) == version {
				return item, nil
			}
		}
		return ExperiencePackManifest{}, ErrNotFound
	}
	return items[len(items)-1], nil
}

func (s *MemoryStore) ListManifests(ctx context.Context, filter ManifestFilter) ([]ExperiencePackManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []ExperiencePackManifest
	for _, versions := range s.manifest {
		if len(versions) == 0 {
			continue
		}
		item := versions[len(versions)-1]
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Enabled != nil && item.Enabled != *filter.Enabled {
			continue
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (s *MemoryStore) AppendSkill(ctx context.Context, packID string, skill SkillAsset) error {
	return s.append(ctx, skill.AssetID, skill, func() { s.skills[packID] = append(s.skills[packID], skill) })
}

func (s *MemoryStore) ListSkills(ctx context.Context, packID string) ([]SkillAsset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]SkillAsset(nil), s.skills[packID]...), nil
}

func (s *MemoryStore) AppendGene(ctx context.Context, packID string, gene GEPGene) error {
	if err := gene.Validate(); err != nil {
		return err
	}
	return s.append(ctx, gene.AssetID, gene, func() { s.genes[packID] = append(s.genes[packID], gene) })
}

func (s *MemoryStore) ListGenes(ctx context.Context, packID string) ([]GEPGene, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]GEPGene(nil), s.genes[packID]...), nil
}

func (s *MemoryStore) AppendCapsule(ctx context.Context, packID string, capsule GEPCapsule) error {
	if err := capsule.Validate(); err != nil {
		return err
	}
	return s.append(ctx, capsule.AssetID, capsule, func() { s.capsules[packID] = append(s.capsules[packID], capsule) })
}

func (s *MemoryStore) ListCapsules(ctx context.Context, packID string) ([]GEPCapsule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]GEPCapsule(nil), s.capsules[packID]...), nil
}

func (s *MemoryStore) AppendEvolutionEvent(ctx context.Context, packID string, event EvolutionEvent) error {
	return s.append(ctx, event.AssetID, event, func() { s.events[packID] = append(s.events[packID], event) })
}

func (s *MemoryStore) ListEvolutionEvents(ctx context.Context, packID string) ([]EvolutionEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]EvolutionEvent(nil), s.events[packID]...), nil
}

func (s *MemoryStore) AppendMemoryGraphEvent(ctx context.Context, packID string, event MemoryGraphEvent) error {
	if event.AssetID == "" {
		event.AssetID = MustHashCanonicalJSON(event)
	}
	return s.append(ctx, event.AssetID, event, func() { s.memory[packID] = append(s.memory[packID], event) })
}

func (s *MemoryStore) ListMemoryGraphEvents(ctx context.Context, packID string) ([]MemoryGraphEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]MemoryGraphEvent(nil), s.memory[packID]...), nil
}

func (s *MemoryStore) AppendAvoidCue(ctx context.Context, packID string, cue AvoidCue) error {
	return s.append(ctx, cue.AssetID, cue, func() { s.avoid[packID] = append(s.avoid[packID], cue) })
}

func (s *MemoryStore) ListAvoidCues(ctx context.Context, packID string) ([]AvoidCue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]AvoidCue(nil), s.avoid[packID]...), nil
}

func (s *MemoryStore) AppendRunnerBinding(ctx context.Context, packID string, binding RunnerBinding) error {
	return s.append(ctx, binding.AssetID, binding, func() { s.runner[packID] = append(s.runner[packID], binding) })
}

func (s *MemoryStore) ListRunnerBindings(ctx context.Context, packID string) ([]RunnerBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]RunnerBinding(nil), s.runner[packID]...), nil
}

func (s *MemoryStore) append(ctx context.Context, assetID AssetID, value any, commit func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !ValidAssetID(assetID) {
		return fmt.Errorf("%w: invalid asset_id %q", ErrValidationFailed, assetID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.assets[assetID]; ok {
		if canonicalEqual(existing, value) {
			return nil
		}
		return ErrConflict
	}
	if err := VerifyStoredAssetID(value, assetID); err != nil {
		return err
	}
	s.assets[assetID] = value
	commit()
	return nil
}
