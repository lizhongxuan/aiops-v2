package policyengine

import (
	"testing"

	"aiops-v2/internal/tooling"
)

func assertDecision(t *testing.T, desc string, got PolicyDecision, wantAction PolicyAction) {
	t.Helper()
	if got.Action != wantAction {
		t.Errorf("%s: got action %q, want %q (reason: %s)", desc, got.Action, wantAction, got.Reason)
	}
}

func toolInput(name string) PolicyInput {
	return PolicyInput{
		ToolName: name,
		Tool:     tooling.ToolMetadata{Name: name},
	}
}

func mcpToolInput(name string) PolicyInput {
	return PolicyInput{
		ToolName: name,
		Tool: tooling.ToolMetadata{
			Name:  name,
			IsMCP: true,
		},
	}
}

func metadataOnlyInput(name string) PolicyInput {
	return PolicyInput{
		Tool: tooling.ToolMetadata{Name: name},
	}
}

func TestChatModePolicy_AllowsReadOnlyTools(t *testing.T) {
	p := &ChatModePolicy{}
	readOnlyTools := []string{"file_read", "host_list", "search_logs", "get_status", "show_info", "ps_aux", "df_usage", "cat_file"}
	for _, name := range readOnlyTools {
		assertDecision(t, "chat/readonly/"+name, p.CheckTool(toolInput(name)), PolicyActionAllow)
	}
}

func TestChatModePolicy_AllowsWebSearch(t *testing.T) {
	p := &ChatModePolicy{}
	for _, name := range []string{"web_search", "search_web", "web_browse"} {
		assertDecision(t, "chat/websearch/"+name, p.CheckTool(toolInput(name)), PolicyActionAllow)
	}
}

func TestModePoliciesAllowUpdatePlanWithoutApproval(t *testing.T) {
	cases := []struct {
		name   string
		policy ModePolicy
		mode   Mode
	}{
		{name: "chat", policy: &ChatModePolicy{}, mode: "chat"},
		{name: "inspect", policy: &InspectModePolicy{}, mode: "inspect"},
		{name: "plan", policy: &PlanModePolicy{}, mode: "plan"},
		{name: "execute", policy: &ExecuteModePolicy{}, mode: "execute"},
	}
	for _, tc := range cases {
		input := toolInput("update_plan")
		input.Mode = tc.mode
		assertDecision(t, tc.name+"/update_plan", tc.policy.CheckTool(input), PolicyActionAllow)
	}
}

func TestChatModePolicy_DeniesMutation(t *testing.T) {
	p := &ChatModePolicy{}
	mutationTools := []string{"file_write", "host_delete", "service_restart", "process_kill", "container_remove", "task_create", "config_update", "command_exec", "script_run", "service_stop"}
	for _, name := range mutationTools {
		assertDecision(t, "chat/mutation/"+name, p.CheckTool(toolInput(name)), PolicyActionDeny)
	}
}

func TestChatModePolicy_DeniesUnknownTool(t *testing.T) {
	p := &ChatModePolicy{}
	assertDecision(t, "chat/unknown", p.CheckTool(toolInput("mysterious_tool")), PolicyActionDeny)
	assertDecision(t, "chat/ui_surface_name", p.CheckTool(toolInput("dashboard_panel")), PolicyActionDeny)
	assertDecision(t, "chat/workspace_name", p.CheckTool(toolInput("workspace_dispatch")), PolicyActionDeny)
}

func TestChatModePolicy_UsesMCPMetadata(t *testing.T) {
	p := &ChatModePolicy{}
	assertDecision(t, "chat/mcp_readonly", p.CheckTool(mcpToolInput("coroot.list_services")), PolicyActionAllow)
	assertDecision(t, "chat/mcp_mutation", p.CheckTool(mcpToolInput("coroot.file_write")), PolicyActionDeny)
}

func TestChatModePolicy_AllowsProviderSafeCorootReadOnlyTools(t *testing.T) {
	p := &ChatModePolicy{}
	for _, tc := range []struct {
		callName      string
		canonicalName string
	}{
		{"coroot_list_services", "coroot.list_services"},
		{"coroot_collect_rca_context", "coroot.collect_rca_context"},
		{"coroot_service_metrics", "coroot.service_metrics"},
		{"coroot_rca_report", "coroot.rca_report"},
		{"coroot_service_topology", "coroot.service_topology"},
		{"coroot_alert_rules", "coroot.alert_rules"},
		{"coroot_incidents", "coroot.incidents"},
		{"coroot_incident_timeline", "coroot.incident_timeline"},
		{"coroot_slo_status", "coroot.slo_status"},
	} {
		input := PolicyInput{
			ToolName: tc.callName,
			Tool: tooling.ToolMetadata{
				Name:      tc.canonicalName,
				IsMCP:     true,
				Domain:    "coroot",
				RiskLevel: tooling.ToolRiskLow,
			},
			Mode: ModeChat,
		}
		assertDecision(t, "chat/coroot_readonly/"+tc.callName, p.CheckTool(input), PolicyActionAllow)
	}
}

