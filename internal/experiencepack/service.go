package experiencepack

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	store     Store
	memStore  *MemoryStore
	retriever *Retriever
}

func NewService(store Store) *Service {
	mem, ok := store.(*MemoryStore)
	if store == nil {
		mem = NewMemoryStore()
		store = mem
	}
	if !ok || mem == nil {
		mem = NewMemoryStore()
	}
	return &Service{store: store, memStore: mem, retriever: NewRetriever(mem)}
}

func (s *Service) Store() Store { return s.store }

func (s *Service) ListCandidates(ctx context.Context) ([]ExperiencePackManifest, error) {
	return s.store.ListManifests(ctx, ManifestFilter{Status: PackStatusCandidate})
}

func (s *Service) ListPacks(ctx context.Context) ([]ExperiencePackManifest, error) {
	return s.store.ListManifests(ctx, ManifestFilter{})
}

func (s *Service) GetPack(ctx context.Context, id string) (ExperiencePackManifest, error) {
	return s.store.GetManifest(ctx, id, "")
}

func (s *Service) Retrieve(ctx context.Context, query RetrievalQuery) ([]ExperienceMatch, error) {
	return s.retriever.Retrieve(ctx, query)
}

func (s *Service) EvaluateSuggestion(input SuggestionInput) SuggestionResult {
	return EvaluateChatSuggestion(input)
}

func (s *Service) GenerateAndPersistCandidate(ctx context.Context, input CandidateInput) (CandidateBundle, error) {
	bundle, err := GenerateCandidate(input)
	if err != nil {
		return CandidateBundle{}, err
	}
	if err := PersistCandidate(ctx, s.store, bundle); err != nil {
		return CandidateBundle{}, err
	}
	return bundle, nil
}

func (s *Service) Review(ctx context.Context, packID string, approve bool) (ExperiencePackManifest, error) {
	manifest, err := s.store.GetManifest(ctx, packID, "")
	if err != nil {
		return ExperiencePackManifest{}, err
	}
	status := "rejected"
	score := 0.0
	if approve {
		manifest.Status = PackStatusApproved
		manifest.ReviewStatus = PackStatusApproved
		status = "approved"
		score = 1.0
	} else {
		manifest.Status = PackStatusRetired
		manifest.ReviewStatus = "rejected"
	}
	event := EvolutionEvent{
		Type:          "EvolutionEvent",
		SchemaVersion: SchemaVersionGEP,
		ID:            "evt_review_" + stableSuffix(packID, time.Now().UTC().Format(time.RFC3339Nano)),
		Intent:        CategoryOptimize,
		Signals:       []string{"experience_pack_review", "review:" + status},
		GenesUsed:     assetRefIDs(manifest.Genes),
		MutationID:    "review_" + stableSuffix(packID),
		BlastRadius: map[string]any{
			"review_only": true,
		},
		Outcome:    Outcome{Status: status, Score: score},
		SourceType: "generated",
	}
	event.AssetID = MustHashCanonicalJSON(event)
	if err := s.store.AppendEvolutionEvent(ctx, manifest.ID, event); err != nil {
		return ExperiencePackManifest{}, err
	}
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	if err := s.store.AppendManifest(ctx, manifest); err != nil {
		return ExperiencePackManifest{}, err
	}
	return manifest, nil
}

func (s *Service) Enable(ctx context.Context, packID string, enabled bool) (ExperiencePackManifest, error) {
	manifest, err := s.store.GetManifest(ctx, packID, "")
	if err != nil {
		return ExperiencePackManifest{}, err
	}
	if enabled {
		if manifest.ReviewStatus != PackStatusApproved {
			return ExperiencePackManifest{}, fmt.Errorf("%w: review is required", ErrStateDenied)
		}
		if err := manifest.CanEnable(); err != nil {
			return ExperiencePackManifest{}, err
		}
		genes, _ := s.store.ListGenes(ctx, packID)
		for _, gene := range genes {
			if report := CheckValidationGate(gene); !report.Passed {
				return ExperiencePackManifest{}, fmt.Errorf("%w: validation gate blocked", ErrValidationFailed)
			}
		}
		event := EvolutionEvent{
			Type:          "EvolutionEvent",
			SchemaVersion: SchemaVersionGEP,
			ID:            "evt_enable_" + stableSuffix(packID, time.Now().UTC().Format(time.RFC3339Nano)),
			Intent:        CategoryOptimize,
			Signals:       []string{"experience_pack_enable"},
			GenesUsed:     assetRefIDs(manifest.Genes),
			MutationID:    "enable_" + stableSuffix(packID),
			BlastRadius:   map[string]any{"enable_only": true},
			Outcome:       Outcome{Status: "enabled", Score: 1.0},
			SourceType:    "generated",
		}
		event.AssetID = MustHashCanonicalJSON(event)
		if err := s.store.AppendEvolutionEvent(ctx, manifest.ID, event); err != nil {
			return ExperiencePackManifest{}, err
		}
		memoryEvent := MemoryGraphEvent{
			Type:      "MemoryGraphEvent",
			Kind:      "outcome",
			ID:        "mge_enable_" + stableSuffix(packID, time.Now().UTC().Format(time.RFC3339Nano)),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Signal:    map[string]any{"items": []string{"experience_pack_enable"}},
			Outcome:   &event.Outcome,
		}
		memoryEvent.AssetID = MustHashCanonicalJSON(memoryEvent)
		if err := s.store.AppendMemoryGraphEvent(ctx, manifest.ID, memoryEvent); err != nil {
			return ExperiencePackManifest{}, err
		}
		manifest.Status = PackStatusEnabled
		manifest.Enabled = true
	} else {
		manifest.Status = PackStatusPaused
		manifest.Enabled = false
	}
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	if err := s.store.AppendManifest(ctx, manifest); err != nil {
		return ExperiencePackManifest{}, err
	}
	return manifest, nil
}

func (s *Service) SaveScopes(ctx context.Context, packID string, scopes []AuthorizationScope) (ExperiencePackManifest, error) {
	manifest, err := s.store.GetManifest(ctx, packID, "")
	if err != nil {
		return ExperiencePackManifest{}, err
	}
	manifest.AuthorizationScopes = scopes
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	if err := s.store.AppendManifest(ctx, manifest); err != nil {
		return ExperiencePackManifest{}, err
	}
	return manifest, nil
}

func (s *Service) ValidationGate(ctx context.Context, packID string) (ValidationReport, error) {
	genes, err := s.store.ListGenes(ctx, packID)
	if err != nil {
		return ValidationReport{}, err
	}
	if len(genes) == 0 {
		return ValidationReport{Passed: false, BlockedReasons: []string{"missing gene"}}, nil
	}
	var tasks []ValidationTask
	for _, gene := range genes {
		report := CheckValidationGate(gene)
		if !report.Passed {
			return report, nil
		}
		tasks = append(tasks, report.CompiledTasks...)
	}
	return ValidationReport{Passed: true, CompiledTasks: tasks}, nil
}

func (s *Service) ListRunnerBindings(ctx context.Context, packID string) ([]RunnerBinding, error) {
	return s.store.ListRunnerBindings(ctx, packID)
}

func assetRefIDs(refs []AssetRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
		}
	}
	return ids
}
