package tooling

import (
	"encoding/json"
	"strings"

	"aiops-v2/internal/runtimecontract"
)

func AssembleOptionsForTurnMetadata(metadata map[string]string) AssembleOptions {
	return ApplyTurnMetadataToAssembleOptions(AssembleOptions{}, metadata)
}

func ApplyTurnMetadataToAssembleOptions(opts AssembleOptions, metadata map[string]string) AssembleOptions {
	opts.TenantID = firstMetadataString(metadata, "tenantId", "tenantID", "tenant_id")
	opts.UserID = firstMetadataString(metadata, "userId", "userID", "user_id")
	if opts.Profile == "" {
		opts.Profile = firstMetadataString(metadata, "profile", "toolProfile", "mcpProfile")
	}
	opts.EnabledPacks = appendUniqueStrings(opts.EnabledPacks, metadataListValue(metadata, "enableToolPack")...)
	if publicWebAllowedForTurn(metadata) {
		opts.EnabledPacks = appendUniqueStrings(opts.EnabledPacks, "public_web")
	}
	opts.EnabledTools = appendUniqueStrings(opts.EnabledTools, metadataListValue(metadata, "enableTool")...)
	opts.RuntimeCapabilities = appendUniqueStrings(opts.RuntimeCapabilities, metadataListValue(metadata, "runtimeCapability")...)
	opts.RuntimeCapabilities = appendUniqueStrings(opts.RuntimeCapabilities, metadataListValue(metadata, "runtimeCapabilities")...)
	opts.ContextArtifactAvailable = opts.ContextArtifactAvailable ||
		metadataBool(metadata, "contextArtifactAvailable") ||
		metadataBool(metadata, "hasContextArtifact") ||
		metadataBool(metadata, "contextArtifactEnabled")
	opts.MCPHealthSnapshot = mergeMCPHealthSnapshot(opts.MCPHealthSnapshot, metadata)
	if metadataTransform := turnMetadataToolMetadataTransform(metadata); metadataTransform != nil {
		if opts.MetadataTransform == nil {
			opts.MetadataTransform = metadataTransform
		} else {
			existingTransform := opts.MetadataTransform
			opts.MetadataTransform = func(meta ToolMetadata) ToolMetadata {
				return metadataTransform(existingTransform(meta))
			}
		}
	}
	metadataFilter := turnMetadataToolFilter(metadata)
	if metadataFilter == nil {
		return opts
	}
	if opts.Filter == nil {
		opts.Filter = metadataFilter
		return opts
	}
	existingFilter := opts.Filter
	opts.Filter = func(t Tool, ctx ToolContext, meta ToolMetadata) bool {
		return existingFilter(t, ctx, meta) && metadataFilter(t, ctx, meta)
	}
	return opts
}

func turnMetadataToolMetadataTransform(metadata map[string]string) func(ToolMetadata) ToolMetadata {
	if len(metadata) == 0 {
		return nil
	}
	hostOS := normalizeHostOS(firstMetadataString(metadata, "aiops.host.os", "host.os", "hostOS"))
	hostArch := strings.TrimSpace(firstMetadataString(metadata, "aiops.host.arch", "host.arch", "hostArch"))
	hostID := strings.TrimSpace(firstMetadataString(metadata, "aiops.host.id", "host.id", "hostId", "aiops.target.hostId"))
	hostTransport := strings.TrimSpace(firstMetadataString(metadata, "aiops.host.transport", "host.transport", "transport"))
	if hostOS == "" && hostArch == "" && hostTransport == "" && !metadataBool(metadata, "aiops.host.metadataAvailable") {
		return nil
	}
	return func(meta ToolMetadata) ToolMetadata {
		if meta.Name != "exec_command" {
			return meta
		}
		meta.Description = execCommandDescriptionForTargetHost(hostID, hostOS, hostArch, hostTransport)
		return meta
	}
}

