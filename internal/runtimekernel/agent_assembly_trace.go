package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func buildAgentAssemblySnapshotForTrace(input agentAssemblyTraceInput) *agentassembly.AgentAssemblySnapshot {
	result, err := buildAgentAssemblyTraceSnapshots(input)
	if err != nil {
		return nil
	}
	return result.Projected
}

type agentAssemblyTraceSnapshots struct {
	Projected *agentassembly.AgentAssemblySnapshot
	Legacy    *agentassembly.AgentAssemblySnapshot
	Shadow    *TurnAssemblyShadowTrace
}

type TurnAssemblyShadowTrace struct {
	AssemblyHash      string                  `json:"assemblyHash"`
	LegacySpecHash    string                  `json:"legacySpecHash"`
	ProjectedSpecHash string                  `json:"projectedSpecHash"`
	Match             bool                    `json:"match"`
	Warning           string                  `json:"warning,omitempty"`
	SourceRefs        []string                `json:"sourceRefs,omitempty"`
	FieldDiffs        []TurnAssemblyFieldDiff `json:"fieldDiffs,omitempty"`
}

type TurnAssemblyFieldDiff struct {
	Field         string `json:"field"`
	LegacyHash    string `json:"legacyHash"`
	ProjectedHash string `json:"projectedHash"`
	Match         bool   `json:"match"`
}

func buildAgentAssemblyTraceSnapshots(input agentAssemblyTraceInput) (agentAssemblyTraceSnapshots, error) {
	buildInput := agentAssemblySnapshotBuildInput(input)
	legacy := agentassembly.Build(buildInput)
	result := agentAssemblyTraceSnapshots{
		Projected: ptrAgentAssemblySnapshot(legacy),
		Legacy:    ptrAgentAssemblySnapshot(legacy),
	}
	if input.TurnAssembly == nil {
		return result, nil
	}
	projected, err := agentassembly.BuildSnapshotFromTurnAssembly(*input.TurnAssembly, buildInput)
	if err != nil {
		return agentAssemblyTraceSnapshots{}, fmt.Errorf("project turn assembly snapshot: %w", err)
	}
	shadow := compareAgentAssemblySnapshots(*input.TurnAssembly, legacy, projected)
	result.Projected = ptrAgentAssemblySnapshot(projected)
	result.Shadow = &shadow
	return result, nil
}

func agentAssemblySnapshotBuildInput(input agentAssemblyTraceInput) agentassembly.BuildInput {
	modelVisible := toolMetadataFromPromptTools(input.CompileContext.AssembledTools)
	hidden := hiddenToolInputsFromPolicy(input.ToolSurfacePolicy)
	return agentassembly.BuildInput{
		AgentKind:         strings.TrimSpace(string(input.AgentKind)),
		Profile:           firstNonBlankRuntimeString(input.CompileContext.Profile, admissionProfileFromTurnAssembly(input.TurnAssembly)),
		RuntimeRole:       string(input.SessionType) + "." + string(input.Mode),
		RouteReason:       routeReasonsFromMetadata(input.Metadata),
		ResourceBindings:  input.ResourceBindings,
		SessionTargets:    sessionTargetRefs(input.SessionTargetSnapshot),
		RoleBindings:      input.RoleBindings,
		RegisteredTools:   modelVisible,
		ModelVisibleTools: modelVisible,
		DispatchableTools: modelVisible,
		HiddenTools:       hidden,
		PolicyHash:        input.ToolSurfacePolicy.Hash,
		Fingerprint:       input.ToolSurfaceFingerprint,
		PromptSections:    input.Compiled.PromptSections,
		ContextSelector: agentassembly.ContextSelectorSnapshot{
			Lifecycle: agentassembly.LifecycleRequestScope,
			Policy:    firstNonBlankRuntimeString(input.CompileContext.EvidencePolicy, input.CompileContext.PlanningPolicy),
			Budget:    input.CompileContext.ToolBudget,
		},
		LoopPolicy: agentassembly.LoopPolicySnapshot{
			Lifecycle:      agentassembly.LifecycleRequestScope,
			ToolCallPolicy: string(input.Mode),
		},
		FinalContract: agentassembly.FinalContractSnapshot{
			Lifecycle: agentassembly.LifecycleRequestScope,
			Shape:     firstNonBlankRuntimeString(input.CompileContext.AnswerStyle, "default"),
		},
		ProfilePromptHash: input.Compiled.Fingerprint.DeveloperHash,
		TraceTags: map[string]string{
			"route_mode": strings.TrimSpace(input.Metadata["aiops.route.mode"]),
			"session":    string(input.SessionType),
			"mode":       string(input.Mode),
		},
	}
}

func admissionProfileFromTurnAssembly(assembly *agentassembly.TurnAssembly) string {
	if assembly == nil {
		return ""
	}
	return strings.TrimSpace(assembly.AdmissionFacts.Profile)
}

