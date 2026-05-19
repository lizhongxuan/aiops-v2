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
	want := []string{"selected_host", "explicit_resource_match", "conversation", "manual_default", "run_record", "coroot", "host_readonly", "docker", "k8s", "hint_provider"}
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

func TestResourceResolverCopiesDiscoveryMetadataIntoParamCandidates(t *testing.T) {
	registry := NewDefaultParamResolverRegistry(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{
				ID:              "k8s:pod:cache/redis-0",
				Name:            "redis-0",
				Type:            "redis",
				Surface:         "kubectl -n cache exec redis-0 --",
				Source:          "k8s",
				Namespace:       "cache",
				Pod:             "redis-0",
				Phase:           "Running",
				ContainerImages: []string{"redis:7.2"},
				Confidence:      0.88,
			},
		},
	})
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "target_instance", Type: "resource_ref", Required: true},
		OperationFrame: OperationFrame{ObjectType: "redis"},
	})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want one k8s candidate", result.Candidates)
	}
	metadata := result.Candidates[0].Metadata
	if metadata["namespace"] != "cache" || metadata["phase"] != "Running" {
		t.Fatalf("candidate metadata = %#v, want namespace and phase", metadata)
	}
	if images, ok := metadata["container_images"].([]string); !ok || len(images) != 1 || images[0] != "redis:7.2" {
		t.Fatalf("candidate metadata images = %#v", metadata["container_images"])
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

func TestSensitiveParamRejectsConfirmedPlaintext(t *testing.T) {
	ledger := NewOperationContextLedger()
	ledger.AddFact(OperationContextFact{Key: "password", Value: "plain-secret", Source: "user_form", Confidence: 0.99, ConfirmedByUser: true, Sensitive: true})
	registry := NewDefaultParamResolverRegistry(nil)
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement: ParamRequirement{ID: "password", Type: "secret_ref", Required: true, Sensitive: true},
		Ledger:      ledger,
	})
	if len(result.Candidates) != 0 {
		t.Fatalf("candidates = %#v, want plaintext secret rejected", result.Candidates)
	}
}

func TestParamHintsRequireCurrentConfirmationAndDoNotOverrideExplicitTarget(t *testing.T) {
	provider := staticHintProvider{
		paramHints: []ParamHint{{
			ParamID:                     "target_instance",
			Value:                       "redis-from-memory",
			Label:                       "redis-from-memory",
			Source:                      "memory_hint",
			Redacted:                    true,
			RequiresCurrentConfirmation: true,
			Confidence:                  0.72,
		}},
	}
	registry := NewParamResolverRegistry(nil, provider)
	result, _ := registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "target_instance", Type: "resource_ref", Required: true},
		OperationFrame: BuildOperationFrame("排查 Redis", nil),
		Manual:         redisParamTestManual("manual-redis-hints"),
	})
	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %#v, want one memory hint candidate", result.Candidates)
	}
	candidate := result.Candidates[0]
	if candidate.Source != "memory_hint" || candidate.Confidence >= 0.85 || candidate.Metadata["requires_current_confirmation"] != true {
		t.Fatalf("candidate = %#v, want low-confidence memory hint requiring confirmation", candidate)
	}

	explicitFrame := BuildOperationFrame("排查 Redis redis-current", map[string]any{"target_name": "redis-current"})
	result, _ = registry.Resolve(context.Background(), ParamResolverRequest{
		Requirement:    ParamRequirement{ID: "target_instance", Type: "resource_ref", Required: true},
		OperationFrame: explicitFrame,
		Manual:         redisParamTestManual("manual-redis-hints"),
		Ledger:         LedgerFromOperationFrame(explicitFrame),
	})
	if len(result.Candidates) != 1 || result.Candidates[0].Value != "redis-current" || result.Candidates[0].Source == "memory_hint" {
		t.Fatalf("candidates = %#v, explicit target must win over hint", result.Candidates)
	}
}

type staticHintProvider struct {
	manualHints []ManualHint
	paramHints  []ParamHint
}

func (p staticHintProvider) ManualHints(context.Context, HintQuery) ([]ManualHint, error) {
	return cloneManualHints(p.manualHints), nil
}

func (p staticHintProvider) ParamHints(context.Context, HintQuery) ([]ParamHint, error) {
	return cloneParamHints(p.paramHints), nil
}

func redisParamTestManual(id string) OpsManual {
	return OpsManual{
		ID:          id,
		Title:       "Redis RCA",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-" + id},
		Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
			Platform:         []string{"vm"},
		},
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
