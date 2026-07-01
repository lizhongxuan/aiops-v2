package envcontext

import (
	"strings"
	"testing"
)

func TestResolveEnvironmentFactsMergesObservedStaticGraphAndToolFacts(t *testing.T) {
	resolution := ResolveEnvironmentFacts(ResolverInput{
		Input: "检查 checkout 服务异常",
		HostFacts: []EnvironmentFact{
			{
				Kind:       FactKindObservedSymptom,
				Subject:    "host:10.0.0.2",
				Value:      "cpu_iowait_high",
				Source:     FactSourceHostObserved,
				Confidence: FactConfidenceObserved,
			},
		},
		ToolFacts: []EnvironmentFact{
			{
				Kind:       FactKindPort,
				Subject:    "service:checkout",
				Value:      "8080",
				Source:     FactSourceToolOutput,
				Confidence: FactConfidenceConfirmed,
			},
		},
		InventoryFacts: []EnvironmentFact{
			{
				Kind:       FactKindHostIdentity,
				Subject:    "checkout-primary",
				Value:      "10.0.0.2",
				Source:     FactSourceInventory,
				Confidence: FactConfidenceInferred,
			},
		},
		OpsGraphFacts: []EnvironmentFact{
			{
				Kind:       FactKindTopology,
				Subject:    "service:checkout",
				Value:      "host:10.0.0.2",
				Source:     FactSourceOpsGraph,
				Confidence: FactConfidenceInferred,
			},
		},
		Now: fixedTime(),
	})

	if len(resolution.ConfirmedFacts) != 2 {
		t.Fatalf("ConfirmedFacts = %#v, want observed host and tool facts", resolution.ConfirmedFacts)
	}
	if len(resolution.InferredFacts) != 2 {
		t.Fatalf("InferredFacts = %#v, want inventory and opsgraph facts", resolution.InferredFacts)
	}
	if len(resolution.MissingFacts) != 1 || resolution.MissingFacts[0] != "target_ref" {
		t.Fatalf("MissingFacts = %#v, want target_ref", resolution.MissingFacts)
	}
	compact := resolution.CompactContext()
	for _, want := range []string{"ConfirmedFacts", "observed_symptom=cpu_iowait_high", "port=8080", "InferredFacts", "host_identity=10.0.0.2", "topology=host:10.0.0.2", "MissingFacts"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("compact context missing %q:\n%s", want, compact)
		}
	}
}
