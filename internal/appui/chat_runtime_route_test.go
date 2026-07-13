package appui

import (
	"strings"
	"testing"

	"aiops-v2/internal/envcontext"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
)

func TestBuildChatRuntimeRoutePlainQuestionUsesAdvisory(t *testing.T) {
	route := BuildChatRuntimeRoute("pg_auto_failover timeline 为什么会比主库高？", nil, UserEvidenceExtraction{})
	if route.Mode != ChatRouteAdvisory {
		t.Fatalf("Mode = %q, want %q", route.Mode, ChatRouteAdvisory)
	}
	if route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = true, want false")
	}
	if !route.AllowsWebLearn {
		t.Fatalf("AllowsWebLearn = false, want true for operational mechanism diagnosis that benefits from public reference knowledge")
	}
}

func TestBuildChatRuntimeRouteExplicitPublicResearchAllowsWebLearn(t *testing.T) {
	route := BuildChatRuntimeRoute("查一下 PostgreSQL checkpoint_completion_target 最新官方文档怎么说？", nil, UserEvidenceExtraction{})
	if route.Mode != ChatRouteAdvisory {
		t.Fatalf("Mode = %q, want %q", route.Mode, ChatRouteAdvisory)
	}
	if !route.AllowsWebLearn {
		t.Fatalf("AllowsWebLearn = false, want true when user explicitly asks for current official public docs")
	}
}

func TestBuildChatRuntimeRouteLocalOnlyLatestDoesNotAllowWebLearn(t *testing.T) {
	route := BuildChatRuntimeRoute("不要联网，只基于本地上下文分析最新日志", nil, UserEvidenceExtraction{})
	if route.AllowsWebLearn {
		t.Fatalf("AllowsWebLearn = true, want false when user explicitly restricts analysis to local context")
	}
}

func TestChatRouteNoHostMentionKeepsAdvisoryMode(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("pg_auto_failover timeline 为什么会比主库高？", nil, UserEvidenceExtraction{})
	applyChatRuntimeRouteHostBinding(&req, route, nil)
	applyChatRuntimeRouteMetadata(&req, route)

	if route.Mode != ChatRouteAdvisory {
		t.Fatalf("Mode = %q, want advisory", route.Mode)
	}
	if req.HostID != "" {
		t.Fatalf("HostID = %q, want empty without explicit host mention", req.HostID)
	}
	if req.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("SessionType = %q, want workspace", req.SessionType)
	}
	if req.Metadata["aiops.route.allowsExecCommand"] != "false" {
		t.Fatalf("metadata = %#v, want host exec disabled", req.Metadata)
	}
	if req.Metadata["aiops.target.hostId"] != "" {
		t.Fatalf("metadata = %#v, must not bind localhost implicitly", req.Metadata)
	}
}

func TestBuildChatRuntimeRoutePastedEvidenceUsesEvidenceRCA(t *testing.T) {
	evidence := UserEvidenceExtraction{HasEvidence: true, EvidenceKinds: []string{"command_output"}}
	route := BuildChatRuntimeRoute("不要执行命令，只基于输出分析", nil, evidence)
	if route.Mode != ChatRouteEvidenceRCA {
		t.Fatalf("Mode = %q, want %q", route.Mode, ChatRouteEvidenceRCA)
	}
	if route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = true, want false")
	}
}

func TestBuildChatRuntimeRouteUserProhibitsHostExec(t *testing.T) {
	evidence := UserEvidenceExtraction{UserProhibitsExec: true}
	mentions := []hostops.HostMention{{Raw: "@local", HostID: "server-local", Resolved: true}}
	route := BuildChatRuntimeRoute("不要执行本机命令，只分析", mentions, evidence)
	if route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = true, want false")
	}
	if !route.UserProhibitedHostExec {
		t.Fatalf("UserProhibitedHostExec = false, want true")
	}
}

func TestBuildChatRuntimeRouteLocalMentionUsesHostBoundOps(t *testing.T) {
	mentions := []hostops.HostMention{{Raw: "@local", HostID: "server-local", Resolved: true}}
	route := BuildChatRuntimeRoute("@local 帮我检查 PG 状态", mentions, UserEvidenceExtraction{})
	if route.Mode != ChatRouteHostBoundOps {
		t.Fatalf("Mode = %q, want %q", route.Mode, ChatRouteHostBoundOps)
	}
	if !route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = false, want true")
	}
}

