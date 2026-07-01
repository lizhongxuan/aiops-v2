package observability

import (
	"encoding/json"
	"testing"
)

func TestEvidencePackRoundTripPreservesDependencyAndMissingEvidence(t *testing.T) {
	pack := EvidencePack{
		Provider: "synthetic",
		Target:   EntityRef{Kind: "service", Name: "service-a"},
		DependencyEdges: []DependencyEdge{
			{From: "service-a", To: "service-b"},
			{From: "service-b", To: "service-c"},
		},
		Hypotheses: []Hypothesis{
			{Entity: "service-c", Summary: "dependency saturation", Confidence: "medium"},
		},
		MissingEvidence: []string{"logs for service-c"},
	}

	data, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}

	var decoded EvidencePack
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.DependencyEdges[1].To != "service-c" ||
		decoded.Hypotheses[0].Entity != "service-c" ||
		decoded.MissingEvidence[0] != "logs for service-c" {
		t.Fatalf("decoded evidence pack = %#v", decoded)
	}
}
