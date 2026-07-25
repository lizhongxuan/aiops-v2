package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/runtimecontract"
)

func validateRuntimeTurnArtifacts(artifacts []runtimecontract.TurnArtifactEnvelope) error {
	seenIDs, seenKinds := map[string]bool{}, map[string]bool{}
	for index, artifact := range artifacts {
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("turn artifact[%d]: %w", index, err)
		}
		if seenIDs[artifact.ID] || seenKinds[artifact.Kind] {
			return fmt.Errorf("turn artifacts must have unique id and kind")
		}
		seenIDs[artifact.ID], seenKinds[artifact.Kind] = true, true
	}
	return nil
}

func cloneRuntimeTurnArtifacts(artifacts []runtimecontract.TurnArtifactEnvelope) []runtimecontract.TurnArtifactEnvelope {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]runtimecontract.TurnArtifactEnvelope, len(artifacts))
	for index := range artifacts {
		cloned[index] = artifacts[index].Clone()
	}
	return cloned
}

func TurnArtifactByKind(snapshot *TurnSnapshot, kind string) (runtimecontract.TurnArtifactEnvelope, bool) {
	if snapshot == nil || strings.TrimSpace(kind) == "" {
		return runtimecontract.TurnArtifactEnvelope{}, false
	}
	for _, artifact := range snapshot.TurnArtifacts {
		if artifact.Kind == kind {
			return artifact.Clone(), true
		}
	}
	return runtimecontract.TurnArtifactEnvelope{}, false
}
