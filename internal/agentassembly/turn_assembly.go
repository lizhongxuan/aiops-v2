package agentassembly

import (
	"fmt"
	"strings"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
)

const TurnAssemblySchemaVersion = "aiops.turn-assembly.v1"

type CapabilityPolicySnapshot = ToolSurfaceSnapshot

type TurnAssemblyInput struct {
	AdmissionFacts      runtimecontract.AdmissionFacts
	AdmissionError      string
	PermissionProfile   string
	CapabilityPolicy    CapabilityPolicySnapshot
	ContextPolicy       ContextSelectorSnapshot
	LoopPolicy          LoopPolicySnapshot
	FinalContractPolicy FinalContractSnapshot
	RollbackPolicy      string
	SourceRefs          []string
}

type TurnAssembly struct {
	SchemaVersion       string                         `json:"schemaVersion"`
	AdmissionFacts      runtimecontract.AdmissionFacts `json:"admissionFacts"`
	PermissionProfile   string                         `json:"permissionProfile,omitempty"`
	CapabilityPolicy    CapabilityPolicySnapshot       `json:"capabilityPolicy"`
	ContextPolicy       ContextSelectorSnapshot        `json:"contextPolicy"`
	LoopPolicy          LoopPolicySnapshot             `json:"loopPolicy"`
	FinalContractPolicy FinalContractSnapshot          `json:"finalContractPolicy"`
	RollbackPolicy      string                         `json:"rollbackPolicy,omitempty"`
	SourceRefs          []string                       `json:"sourceRefs,omitempty"`
	Hash                string                         `json:"hash"`
}

func BuildTurnAssembly(input TurnAssemblyInput) (TurnAssembly, error) {
	frozen, err := cloneJSONValue(input)
	if err != nil {
		return TurnAssembly{}, fmt.Errorf("clone turn assembly input: %w", err)
	}
	if err := runtimecontract.ValidateAdmissionFactsIntegrity(frozen.AdmissionFacts); err != nil {
		return TurnAssembly{}, fmt.Errorf("invalid admission facts: %w", err)
	}
	if strings.TrimSpace(frozen.AdmissionError) != "" {
		return TurnAssembly{}, fmt.Errorf("admission facts validation failed")
	}
	inputPermission := strings.TrimSpace(frozen.PermissionProfile)
	admissionPermission := strings.TrimSpace(frozen.AdmissionFacts.PermissionProfile)
	permissionProfile := firstNonEmptyAssemblyValue(inputPermission, admissionPermission)
	if inputPermission != "" && admissionPermission != "" && inputPermission != admissionPermission {
		return TurnAssembly{}, fmt.Errorf("permission profile conflicts with admission facts")
	}
	capabilityPolicy, err := normalizeCapabilityPolicy(frozen.CapabilityPolicy)
	if err != nil {
		return TurnAssembly{}, err
	}
	frozen.ContextPolicy.Hash = ""
	frozen.FinalContractPolicy.Hash = ""
	assembly := TurnAssembly{
		SchemaVersion:       TurnAssemblySchemaVersion,
		AdmissionFacts:      frozen.AdmissionFacts,
		PermissionProfile:   permissionProfile,
		CapabilityPolicy:    capabilityPolicy,
		ContextPolicy:       normalizeContextSelector(frozen.ContextPolicy),
		LoopPolicy:          normalizeLoopPolicy(frozen.LoopPolicy),
		FinalContractPolicy: normalizeFinalContract(frozen.FinalContractPolicy),
		RollbackPolicy:      strings.TrimSpace(frozen.RollbackPolicy),
		SourceRefs:          uniqueSortedStrings(append(append([]string(nil), frozen.AdmissionFacts.SourceRefs...), frozen.SourceRefs...)),
	}
	if turnAssemblyMutates(assembly.AdmissionFacts.Intent) {
		if assembly.PermissionProfile == "" {
			return TurnAssembly{}, fmt.Errorf("mutation turn assembly requires permission profile")
		}
		if assembly.RollbackPolicy == "" {
			return TurnAssembly{}, fmt.Errorf("mutation turn assembly requires rollback policy")
		}
	}
	admissionControlHash := StableHash("turn-assembly.admission-control", map[string]any{
		"intent": assembly.AdmissionFacts.Intent, "userConstraints": assembly.AdmissionFacts.UserConstraints,
		"sessionTarget": assembly.AdmissionFacts.SessionTarget, "resourceBindings": assembly.AdmissionFacts.ResourceBindings,
		"roleBindings": assembly.AdmissionFacts.RoleBindings, "agentKind": assembly.AdmissionFacts.AgentKind,
		"profile": assembly.AdmissionFacts.Profile, "permissionProfile": assembly.AdmissionFacts.PermissionProfile,
		"sourceRefs": assembly.AdmissionFacts.SourceRefs,
	})
	assembly.Hash = StableHash("turn-assembly", map[string]any{
		"schemaVersion": assembly.SchemaVersion, "admissionControlHash": admissionControlHash,
		"permissionProfile": assembly.PermissionProfile, "capabilityPolicyHash": assembly.CapabilityPolicy.Hash,
		"contextPolicyHash": assembly.ContextPolicy.Hash, "loopPolicyHash": assembly.LoopPolicy.Hash,
		"finalContractPolicyHash": assembly.FinalContractPolicy.Hash, "rollbackPolicy": assembly.RollbackPolicy,
		"sourceRefs": assembly.SourceRefs,
	})
	return assembly, nil
}

