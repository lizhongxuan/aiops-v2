package coroot

import "testing"

func TestMapCorootRCAContextToEvidencePackPreservesDependencyRootCause(t *testing.T) {
	payload := map[string]any{
		"service": "service-a",
		"dependencies": []any{
			map[string]any{"from": "service-a", "to": "service-b", "status": "degraded"},
			map[string]any{"from": "service-b", "to": "service-c", "status": "degraded"},
		},
		"hypotheses": []any{
			map[string]any{"entity": "service-c", "summary": "CPU saturation", "confidence": "high"},
		},
	}

	pack := MapCorootEvidencePack("env-a", payload)
	if pack.Provider != "coroot" || pack.Project != "env-a" || pack.Target.Name != "service-a" {
		t.Fatalf("pack target = %#v", pack)
	}
	if len(pack.DependencyEdges) != 2 || pack.DependencyEdges[1].To != "service-c" {
		t.Fatalf("edges = %#v", pack.DependencyEdges)
	}
	if pack.Hypotheses[0].Entity != "service-c" {
		t.Fatalf("hypotheses = %#v", pack.Hypotheses)
	}
}

func TestMapCorootRCAContextToEvidencePackTreatsErrorAsMissingEvidence(t *testing.T) {
	payload := map[string]any{
		"status":  "error",
		"service": "service-a",
		"error": map[string]any{
			"kind":    "unavailable",
			"message": "Coroot endpoint is unavailable",
		},
	}

	pack := MapCorootEvidencePack("env-a", payload)
	if pack.Provider != "coroot" || pack.Target.Name != "service-a" {
		t.Fatalf("pack target = %#v", pack)
	}
	if len(pack.MissingEvidence) == 0 {
		t.Fatalf("missing evidence is empty: %#v", pack)
	}
	for _, hypothesis := range pack.Hypotheses {
		if hypothesis.Entity == "service-a" && hypothesis.Summary == "service absent" {
			t.Fatalf("error-shaped payload must not claim service absence: %#v", pack.Hypotheses)
		}
	}
}