func execCommandDescriptionForTargetHost(hostID, hostOS, hostArch, hostTransport string) string {
	base := "Execute a terminal command on the selected host. For server-local this runs locally in the ai-server environment; for managed remote hosts this sends read-only commands to the selected host-agent over gRPC/HTTP, and for inventory hosts with stored SSH credentials the runtime may use a read-only SSH fallback through the same exec_command tool. Prefer explicit command + args. For read-only inspection, do not wrap commands in sh/bash/zsh -c and do not use pipes, redirection, or command chaining; use narrower commands or native flags instead. Read-only inspection commands, including safe curl GET/HEAD requests, are allowed in chat; for HTTP status checks use curl -fsS -o /dev/null -w %{http_code} URL or curl -fsSI URL, and do not use -o %{http_code}. Mutation commands must go through the runtime approval gate, so call the scoped command instead of asking for prose approval."
	target := targetHostDescription(hostID, hostOS, hostArch, hostTransport)
	switch hostOS {
	case "darwin":
		return base + target + " For host resource inspection on macOS, prefer uptime, sysctl -n hw.ncpu, vm_stat, df -h, and top -l 1 -s 0; avoid Linux-only commands such as lscpu, nproc, free -h, and /proc/*."
	case "linux":
		return base + target + " For host resource inspection on Linux, prefer uptime, nproc, free -h, df -hT -x tmpfs -x devtmpfs, and cat /proc/loadavg; avoid macOS-only commands such as sysctl -n hw.ncpu, vm_stat, and top -l."
	case "windows":
		return base + target + " Choose Windows-compatible commands for the selected host; prefer PowerShell when the runtime exposes a PowerShell-capable tool, and avoid Unix-only paths or /proc/*."
	default:
		return base + target + " Target OS is unknown; verify the selected host OS with a small read-only command such as uname before using OS-specific commands, and do not use commands for another OS unless evidence confirms compatibility."
	}
}

func targetHostDescription(hostID, hostOS, hostArch, hostTransport string) string {
	parts := make([]string, 0, 4)
	if hostID != "" {
		parts = append(parts, "host="+hostID)
	}
	if hostOS != "" {
		parts = append(parts, "os="+hostOS)
	}
	if hostArch != "" {
		parts = append(parts, "arch="+hostArch)
	}
	if hostTransport != "" {
		parts = append(parts, "transport="+hostTransport)
	}
	if len(parts) == 0 {
		return ""
	}
	return " Target host metadata: " + strings.Join(parts, ", ") + "."
}

func normalizeHostOS(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "macos", "mac", "osx":
		return "darwin"
	case "gnu/linux":
		return "linux"
	case "win":
		return "windows"
	default:
		return value
	}
}

func mergeMCPHealthSnapshot(existing map[string]string, metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return existing
	}
	out := existing
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		const prefix = "mcpHealth."
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		serverID := strings.TrimSpace(strings.TrimPrefix(key, prefix))
		status := strings.TrimSpace(value)
		if serverID == "" || status == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[serverID] = status
	}
	return out
}