func (assembly TurnAssembly) Validate() error {
	if strings.TrimSpace(assembly.SchemaVersion) != TurnAssemblySchemaVersion {
		return fmt.Errorf("invalid turn assembly schema version")
	}
	rebuilt, err := rebuildTurnAssembly(assembly)
	if err != nil {
		return err
	}
	if strings.TrimSpace(assembly.Hash) == "" || assembly.Hash != rebuilt.Hash {
		return fmt.Errorf("turn assembly hash mismatch")
	}
	return nil
}

func rebuildTurnAssembly(assembly TurnAssembly) (TurnAssembly, error) {
	return BuildTurnAssembly(TurnAssemblyInput{
		AdmissionFacts: assembly.AdmissionFacts, PermissionProfile: assembly.PermissionProfile,
		CapabilityPolicy: assembly.CapabilityPolicy, ContextPolicy: assembly.ContextPolicy,
		LoopPolicy: assembly.LoopPolicy, FinalContractPolicy: assembly.FinalContractPolicy,
		RollbackPolicy: assembly.RollbackPolicy, SourceRefs: assembly.SourceRefs,
	})
}

func normalizeCapabilityPolicy(input CapabilityPolicySnapshot) (CapabilityPolicySnapshot, error) {
	input.Lifecycle = LifecycleDispatchScope
	input.PolicyHash = strings.TrimSpace(input.PolicyHash)
	input.Fingerprint = strings.TrimSpace(input.Fingerprint)
	sortToolSurfaceItems(input.RegisteredTools)
	sortToolSurfaceItems(input.ModelVisibleTools)
	sortToolSurfaceItems(input.DispatchableTools)
	sortToolSurfaceItems(input.HiddenTools)
	if len(input.DispatchableTools) == 0 {
		input.DispatchableTools = append([]ToolSurfaceItem(nil), input.ModelVisibleTools...)
	}
	if len(input.RegisteredTools) == 0 {
		input.RegisteredTools = append([]ToolSurfaceItem(nil), input.DispatchableTools...)
	}
	if input.Fingerprint == "" {
		input.Fingerprint = StableHash("tool-surface.fingerprint", map[string]any{
			"visible": input.ModelVisibleTools, "dispatchable": input.DispatchableTools,
			"hidden": input.HiddenTools, "policyHash": input.PolicyHash,
		})
	}
	input.Hash = StableHash("tool-surface.snapshot", map[string]any{
		"registered": input.RegisteredTools, "visible": input.ModelVisibleTools,
		"dispatchable": input.DispatchableTools, "hidden": input.HiddenTools,
		"policyHash": input.PolicyHash, "fingerprint": input.Fingerprint,
	})
	if err := input.Validate(); err != nil {
		return CapabilityPolicySnapshot{}, fmt.Errorf("invalid capability policy: %w", err)
	}
	return input, nil
}

func turnAssemblyMutates(frame runtimecontract.IntentFrame) bool {
	return frame.Kind == runtimecontract.IntentKindChange || frame.Kind == runtimecontract.IntentKindConfigure ||
		runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskWrite) ||
		runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) ||
		runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskDestruct)
}

func firstNonEmptyAssemblyValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func turnAssemblySessionTargets(facts runtimecontract.AdmissionFacts) []resourcebinding.ResourceRef {
	if facts.SessionTarget.IdentityHash() == "" {
		return nil
	}
	return []resourcebinding.ResourceRef{facts.SessionTarget}
}
