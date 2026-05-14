package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/experiencepack"
)

const (
	gormNamespaceExperiencePackManifests         = "experience_pack_manifests"
	gormNamespaceExperiencePackSkills            = "experience_pack_skills"
	gormNamespaceExperiencePackGenes             = "experience_pack_genes"
	gormNamespaceExperiencePackCapsules          = "experience_pack_capsules"
	gormNamespaceExperiencePackEvolutionEvents   = "experience_pack_evolution_events"
	gormNamespaceExperiencePackMemoryGraphEvents = "experience_pack_memory_graph_events"
	gormNamespaceExperiencePackAvoidCues         = "experience_pack_avoid_cues"
	gormNamespaceExperiencePackRunnerBindings    = "experience_pack_runner_bindings"
)

var _ experiencepack.Store = (*GormStore)(nil)

func (s *GormStore) AppendManifest(ctx context.Context, manifest experiencepack.ExperiencePackManifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackManifests, manifest.ID, manifest.AssetID, manifest)
}

func (s *GormStore) GetManifest(ctx context.Context, packID string, version string) (experiencepack.ExperiencePackManifest, error) {
	if err := ctx.Err(); err != nil {
		return experiencepack.ExperiencePackManifest{}, err
	}
	items, err := s.listManifestVersions(packID)
	if err != nil {
		return experiencepack.ExperiencePackManifest{}, err
	}
	if len(items) == 0 {
		return experiencepack.ExperiencePackManifest{}, experiencepack.ErrNotFound
	}
	if version != "" {
		for _, item := range items {
			if fmt.Sprint(item.Metadata["version"]) == version {
				return item, nil
			}
		}
		return experiencepack.ExperiencePackManifest{}, experiencepack.ErrNotFound
	}
	return items[len(items)-1], nil
}

func (s *GormStore) ListManifests(ctx context.Context, filter experiencepack.ManifestFilter) ([]experiencepack.ExperiencePackManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var all []experiencepack.ExperiencePackManifest
	if err := s.listKV(gormNamespaceExperiencePackManifests, &all); err != nil {
		return nil, err
	}
	latest := map[string]experiencepack.ExperiencePackManifest{}
	for _, item := range all {
		latest[item.ID] = item
	}
	result := make([]experiencepack.ExperiencePackManifest, 0, len(latest))
	for _, item := range latest {
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

func (s *GormStore) AppendSkill(ctx context.Context, packID string, skill experiencepack.SkillAsset) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackSkills, packID, skill.AssetID, skill)
}

func (s *GormStore) ListSkills(ctx context.Context, packID string) ([]experiencepack.SkillAsset, error) {
	var items []experiencepack.SkillAsset
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackSkills, packID, &items)
}

func (s *GormStore) AppendGene(ctx context.Context, packID string, gene experiencepack.GEPGene) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := gene.Validate(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackGenes, packID, gene.AssetID, gene)
}

func (s *GormStore) ListGenes(ctx context.Context, packID string) ([]experiencepack.GEPGene, error) {
	var items []experiencepack.GEPGene
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackGenes, packID, &items)
}

func (s *GormStore) AppendCapsule(ctx context.Context, packID string, capsule experiencepack.GEPCapsule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := capsule.Validate(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackCapsules, packID, capsule.AssetID, capsule)
}

func (s *GormStore) ListCapsules(ctx context.Context, packID string) ([]experiencepack.GEPCapsule, error) {
	var items []experiencepack.GEPCapsule
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackCapsules, packID, &items)
}

func (s *GormStore) AppendEvolutionEvent(ctx context.Context, packID string, event experiencepack.EvolutionEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackEvolutionEvents, packID, event.AssetID, event)
}

func (s *GormStore) ListEvolutionEvents(ctx context.Context, packID string) ([]experiencepack.EvolutionEvent, error) {
	var items []experiencepack.EvolutionEvent
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackEvolutionEvents, packID, &items)
}

func (s *GormStore) AppendMemoryGraphEvent(ctx context.Context, packID string, event experiencepack.MemoryGraphEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event.AssetID == "" {
		event.AssetID = experiencepack.MustHashCanonicalJSON(event)
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackMemoryGraphEvents, packID, event.AssetID, event)
}

func (s *GormStore) ListMemoryGraphEvents(ctx context.Context, packID string) ([]experiencepack.MemoryGraphEvent, error) {
	var items []experiencepack.MemoryGraphEvent
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackMemoryGraphEvents, packID, &items)
}

func (s *GormStore) AppendAvoidCue(ctx context.Context, packID string, cue experiencepack.AvoidCue) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackAvoidCues, packID, cue.AssetID, cue)
}

func (s *GormStore) ListAvoidCues(ctx context.Context, packID string) ([]experiencepack.AvoidCue, error) {
	var items []experiencepack.AvoidCue
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackAvoidCues, packID, &items)
}

func (s *GormStore) AppendRunnerBinding(ctx context.Context, packID string, binding experiencepack.RunnerBinding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.appendExperienceAsset(gormNamespaceExperiencePackRunnerBindings, packID, binding.AssetID, binding)
}

func (s *GormStore) ListRunnerBindings(ctx context.Context, packID string) ([]experiencepack.RunnerBinding, error) {
	var items []experiencepack.RunnerBinding
	return items, s.listExperienceAssets(ctx, gormNamespaceExperiencePackRunnerBindings, packID, &items)
}

func (s *GormStore) appendExperienceAsset(namespace, packID string, assetID experiencepack.AssetID, value any) error {
	if !experiencepack.ValidAssetID(assetID) {
		return fmt.Errorf("%w: invalid asset_id %q", experiencepack.ErrValidationFailed, assetID)
	}
	key := experienceAssetKey(packID, assetID)
	var existing any
	ok, err := s.loadKV(namespace, key, &existing)
	if err != nil {
		return err
	}
	if ok {
		if experienceCanonicalEqual(existing, value) {
			return nil
		}
		return experiencepack.ErrConflict
	}
	if err := experiencepack.VerifyStoredAssetID(value, assetID); err != nil {
		return err
	}
	return s.saveKV(namespace, key, value)
}

func (s *GormStore) listExperienceAssets(ctx context.Context, namespace, packID string, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.listKVWithPrefix(namespace, packID+"/", out)
}

func (s *GormStore) listManifestVersions(packID string) ([]experiencepack.ExperiencePackManifest, error) {
	var items []experiencepack.ExperiencePackManifest
	if err := s.listKVWithPrefix(gormNamespaceExperiencePackManifests, packID+"/", &items); err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return string(items[i].AssetID) < string(items[j].AssetID) })
	return items, nil
}

func (s *GormStore) listKVWithPrefix(namespace, prefix string, out any) error {
	var records []gormKVRecord
	if err := s.db.Where("namespace = ? AND record_key LIKE ?", namespace, prefix+"%").Order("created_at ASC, record_key ASC").Find(&records).Error; err != nil {
		return err
	}
	raw := make([]json.RawMessage, 0, len(records))
	for _, record := range records {
		raw = append(raw, json.RawMessage(record.Payload))
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, out)
}

func experienceAssetKey(packID string, assetID experiencepack.AssetID) string {
	return strings.TrimSpace(packID) + "/" + string(assetID)
}

func experienceCanonicalEqual(left, right any) bool {
	leftID, leftErr := experiencepack.HashCanonicalJSON(left)
	rightID, rightErr := experiencepack.HashCanonicalJSON(right)
	return leftErr == nil && rightErr == nil && leftID == rightID
}