func firstMetadataString(metadata map[string]string, keys ...string) string {
	if len(metadata) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func turnMetadataToolFilter(metadata map[string]string) func(Tool, ToolContext, ToolMetadata) bool {
	if len(metadata) == 0 {
		return nil
	}
	return func(_ Tool, _ ToolContext, meta ToolMetadata) bool {
		return ToolVisibilityDecisionForTurnMetadata(meta, metadata).Visible
	}
}

type TurnMetadataToolVisibilityDecision struct {
	Visible bool   `json:"visible"`
	Reason  string `json:"reason,omitempty"`
}

func IsToolVisibleForTurnMetadata(meta ToolMetadata, metadata map[string]string) bool {
	return ToolVisibilityDecisionForTurnMetadata(meta, metadata).Visible
}

// IntentToolPackCanAutoEnable prevents implicit text intent from loading
// mention-gated external tool packs. Explicit metadata from the composer or
// transport layer is still honored.
func IntentToolPackCanAutoEnable(metadata map[string]string, pack string) bool {
	pack = strings.ToLower(strings.TrimSpace(pack))
	if pack == "" {
		return false
	}
	if metadataBool(metadata, toolPackAllowedMetadataKey(pack)) {
		return true
	}
	switch {
	case strings.Contains(pack, "coroot"):
		return metadataBool(metadata, "aiops.coroot.explicitMention") ||
			metadataBool(metadata, "aiops.coroot.explicitRCA") ||
			metadataBool(metadata, "aiops.tool.corootRCAAllowed")
	case strings.Contains(pack, "opsgraph") || strings.Contains(pack, "ops_graph"):
		return metadataBool(metadata, "aiops.opsGraph.explicitMention") ||
			metadataBool(metadata, "aiops.ops_graph.explicitMention")
	case strings.Contains(pack, "ops_manual") || strings.Contains(pack, "ops_manus"):
		return metadataBool(metadata, "aiops.opsManuals.explicitMention")
	default:
		return true
	}
}

func ToolVisibilityDecisionForTurnMetadata(meta ToolMetadata, metadata map[string]string) TurnMetadataToolVisibilityDecision {
	if decision, ok := toolVisibilityDecisionFromIntentMetadata(meta, metadata); ok {
		return decision
	}
	switch {
	case opsManualsOptedOut(metadata):
		switch meta.Name {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			return turnMetadataHidden("opsmanual_opted_out")
		default:
			return turnMetadataVisible()
		}
	case opsManualReferenceOnly(metadata) && meta.Name == "run_ops_manual_preflight":
		return turnMetadataHidden("opsmanual_reference_only")
	case isOpsManualToolName(meta.Name) && !opsManualsExplicitlyRequested(metadata):
		return turnMetadataHidden("opsmanual_not_requested")
	case meta.Name == "search_ops_manuals" && opsManualsExplicitlyRequested(metadata):
		return turnMetadataVisible()
	case meta.Name == "exec_command":
		if _, ok := metadata["aiops.tool.execCommandAllowed"]; ok {
			if metadataBool(metadata, "aiops.tool.execCommandAllowed") {
				return turnMetadataVisible()
			}
			return turnMetadataHidden("host_exec_disallowed")
		}
		return turnMetadataVisible()
	case isCorootRCATool(meta):
		if _, ok := metadata["aiops.tool.corootRCAAllowed"]; ok {
			if metadataBool(metadata, "aiops.tool.corootRCAAllowed") {
				return turnMetadataVisible()
			}
			return turnMetadataHidden("coroot_rca_not_allowed")
		}
		if corootExplicitlyRequested(meta, metadata) {
			return turnMetadataVisible()
		}
		return turnMetadataHidden("coroot_not_requested")
	case isCorootTool(meta) && !corootExplicitlyRequested(meta, metadata):
		return turnMetadataHidden("coroot_not_requested")
	case isOpsGraphTool(meta) && !opsGraphExplicitlyRequested(meta, metadata):
		return turnMetadataHidden("opsgraph_not_requested")
	case hostBoundOpsShouldHideAmbientTool(meta, metadata):
		return turnMetadataHidden("host_bound_direct_surface")
	case noHostAnalysisShouldHideAmbientTool(meta, metadata):
		return turnMetadataHidden("analysis_discovery_only")
	case meta.Name == "resolve_ops_manual_params":
		if metadataBool(metadata, "opsManualMatched") || opsManualParamFormSubmitted(metadata) {
			return turnMetadataVisible()
		}
		return turnMetadataHidden("opsmanual_params_missing")
	case meta.Name == "run_ops_manual_preflight":
		if (metadataBool(metadata, "opsManualParamsResolved") && metadataListContains(metadata, "enableTool", "run_ops_manual_preflight")) ||
			metadataBool(metadata, "opsManualDirectExecute") {
			return turnMetadataVisible()
		}
		return turnMetadataHidden("opsmanual_preflight_not_enabled")
	default:
		return turnMetadataVisible()
	}
}

func toolVisibilityDecisionFromIntentMetadata(meta ToolMetadata, metadata map[string]string) (TurnMetadataToolVisibilityDecision, bool) {
	frame, ok := intentFrameFromTurnMetadata(metadata)
	if !ok {
		return TurnMetadataToolVisibilityDecision{}, false
	}
	decision := DecideToolSurface(frame, approvalSnapshotFromTurnMetadata(metadata), nil)
	switch {
	case matchesName(meta, "tool_search"):
		if directPublicWebSurfaceShouldHideToolSearch(decision) {
			return turnMetadataHidden("public_web_direct_surface"), true
		}
		return turnMetadataVisible(), true
	case meta.Name == "exec_command":
		if decision.AllowHostExec {
			return turnMetadataVisible(), true
		}
		return turnMetadataHidden("host_exec_disallowed"), true
	case toolBelongsToPack(meta, "public_web"):
		if decision.AllowPublicWeb {
			return turnMetadataVisible(), true
		}
		return turnMetadataHidden("public_web_not_allowed"), true
	case isOpsManualToolName(meta.Name):
		if opsManualsOptedOut(metadata) {
			return turnMetadataHidden("opsmanual_opted_out"), true
		}
		if !opsManualsExplicitlyRequested(metadata) {
			return turnMetadataHidden("opsmanual_not_requested"), true
		}
		if !decision.AllowOpsManual {
			return turnMetadataHidden("opsmanual_not_allowed"), true
		}
		if meta.Name == "search_ops_manuals" {
			return turnMetadataVisible(), true
		}
	}
	return TurnMetadataToolVisibilityDecision{}, false
}

func intentFrameFromTurnMetadata(metadata map[string]string) (runtimecontract.IntentFrame, bool) {
	if len(metadata) == 0 {
		return runtimecontract.IntentFrame{}, false
	}
	if raw := strings.TrimSpace(metadata[runtimecontract.MetadataIntentFrame]); raw != "" {
		var frame runtimecontract.IntentFrame
		if err := json.Unmarshal([]byte(raw), &frame); err == nil {
			return runtimecontract.NormalizeIntentFrame(frame), true
		}
	}
	frame := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKind(strings.TrimSpace(metadata[runtimecontract.MetadataIntentKind])),
		DataScopes: dataScopesFromMetadataValue(metadata[runtimecontract.MetadataIntentDataScopes]),
		RiskBudget: actionRisksFromMetadataValue(metadata[runtimecontract.MetadataIntentRiskBudget]),
		Confidence: strings.TrimSpace(metadata[runtimecontract.MetadataIntentConfidence]),
	}
	if frame.Kind == "" && len(frame.DataScopes) == 0 && len(frame.RiskBudget) == 0 {
		return runtimecontract.IntentFrame{}, false
	}
	return runtimecontract.NormalizeIntentFrame(frame), true
}

