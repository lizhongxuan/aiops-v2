package experiencepack

import "context"

type ManifestFilter struct {
	Status  string
	Enabled *bool
	Limit   int
}

type Store interface {
	AppendManifest(ctx context.Context, manifest ExperiencePackManifest) error
	GetManifest(ctx context.Context, packID string, version string) (ExperiencePackManifest, error)
	ListManifests(ctx context.Context, filter ManifestFilter) ([]ExperiencePackManifest, error)
	AppendSkill(ctx context.Context, packID string, skill SkillAsset) error
	ListSkills(ctx context.Context, packID string) ([]SkillAsset, error)
	AppendGene(ctx context.Context, packID string, gene GEPGene) error
	ListGenes(ctx context.Context, packID string) ([]GEPGene, error)
	AppendCapsule(ctx context.Context, packID string, capsule GEPCapsule) error
	ListCapsules(ctx context.Context, packID string) ([]GEPCapsule, error)
	AppendEvolutionEvent(ctx context.Context, packID string, event EvolutionEvent) error
	ListEvolutionEvents(ctx context.Context, packID string) ([]EvolutionEvent, error)
	AppendMemoryGraphEvent(ctx context.Context, packID string, event MemoryGraphEvent) error
	ListMemoryGraphEvents(ctx context.Context, packID string) ([]MemoryGraphEvent, error)
	AppendAvoidCue(ctx context.Context, packID string, cue AvoidCue) error
	ListAvoidCues(ctx context.Context, packID string) ([]AvoidCue, error)
	AppendRunnerBinding(ctx context.Context, packID string, binding RunnerBinding) error
	ListRunnerBindings(ctx context.Context, packID string) ([]RunnerBinding, error)
}
