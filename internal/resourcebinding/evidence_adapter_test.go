package resourcebinding

import "testing"

func TestEvidenceRefBindsExplicitResource(t *testing.T) {
	evidence := BuildEvidenceRef(EvidenceInput{
		ID:          "ev-1",
		ResourceRef: ResourceRef{Type: ResourceTypeHost, ID: "host-a"},
		Source:      EvidenceSourceTool,
		Kind:        EvidenceKindCommandOutput,
	})

	if evidence.ResourceRef.Type != ResourceTypeHost || evidence.ResourceRef.ID != "host-a" {
		t.Fatalf("evidence resource = %+v, want host-a", evidence.ResourceRef)
	}
	if evidence.TraceHash == "" {
		t.Fatalf("evidence TraceHash is empty")
	}
}

func TestUserEvidenceWithoutResourceStaysSessionLevel(t *testing.T) {
	evidence := BuildEvidenceRef(EvidenceInput{
		ID:     "ev-user",
		Source: EvidenceSourceUser,
		Kind:   EvidenceKindObservation,
	})

	if evidence.ResourceRef.Type != ResourceTypeSession {
		t.Fatalf("evidence resource type = %q, want session", evidence.ResourceRef.Type)
	}
	if evidence.ResourceRef.ID != SessionResourceID {
		t.Fatalf("evidence resource id = %q, want session", evidence.ResourceRef.ID)
	}
	if evidence.ResourceRef.Type == ResourceTypeHost {
		t.Fatalf("session-level evidence forged a host binding: %+v", evidence)
	}
}

func TestHypothesisEvidenceKindIsExplicit(t *testing.T) {
	evidence := BuildEvidenceRef(EvidenceInput{
		ID:          "hyp-1",
		ResourceRef: ResourceRef{Type: ResourceTypeService, ID: "svc-a"},
		Source:      EvidenceSourceRuntime,
		Kind:        EvidenceKindHypothesis,
	})

	if evidence.Kind != EvidenceKindHypothesis {
		t.Fatalf("evidence kind = %q, want hypothesis", evidence.Kind)
	}
}
