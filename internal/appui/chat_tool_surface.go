package appui

import (
	"regexp"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

var (
	explicitOpsManualMentionPattern = regexp.MustCompile(`(?i)(^|[^\pL\pN_])@(ops_manual|ops_manuals)([^\pL\pN_]|$)`)
	explicitOpsGraphMentionPattern  = regexp.MustCompile(`(?i)(^|[^\pL\pN_])@ops_graph([^\pL\pN_]|$)`)
)

func applyChatRuntimeToolSurfaceMetadata(req *runtimekernel.TurnRequest, route ChatRuntimeRoute) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["toolProfile"] = string(route.Mode)
	if route.Mode == ChatRouteHostBoundOps {
		req.Metadata["profile"] = runtimekernel.RuntimePromptProfileHostWorker
	}
	req.Metadata["aiops.tool.execCommandAllowed"] = boolMetadataString(route.AllowsExecCommand)
	req.Metadata["aiops.tool.hostMutationAllowed"] = boolMetadataString(route.Mode == ChatRouteHostBoundOps)
	req.Metadata["aiops.tool.corootRCAAllowed"] = boolMetadataString(route.AllowsCorootRCA)
	applyCorootToolPackGateMetadata(req.Metadata, route.AllowsCorootRCA)
	if route.AllowsCorootRCA {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "mcp_dynamic_coroot")
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "coroot_rca")
	}
	if len(route.TargetRefs) > 0 {
		req.Metadata["aiops.tool.targetRefs"] = strings.Join(routeTargetRefIDs(route.TargetRefs), ",")
		req.Metadata["aiops.tool.targetCompatibility"] = "target_refs_available"
	}
	if strings.TrimSpace(route.EnvironmentReadOnlyReason) != "" {
		req.Metadata["aiops.tool.targetCompatibility"] = "conflict"
	}
	if tooling.ExplicitToolSearchDiscoveryRequested(req.Input) {
		req.Metadata["aiops.toolSearch.enabled"] = "true"
		req.Metadata["enableTool"] = appendMetadataListValue(req.Metadata["enableTool"], "tool_search")
	}
	webSearchPolicy := runtimekernel.EvaluateWebSearchPolicy(runtimekernel.WebSearchPolicyInput{
		UserInput:             req.Input,
		PublicWebAvailable:    route.AllowsWebLearn,
		CurrentOrPrivateScope: route.RequiresHostBinding || route.Mode == ChatRouteHostBoundOps || route.Mode == ChatRouteMultiHostOps,
	})
	applyWebSearchPolicyMetadata(req.Metadata, webSearchPolicy)
	if webSearchPolicy.Level == runtimekernel.WebSearchEnabled {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "public_web")
		req.Metadata["aiops.weblearn.enabled"] = "true"
		req.Metadata["aiops.weblearn.sourcePolicy"] = "official_first"
		req.Metadata["aiops.weblearn.requiredWhenUnfamiliar"] = "false"
	}
	if hasExplicitOpsManualMention(req.Input) {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "ops_manual_flow")
		req.Metadata["enableTool"] = appendMetadataListValue(req.Metadata["enableTool"], "search_ops_manuals")
		req.Metadata["aiops.opsManuals.explicitMention"] = "true"
	}
	if hasExplicitOpsGraphMention(req.Input) {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "opsgraph")
		req.Metadata["aiops.opsGraph.explicitMention"] = "true"
	}
	if route.Mode == ChatRouteMultiHostOps {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "host_ops")
		applyHostOpsManagerRuntimeMetadata(req.Metadata)
	}
	applyWorkflowAgentRuntimeMetadata(req)
}

func applyWebSearchPolicyMetadata(metadata map[string]string, decision runtimekernel.WebSearchPolicyDecision) {
	if metadata == nil {
		return
	}
	metadata["aiops.webSearch.policy"] = string(decision.Level)
	metadata["aiops.webSearch.reason"] = strings.TrimSpace(decision.Reason)
	if len(decision.ReasonCodes) > 0 {
		metadata["aiops.webSearch.reasonCodes"] = strings.Join(decision.ReasonCodes, ",")
	}
	if len(decision.QuerySeeds) > 0 {
		metadata["aiops.webSearch.querySeeds"] = strings.Join(decision.QuerySeeds, "\n")
	}
	if strings.TrimSpace(decision.DisabledBy) != "" {
		metadata["aiops.webSearch.disabledBy"] = strings.TrimSpace(decision.DisabledBy)
	}
	metadata["aiops.webSearch.requireCitations"] = boolMetadataString(decision.RequireCitations)
}

func hasExplicitOpsManualMention(input string) bool {
	return explicitOpsManualMentionPattern.MatchString(input)
}

func hasExplicitOpsGraphMention(input string) bool {
	return explicitOpsGraphMentionPattern.MatchString(input)
}

func applyCorootToolPackGateMetadata(metadata map[string]string, allowed bool) {
	if metadata == nil {
		return
	}
	value := boolMetadataString(allowed)
	for _, pack := range []string{
		"coroot_admin_read",
		"coroot_config_read",
		"coroot_dashboard",
		"coroot_deployments",
		"coroot_incident",
		"coroot_logs",
		"coroot_metrics",
		"coroot_nodes",
		"coroot_profiling",
		"coroot_rca",
		"coroot_rca_reference",
		"coroot_risks",
		"coroot_topology",
		"coroot_traces",
		"mcp_dynamic_coroot",
	} {
		metadata["aiops.toolPack."+pack+".allowed"] = value
	}
}
