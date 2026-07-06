package agentassembly

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestPromptSectionSnapshotHashChangesOnlyForPromptSectionChanges(t *testing.T) {
	base := PromptSectionSnapshotFromTrace([]promptcompiler.PromptSectionTrace{{ID: "profile.writer", Source: "profile", Hash: "sha256:old"}})
	next := PromptSectionSnapshotFromTrace([]promptcompiler.PromptSectionTrace{{ID: "profile.writer", Source: "profile", Hash: "sha256:new"}})

	if base.Hash == next.Hash {
		t.Fatalf("prompt section hash did not change")
	}
}