func TestChatRouteExplicitHostMentionEnablesHostContext(t *testing.T) {
	mentions := []hostops.HostMention{{Raw: "@local", HostID: "server-local", Resolved: true}}
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("@local 帮我检查 PG 状态", mentions, UserEvidenceExtraction{})
	applyChatRuntimeRouteHostBinding(&req, route, mentions)
	applyChatRuntimeRouteMetadata(&req, route)
	applyChatRuntimeResourceProjection(&req, mentions)

	if route.Mode != ChatRouteHostBoundOps {
		t.Fatalf("Mode = %q, want host-bound ops", route.Mode)
	}
	if !route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = false, want true")
	}
	if req.HostID != "server-local" || req.Metadata["aiops.target.hostId"] != "server-local" {
		t.Fatalf("request = %#v metadata=%#v, want only explicit host target", req, req.Metadata)
	}
	if len(req.ResourceBindings) != 1 || !req.ResourceBindings[0].Verified() || req.ResourceBindings[0].Ref.ID != "server-local" {
		t.Fatalf("resource bindings = %#v, want verified server-local", req.ResourceBindings)
	}
}

func TestChatRouteAdvisoryDoesNotForgeResourceBinding(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("解释一下 checkpoint", nil, UserEvidenceExtraction{})
	applyChatRuntimeRouteHostBinding(&req, route, nil)
	applyChatRuntimeRouteMetadata(&req, route)
	applyChatRuntimeResourceProjection(&req, nil)

	if len(req.ResourceBindings) != 0 {
		t.Fatalf("resource bindings = %#v, want none for advisory route", req.ResourceBindings)
	}
}

