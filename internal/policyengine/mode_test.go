package policyengine

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func assertDecision(t *testing.T, desc string, got PolicyDecision, wantAction PolicyAction) {
	t.Helper()
	if got.Action != wantAction {
		t.Errorf("%s: got action %q, want %q (reason: %s)", desc, got.Action, wantAction, got.Reason)
	}
}

// ---------------------------------------------------------------------------
// ChatModePolicy tests
// ---------------------------------------------------------------------------

func TestChatModePolicy_AllowsReadOnlyTools(t *testing.T) {
	p := &ChatModePolicy{}
	readOnlyTools := []string{"file_read", "host_list", "search_logs", "get_status", "show_info", "ps_aux", "df_usage", "cat_file"}
	for _, name := range readOnlyTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "chat/readonly/"+name, d, PolicyActionAllow)
	}
}

func TestChatModePolicy_AllowsSkills(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("summarize_conversation", KindSkill)
	assertDecision(t, "chat/skill", d, PolicyActionAllow)
}

func TestChatModePolicy_AllowsWebSearch(t *testing.T) {
	p := &ChatModePolicy{}
	tools := []string{"web_search", "search_web", "web_browse"}
	for _, name := range tools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "chat/websearch/"+name, d, PolicyActionAllow)
	}
}

func TestChatModePolicy_DeniesMutation(t *testing.T) {
	p := &ChatModePolicy{}
	mutationTools := []string{"file_write", "host_delete", "service_restart", "process_kill", "container_remove", "task_create", "config_update", "command_exec", "script_run", "service_stop"}
	for _, name := range mutationTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "chat/mutation/"+name, d, PolicyActionDeny)
	}
}

func TestChatModePolicy_DeniesWorkspace(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("workspace_dispatch", KindWorkspace)
	assertDecision(t, "chat/workspace", d, PolicyActionDeny)
}

func TestChatModePolicy_DeniesUnknownTool(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("mysterious_tool", KindTool)
	assertDecision(t, "chat/unknown", d, PolicyActionDeny)
}

func TestChatModePolicy_AllowsUISurface(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("dashboard_panel", KindUISurface)
	assertDecision(t, "chat/ui_surface", d, PolicyActionAllow)
}

func TestChatModePolicy_DeniesMCPMutation(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("mcp_file_write", KindMCPTool)
	assertDecision(t, "chat/mcp_mutation", d, PolicyActionDeny)
}

func TestChatModePolicy_AllowsMCPReadOnly(t *testing.T) {
	p := &ChatModePolicy{}
	d := p.CheckCapability("mcp_list_services", KindMCPTool)
	assertDecision(t, "chat/mcp_readonly", d, PolicyActionAllow)
}

// ---------------------------------------------------------------------------
// InspectModePolicy tests
// ---------------------------------------------------------------------------

func TestInspectModePolicy_AllowsReadOnlyTools(t *testing.T) {
	p := &InspectModePolicy{}
	readOnlyTools := []string{"file_read", "host_list", "search_logs", "get_metrics", "show_config", "status_check", "ls_dir", "cat_file", "head_file", "tail_log"}
	for _, name := range readOnlyTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "inspect/readonly/"+name, d, PolicyActionAllow)
	}
}

func TestInspectModePolicy_AllowsWebSearch(t *testing.T) {
	p := &InspectModePolicy{}
	d := p.CheckCapability("web_search", KindTool)
	assertDecision(t, "inspect/websearch", d, PolicyActionAllow)
}

func TestInspectModePolicy_DeniesMutation(t *testing.T) {
	p := &InspectModePolicy{}
	mutationTools := []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec"}
	for _, name := range mutationTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "inspect/mutation/"+name, d, PolicyActionDeny)
	}
}

func TestInspectModePolicy_AllowsSkills(t *testing.T) {
	p := &InspectModePolicy{}
	d := p.CheckCapability("analyze_logs", KindSkill)
	assertDecision(t, "inspect/skill", d, PolicyActionAllow)
}

func TestInspectModePolicy_DeniesWorkspace(t *testing.T) {
	p := &InspectModePolicy{}
	d := p.CheckCapability("workspace_dispatch", KindWorkspace)
	assertDecision(t, "inspect/workspace", d, PolicyActionDeny)
}

func TestInspectModePolicy_DeniesUnknownTool(t *testing.T) {
	p := &InspectModePolicy{}
	d := p.CheckCapability("mysterious_tool", KindTool)
	assertDecision(t, "inspect/unknown", d, PolicyActionDeny)
}

// ---------------------------------------------------------------------------
// PlanModePolicy tests
// ---------------------------------------------------------------------------

