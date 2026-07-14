package appui

import (
	"strings"
	"testing"

	"aiops-v2/internal/envcontext"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

func TestChatRuntimeToolSurfaceAdvisorDisallowsExecWithoutDefaultWeb(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec false", req.Metadata)
	}
	if req.Metadata["aiops.toolPack.coroot_rca.allowed"] != "false" {
		t.Fatalf("metadata = %#v, want Coroot RCA pack gated by default", req.Metadata)
	}
	for _, pack := range []string{"coroot_metrics", "coroot_nodes", "coroot_logs", "mcp_dynamic_coroot"} {
		if req.Metadata["aiops.toolPack."+pack+".allowed"] != "false" {
			t.Fatalf("metadata = %#v, want %s gated by default", req.Metadata, pack)
		}
	}
	if strings.Contains(req.Metadata["enableToolPack"], "public_web") {
		t.Fatalf("metadata = %#v, should not enable public_web for default advisory route", req.Metadata)
	}
	if req.Metadata["aiops.weblearn.enabled"] == "true" {
		t.Fatalf("metadata = %#v, should not enable WebLearn for default advisory route", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceAdvisorAllowsExplicitWebLearn(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "查一下 PostgreSQL 官方文档确认 recovery_target_timeline 行为", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: true})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec false", req.Metadata)
	}
	if !strings.Contains(req.Metadata["enableToolPack"], "public_web") {
		t.Fatalf("metadata = %#v, want public_web enabled when route allows WebLearn", req.Metadata)
	}
	if req.Metadata["aiops.weblearn.sourcePolicy"] != "official_first" || req.Metadata["aiops.weblearn.requiredWhenUnfamiliar"] != "false" {
		t.Fatalf("metadata = %#v, want optional official-first WebLearn policy", req.Metadata)
	}
	if req.Metadata["aiops.webSearch.policy"] != string(runtimekernel.WebSearchEnabled) {
		t.Fatalf("metadata = %#v, want enabled webSearch policy", req.Metadata)
	}
	if req.Metadata["aiops.webSearch.requireCitations"] == "true" || req.Metadata["aiops.webSearch.querySeeds"] == "" {
		t.Fatalf("metadata = %#v, want optional citations and query seeds", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceWebSearchPolicyDisabledForSimpleQuestion(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "解释一下 Linux load average 是什么", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: true})
	if req.Metadata["aiops.webSearch.policy"] != string(runtimekernel.WebSearchDisabled) {
		t.Fatalf("metadata = %#v, want disabled webSearch policy", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableToolPack"], "public_web") || req.Metadata["aiops.weblearn.enabled"] == "true" {
		t.Fatalf("metadata = %#v, should not enable public_web for disabled policy", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceWebSearchPolicyCurrentScopeWins(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "@server-local 查看 PostgreSQL 当前 CPU 和进程情况", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true, AllowsWebLearn: true, RequiresHostBinding: true})
	if req.Metadata["aiops.webSearch.policy"] != string(runtimekernel.WebSearchDisabled) {
		t.Fatalf("metadata = %#v, want disabled webSearch policy for current host scope", req.Metadata)
	}
	if req.Metadata["aiops.webSearch.disabledBy"] != "scope" {
		t.Fatalf("metadata = %#v, want scope disabledBy", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableToolPack"], "public_web") {
		t.Fatalf("metadata = %#v, should not enable public_web for current host scope", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceEnablesToolSearchOnlyForExplicitMention(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "use tool_search to discover a deferred pack", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory})
	if req.Metadata["aiops.toolSearch.enabled"] != "true" || !strings.Contains(req.Metadata["enableTool"], "tool_search") {
		t.Fatalf("metadata = %#v, want explicit tool_search enabled", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceDoesNotEnableToolSearchForBugDiscussion(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "为什么查看 CPU 要大量使用 tool_search，应该直接用 exec tool", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true})
	if req.Metadata["aiops.toolSearch.enabled"] == "true" || strings.Contains(req.Metadata["enableTool"], "tool_search") {
		t.Fatalf("metadata = %#v, should not enable tool_search for bug discussion", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceHostBoundAllowsExec(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "true" {
		t.Fatalf("metadata = %#v, want exec true", req.Metadata)
	}
	if req.Metadata["toolProfile"] != string(ChatRouteHostBoundOps) {
		t.Fatalf("metadata = %#v, want host-bound tool profile", req.Metadata)
	}
	if req.Metadata["profile"] != runtimekernel.RuntimePromptProfileHostWorker {
		t.Fatalf("metadata = %#v, want host worker runtime profile", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceHostBoundRuntimeProfileExposesExecOnly(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "@server-local 查看 CPU 情况", Metadata: map[string]string{}}
	route := ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true, RequiresHostBinding: true}
	frame := BuildIntentFrame(req.Input, BuildEvidenceEnvelope(req.Input, nil, nil), nil)
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(frame, route)
	applyChatRuntimeRouteMetadata(&req, route)
	applyIntentFrameRouteMetadata(&req, route, intentRoute, route, frame, intentFrameRoutingTraceOnly)
	applyChatRuntimeToolSurfaceMetadata(&req, route)

	registry := tooling.NewRegistry()
	for _, meta := range []tooling.ToolMetadata{
		{Name: "exec_command", AlwaysLoad: true, Layer: tooling.ToolLayerCore, Profiles: []string{runtimekernel.RuntimePromptProfileHostWorker}},
		{Name: "tool_search", AlwaysLoad: true, Layer: tooling.ToolLayerCore},
		{Name: "web_search", AlwaysLoad: true, Layer: tooling.ToolLayerCore, Pack: "public_web"},
	} {
		if err := registry.Register(&tooling.StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	names := toolNamesForChatToolSurfaceTest(registry.CompileContextWithMetadata("host", "execute", req.Metadata))
	if !containsChatToolSurfaceName(names, "exec_command") {
		t.Fatalf("assembled tools = %v, want exec_command for host-bound CPU request", names)
	}
	for _, forbidden := range []string{"tool_search", "web_search"} {
		if containsChatToolSurfaceName(names, forbidden) {
			t.Fatalf("assembled tools = %v, should not include %s for host-bound CPU request", names, forbidden)
		}
	}
}

func TestChatRuntimeToolSurfaceEvidenceRCARequiresExplicitOpsManualMention(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec false for evidence RCA", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableToolPack"], "ops_manual_flow") {
		t.Fatalf("metadata = %#v, ops_manual_flow requires @ops_manual or a manual chip", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableTool"], "search_ops_manuals") {
		t.Fatalf("metadata = %#v, evidence RCA must not explicitly enable search_ops_manuals without @ops_manual", req.Metadata)
	}
	req = runtimekernel.TurnRequest{Input: "@ops_manus 旧 typo 不应触发手册检索", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA})
	if strings.Contains(req.Metadata["enableToolPack"], "ops_manual_flow") || strings.Contains(req.Metadata["enableTool"], "search_ops_manuals") {
		t.Fatalf("metadata = %#v, old @ops_manus typo should not enable ops manual search", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceEnablesOpsManualSearchForExplicitMention(t *testing.T) {
	for _, input := range []string{
		"@ops_manual 按运维手册分析这段报错",
		"@ops_manuals 按运维手册分析这段报错",
	} {
		req := runtimekernel.TurnRequest{Input: input, Metadata: map[string]string{}}
		applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA})
		if !strings.Contains(req.Metadata["enableToolPack"], "ops_manual_flow") {
			t.Fatalf("input %q metadata = %#v, want ops_manual_flow enabled", input, req.Metadata)
		}
		if !strings.Contains(req.Metadata["enableTool"], "search_ops_manuals") {
			t.Fatalf("input %q metadata = %#v, want search_ops_manuals explicitly enabled", input, req.Metadata)
		}
	}
}

func TestChatRuntimeToolSurfaceEnablesOpsGraphForExplicitMention(t *testing.T) {
	req := runtimekernel.TurnRequest{Input: "@ops_graph 看 order-api 影响哪些业务", Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA})
	if !strings.Contains(req.Metadata["enableToolPack"], "opsgraph") {
		t.Fatalf("metadata = %#v, want opsgraph pack enabled", req.Metadata)
	}
	if req.Metadata["aiops.opsGraph.explicitMention"] != "true" {
		t.Fatalf("metadata = %#v, want explicit ops graph mention", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceAllowsExplicitCorootRCAPacks(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA, AllowsCorootRCA: true})
	if req.Metadata["aiops.toolPack.coroot_rca.allowed"] != "true" {
		t.Fatalf("metadata = %#v, want explicit Coroot RCA pack allowed", req.Metadata)
	}
	if req.Metadata["aiops.toolPack.coroot_rca_reference.allowed"] != "true" {
		t.Fatalf("metadata = %#v, want explicit Coroot RCA reference pack allowed", req.Metadata)
	}
	for _, pack := range []string{"coroot_metrics", "coroot_nodes", "coroot_logs", "mcp_dynamic_coroot"} {
		if req.Metadata["aiops.toolPack."+pack+".allowed"] != "true" {
			t.Fatalf("metadata = %#v, want explicit %s allowed", req.Metadata, pack)
		}
	}
}

func TestChatRuntimeToolSurfaceMultiHostEnablesHostOpsPack(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteMultiHostOps})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want manager exec false", req.Metadata)
	}
	if !strings.Contains(req.Metadata["enableToolPack"], "host_ops") {
		t.Fatalf("metadata = %#v, want host_ops pack", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceWorkflowAgentEnablesOnlyWorkflowEditorPack(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{
		"aiops.workflowAgent.enabled": "true",
		"aiops.workflow.id":           "workflow",
		"aiops.workflow.baseRevision": "rev",
	}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory})
	applyWorkflowAgentRuntimeMetadata(&req)

	if req.Metadata["profile"] != runtimekernel.RuntimePromptProfileWorkflowAgent || req.Metadata["toolProfile"] != runtimekernel.RuntimePromptProfileWorkflowAgent {
		t.Fatalf("metadata = %#v, want workflow agent profile", req.Metadata)
	}
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" || req.Metadata["aiops.tool.hostMutationAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want host exec/mutation disabled", req.Metadata)
	}
	if !strings.Contains(req.Metadata["enableToolPack"], "workflow_editor") {
		t.Fatalf("enableToolPack = %q, want workflow_editor", req.Metadata["enableToolPack"])
	}

	registry := tooling.NewRegistry()
	for _, meta := range []tooling.ToolMetadata{
		{Name: "exec_command", AlwaysLoad: true, Layer: tooling.ToolLayerCore, Profiles: []string{runtimekernel.RuntimePromptProfileHostWorker}},
		{Name: "workflow.inspect", Layer: tooling.ToolLayerMCP, Pack: "workflow_editor", Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}},
		{Name: "workflow.apply_patch", Layer: tooling.ToolLayerMCP, Pack: "workflow_editor", Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}},
		{Name: "search_ops_manuals", Layer: tooling.ToolLayerMCP, Pack: "ops_manual_flow"},
	} {
		if err := registry.Register(&tooling.StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	names := toolNamesForChatToolSurfaceTest(registry.CompileContextWithMetadata("workspace", "plan", req.Metadata))
	for _, want := range []string{"workflow.inspect", "workflow.apply_patch"} {
		if !containsChatToolSurfaceName(names, want) {
			t.Fatalf("assembled tools = %v, want %s", names, want)
		}
	}
	for _, forbidden := range []string{"exec_command", "search_ops_manuals"} {
		if containsChatToolSurfaceName(names, forbidden) {
			t.Fatalf("assembled tools = %v, should not include %s", names, forbidden)
		}
	}
}

func TestChatRuntimeToolSurfaceCarriesEnvironmentTargetRefs(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{
		Mode: ChatRouteHostBoundOps,
		TargetRefs: []envcontext.TargetRef{
			{ID: "host:10.0.0.1", Kind: envcontext.TargetKindHost, Address: "10.0.0.1"},
		},
	})
	if req.Metadata["aiops.tool.targetRefs"] != "host:10.0.0.1" {
		t.Fatalf("metadata = %#v, want resolver target refs for ToolSearch", req.Metadata)
	}
	if req.Metadata["aiops.tool.targetCompatibility"] != "target_refs_available" {
		t.Fatalf("metadata = %#v, want target compatibility available", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceMarksEnvironmentConflict(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{
		Mode:                      ChatRouteEvidenceRCA,
		EnvironmentReadOnlyReason: "target_conflict_requires_clarification",
	})
	if req.Metadata["aiops.tool.targetCompatibility"] != "conflict" {
		t.Fatalf("metadata = %#v, want target compatibility conflict", req.Metadata)
	}
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec disabled for conflict", req.Metadata)
	}
}

func toolNamesForChatToolSurfaceTest(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef != nil {
			names = append(names, toolDef.Metadata().Name)
		}
	}
	return names
}

func containsChatToolSurfaceName(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