func TestChatRouteTraceOnlySessionTargetSnapshot(t *testing.T) {
	mentions := []hostops.HostMention{
		{TokenID: "m2", Raw: "@hostB", HostID: "host-b", DisplayName: "hostB", Source: hostops.HostMentionSourceInventory, Resolved: true},
		{TokenID: "m1", Raw: "@hostA", HostID: "host-a", DisplayName: "hostA", Source: hostops.HostMentionSourceInventory, Resolved: true},
	}
	req := runtimekernel.TurnRequest{TurnID: "turn-1", Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("@hostA @hostB 检查 PG", mentions, UserEvidenceExtraction{})
	applyChatRuntimeRouteHostBinding(&req, route, mentions)
	applyChatRuntimeResourceProjection(&req, mentions)
	applyChatRuntimeSessionTargetRoleTrace(&req, nil, "@hostA @hostB 检查 PG", mentions)

	if req.SessionTargetSnapshot == nil || req.SessionTargetSnapshot.BindingMode != "multi_host" {
		t.Fatalf("session target = %#v, want multi_host", req.SessionTargetSnapshot)
	}
	if got := req.SessionTargetSnapshot.HostIDs; len(got) != 2 || got[0] != "host-a" || got[1] != "host-b" {
		t.Fatalf("host ids = %#v, want sorted host-a host-b", got)
	}
	if req.HostID != "" {
		t.Fatalf("HostID = %q, want empty for multi-host route", req.HostID)
	}
}

func TestChatRouteReadsSessionTargetTraceWithoutRestoringRoute(t *testing.T) {
	session := &runtimekernel.SessionState{
		SessionTargetSnapshot: resourcebinding.NewSessionTargetSnapshot(resourcebinding.SessionTargetInput{
			HostIDs:           []string{"host-a", "host-b"},
			SourceTurnID:      "turn-1",
			ExpiresAfterTurns: 2,
		}),
	}
	req := runtimekernel.TurnRequest{TurnID: "turn-2", Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("继续检查复制状态", nil, UserEvidenceExtraction{})
	applyChatRuntimeRouteHostBinding(&req, route, nil)
	applyChatRuntimeResourceProjection(&req, nil)
	applyChatRuntimeSessionTargetRoleTrace(&req, session, "继续检查复制状态", nil)

	if req.HostID != "" || req.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("request = %#v, want no route restoration in trace-only phase", req)
	}
	if req.SessionTargetSnapshot == nil || req.SessionTargetSnapshot.ExpiresAfterTurns != 1 {
		t.Fatalf("session target = %#v, want next-turn trace snapshot", req.SessionTargetSnapshot)
	}
}

func TestChatRouteUsesEnvironmentTargetRefForIPMention(t *testing.T) {
	mentions := []hostops.HostMention{{Raw: "@10.0.0.1", Address: "10.0.0.1", Source: hostops.HostMentionSourceIPLiteral, Resolved: true}}
	route := BuildChatRuntimeRoute("@10.0.0.1 检查 systemd 服务状态", mentions, UserEvidenceExtraction{})
	if len(route.TargetRefs) != 1 || route.TargetRefs[0].ID != "host:10.0.0.1" {
		t.Fatalf("TargetRefs = %#v, want resolver host target", route.TargetRefs)
	}
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeRouteHostBinding(&req, route, mentions)
	applyChatRuntimeRouteMetadata(&req, route)

	if req.HostID != "10.0.0.1" || req.Metadata["aiops.target.refs"] != "host:10.0.0.1" {
		t.Fatalf("request = %#v metadata=%#v, want IP target ref binding", req, req.Metadata)
	}
}

func TestChatRouteEnvironmentConflictDowngradesToReadOnly(t *testing.T) {
	mentions := []hostops.HostMention{{Raw: "@10.0.0.1", Address: "10.0.0.1", Source: hostops.HostMentionSourceIPLiteral, Resolved: true}}
	resolution := envcontext.ResolveEnvironmentFacts(envcontext.ResolverInput{
		Input: "@10.0.0.1 @Coroot 分析 checkout 服务异常",
		CorootFacts: []envcontext.EnvironmentFact{
			{
				Kind:       envcontext.FactKindTopology,
				Subject:    "service:checkout",
				Value:      "host:10.0.0.2",
				Source:     envcontext.FactSourceCoroot,
				Confidence: envcontext.FactConfidenceObserved,
			},
		},
	})
	route := BuildChatRuntimeRouteWithEnvironment("@10.0.0.1 @Coroot 分析 checkout 服务异常", mentions, UserEvidenceExtraction{}, resolution)
	if route.Mode != ChatRouteEvidenceRCA || route.AllowsExecCommand {
		t.Fatalf("route = %#v, want read-only evidence RCA when environment target conflicts", route)
	}
	if route.EnvironmentReadOnlyReason != "target_conflict_requires_clarification" {
		t.Fatalf("route = %#v, want conflict read-only reason", route)
	}
}

func TestBuildChatRuntimeRouteMultipleHostsUsesMultiHostOps(t *testing.T) {
	mentions := []hostops.HostMention{
		{Raw: "@hostA", HostID: "host-a", Resolved: true},
		{Raw: "@hostB", HostID: "host-b", Resolved: true},
	}
	route := BuildChatRuntimeRoute("@hostA @hostB 对比 PG 状态", mentions, UserEvidenceExtraction{})
	if route.Mode != ChatRouteMultiHostOps {
		t.Fatalf("Mode = %q, want %q", route.Mode, ChatRouteMultiHostOps)
	}
	if route.AllowsExecCommand {
		t.Fatalf("manager AllowsExecCommand = true, want false")
	}
}

func TestMultiHostProfileEnablesHostManagerRuntimeMetadata(t *testing.T) {
	mentions := []hostops.HostMention{
		{Raw: "@hostA", HostID: "host-a", Resolved: true},
		{Raw: "@hostB", HostID: "host-b", Resolved: true},
	}
	route := BuildChatRuntimeRoute("@hostA @hostB 对比 PG 状态", mentions, UserEvidenceExtraction{})
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	applyChatRuntimeRouteHostBinding(&req, route, mentions)
	applyChatRuntimeToolSurfaceMetadata(&req, route)
	applyChatRuntimeRouteMetadata(&req, route)

	if req.SessionType != runtimekernel.SessionTypeWorkspace || req.Mode != runtimekernel.ModePlan {
		t.Fatalf("request session/mode = %s/%s, want workspace/plan", req.SessionType, req.Mode)
	}
	for key, want := range map[string]string{
		"profile":        "host_manager",
		"agentProfile":   "host_manager",
		"runtimeProfile": "manager_agent_full_runtime",
	} {
		if got := req.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %q, want %q; metadata=%#v", key, got, want, req.Metadata)
		}
	}
	if !strings.Contains(req.Metadata["enableToolPack"], hostops.ToolPackHostOps) {
		t.Fatalf("enableToolPack = %q, want hostops pack", req.Metadata["enableToolPack"])
	}
}

