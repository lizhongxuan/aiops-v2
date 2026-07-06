package agentassembly

import (
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

type BuildInput struct {
	AgentKind         string
	Profile           string
	RuntimeRole       string
	RouteReason       []string
	ResourceBindings  []resourcebinding.ResourceBindingSnapshot
	SessionTargets    []resourcebinding.ResourceRef
	RoleBindings      []resourcebinding.ResourceRoleBinding
	RegisteredTools   []tooling.ToolMetadata
	ModelVisibleTools []tooling.ToolMetadata
	DispatchableTools []tooling.ToolMetadata
	HiddenTools       []HiddenToolInput
	PolicyHash        string
	Fingerprint       string
	PromptSections    []promptcompiler.PromptSectionTrace
	ContextSelector   ContextSelectorSnapshot
	LoopPolicy        LoopPolicySnapshot
	FinalContract     FinalContractSnapshot
	ProfilePromptHash string
	TraceTags         map[string]string
}

func Build(input BuildInput) AgentAssemblySnapshot {
	contextSelector := normalizeContextSelector(input.ContextSelector)
	loopPolicy := normalizeLoopPolicy(input.LoopPolicy)
	finalContract := normalizeFinalContract(input.FinalContract)
	snapshot := AgentAssemblySnapshot{
		AgentKind:        strings.TrimSpace(input.AgentKind),
		Profile:          strings.TrimSpace(input.Profile),
		RuntimeRole:      strings.TrimSpace(input.RuntimeRole),
		RouteReason:      uniqueSortedStrings(input.RouteReason),
		ResourceBindings: append([]resourcebinding.ResourceBindingSnapshot(nil), input.ResourceBindings...),
		SessionTargets:   append([]resourcebinding.ResourceRef(nil), input.SessionTargets...),
		RoleBindings:     append([]resourcebinding.ResourceRoleBinding(nil), input.RoleBindings...),
		ToolSurface: BuildToolSurfaceSnapshot(ToolSurfaceInput{
			ResourceBindings:  input.ResourceBindings,
			RegisteredTools:   input.RegisteredTools,
			ModelVisibleTools: input.ModelVisibleTools,
			DispatchableTools: input.DispatchableTools,
			HiddenTools:       input.HiddenTools,
			PolicyHash:        input.PolicyHash,
			Fingerprint:       input.Fingerprint,
		}),
		ContextSelector:   contextSelector,
		PromptSections:    PromptSectionSnapshotFromTrace(input.PromptSections),
		LoopPolicy:        loopPolicy,
		FinalContract:     finalContract,
		ProfilePromptHash: strings.TrimSpace(input.ProfilePromptHash),
		TraceTags:         cloneStringMap(input.TraceTags),
		Lifecycle:         LifecycleTurnScope,
	}
	snapshot.SpecHash = StableHash("agent-assembly.snapshot", map[string]any{
		"agentKind":         snapshot.AgentKind,
		"profile":           snapshot.Profile,
		"runtimeRole":       snapshot.RuntimeRole,
		"routeReason":       snapshot.RouteReason,
		"resources":         snapshot.ResourceBindings,
		"sessionTargets":    snapshot.SessionTargets,
		"roleBindings":      snapshot.RoleBindings,
		"toolSurfaceHash":   snapshot.ToolSurface.Hash,
		"contextHash":       snapshot.ContextSelector.Hash,
		"promptSectionHash": snapshot.PromptSections.Hash,
		"loopHash":          snapshot.LoopPolicy.Hash,
		"finalHash":         snapshot.FinalContract.Hash,
		"profilePromptHash": snapshot.ProfilePromptHash,
		"traceTags":         snapshot.TraceTags,
	})
	return snapshot
}

func normalizeContextSelector(input ContextSelectorSnapshot) ContextSelectorSnapshot {
	input.Policy = strings.TrimSpace(input.Policy)
	input.Budget = strings.TrimSpace(input.Budget)
	if input.Lifecycle == "" {
		input.Lifecycle = LifecycleRequestScope
	}
	input.Hash = StableHash("context-selector.snapshot", input)
	return input
}

func normalizeLoopPolicy(input LoopPolicySnapshot) LoopPolicySnapshot {
	input.ToolCallPolicy = strings.TrimSpace(input.ToolCallPolicy)
	if input.Lifecycle == "" {
		input.Lifecycle = LifecycleRequestScope
	}
	input.Hash = StableHash("loop-policy.snapshot", map[string]any{
		"lifecycle":      input.Lifecycle,
		"maxIterations":  input.MaxIterations,
		"toolCallPolicy": input.ToolCallPolicy,
	})
	return input
}

func normalizeFinalContract(input FinalContractSnapshot) FinalContractSnapshot {
	input.Shape = strings.TrimSpace(input.Shape)
	if input.Lifecycle == "" {
		input.Lifecycle = LifecycleRequestScope
	}
	input.Hash = StableHash("final-contract.snapshot", input)
	return input
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}
