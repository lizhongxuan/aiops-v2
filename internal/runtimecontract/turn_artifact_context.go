package runtimecontract

import (
	"context"
	"errors"
)

type turnArtifactContextKey struct{}

// ContextWithTurnArtifacts installs an immutable generic artifact set for
// downstream adapters. Domain-specific interpretation remains outside the
// RuntimeKernel package.
func ContextWithTurnArtifacts(ctx context.Context, artifacts []TurnArtifactEnvelope) (context.Context, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cloned := make([]TurnArtifactEnvelope, len(artifacts))
	seen := map[string]bool{}
	for index := range artifacts {
		if err := artifacts[index].Validate(); err != nil {
			return nil, err
		}
		if seen[artifacts[index].Kind] {
			return nil, errors.New("duplicate turn artifact kind")
		}
		seen[artifacts[index].Kind] = true
		cloned[index] = artifacts[index].Clone()
	}
	return context.WithValue(ctx, turnArtifactContextKey{}, cloned), nil
}

// TurnArtifactFromContext returns a defensive copy of one artifact.
func TurnArtifactFromContext(ctx context.Context, kind string) (TurnArtifactEnvelope, bool) {
	if ctx == nil {
		return TurnArtifactEnvelope{}, false
	}
	artifacts, ok := ctx.Value(turnArtifactContextKey{}).([]TurnArtifactEnvelope)
	if !ok {
		return TurnArtifactEnvelope{}, false
	}
	for _, artifact := range artifacts {
		if artifact.Kind == kind {
			return artifact.Clone(), true
		}
	}
	return TurnArtifactEnvelope{}, false
}
