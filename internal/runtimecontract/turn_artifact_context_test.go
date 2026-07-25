package runtimecontract

import (
	"context"
	"testing"
)

func TestTurnArtifactContextClonesAndRejectsDuplicateKinds(t *testing.T) {
	artifact, err := NewInlineTurnArtifact("workflow.context", map[string]string{"revision": "rv-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := ContextWithTurnArtifacts(context.Background(), []TurnArtifactEnvelope{artifact})
	if err != nil {
		t.Fatal(err)
	}
	artifact.Inline[1] = 'X'
	found, ok := TurnArtifactFromContext(ctx, "workflow.context")
	if !ok || found.Validate() != nil {
		t.Fatalf("artifact lookup = %#v, %v", found, ok)
	}
	found.Inline[1] = 'Y'
	again, _ := TurnArtifactFromContext(ctx, "workflow.context")
	if err := again.Validate(); err != nil {
		t.Fatalf("lookup exposed mutable bytes: %v", err)
	}
	if _, err := ContextWithTurnArtifacts(ctx, []TurnArtifactEnvelope{again, again}); err == nil {
		t.Fatal("duplicate artifact kind was accepted")
	}
}