func approvalSnapshotFromTurnMetadata(metadata map[string]string) ApprovalSnapshot {
	return ApprovalSnapshot{
		HostExecApproved: metadataBool(metadata, "aiops.tool.execCommandAllowed"),
	}
}

func dataScopesFromMetadataValue(raw string) []runtimecontract.DataScope {
	values := splitIntentMetadataList(raw)
	out := make([]runtimecontract.DataScope, 0, len(values))
	for _, value := range values {
		out = append(out, runtimecontract.DataScope(value))
	}
	return out
}

func actionRisksFromMetadataValue(raw string) []runtimecontract.ActionRisk {
	values := splitIntentMetadataList(raw)
	out := make([]runtimecontract.ActionRisk, 0, len(values))
	for _, value := range values {
		out = append(out, runtimecontract.ActionRisk(value))
	}
	return out
}

func splitIntentMetadataList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func hostBoundOpsShouldHideAmbientTool(meta ToolMetadata, metadata map[string]string) bool {
	mode := strings.ToLower(strings.TrimSpace(firstMetadataString(metadata, "aiops.route.mode", "toolProfile", "profile")))
	if mode != "host_bound_ops" {
		return false
	}
	if !metadataBool(metadata, "aiops.route.requiresHostBinding") && !metadataBool(metadata, "aiops.tool.execCommandAllowed") {
		return false
	}
	if matchesName(meta, "exec_command") || matchesName(meta, "tool_search") {
		return false
	}
	if turnMetadataExplicitlyEnablesTool(meta, metadata) {
		return false
	}
	if publicWebAllowedForTurn(metadata) && toolBelongsToPack(meta, "public_web") {
		return false
	}
	return true
}