func TestWorkflowAgentRuntimeMetadataWinsOverOrdinaryHostRoute(t *testing.T) {
	mentions := []hostops.HostMention{{Raw: "@local", HostID: "server-local", Resolved: true}}
	route := BuildChatRuntimeRoute("@local 修改当前 Workflow 的执行步骤", mentions, UserEvidenceExtraction{})
	req := runtimekernel.TurnRequest{
		Input: "@local 修改当前 Workflow 的执行步骤",
		Metadata: map[string]string{
			"aiops.workflowAgent.enabled":   "true",
			"aiops.workflow.sessionIntent":  "edit",
			"aiops.workflow.id":             "workflow",
			"aiops.workflow.baseRevision":   "rev",
			"aiops.workflow.selectedNodeId": "execute",
		},
	}

	applyChatRuntimeRouteHostBinding(&req, route, mentions)
	applyChatRuntimeToolSurfaceMetadata(&req, route)
	applyChatRuntimeRouteMetadata(&req, route)
	applyWorkflowAgentRuntimeMetadata(&req)

	if req.HostID != "" {
		t.Fatalf("HostID = %q, want empty for workflow agent drawer route", req.HostID)
	}
	if req.SessionType != runtimekernel.SessionTypeWorkspace || req.Mode != runtimekernel.ModePlan {
		t.Fatalf("request session/mode = %s/%s, want workspace/plan", req.SessionType, req.Mode)
	}
	for key, want := range map[string]string{
		"profile":                          runtimekernel.RuntimePromptProfileWorkflowAgent,
		"toolProfile":                      runtimekernel.RuntimePromptProfileWorkflowAgent,
		"aiops.tool.execCommandAllowed":    "false",
		"aiops.tool.hostMutationAllowed":   "false",
		"aiops.workflowAgent.routeApplied": "true",
	} {
		if got := req.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %q, want %q; metadata=%#v", key, got, want, req.Metadata)
		}
	}
	if !strings.Contains(req.Metadata["enableToolPack"], "workflow_editor") {
		t.Fatalf("enableToolPack = %q, want workflow_editor", req.Metadata["enableToolPack"])
	}
}

func TestApplyRuntimeMutationPoliciesProjectsTrustedTypedPolicies(t *testing.T) {
	mutation := runtimekernel.TurnRequest{}
	applyRuntimeMutationPolicies(&mutation, runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindChange,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskWrite},
	})
	if mutation.PermissionProfile != runtimekernel.RuntimePermissionProfileApprovalRequired ||
		mutation.RollbackPolicy != runtimekernel.RuntimeRollbackPolicyActionContractRequired {
		t.Fatalf("mutation policies = %q/%q", mutation.PermissionProfile, mutation.RollbackPolicy)
	}

	hostExec := runtimekernel.TurnRequest{}
	applyRuntimeMutationPolicies(&hostExec, runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindVerify,
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
	})
	if hostExec.PermissionProfile == "" || hostExec.RollbackPolicy == "" {
		t.Fatalf("host execution is missing high-risk policies: %#v", hostExec)
	}

	readOnly := runtimekernel.TurnRequest{}
	applyRuntimeMutationPolicies(&readOnly, runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindExplain})
	if readOnly.PermissionProfile != "" || readOnly.RollbackPolicy != "" {
		t.Fatalf("risk-free read received mutation policies: %#v", readOnly)
	}
}

func TestWorkflowAgentRuntimeMetadataRequiresExplicitDrawerFlag(t *testing.T) {
	req := runtimekernel.TurnRequest{
		HostID:      "server-local",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Metadata: map[string]string{
			"aiops.workflow.id": "workflow",
		},
	}

	applyWorkflowAgentRuntimeMetadata(&req)

	if req.HostID != "server-local" || req.SessionType != runtimekernel.SessionTypeHost || req.Mode != runtimekernel.ModeExecute {
		t.Fatalf("request = %#v, should not change without explicit workflow agent flag", req)
	}
	if req.Metadata["profile"] == runtimekernel.RuntimePromptProfileWorkflowAgent || strings.Contains(req.Metadata["enableToolPack"], "workflow_editor") {
		t.Fatalf("metadata = %#v, should not enable workflow agent without explicit drawer flag", req.Metadata)
	}
}

func TestBuildChatRuntimeRouteCorootMentionBecomesReadOnlyEvidenceRCA(t *testing.T) {
	route := BuildChatRuntimeRoute("@Coroot 分析 order-api 延迟", nil, UserEvidenceExtraction{})
	if route.Mode != ChatRouteEvidenceRCA {
		t.Fatalf("Mode = %q, want %q for Coroot evidence route", route.Mode, ChatRouteEvidenceRCA)
	}
	if route.AllowsExecCommand || route.RequiresHostBinding {
		t.Fatalf("route = %#v, Coroot route should not become host-bound ops", route)
	}
	if !route.AllowsCorootRCA {
		t.Fatalf("AllowsCorootRCA = false, want true")
	}
}