func compareAgentAssemblySnapshots(assembly agentassembly.TurnAssembly, legacy, projected agentassembly.AgentAssemblySnapshot) TurnAssemblyShadowTrace {
	fields := []struct {
		name      string
		legacy    any
		projected any
	}{
		{name: "agentKind", legacy: legacy.AgentKind, projected: projected.AgentKind},
		{name: "profile", legacy: legacy.Profile, projected: projected.Profile},
		{name: "routeReason", legacy: legacy.RouteReason, projected: projected.RouteReason},
		{name: "resourceBindings", legacy: legacy.ResourceBindings, projected: projected.ResourceBindings},
		{name: "sessionTargets", legacy: legacy.SessionTargets, projected: projected.SessionTargets},
		{name: "roleBindings", legacy: legacy.RoleBindings, projected: projected.RoleBindings},
		{name: "toolSurface", legacy: legacy.ToolSurface, projected: projected.ToolSurface},
		{name: "contextSelector", legacy: legacy.ContextSelector, projected: projected.ContextSelector},
		{name: "loopPolicy", legacy: legacy.LoopPolicy, projected: projected.LoopPolicy},
		{name: "finalContract", legacy: legacy.FinalContract, projected: projected.FinalContract},
	}
	diffs := make([]TurnAssemblyFieldDiff, 0, len(fields))
	allMatch := true
	for _, field := range fields {
		legacyHash := agentassembly.StableHash("turn-assembly-shadow."+field.name, field.legacy)
		projectedHash := agentassembly.StableHash("turn-assembly-shadow."+field.name, field.projected)
		match := legacyHash == projectedHash
		allMatch = allMatch && match
		diffs = append(diffs, TurnAssemblyFieldDiff{
			Field: field.name, LegacyHash: legacyHash, ProjectedHash: projectedHash, Match: match,
		})
	}
	shadow := TurnAssemblyShadowTrace{
		AssemblyHash: assembly.Hash, LegacySpecHash: legacy.SpecHash, ProjectedSpecHash: projected.SpecHash,
		Match: allMatch, SourceRefs: append([]string(nil), assembly.SourceRefs...), FieldDiffs: diffs,
	}
	if !allMatch {
		shadow.Warning = "turn_assembly_shadow_mismatch"
	}
	return shadow
}

type agentAssemblyTraceInput struct {
	AgentKind              modelrouter.AgentKind
	SessionType            SessionType
	Mode                   Mode
	Metadata               map[string]string
	CompileContext         promptcompiler.CompileContext
	Compiled               promptcompiler.CompiledPrompt
	ToolSurfacePolicy      tooling.ToolSurfacePolicySnapshot
	ToolSurfaceFingerprint string
	ResourceBindings       []resourcebinding.ResourceBindingSnapshot
	SessionTargetSnapshot  *resourcebinding.SessionTargetSnapshot
	RoleBindings           []resourcebinding.ResourceRoleBinding
	TurnAssembly           *agentassembly.TurnAssembly
}

func toolMetadataFromPromptTools(tools []promptcompiler.Tool) []tooling.ToolMetadata {
	if len(tools) == 0 {
		return nil
	}
	out := make([]tooling.ToolMetadata, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		out = append(out, tool.Metadata())
	}
	return out
}

func hiddenToolInputsFromPolicy(policy tooling.ToolSurfacePolicySnapshot) []agentassembly.HiddenToolInput {
	if len(policy.HiddenTools) == 0 {
		return nil
	}
	out := make([]agentassembly.HiddenToolInput, 0, len(policy.HiddenTools))
	for _, hidden := range policy.HiddenTools {
		out = append(out, agentassembly.HiddenToolInput{Name: hidden.Name, Reason: hidden.Reason})
	}
	return out
}

func routeReasonsFromMetadata(metadata map[string]string) []string {
	if len(metadata) == 0 {
		return nil
	}
	var reasons []string
	for _, key := range []string{"aiops.route.mode", "aiops.route.activeSource", "aiops.target.binding"} {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			reasons = append(reasons, key+"="+value)
		}
	}
	return reasons
}

func sessionTargetRefs(snapshot *resourcebinding.SessionTargetSnapshot) []resourcebinding.ResourceRef {
	if snapshot == nil {
		return nil
	}
	out := make([]resourcebinding.ResourceRef, 0, len(snapshot.HostIDs))
	for _, hostID := range snapshot.HostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID == "" {
			continue
		}
		out = append(out, resourcebinding.ResourceRef{Type: resourcebinding.ResourceTypeHost, ID: hostID})
	}
	return out
}

func ptrAgentAssemblySnapshot(snapshot agentassembly.AgentAssemblySnapshot) *agentassembly.AgentAssemblySnapshot {
	return &snapshot
}