func noHostAnalysisShouldHideAmbientTool(meta ToolMetadata, metadata map[string]string) bool {
	if !isNoHostAdvisorTurn(metadata) && !isNoHostUserEvidenceRCATurn(metadata) {
		return false
	}
	if matchesName(meta, "tool_search") {
		return publicWebAllowedForTurn(metadata)
	}
	if isOpsManualToolName(meta.Name) && opsManualsExplicitlyRequested(metadata) {
		return false
	}
	if turnMetadataExplicitlyEnablesTool(meta, metadata) {
		return false
	}
	if publicWebAllowedForTurn(metadata) && toolBelongsToPack(meta, "public_web") {
		return false
	}
	return true
}

func isNoHostAdvisorTurn(metadata map[string]string) bool {
	mode := strings.ToLower(strings.TrimSpace(firstMetadataString(metadata, "aiops.route.mode", "toolProfile", "profile")))
	switch mode {
	case "chat_advisory", "advisor":
	default:
		return false
	}
	if metadataBool(metadata, "aiops.route.requiresHostBinding") || metadataBool(metadata, "aiops.tool.execCommandAllowed") {
		return false
	}
	binding := strings.ToLower(strings.TrimSpace(firstMetadataString(metadata, "aiops.target.binding")))
	return binding == "" || binding == "none"
}

func isNoHostUserEvidenceRCATurn(metadata map[string]string) bool {
	mode := strings.ToLower(strings.TrimSpace(firstMetadataString(metadata, "aiops.route.mode", "toolProfile", "profile")))
	if mode != "evidence_rca" {
		return false
	}
	if !metadataBool(metadata, "aiops.userEvidence.present") {
		return false
	}
	if metadataBool(metadata, "aiops.route.requiresHostBinding") || metadataBool(metadata, "aiops.tool.execCommandAllowed") {
		return false
	}
	binding := strings.ToLower(strings.TrimSpace(firstMetadataString(metadata, "aiops.target.binding")))
	return binding == "" || binding == "none"
}

func turnMetadataExplicitlyEnablesTool(meta ToolMetadata, metadata map[string]string) bool {
	return toolEnabled(meta, metadataListValue(metadata, "enableTool")) ||
		packEnabledForMeta(meta, metadataListValue(metadata, "enableToolPack"))
}

func publicWebAllowedForTurn(metadata map[string]string) bool {
	return metadataBool(metadata, "aiops.route.allowsWebLearn") ||
		metadataBool(metadata, "aiops.weblearn.enabled") ||
		metadataListContains(metadata, "enableToolPack", "public_web")
}

func directPublicWebSurfaceShouldHideToolSearch(decision SurfaceDecision) bool {
	return decision.AllowPublicWeb && !decision.AllowOpsManual && !decision.AllowHostExec
}

func toolBelongsToPack(meta ToolMetadata, pack string) bool {
	if strings.EqualFold(strings.TrimSpace(pack), "public_web") &&
		(matchesName(meta, "web_search") || matchesName(meta, "browse_url")) {
		return true
	}
	return packEnabledForMeta(meta, []string{pack})
}

func turnMetadataVisible() TurnMetadataToolVisibilityDecision {
	return TurnMetadataToolVisibilityDecision{Visible: true}
}

func turnMetadataHidden(reason string) TurnMetadataToolVisibilityDecision {
	return TurnMetadataToolVisibilityDecision{Reason: strings.TrimSpace(reason)}
}

