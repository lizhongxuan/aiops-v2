package appui

import (
	"strings"
	"testing"

	"aiops-v2/internal/envcontext"
	"aiops-v2/internal/runtimekernel"
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
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: true})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec false", req.Metadata)
	}
	if !strings.Contains(req.Metadata["enableToolPack"], "public_web") {
		t.Fatalf("metadata = %#v, want public_web enabled when route allows WebLearn", req.Metadata)
	}
	if req.Metadata["aiops.weblearn.sourcePolicy"] != "official_first" || req.Metadata["aiops.weblearn.requiredWhenUnfamiliar"] != "true" {
		t.Fatalf("metadata = %#v, want official-first required WebLearn policy", req.Metadata)
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
}

func TestChatRuntimeToolSurfaceEvidenceRCARequiresExplicitOpsManualMention(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeToolSurfaceMetadata(&req, ChatRuntimeRoute{Mode: ChatRouteEvidenceRCA})
	if req.Metadata["aiops.tool.execCommandAllowed"] != "false" {
		t.Fatalf("metadata = %#v, want exec false for evidence RCA", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableToolPack"], "ops_manual_flow") {
		t.Fatalf("metadata = %#v, ops_manual_flow requires @ops_manuals/@ops_manus", req.Metadata)
	}
	if strings.Contains(req.Metadata["enableTool"], "search_ops_manuals") {
		t.Fatalf("metadata = %#v, evidence RCA must not explicitly enable search_ops_manuals without @ops_manuals", req.Metadata)
	}
}

func TestChatRuntimeToolSurfaceEnablesOpsManualSearchForExplicitMention(t *testing.T) {
	for _, input := range []string{
		"@ops_manuals 按运维手册分析这段报错",
		"@ops_manus 按运维手册分析这段报错",
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
