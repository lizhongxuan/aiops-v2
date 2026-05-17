package opsmanual

import (
	"context"
	"testing"
)

func TestParamResolverRegistryOrderAndOutcomes(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{
		resources: []ResourceCandidate{{ID: "docker:redis-a", Name: "redis-a", Type: "redis", Source: "docker", Confidence: 0.92}},
	})
	names := registry.Names()
	want := []string{"selected_host", "explicit_resource_match", "conversation", "manual_default", "run_record", "coroot", "host_readonly", "docker", "k8s"}
	if len(names) != len(want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %#v, want order %#v", names, want)
		}
	}

	ledger := NewOperationContextLedger()
	ledger.AddFact(OperationContextFact{Key: "target_host", Value: "server-local", Source: "selected_host", Confidence: 0.95})
	resolved, logs := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "target_host", Type: "host_ref", Required: true},
		Ledger:      ledger,
	})
	if len(resolved.Candidates) != 1 || resolved.Candidates[0].Value != "server-local" || logs[0].Resolver != "selected_host" {
		t.Fatalf("resolved = %#v logs=%#v, want selected host candidate", resolved, logs)
	}
}

func TestParamResolversReturnAmbiguousAndMissing(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:redis-a", Name: "redis-a", Type: "redis", Source: "docker", Confidence: 0.91},
			{ID: "docker:redis-b", Name: "redis-b", Type: "redis", Source: "docker", Confidence: 0.91},
		},
	})
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "redis_instance", Type: "resource_ref", Required: true},
		OperationFrame: OperationFrame{ObjectType: "redis"},
	})
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %#v, want two redis candidates", result.Candidates)
	}
	emptyRegistry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{})
	missing, _ := emptyRegistry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "backup_path", Type: "path", Required: true},
	})
	if len(missing.Candidates) != 0 {
		t.Fatalf("missing candidates = %#v, want none", missing.Candidates)
	}
}

func TestDockerResolverFiltersByManualApplicability(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:aiops-redis", Name: "aiops-redis", Type: "redis", Surface: "docker exec aiops-redis", Source: "docker", Confidence: 0.92},
			{ID: "k8s:service:cache/redis", Name: "redis", Type: "redis", Surface: "kubectl -n cache get service redis", Source: "k8s", Namespace: "cache", Service: "redis", Confidence: 0.88},
		},
	})
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "target_instance", Type: "resource_ref", Required: true},
		OperationFrame: OperationFrame{ObjectType: "redis"},
		Manual: OpsManual{Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			Platform:         []string{"vm"},
			ExecutionSurface: []string{"ssh"},
		}},
	})
	if len(result.Candidates) != 1 || result.Candidates[0].Value != "docker:aiops-redis" {
		t.Fatalf("candidates = %#v, want only docker candidate for vm/ssh manual", result.Candidates)
	}
}

func TestK8sApplicabilityFiltersOutDockerCandidates(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:aiops-redis", Name: "aiops-redis", Type: "redis", Surface: "docker exec aiops-redis", Source: "docker", Confidence: 0.92},
			{ID: "k8s:service:cache/redis", Name: "redis", Type: "redis", Surface: "kubectl -n cache get service redis", Source: "k8s", Namespace: "cache", Service: "redis", Confidence: 0.88},
		},
	})
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "target_instance", Type: "resource_ref", Required: true},
		OperationFrame: OperationFrame{ObjectType: "redis"},
		Manual: OpsManual{Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			Platform:         []string{"kubernetes"},
			ExecutionSurface: []string{"kubectl"},
		}},
	})
	if len(result.Candidates) != 1 || result.Candidates[0].Value != "k8s:service:cache/redis" {
		t.Fatalf("candidates = %#v, want only k8s candidate for kubernetes/kubectl manual", result.Candidates)
	}
}

func TestSensitiveParamIgnoresLowConfidenceConversationGuess(t *testing.T) {
	ledger := NewOperationContextLedger()
	ledger.AddFact(OperationContextFact{Key: "password", Value: "guess", Source: "conversation", Confidence: 0.4, Sensitive: true})
	registry := NewDefaultParamResolverRegistry(nil)
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "password", Type: "secret_ref", Required: true, Sensitive: true},
		Ledger:      ledger,
	})
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want no low-confidence sensitive guess", result.Candidates)
	}
}

func TestSensitiveParamAcceptsUserConfirmedSecretRef(t *testing.T) {
	ledger := NewOperationContextLedger()
	ledger.AddFact(OperationContextFact{Key: "password", Value: "secret://team/db-password", Source: "user", Confidence: 0.99, ConfirmedByUser: true, Sensitive: true})
	registry := NewDefaultParamResolverRegistry(nil)
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "password", Type: "secret_ref", Required: true, Sensitive: true},
		Ledger:      ledger,
	})
	if len(result.Candidates) != 1 || result.Candidates[0].Value != "secret://team/db-password" {
		t.Fatalf("candidates = %#v, want user-confirmed secret ref", result.Candidates)
	}
}

type fakeResourceDiscovery struct {
	resources []ResourceCandidate
	surfaces  []ParamCandidate
}

func (f fakeResourceDiscovery) DiscoverHostResources(context.Context, string) ([]ResourceCandidate, error) {
	return f.resources, nil
}

func (f fakeResourceDiscovery) DiscoverExecutionSurfaces(context.Context, string) ([]ParamCandidate, error) {
	return f.surfaces, nil
}