func TestApplyChatRuntimeRouteMetadataMarksExplicitCorootRCA(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	route := BuildChatRuntimeRoute("@Coroot 分析 order-api 延迟", nil, UserEvidenceExtraction{})
	applyChatRuntimeRouteMetadata(&req, route)
	if req.Metadata["aiops.coroot.explicitRCA"] != "true" {
		t.Fatalf("metadata = %#v, want explicit Coroot RCA marker", req.Metadata)
	}
	if req.Metadata["aiops.route.allowsCorootRCA"] != "true" {
		t.Fatalf("metadata = %#v, want Coroot RCA allowed", req.Metadata)
	}
	if req.Metadata["aiops.mentions.observabilityProvider"] != "coroot" {
		t.Fatalf("metadata = %#v, want structured observability provider marker", req.Metadata)
	}
}

func TestApplyChatRuntimeRouteMetadata(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	route := ChatRuntimeRoute{
		Mode:              ChatRouteAdvisory,
		Reasons:           []string{"no host mentions"},
		AllowsWebLearn:    true,
		AllowsExecCommand: false,
		Confidence:        "high",
	}
	applyChatRuntimeRouteMetadata(&req, route)
	if req.Metadata["aiops.route.mode"] != string(ChatRouteAdvisory) {
		t.Fatalf("metadata = %#v, want route mode", req.Metadata)
	}
	if req.Metadata["aiops.route.allowsExecCommand"] != "false" {
		t.Fatalf("metadata = %#v, want exec disallowed", req.Metadata)
	}
	if req.Metadata["aiops.route.allowsWebLearn"] != "true" {
		t.Fatalf("metadata = %#v, want web learn allowed", req.Metadata)
	}
}

func TestBuildChatRuntimeRouteFromIntentFrameUnknownKeepsLegacy(t *testing.T) {
	legacy := ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true, Confidence: "high"}
	route := BuildChatRuntimeRouteFromIntentFrame(runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindUnknown}, legacy)

	if route.Mode != legacy.Mode || route.AllowsExecCommand != legacy.AllowsExecCommand {
		t.Fatalf("route = %#v, want legacy %#v for unknown intent", route, legacy)
	}
}

func TestBuildChatRuntimeRouteFromIntentFrameResearchAllowsWebLearn(t *testing.T) {
	frame := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
		Confidence: runtimecontract.ConfidenceMedium,
	}
	route := BuildChatRuntimeRouteFromIntentFrame(frame, ChatRuntimeRoute{Mode: ChatRouteAdvisory})

	if route.Mode != ChatRouteAdvisory {
		t.Fatalf("Mode = %q, want advisory", route.Mode)
	}
	if !route.AllowsWebLearn {
		t.Fatalf("AllowsWebLearn = false, want true from structured public_web scope")
	}
	if route.AllowsExecCommand {
		t.Fatalf("AllowsExecCommand = true, want false for research route")
	}
}

func TestApplyIntentFrameRouteMetadataWritesShadowDiffWithoutChangingLegacy(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	legacy := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: false, Confidence: "high"}
	frame := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
		Confidence: runtimecontract.ConfidenceMedium,
	}
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(frame, legacy)

	applyChatRuntimeRouteMetadata(&req, legacy)
	applyIntentFrameRouteMetadata(&req, legacy, intentRoute, legacy, frame, intentFrameRoutingTraceOnly)
	if req.IntentFrame == nil || req.IntentFrame.Kind != runtimecontract.IntentKindResearch {
		t.Fatalf("typed intent frame = %#v, want research", req.IntentFrame)
	}

	if req.Metadata["aiops.route.mode"] != string(ChatRouteAdvisory) || req.Metadata["aiops.route.allowsWebLearn"] != "false" {
		t.Fatalf("legacy metadata changed unexpectedly: %#v", req.Metadata)
	}
	if req.Metadata[runtimecontract.MetadataIntentKind] != string(runtimecontract.IntentKindResearch) {
		t.Fatalf("metadata = %#v, want intent kind", req.Metadata)
	}
	if req.Metadata[runtimecontract.MetadataIntentFrame] == "" {
		t.Fatalf("metadata = %#v, want serialized intent frame", req.Metadata)
	}
	if req.Metadata[runtimecontract.MetadataLegacyRoute] == "" || req.Metadata[runtimecontract.MetadataIntentRoute] == "" {
		t.Fatalf("metadata = %#v, want legacy and intent route snapshots", req.Metadata)
	}
	if !strings.Contains(req.Metadata[runtimecontract.MetadataRouteDiff], "allowsWebLearn") {
		t.Fatalf("route diff = %q, want allowsWebLearn diff", req.Metadata[runtimecontract.MetadataRouteDiff])
	}
}

