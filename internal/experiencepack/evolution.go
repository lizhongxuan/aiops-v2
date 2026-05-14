package experiencepack

import "context"

type EvolutionTrigger struct {
	Kind          string
	MatchedPackID string
	MatchedGeneID string
	Outcome       string
	Trajectory    Trajectory
	Env           EnvironmentFingerprint
}

func ApplyEvolution(ctx context.Context, store Store, trigger EvolutionTrigger) (CandidateBundle, *AvoidCue, error) {
	input := CandidateInput{PackID: trigger.MatchedPackID, MatchedGene: trigger.MatchedGeneID, Trajectory: trigger.Trajectory, Env: trigger.Env}
	if trigger.MatchedPackID == "" && trigger.Outcome == "success" {
		input.Category = CategoryInnovate
		bundle, err := GenerateCandidate(input)
		if err != nil {
			return CandidateBundle{}, nil, err
		}
		return bundle, nil, PersistCandidate(ctx, store, bundle)
	}
	bundle, err := GenerateCandidate(input)
	if err != nil {
		return CandidateBundle{}, nil, err
	}
	if trigger.MatchedPackID != "" {
		bundle = bindBundleToMatchedGene(bundle, trigger.MatchedGeneID)
	}
	if trigger.Outcome == "failed" {
		bundle.Capsule.Outcome = Outcome{Status: "failed", Score: 0.2}
		bundle = rehashEvolutionBundle(bundle)
		cue := AvoidCueFromFailedCapsule(bundle.Capsule)
		if err := persistMatchedEvolution(ctx, store, bundle); err != nil {
			return CandidateBundle{}, nil, err
		}
		if err := store.AppendAvoidCue(ctx, bundle.Manifest.ID, cue); err != nil {
			return CandidateBundle{}, nil, err
		}
		return bundle, &cue, nil
	}
	if trigger.MatchedPackID != "" {
		bundle = rehashEvolutionBundle(bundle)
		return bundle, nil, persistMatchedEvolution(ctx, store, bundle)
	}
	return bundle, nil, PersistCandidate(ctx, store, bundle)
}

func bindBundleToMatchedGene(bundle CandidateBundle, geneID string) CandidateBundle {
	if geneID == "" {
		return bundle
	}
	bundle.Capsule.Gene = geneID
	bundle.Capsule.GenesUsed = []string{geneID}
	bundle.EvolutionEvent.GenesUsed = []string{geneID}
	for idx := range bundle.MemoryGraphEvents {
		bundle.MemoryGraphEvents[idx].Gene = map[string]any{"id": geneID}
	}
	return bundle
}

func rehashEvolutionBundle(bundle CandidateBundle) CandidateBundle {
	bundle.Capsule.AssetID = MustHashCanonicalJSON(bundle.Capsule)
	bundle.EvolutionEvent.Outcome = bundle.Capsule.Outcome
	bundle.EvolutionEvent.CapsuleID = bundle.Capsule.ID
	bundle.EvolutionEvent.AssetID = MustHashCanonicalJSON(bundle.EvolutionEvent)
	for idx := range bundle.MemoryGraphEvents {
		bundle.MemoryGraphEvents[idx].Outcome = &bundle.Capsule.Outcome
		bundle.MemoryGraphEvents[idx].AssetID = MustHashCanonicalJSON(bundle.MemoryGraphEvents[idx])
	}
	return bundle
}

func persistMatchedEvolution(ctx context.Context, store Store, bundle CandidateBundle) error {
	if err := store.AppendCapsule(ctx, bundle.Manifest.ID, bundle.Capsule); err != nil {
		return err
	}
	if err := store.AppendEvolutionEvent(ctx, bundle.Manifest.ID, bundle.EvolutionEvent); err != nil {
		return err
	}
	for _, event := range bundle.MemoryGraphEvents {
		if err := store.AppendMemoryGraphEvent(ctx, bundle.Manifest.ID, event); err != nil {
			return err
		}
	}
	return nil
}