func isCorootRCATool(meta ToolMetadata) bool {
	name := strings.ToLower(strings.TrimSpace(meta.Name))
	pack := strings.ToLower(strings.TrimSpace(meta.Pack))
	domain := strings.ToLower(strings.TrimSpace(meta.Domain))
	if name == "coroot_collect_rca_context" ||
		name == "coroot.collect_rca_context" ||
		name == "coroot.rca_report" ||
		strings.Contains(name, "coroot_rca") {
		return true
	}
	if domain != "coroot" && !strings.HasPrefix(name, "coroot.") && !strings.Contains(pack, "coroot_rca") {
		return false
	}
	switch pack {
	case "coroot_rca", "coroot_rca_reference":
		return true
	default:
		return domain == "coroot" && strings.Contains(pack, "rca")
	}
}

func isCorootTool(meta ToolMetadata) bool {
	name := strings.ToLower(strings.TrimSpace(meta.Name))
	pack := strings.ToLower(strings.TrimSpace(meta.Pack))
	domain := strings.ToLower(strings.TrimSpace(meta.Domain))
	return domain == "coroot" || strings.HasPrefix(name, "coroot.") || strings.HasPrefix(pack, "coroot")
}

func corootExplicitlyRequested(meta ToolMetadata, metadata map[string]string) bool {
	return metadataBool(metadata, "aiops.coroot.explicitMention") ||
		metadataBool(metadata, "aiops.coroot.explicitRCA") ||
		metadataBool(metadata, "aiops.tool.corootRCAAllowed") ||
		turnMetadataExplicitlyEnablesTool(meta, metadata)
}

func isOpsGraphTool(meta ToolMetadata) bool {
	name := strings.ToLower(strings.TrimSpace(meta.Name))
	pack := strings.ToLower(strings.TrimSpace(meta.Pack))
	domain := strings.ToLower(strings.TrimSpace(meta.Domain))
	return domain == "opsgraph" || strings.HasPrefix(name, "opsgraph.") || pack == "opsgraph"
}

func opsGraphExplicitlyRequested(meta ToolMetadata, metadata map[string]string) bool {
	return metadataBool(metadata, "aiops.opsGraph.explicitMention") ||
		metadataBool(metadata, "aiops.ops_graph.explicitMention") ||
		turnMetadataExplicitlyEnablesTool(meta, metadata)
}

func appendUniqueStrings(existing []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		found := false
		for _, current := range existing {
			if current == value {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, value)
		}
	}
	return existing
}

func metadataBool(metadata map[string]string, key string) bool {
	if len(metadata) == 0 {
		return false
	}
	value := strings.TrimSpace(metadata[key])
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}

func toolPackAllowedMetadataKey(pack string) string {
	pack = strings.ToLower(strings.TrimSpace(pack))
	if pack == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_", "\\", "_", ":", "_", ".", "_")
	return "aiops.toolPack." + replacer.Replace(pack) + ".allowed"
}

func metadataListContains(metadata map[string]string, key, want string) bool {
	for _, value := range metadataListValue(metadata, key) {
		if value == want {
			return true
		}
	}
	return false
}

func metadataListValue(metadata map[string]string, key string) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw := strings.TrimSpace(metadata[key])
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	values := make([]string, 0, len(fields))
	for _, field := range fields {
		if value := strings.TrimSpace(field); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func isOpsManualToolName(name string) bool {
	switch name {
	case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
		return true
	default:
		return false
	}
}

func opsManualsExplicitlyRequested(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	for _, name := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"} {
		if metadataListContains(metadata, "enableTool", name) {
			return true
		}
	}
	if metadataBool(metadata, "aiops.opsManuals.explicitMention") ||
		metadataBool(metadata, "opsManualMatched") ||
		metadataBool(metadata, "opsManualParamsResolved") ||
		metadataBool(metadata, "opsManualDirectExecute") ||
		opsManualParamFormSubmitted(metadata) {
		return true
	}
	action := strings.ToLower(strings.TrimSpace(metadata["opsManualAction"]))
	switch action {
	case "use_ops_manual", "reference_ops_manual", "run_ops_manual_preflight":
		return true
	default:
		return false
	}
}

func opsManualsOptedOut(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "skip_ops_manual") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualSkipped"]), "true")
}

func opsManualReferenceOnly(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "reference_ops_manual")
}

func opsManualParamFormSubmitted(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "submit_ops_manual_param_form")
}
