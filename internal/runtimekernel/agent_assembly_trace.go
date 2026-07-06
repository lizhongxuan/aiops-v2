package runtimekernel

import (
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

func buildAgentAssemblySnapshotForTrace(input agentAssemblyTraceInput) *agentassembly.AgentAssemblySnapshot {
	modelVisible := toolMetadataFromPromptTools(input.CompileContext.AssembledTools)
	hidden := hiddenToolInputsFromPolicy(input.ToolSurfacePolicy)
	return ptrAgentAssemblySnapshot(agentassembly.Build(agentassembly.BuildInput{
		AgentKind:         strings.TrimSpace(string(input.AgentKind)),
		Profile:           firstNonBlankRuntimeString(input.CompileContext.Profile, input.Metadata["profile"], input.Metadata["toolProfile"]),
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
	}))
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
