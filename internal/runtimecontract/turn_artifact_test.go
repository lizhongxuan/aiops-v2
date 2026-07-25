package runtimecontract

import (
	"encoding/json"
	"testing"
)

func TestTurnArtifactInlineIsImmutableDigestAddressedAndRoundTrips(t *testing.T) {
	value := map[string]any{"revision": "rv-7", "selection": []string{"node-b", "node-a"}}
	artifact, err := NewInlineTurnArtifact("workflow.context", value)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ID == "" || !validTurnArtifactDigest(artifact.Digest) || artifact.Resource != nil {
		t.Fatalf("artifact = %#v", artifact)
	}
	value["revision"] = "mutated"
	if string(artifact.Inline) != `{"revision":"rv-7","selection":["node-b","node-a"]}` {
		t.Fatalf("artifact body changed with source: %s", artifact.Inline)
	}

	raw, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TurnArtifactEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	decoded.Inline[1] = 'X'
	if err := decoded.Validate(); err == nil {
		t.Fatal("tampered inline artifact passed digest validation")
	}
}

func TestTurnArtifactResourceRequiresExactDigestAndOneBody(t *testing.T) {
	inline, err := NewInlineTurnArtifact("source", map[string]string{"ok": "true"})
	if err != nil {
		t.Fatal(err)
	}
	resource, err := NewResourceTurnArtifact("workflow.context", TurnArtifactResourceRef{URI: "workflow-resource://opaque", Digest: inline.Digest, ContentType: "application/json", Bytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	if resource.Resource == nil || len(resource.Inline) != 0 || resource.Digest != inline.Digest {
		t.Fatalf("resource artifact = %#v", resource)
	}
	tampered := resource.Clone()
	tampered.Resource.Digest = "sha256:" + "0" + tampered.Resource.Digest[len("sha256:")+1:]
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered resource digest was accepted")
	}
	both := resource.Clone()
	both.Inline = json.RawMessage(`{}`)
	if err := both.Validate(); err == nil {
		t.Fatal("artifact with inline and resource bodies was accepted")
	}
}
