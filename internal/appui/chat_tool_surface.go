package appui

import (
	"regexp"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

var (
	explicitOpsManualMentionPattern = regexp.MustCompile(`(?i)(^|[^\pL\pN_])@(ops_manuals|ops_manus)([^\pL\pN_]|$)`)
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
	req.Metadata["aiops.tool.execCommandAllowed"] = boolMetadataString(route.AllowsExecCommand)
	req.Metadata["aiops.tool.hostMutationAllowed"] = boolMetadataString(route.Mode == ChatRouteHostBoundOps)
	req.Metadata["aiops.tool.corootRCAAllowed"] = boolMetadataString(route.AllowsCorootRCA)
	applyCorootToolPackGateMetadata(req.Metadata, route.AllowsCorootRCA)
	if len(route.TargetRefs) > 0 {
		req.Metadata["aiops.tool.targetRefs"] = strings.Join(routeTargetRefIDs(route.TargetRefs), ",")
		req.Metadata["aiops.tool.targetCompatibility"] = "target_refs_available"
	}
	if strings.TrimSpace(route.EnvironmentReadOnlyReason) != "" {
		req.Metadata["aiops.tool.targetCompatibility"] = "conflict"
	}
	if route.AllowsWebLearn {
		req.Metadata["enableToolPack"] = appendMetadataListValue(req.Metadata["enableToolPack"], "public_web")
		req.Metadata["aiops.weblearn.enabled"] = "true"
		req.Metadata["aiops.weblearn.sourcePolicy"] = "official_first"
		req.Metadata["aiops.weblearn.requiredWhenUnfamiliar"] = "true"
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