func TestModePoliciesAllowExplicitLowRiskReadMetadata(t *testing.T) {
	cases := []struct {
		name   string
		policy ModePolicy
		mode   Mode
	}{
		{name: "chat", policy: &ChatModePolicy{}, mode: ModeChat},
		{name: "inspect", policy: &InspectModePolicy{}, mode: ModeInspect},
		{name: "plan", policy: &PlanModePolicy{}, mode: ModePlan},
	}
	for _, tc := range cases {
		input := PolicyInput{
			ToolName: "observability_collect_context",
			Tool: tooling.ToolMetadata{
				Name:      "observability.collect_context",
				IsMCP:     true,
				Domain:    "observability",
				RiskLevel: tooling.ToolRiskLow,
				Discovery: tooling.ToolDiscoveryMetadata{
					PermissionScope: "read",
					OperationKinds:  []string{"read", "summarize"},
				},
			},
			Mode: tc.mode,
		}
		assertDecision(t, tc.name+"/metadata_readonly", tc.policy.CheckTool(input), PolicyActionAllow)
	}
}

func TestChatModePolicyDoesNotTreatLowRiskMetadataAsReadOnlyWithoutReadScope(t *testing.T) {
	p := &ChatModePolicy{}
	input := PolicyInput{
		ToolName: "observability_collect_context",
		Tool: tooling.ToolMetadata{
			Name:      "observability.collect_context",
			IsMCP:     true,
			Domain:    "observability",
			RiskLevel: tooling.ToolRiskLow,
		},
		Mode: ModeChat,
	}
	assertDecision(t, "chat/low_risk_without_read_scope", p.CheckTool(input), PolicyActionDeny)
}

func TestChatModePolicyDoesNotAllowExplicitReadMetadataWhenMutating(t *testing.T) {
	p := &ChatModePolicy{}
	input := PolicyInput{
		ToolName: "observability_collect_context",
		Tool: tooling.ToolMetadata{
			Name:      "observability.collect_context",
			IsMCP:     true,
			Domain:    "observability",
			RiskLevel: tooling.ToolRiskLow,
			Mutating:  true,
			Discovery: tooling.ToolDiscoveryMetadata{
				PermissionScope: "read",
				OperationKinds:  []string{"read"},
			},
		},
		Mode: ModeChat,
	}
	assertDecision(t, "chat/read_metadata_mutating", p.CheckTool(input), PolicyActionDeny)
}

func TestInspectModePolicy_AllowsReadOnlyAndWebSearch(t *testing.T) {
	p := &InspectModePolicy{}
	for _, name := range []string{"file_read", "host_list", "search_logs", "get_metrics", "show_config", "status_check", "ls_dir", "cat_file", "head_file", "tail_log", "web_search"} {
		assertDecision(t, "inspect/"+name, p.CheckTool(toolInput(name)), PolicyActionAllow)
	}
}

func TestInspectModePolicy_DeniesMutationAndUnknown(t *testing.T) {
	p := &InspectModePolicy{}
	for _, name := range []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec", "analyze_logs", "workspace_dispatch", "mysterious_tool"} {
		assertDecision(t, "inspect/"+name, p.CheckTool(toolInput(name)), PolicyActionDeny)
	}
}

func TestPlanModePolicy_AllowsReadOnlySearchAndPlanArtifactTools(t *testing.T) {
	p := &PlanModePolicy{}
	for _, name := range []string{"file_read", "host_list", "search_logs", "get_metrics", "web_search", "update_plan", "enter_plan_mode", "exit_plan_mode", "request_plan_approval", "claim_next_task"} {
		assertDecision(t, "plan/"+name, p.CheckTool(toolInput(name)), PolicyActionAllow)
	}
}

func TestPlanModePolicyAllowsTypedOrchestrationControlIndependentOfToolName(t *testing.T) {
	p := &PlanModePolicy{}
	for _, name := range []string{"coordinate_children", "file_write"} {
		input := PolicyInput{
			ToolName: name,
			Tool: tooling.ToolMetadata{
				Name:      name,
				RiskLevel: tooling.ToolRiskLow,
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "orchestration_control",
					OperationKinds: []string{"spawn_child_agents"},
				},
			},
			Mode: ModePlan,
		}
		assertDecision(t, "plan/orchestration_control/"+name, p.CheckTool(input), PolicyActionAllow)
	}
}

