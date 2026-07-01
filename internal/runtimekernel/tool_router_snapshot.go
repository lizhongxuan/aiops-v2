package runtimekernel

import (
	"strings"

	"aiops-v2/internal/tooling"
)

type RuntimeToolRouterSnapshot struct {
	RegisteredTools   []string            `json:"registeredTools,omitempty"`
	ModelVisibleTools []string            `json:"modelVisibleTools,omitempty"`
	DispatchableTools []string            `json:"dispatchableTools,omitempty"`
	HiddenReasons     map[string][]string `json:"hiddenReasons,omitempty"`
	PolicyHash        string              `json:"policyHash,omitempty"`
	Fingerprint       string              `json:"fingerprint,omitempty"`
}

func RuntimeToolRouterSnapshotFromPolicy(registered []string, policy tooling.ToolSurfacePolicySnapshot, visible []string, dispatchable []string, fingerprint string) RuntimeToolRouterSnapshot {
	if len(dispatchable) == 0 {
		dispatchable = dispatchableToolNamesFromPolicy(policy, visible)
	}
	return RuntimeToolRouterSnapshot{
		RegisteredTools:   uniqueSortedTraceStrings(registered),
		ModelVisibleTools: uniqueSortedTraceStrings(visible),
		DispatchableTools: uniqueSortedTraceStrings(dispatchable),
		HiddenReasons:     hiddenReasonsFromToolSurfacePolicy(policy),
		PolicyHash:        strings.TrimSpace(policy.Hash),
		Fingerprint:       strings.TrimSpace(fingerprint),
	}
}

func runtimeToolRouterSnapshotFromTurnSnapshot(snapshot *TurnSnapshot) RuntimeToolRouterSnapshot {
	if snapshot == nil || snapshot.ToolSurfaceSnapshot == nil {
		return RuntimeToolRouterSnapshot{}
	}
	ref := snapshot.ToolSurfaceSnapshot
	policy := tooling.ToolSurfacePolicySnapshot{}
	if ref.PolicySnapshot != nil {
		policy = *ref.PolicySnapshot
	}
	if strings.TrimSpace(policy.Hash) == "" {
		policy.Hash = strings.TrimSpace(ref.PolicySnapshotHash)
	}
	return RuntimeToolRouterSnapshotFromPolicy(ref.ToolNames, policy, ref.ToolNames, nil, ref.Fingerprint)
}

func dispatchableToolNamesFromPolicy(policy tooling.ToolSurfacePolicySnapshot, visible []string) []string {
	allowed := make([]string, 0, len(visible))
	visibleSet := map[string]struct{}{}
	for _, name := range visible {
		name = strings.TrimSpace(name)
		if name != "" {
			visibleSet[strings.ToLower(name)] = struct{}{}
		}
	}
	seenDecisions := false
	for _, decision := range policy.SurfaceDecisions {
		name := strings.TrimSpace(decision.Name)
		if name == "" {
			continue
		}
		seenDecisions = true
		if len(visibleSet) > 0 {
			if _, ok := visibleSet[strings.ToLower(name)]; !ok {
				continue
			}
		}
		if decision.Visible && decision.DispatchAction == tooling.SurfaceDispatchAllow {
			allowed = append(allowed, name)
		}
	}
	if seenDecisions {
		return allowed
	}
	return append([]string(nil), visible...)
}