func TestApplyIntentFrameRouteMetadataAddsHostExecForActiveHostBoundRoute(t *testing.T) {
	req := runtimekernel.TurnRequest{Metadata: map[string]string{}}
	legacy := ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true, RequiresHostBinding: true, Confidence: "high"}
	frame := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindUnknown}
	intentRoute := BuildChatRuntimeRouteFromIntentFrame(frame, legacy)

	applyChatRuntimeRouteMetadata(&req, legacy)
	applyIntentFrameRouteMetadata(&req, legacy, intentRoute, legacy, frame, intentFrameRoutingTraceOnly)

	if req.Metadata[runtimecontract.MetadataIntentKind] != string(runtimecontract.IntentKindVerify) {
		t.Fatalf("metadata = %#v, want verify intent for active host-bound route", req.Metadata)
	}
	if !strings.Contains(req.Metadata[runtimecontract.MetadataIntentDataScopes], string(runtimecontract.DataScopeLocalRuntime)) {
		t.Fatalf("metadata dataScopes = %q, want local_runtime", req.Metadata[runtimecontract.MetadataIntentDataScopes])
	}
	if !strings.Contains(req.Metadata[runtimecontract.MetadataIntentRiskBudget], string(runtimecontract.ActionRiskHostExec)) {
		t.Fatalf("metadata riskBudget = %q, want host_exec", req.Metadata[runtimecontract.MetadataIntentRiskBudget])
	}
	if !strings.Contains(req.Metadata[runtimecontract.MetadataIntentFrame], "host_runtime_inspection") {
		t.Fatalf("intent frame = %q, want host runtime capability", req.Metadata[runtimecontract.MetadataIntentFrame])
	}
}

func TestSelectActiveChatRuntimeRouteDefaultsToTraceOnlyLegacy(t *testing.T) {
	t.Setenv("AIOPS_INTENT_FRAME_"+"ROUTING", "active")
	legacy := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: false}
	intentRoute := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: true}
	frame := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindResearch, Confidence: runtimecontract.ConfidenceMedium}

	active, mode := selectActiveChatRuntimeRoute(legacy, intentRoute, frame, intentFrameRoutingTraceOnly)
	if mode != "trace_only" {
		t.Fatalf("mode = %q, want trace_only", mode)
	}
	if active.AllowsWebLearn {
		t.Fatalf("active route = %#v, want legacy route in trace_only", active)
	}
}

func TestSelectActiveChatRuntimeRouteUsesIntentWhenFeatureActive(t *testing.T) {
	legacy := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: false}
	intentRoute := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsWebLearn: true}
	frame := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindResearch, Confidence: runtimecontract.ConfidenceMedium}

	active, mode := selectActiveChatRuntimeRoute(legacy, intentRoute, frame, intentFrameRoutingActive)
	if mode != "active" {
		t.Fatalf("mode = %q, want active", mode)
	}
	if !active.AllowsWebLearn {
		t.Fatalf("active route = %#v, want intent route in active mode", active)
	}
}

func TestSelectActiveChatRuntimeRouteFallsBackForUnknownIntent(t *testing.T) {
	legacy := ChatRuntimeRoute{Mode: ChatRouteHostBoundOps, AllowsExecCommand: true}
	intentRoute := ChatRuntimeRoute{Mode: ChatRouteAdvisory, AllowsExecCommand: false}
	frame := runtimecontract.IntentFrame{Kind: runtimecontract.IntentKindUnknown, Confidence: runtimecontract.ConfidenceLow}

	active, _ := selectActiveChatRuntimeRoute(legacy, intentRoute, frame, intentFrameRoutingActive)
	if active.Mode != legacy.Mode || active.AllowsExecCommand != legacy.AllowsExecCommand {
		t.Fatalf("active route = %#v, want legacy fallback %#v", active, legacy)
	}
}