func TestPlanModePolicyFailsClosedForUnsafeOrchestrationControlGovernance(t *testing.T) {
	p := &PlanModePolicy{}
	tests := []struct {
		name             string
		mutating         bool
		requiresApproval bool
		risk             tooling.ToolRiskLevel
	}{
		{name: "mutating", mutating: true, risk: tooling.ToolRiskLow},
		{name: "approval_required", requiresApproval: true, risk: tooling.ToolRiskLow},
		{name: "medium_risk", risk: tooling.ToolRiskMedium},
		{name: "high_risk", risk: tooling.ToolRiskHigh},
	}
	for _, tc := range tests {
		input := PolicyInput{
			ToolName: "coordinate_children",
			Tool: tooling.ToolMetadata{
				Name:             "coordinate_children",
				Mutating:         tc.mutating,
				RequiresApproval: tc.requiresApproval,
				RiskLevel:        tc.risk,
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "orchestration_control",
					OperationKinds: []string{"send_message"},
				},
			},
			Mode: ModePlan,
		}
		assertDecision(t, "plan/orchestration_control/"+tc.name, p.CheckTool(input), PolicyActionDeny)
	}
}

func TestChatAndInspectModesDoNotAllowOrchestrationControlCapability(t *testing.T) {
	input := PolicyInput{
		ToolName: "read_orchestration_control",
		Tool: tooling.ToolMetadata{
			Name:      "read_orchestration_control",
			RiskLevel: tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "orchestration_control",
				OperationKinds: []string{"send_message"},
			},
		},
	}
	for _, tc := range []struct {
		name   string
		mode   Mode
		policy ModePolicy
	}{
		{name: "chat", mode: ModeChat, policy: &ChatModePolicy{}},
		{name: "inspect", mode: ModeInspect, policy: &InspectModePolicy{}},
	} {
		input.Mode = tc.mode
		assertDecision(t, tc.name+"/orchestration_control", tc.policy.CheckTool(input), PolicyActionDeny)
	}
}

func TestPlanModeOnlyAllowsReadOnlyAndPlanArtifactTools(t *testing.T) {
	p := &PlanModePolicy{}
	for _, name := range []string{"draft_proposal", "propose_changes", "schedule_task", "preview_changes", "plan_database_update", "draft_config_write", "propose_restart"} {
		input := toolInput(name)
		input.Tool.Mutating = true
		assertDecision(t, "plan/deny_disguised_mutation/"+name, p.CheckTool(input), PolicyActionDeny)
	}
	for _, name := range []string{"update_plan", "enter_plan_mode", "exit_plan_mode", "request_plan_approval", "claim_next_task"} {
		input := toolInput(name)
		input.Tool.Mutating = true
		assertDecision(t, "plan/allow_exact_artifact/"+name, p.CheckTool(input), PolicyActionAllow)
	}
}

func TestPlanModePolicy_DeniesMutationAndUnknown(t *testing.T) {
	p := &PlanModePolicy{}
	for _, name := range []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec", "create_plan", "draft_proposal", "propose_changes", "schedule_task", "preview_changes", "summarize", "workspace_dispatch", "mysterious_tool"} {
		assertDecision(t, "plan/"+name, p.CheckTool(toolInput(name)), PolicyActionDeny)
	}
}

func TestExecuteModePolicy_AllowsNonMutationTools(t *testing.T) {
	p := &ExecuteModePolicy{}
	for _, name := range []string{"file_read", "host_list", "search_logs", "get_metrics", "web_search", "summarize", "workspace_dispatch", "draft_proposal", "mysterious_tool"} {
		assertDecision(t, "execute/"+name, p.CheckTool(toolInput(name)), PolicyActionAllow)
	}
}

func TestModePoliciesTreatPreflightToolsAsReadOnly(t *testing.T) {
	cases := []struct {
		name   string
		policy ModePolicy
	}{
		{name: "chat", policy: &ChatModePolicy{}},
		{name: "inspect", policy: &InspectModePolicy{}},
		{name: "plan", policy: &PlanModePolicy{}},
		{name: "execute", policy: &ExecuteModePolicy{}},
	}
	for _, tc := range cases {
		assertDecision(t, tc.name+"/run_ops_manual_preflight", tc.policy.CheckTool(toolInput("run_ops_manual_preflight")), PolicyActionAllow)
	}
}

func TestExecuteModePolicy_MutationNeedsApproval(t *testing.T) {
	p := &ExecuteModePolicy{}
	for _, name := range []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec"} {
		d := p.CheckTool(toolInput(name))
		assertDecision(t, "execute/mutation/"+name, d, PolicyActionNeedApproval)
		if d.Approval == nil {
			t.Fatalf("execute/mutation/%s: expected non-nil Approval", name)
		}
		if d.Approval.ToolName != name {
			t.Fatalf("execute/mutation/%s: approval.ToolName = %q, want %q", name, d.Approval.ToolName, name)
		}
	}
}