func TestPlanModePolicy_AllowsReadOnlyTools(t *testing.T) {
	p := &PlanModePolicy{}
	readOnlyTools := []string{"file_read", "host_list", "search_logs", "get_metrics"}
	for _, name := range readOnlyTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "plan/readonly/"+name, d, PolicyActionAllow)
	}
}

func TestPlanModePolicy_AllowsPlanTools(t *testing.T) {
	p := &PlanModePolicy{}
	planTools := []string{"create_plan", "draft_proposal", "propose_changes", "schedule_task", "preview_changes"}
	for _, name := range planTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "plan/plan_tool/"+name, d, PolicyActionAllow)
	}
}

func TestPlanModePolicy_DeniesMutation(t *testing.T) {
	p := &PlanModePolicy{}
	mutationTools := []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec"}
	for _, name := range mutationTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "plan/mutation/"+name, d, PolicyActionDeny)
	}
}

func TestPlanModePolicy_AllowsWebSearch(t *testing.T) {
	p := &PlanModePolicy{}
	d := p.CheckCapability("web_search", KindTool)
	assertDecision(t, "plan/websearch", d, PolicyActionAllow)
}

func TestPlanModePolicy_AllowsSkills(t *testing.T) {
	p := &PlanModePolicy{}
	d := p.CheckCapability("summarize", KindSkill)
	assertDecision(t, "plan/skill", d, PolicyActionAllow)
}

func TestPlanModePolicy_DeniesWorkspace(t *testing.T) {
	p := &PlanModePolicy{}
	d := p.CheckCapability("workspace_dispatch", KindWorkspace)
	assertDecision(t, "plan/workspace", d, PolicyActionDeny)
}

func TestPlanModePolicy_DeniesUnknownTool(t *testing.T) {
	p := &PlanModePolicy{}
	d := p.CheckCapability("mysterious_tool", KindTool)
	assertDecision(t, "plan/unknown", d, PolicyActionDeny)
}

// ---------------------------------------------------------------------------
// ExecuteModePolicy tests
// ---------------------------------------------------------------------------

func TestExecuteModePolicy_AllowsReadOnlyTools(t *testing.T) {
	p := &ExecuteModePolicy{}
	readOnlyTools := []string{"file_read", "host_list", "search_logs", "get_metrics"}
	for _, name := range readOnlyTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "execute/readonly/"+name, d, PolicyActionAllow)
	}
}

func TestExecuteModePolicy_MutationNeedsApproval(t *testing.T) {
	p := &ExecuteModePolicy{}
	mutationTools := []string{"file_write", "host_delete", "service_restart", "process_kill", "command_exec"}
	for _, name := range mutationTools {
		d := p.CheckCapability(name, KindTool)
		assertDecision(t, "execute/mutation/"+name, d, PolicyActionNeedApproval)
		if d.Approval == nil {
			t.Errorf("execute/mutation/%s: expected non-nil Approval", name)
		} else if d.Approval.ToolName != name {
			t.Errorf("execute/mutation/%s: approval.ToolName = %q, want %q", name, d.Approval.ToolName, name)
		}
	}
}

func TestExecuteModePolicy_AllowsWebSearch(t *testing.T) {
	p := &ExecuteModePolicy{}
	d := p.CheckCapability("web_search", KindTool)
	assertDecision(t, "execute/websearch", d, PolicyActionAllow)
}

func TestExecuteModePolicy_AllowsSkills(t *testing.T) {
	p := &ExecuteModePolicy{}
	d := p.CheckCapability("summarize", KindSkill)
	assertDecision(t, "execute/skill", d, PolicyActionAllow)
}

func TestExecuteModePolicy_AllowsWorkspace(t *testing.T) {
	p := &ExecuteModePolicy{}
	d := p.CheckCapability("workspace_dispatch", KindWorkspace)
	assertDecision(t, "execute/workspace", d, PolicyActionAllow)
}

func TestExecuteModePolicy_AllowsPlanTools(t *testing.T) {
	p := &ExecuteModePolicy{}
	// "draft_proposal" is a plan tool that doesn't match mutation patterns.
	d := p.CheckCapability("draft_proposal", KindTool)
	assertDecision(t, "execute/plan", d, PolicyActionAllow)
}

// ---------------------------------------------------------------------------
// NewDefaultModePolicies tests
// ---------------------------------------------------------------------------

func TestNewDefaultModePolicies_ReturnsAllFourModes(t *testing.T) {
	policies := NewDefaultModePolicies()

	expectedModes := []Mode{
		ModeChat,
		ModeInspect,
		ModePlan,
		ModeExecute,
	}

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

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

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
