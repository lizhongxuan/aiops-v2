package opsmanual

import "testing"

func TestPostgresCapabilityPackContributesProbeMappingsOnlyThroughRegistry(t *testing.T) {
	registry := NewCapabilityRegistry(PostgresCapabilityPack())
	if got := registry.DetectObjectType("PG 主从集群异常"); got != "postgresql" {
		t.Fatalf("DetectObjectType = %q, want postgresql", got)
	}
	probes := registry.PreflightProbesFor("postgresql", "repair")
	if len(probes) == 0 {
		t.Fatalf("expected postgres probes from capability pack")
	}
	for _, probe := range probes {
		if !probe.ReadOnly {
			t.Fatalf("postgres evidence probes must be readonly: %#v", probe)
		}
	}
}