func TestModePoliciesUseMetadataNameWhenToolNameEmpty(t *testing.T) {
	tests := []struct {
		name   string
		policy ModePolicy
	}{
		{name: "chat", policy: &ChatModePolicy{}},
		{name: "inspect", policy: &InspectModePolicy{}},
		{name: "plan", policy: &PlanModePolicy{}},
		{name: "execute", policy: &ExecuteModePolicy{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertDecision(t, tc.name+"/metadata_only_read", tc.policy.CheckTool(metadataOnlyInput("file_read")), PolicyActionAllow)
		})
	}
}

func TestEngineCheckToolCallUsesMCPMetadata(t *testing.T) {
	engine := &Engine{
		ModePolicy: NewDefaultModePolicies(),
	}

	decision := engine.CheckToolCall(t.Context(), PolicyInput{
		ToolName: "coroot.file_write",
		Tool:     tooling.ToolMetadata{Name: "coroot.file_write", IsMCP: true},
		Mode:     ModeChat,
	})

	assertDecision(t, "engine/chat/mcp_metadata_mutation", decision, PolicyActionDeny)
}

func TestNewDefaultModePolicies_ReturnsAllFourModes(t *testing.T) {
	policies := NewDefaultModePolicies()
	expectedModes := []Mode{ModeChat, ModeInspect, ModePlan, ModeExecute}

	for _, mode := range expectedModes {
		if _, ok := policies[mode]; !ok {
			t.Errorf("NewDefaultModePolicies missing mode %q", mode)
		}
	}

	if len(policies) != 4 {
		t.Errorf("NewDefaultModePolicies returned %d policies, want 4", len(policies))
	}
}

func TestNewDefaultModePolicies_CorrectTypes(t *testing.T) {
	policies := NewDefaultModePolicies()

	if _, ok := policies[ModeChat].(*ChatModePolicy); !ok {
		t.Error("chat mode policy is not *ChatModePolicy")
	}
	if _, ok := policies[ModeInspect].(*InspectModePolicy); !ok {
		t.Error("inspect mode policy is not *InspectModePolicy")
	}
	if _, ok := policies[ModePlan].(*PlanModePolicy); !ok {
		t.Error("plan mode policy is not *PlanModePolicy")
	}
	if _, ok := policies[ModeExecute].(*ExecuteModePolicy); !ok {
		t.Error("execute mode policy is not *ExecuteModePolicy")
	}
}

func TestIsReadOnly(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"file_read", true},
		{"host_list", true},
		{"search_logs", true},
		{"get_status", true},
		{"show_config", true},
		{"status_check", true},
		{"info_panel", true},
		{"ps_aux", true},
		{"df_usage", true},
		{"top_processes", true},
		{"cat_file", true},
		{"head_lines", true},
		{"tail_log", true},
		{"ls_directory", true},
		{"run_ops_manual_preflight", true},
		{"coroot.service_metrics", true},
		{"coroot_incidents", true},
		{"coroot_rca_report", true},
		{"file_write", false},
		{"mysterious_tool", false},
	}
	for _, tc := range cases {
		if got := isReadOnly(tc.name); got != tc.want {
			t.Errorf("isReadOnly(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsMutation(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"file_write", true},
		{"host_delete", true},
		{"container_remove", true},
		{"task_create", true},
		{"config_update", true},
		{"command_exec", true},
		{"script_run", true},
		{"service_restart", true},
		{"service_stop", true},
		{"process_kill", true},
		{"run_ops_manual_preflight", false},
		{"file_read", false},
		{"mysterious_tool", false},
	}
	for _, tc := range cases {
		if got := isMutation(tc.name); got != tc.want {
			t.Errorf("isMutation(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsWebSearch(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"web_search", true},
		{"search_web", true},
		{"web_browse", true},
		{"file_read", false},
		{"mysterious_tool", false},
	}
	for _, tc := range cases {
		if got := isWebSearch(tc.name); got != tc.want {
			t.Errorf("isWebSearch(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsPlanTool(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"update_plan", true},
		{"enter_plan_mode", true},
		{"exit_plan_mode", true},
		{"request_plan_approval", true},
		{"claim_next_task", true},
		{"create_plan", false},
		{"draft_proposal", false},
		{"propose_changes", false},
		{"schedule_task", false},
		{"preview_changes", false},
		{"file_read", false},
	}
	for _, tc := range cases {
		if got := isPlanTool(tc.name); got != tc.want {
			t.Errorf("isPlanTool(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
