package runtimekernel

import (
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
)

type RuntimeToolRouterSnapshot = StepToolRouter

func RuntimeToolRouterSnapshotFromPolicy(registered []string, policy tooling.ToolSurfacePolicySnapshot, visible []string, dispatchable []string, fingerprint string) RuntimeToolRouterSnapshot {
	if dispatchable == nil {
		dispatchable = append([]string(nil), visible...)
	}
	router, err := BuildStepToolRouter(StepToolRouterInput{
		Registered:        registered,
		ModelVisible:      visible,
		Dispatchable:      dispatchable,
		HiddenReasons:     hiddenReasonsFromToolSurfacePolicy(policy),
		PolicyHash:        policy.Hash,
		SourceFingerprint: fingerprint,
	})
	if err != nil {
		return RuntimeToolRouterSnapshot{
			RegisteredTools:   uniqueSortedTraceStrings(registered),
			ModelVisibleTools: uniqueSortedTraceStrings(visible),
			DispatchableTools: uniqueSortedTraceStrings(dispatchable),
			HiddenReasons:     hiddenReasonsFromToolSurfacePolicy(policy),
			PolicyHash:        strings.TrimSpace(policy.Hash),
		}
	}
	return router
}

func hydrateStepToolRouterForDispatch(tools []promptcompiler.Tool, router StepToolRouter) StepToolRouter {
	if strings.TrimSpace(router.Fingerprint) != "" {
		return router
	}
	registered := router.RegisteredTools
	visible := router.ModelVisibleTools
	dispatchable := router.DispatchableTools
	if len(registered) == 0 && len(visible) == 0 && len(dispatchable) == 0 && len(router.HiddenReasons) == 0 {
		registered = toolNames(tools)
		visible = append([]string(nil), registered...)
		dispatchable = append([]string(nil), registered...)
	}
	built, err := BuildStepToolRouter(StepToolRouterInput{
		Registered:                  registered,
		ModelVisible:                visible,
		Dispatchable:                dispatchable,
		HiddenReasons:               router.HiddenReasons,
		HostInternalDispatchReasons: router.HostInternalDispatchReasons,
		PolicyHash:                  router.PolicyHash,
	})
	if err != nil {
		return router
	}
	return built
}

func runtimeToolRouterSnapshotFromTurnSnapshot(snapshot *TurnSnapshot) RuntimeToolRouterSnapshot {
	if snapshot == nil || snapshot.ToolSurfaceSnapshot == nil {
		return RuntimeToolRouterSnapshot{}
	}
	ref := snapshot.ToolSurfaceSnapshot
	if ref.StepRouter != nil {
		return cloneStepToolRouter(*ref.StepRouter)
	}
	policy := tooling.ToolSurfacePolicySnapshot{}
	if ref.PolicySnapshot != nil {
		policy = *ref.PolicySnapshot
	}
	if strings.TrimSpace(policy.Hash) == "" {
		policy.Hash = strings.TrimSpace(ref.PolicySnapshotHash)
	}
	return RuntimeToolRouterSnapshotFromPolicy(ref.ToolNames, policy, ref.ToolNames, append([]string{}, ref.ToolNames...), ref.Fingerprint)
}
