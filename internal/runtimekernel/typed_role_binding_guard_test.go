package runtimekernel

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestTypedRoleBindingGuardUsesCurrentFrozenAdmissionInsteadOfPriorSessionState(t *testing.T) {
	currentRole := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-current"},
		Role:         "primary",
		SourceTurnID: "turn-current",
		Confidence:   1,
	})
	priorRole := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-prior"},
		Role:         "standby",
		SourceTurnID: "turn-prior",
		Confidence:   1,
	})
	snapshot := typedRoleBindingGuardSnapshot(t, "host-current", currentRole)
	session := &SessionState{
		HostID:               "host-prior",
		ResourceRoleBindings: []resourcebinding.ResourceRoleBinding{priorRole},
		RoleBindingConflicts: []resourcebinding.RoleBindingConflict{{
			ResourceID: "host-prior",
			Role:       "standby",
			Reasons:    []string{"historical_conflict"},
		}},
	}

	config := roleBindingGuardConfigFromSession(session, snapshot)

	if !config.Enabled || config.BoundHostID != "host-current" || config.BoundRole != resourcebinding.NormalizeRole(currentRole.Role) {
		t.Fatalf("guard authority = %#v, want current frozen host/role", config)
	}
	if config.RoleBindingHash != currentRole.TraceHash {
		t.Fatalf("role binding hash = %q, want current frozen hash %q", config.RoleBindingHash, currentRole.TraceHash)
	}
	if len(config.RoleBindings) != 1 || config.RoleBindings[0].ResourceRef.ID != "host-current" {
		t.Fatalf("role bindings = %#v, want only current frozen binding", config.RoleBindings)
	}
	if len(config.RoleConflicts) != 0 {
		t.Fatalf("historical conflicts entered current guard: %#v", config.RoleConflicts)
	}
	reason, blocked := (&ToolDispatcher{roleBindingGuard: config}).checkRoleBindingGuard(ToolCall{
		Arguments: json.RawMessage(`{"hostId":"host-current","targetRole":"primary","roleBindingHash":"` + currentRole.TraceHash + `"}`),
	}, tooling.ToolMetadata{Mutating: true})
	if blocked {
		t.Fatalf("current frozen binding blocked by prior session state: %s", reason)
	}
}

func TestTypedRoleBindingGuardMetadataSpoofCannotOverrideFrozenAdmission(t *testing.T) {
	currentRole := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-typed"},
		Role:         "primary",
		SourceTurnID: "turn-typed",
		Confidence:   1,
	})
	snapshot := typedRoleBindingGuardSnapshot(t, "host-typed", currentRole)
	snapshot.Metadata["boundHostId"] = "host-spoofed"
	snapshot.Metadata["boundRole"] = "standby"
	snapshot.Metadata["roleBindingHash"] = "sha256:spoofed"

	config := roleBindingGuardConfigFromSession(nil, snapshot)

	if config.BoundHostID != "host-typed" || config.BoundRole != resourcebinding.NormalizeRole(currentRole.Role) || config.RoleBindingHash != currentRole.TraceHash {
		t.Fatalf("metadata spoof overrode frozen admission: %#v", config)
	}
}

func TestTypedRoleBindingGuardMissingFrozenAssemblyBlocksMutation(t *testing.T) {
	config := roleBindingGuardConfigFromSession(&SessionState{HostID: "host-session"}, &TurnSnapshot{
		Metadata: map[string]string{metadataRoleBindingGuardEnabled: "true"},
	})
	reason, blocked := (&ToolDispatcher{roleBindingGuard: config}).checkRoleBindingGuard(ToolCall{
		Arguments: json.RawMessage(`{"hostId":"host-session"}`),
	}, tooling.ToolMetadata{Mutating: true})

	if !blocked {
		t.Fatalf("mutation allowed without a valid frozen assembly: config=%#v reason=%q", config, reason)
	}
}

func TestTypedRoleBindingGuardUsesFrozenConflictsWithoutMetadataFlag(t *testing.T) {
	role := resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
		ResourceRef:  resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: "host-conflict"},
		Role:         "primary",
		SourceTurnID: "turn-conflict",
		Confidence:   1,
	})
	conflict := resourcebinding.RoleBindingConflict{
		ResourceID: "host-conflict",
		Role:       "primary",
		Reasons:    []string{"needs_confirmation"},
	}
	withConflict := typedRoleBindingGuardSnapshot(t, "host-conflict", role, conflict)
	withoutConflict := typedRoleBindingGuardSnapshot(t, "host-conflict", role)
	delete(withConflict.Metadata, metadataRoleBindingGuardEnabled)

	config := roleBindingGuardConfigFromSession(nil, withConflict)
	if !config.Enabled || len(config.RoleConflicts) != 1 {
		t.Fatalf("guard config = %#v, want frozen conflict enabled without metadata", config)
	}
	if withConflict.TurnAssembly.Hash == withoutConflict.TurnAssembly.Hash {
		t.Fatal("TurnAssembly hash ignored frozen role conflicts")
	}
	reason, blocked := (&ToolDispatcher{roleBindingGuard: config}).checkRoleBindingGuard(ToolCall{
		Arguments: json.RawMessage(`{"hostId":"host-conflict"}`),
	}, tooling.ToolMetadata{Mutating: true})
	if !blocked || reason == "" {
		t.Fatalf("mutation allowed with frozen role conflict: blocked=%v reason=%q", blocked, reason)
	}
}

func typedRoleBindingGuardSnapshot(t *testing.T, hostID string, role resourcebinding.ResourceRoleBinding, conflicts ...resourcebinding.RoleBindingConflict) *TurnSnapshot {
	t.Helper()
	target := resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: hostID}
	facts, err := runtimecontract.BuildAdmissionFacts(runtimecontract.AdmissionInput{
		Intent:        &runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindDiagnose, RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly}},
		SessionTarget: target,
		TargetRefs:    []resourcebinding.ResourceRef{target},
		RoleBindings:  []resourcebinding.ResourceRoleBinding{role},
		RoleConflicts: conflicts,
		SourceRefs:    []string{"typed-role-binding-guard-test"},
	})
	if err != nil {
		t.Fatalf("BuildAdmissionFacts() error = %v", err)
	}
	assembly, err := agentassembly.BuildTurnAssembly(agentassembly.TurnAssemblyInput{
		AdmissionFacts:      facts,
		CapabilityPolicy:    agentassembly.CapabilityPolicySnapshot{PolicyHash: "sha256:typed-policy"},
		ContextPolicy:       agentassembly.ContextSelectorSnapshot{Policy: "bounded"},
		LoopPolicy:          agentassembly.LoopPolicySnapshot{MaxIterations: 2, ToolCallPolicy: "governed"},
		FinalContractPolicy: agentassembly.FinalContractSnapshot{Shape: "typed"},
		RollbackPolicy:      "not-required-for-read-only",
		SourceRefs:          []string{"typed-role-binding-guard-test"},
	})
	if err != nil {
		t.Fatalf("BuildTurnAssembly() error = %v", err)
	}
	return &TurnSnapshot{
		ID:           "turn-typed-role-binding-guard",
		SessionID:    "session-typed-role-binding-guard",
		TurnAssembly: &assembly,
		Metadata:     map[string]string{metadataRoleBindingGuardEnabled: "true"},
	}
}
