package opsmanual

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCapabilityRegistryBuiltinAliasesWinConflicts(t *testing.T) {
	registry := NewCapabilityRegistry(
		OpsManualCapabilityPack{
			ID:      "external-conflict",
			Enabled: true,
			ObjectAliases: []CapabilityAlias{{
				Value:   "cache_cluster",
				Needles: []string{"redis"},
			}},
		},
	)
	for _, pack := range OpsManualCoreCapabilityPacks() {
		if err := registry.RegisterPack(pack); err != nil {
			t.Fatalf("RegisterPack(%s) error = %v", pack.ID, err)
		}
	}

	if got := registry.DetectObjectType("Redis used_memory_rss rising"); got != "redis" {
		t.Fatalf("DetectObjectType() = %q, want redis from built-in pack", got)
	}
	if got := registry.DetectOperationType("PG backup"); got != "backup" {
		t.Fatalf("DetectOperationType() = %q, want backup from built-in pack", got)
	}
}

func TestCapabilityRegistryDoesNotLowerCoreRisk(t *testing.T) {
	registry := NewCapabilityRegistry(
		OpsManualCapabilityPack{
			ID:      "opsmanual-core.preflight",
			BuiltIn: true,
			Enabled: true,
			PreflightProbes: []CapabilityPreflightProbe{{
				ID:         "postgresql_restore_probe",
				TargetType: "postgresql",
				Action:     "restore",
				RiskLevel:  "high",
			}},
		},
		OpsManualCapabilityPack{
			ID:      "external-lower-risk",
			Enabled: true,
			PreflightProbes: []CapabilityPreflightProbe{{
				ID:         "postgresql_restore_probe",
				TargetType: "postgresql",
				Action:     "restore",
				RiskLevel:  "read_only",
			}},
		},
	)

	probe, ok := registry.PreflightProbeFor("postgresql", "restore")
	if !ok {
		t.Fatalf("PreflightProbeFor(postgresql, restore) not found")
	}
	if probe.RiskLevel != "high" {
		t.Fatalf("RiskLevel = %q, want high", probe.RiskLevel)
	}
}

func TestCapabilityRegistryDisabledPackUnavailable(t *testing.T) {
	registry := NewCapabilityRegistry(OpsManualCapabilityPack{
		ID:      "opsmanual-core.kubernetes",
		BuiltIn: true,
		Enabled: false,
		ResourceProviders: []CapabilityResourceProvider{{
			ID: "kubernetes",
		}},
	})

	if providers := registry.ResourceDiscoveryProviders(); len(providers) != 0 {
		t.Fatalf("ResourceDiscoveryProviders() = %#v, want none", providers)
	}
	msg := registry.UnavailableMessage("kubernetes")
	if !strings.Contains(msg, "opsmanual-core.kubernetes") || !strings.Contains(msg, "unavailable") {
		t.Fatalf("UnavailableMessage() = %q, want clear disabled pack message", msg)
	}
}

func TestLocalResourceDiscoveryDoesNotCallDisabledKubernetesProvider(t *testing.T) {
	registry := NewCapabilityRegistry()
	for _, pack := range OpsManualCoreCapabilityPacks() {
		if pack.ID == "opsmanual-core.kubernetes" {
			pack.Enabled = false
		}
		if err := registry.RegisterPack(pack); err != nil {
			t.Fatalf("RegisterPack(%s) error = %v", pack.ID, err)
		}
	}
	var commands []string
	discovery := NewLocalResourceDiscoveryWithRunnerAndRegistry(func(_ context.Context, command string, args ...string) ([]byte, error) {
		commands = append(commands, command+" "+strings.Join(args, " "))
		switch command {
		case "docker", "ps":
			return []byte(""), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}, registry)

	resources, err := discovery.DiscoverHostResources(context.Background(), "server-local")
	if err != nil {
		t.Fatalf("DiscoverHostResources() error = %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("resources = %#v, want no candidates", resources)
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "kubectl ") {
			t.Fatalf("disabled Kubernetes provider was called: %s", command)
		}
	}
	if msg := registry.UnavailableMessage("kubernetes"); !strings.Contains(msg, "opsmanual-core.kubernetes") {
		t.Fatalf("UnavailableMessage(kubernetes) = %q, want disabled pack id", msg)
	}
}

func TestBuildOperationFrameWithCapabilityRegistryUsesPackAliases(t *testing.T) {
	registry := NewCapabilityRegistry(OpsManualCapabilityPack{
		ID:      "external-mongo",
		Enabled: true,
		ObjectAliases: []CapabilityAlias{{
			Value:   "mongodb",
			Needles: []string{"mongo"},
		}},
		OperationAliases: []CapabilityAlias{{
			Value:   "backup",
			Needles: []string{"snapshot"},
		}},
		ParameterHints: []CapabilityParameterHint{{
			ID:         "target_instance",
			TargetType: "mongodb",
			Action:     "backup",
			Required:   true,
			Source:     "capability_pack:external-mongo",
		}},
	})

	frame := BuildOperationFrameWithCapabilityRegistry("take a mongo snapshot", nil, registry)
	if frame.Target.Type != "mongodb" || frame.Operation.Action != "backup" {
		t.Fatalf("frame target/action = %q/%q, want mongodb/backup", frame.Target.Type, frame.Operation.Action)
	}
	if !contains(frame.Evidence.Missing, "target_instance") {
		t.Fatalf("missing = %#v, want target_instance from capability hint", frame.Evidence.Missing)
	}
}

func TestWorkflowAnalyzerAddsCapabilityPreflightProbeMetadata(t *testing.T) {
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID: "self-healing-kubelet",
		RawYAML:    loadWorkflowReverseFixture(t, "self_healing_kubelet.yaml"),
	})
	if err != nil {
		t.Fatalf("AnalyzeWorkflowForManual() error = %v", err)
	}
	rawProbe, ok := analysis.XOpsManual["preflight_probe"].(map[string]any)
	if !ok {
		t.Fatalf("XOpsManual[preflight_probe] = %#v, want capability probe metadata", analysis.XOpsManual["preflight_probe"])
	}
	if rawProbe["id"] != "kubelet_repair_probe" || rawProbe["type"] != "capability_pack" {
		t.Fatalf("preflight_probe = %#v, want kubelet capability probe", rawProbe)
	}
	if analysis.Operation.RiskLevel != "high" {
		t.Fatalf("RiskLevel = %q, want high from capability probe", analysis.Operation.RiskLevel)
	}
	if len(analysis.Evidence["preflight_probe"]) == 0 {
		t.Fatalf("Evidence[preflight_probe] empty, want capability evidence")
	}
}
