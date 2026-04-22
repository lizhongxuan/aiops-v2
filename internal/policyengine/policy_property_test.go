package policyengine

// Feature: aiops-codex-eino-rewrite, Property 13: Mode-能力边界执行
// Feature: aiops-codex-eino-rewrite, Property 14: 三层策略检查管道
// Feature: aiops-codex-eino-rewrite, Property 15: CompletionPolicy Final Gate
// Feature: aiops-codex-eino-rewrite, Property 23: 三层能力分类
// Feature: aiops-codex-eino-rewrite, Property 24: 层级审批执行
// Feature: aiops-codex-eino-rewrite, Property 25: 高风险命令 TTL 强制

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/tooling"

	"pgregory.net/rapid"
)

// **Validates: Requirements 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 8.1, 8.2, 8.3, 8.5**

// ---------------------------------------------------------------------------
// Generators
// ---------------------------------------------------------------------------

// readOnlyToolNames are tool names that match read-only patterns.
var readOnlyToolNames = []string{
	"file_read", "host_list", "search_logs", "get_status",
	"show_config", "info_panel", "ps_aux", "df_usage",
	"top_processes", "cat_file", "head_lines", "tail_log", "ls_dir",
}

// mutationToolNames are tool names that match mutation patterns.
var mutationToolNames = []string{
	"file_write", "host_delete", "container_remove", "task_create",
	"config_update", "command_exec", "script_run", "service_restart",
	"service_stop", "process_kill",
}

// genReadOnlyToolName generates a tool name matching read-only patterns.
func genReadOnlyToolName() *rapid.Generator[string] {
	return rapid.SampledFrom(readOnlyToolNames)
}

// genMutationToolName generates a tool name matching mutation patterns.
func genMutationToolName() *rapid.Generator[string] {
	return rapid.SampledFrom(mutationToolNames)
}

// genMode generates one of the four canonical modes.
func genMode() *rapid.Generator[Mode] {
	return rapid.SampledFrom([]Mode{ModeChat, ModeInspect, ModePlan, ModeExecute})
}

// ---------------------------------------------------------------------------
// Property 13: Mode-能力边界执行
// ---------------------------------------------------------------------------

// TestProperty13_ChatModeDeniesMutation verifies that for any mutation tool name
// chat mode always denies the operation.
func TestProperty13_ChatModeDeniesMutation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := &ChatModePolicy{}
		toolName := genMutationToolName().Draw(t, "mutationTool")

		decision := policy.CheckTool(PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
		})

		if decision.Action != PolicyActionDeny {
			t.Fatalf("chat mode should deny mutation tool %q, got action=%s",
				toolName, decision.Action)
		}
	})
}

// TestProperty13_InspectModeDeniesMutation verifies that for any mutation tool name
// inspect mode always denies the operation.
func TestProperty13_InspectModeDeniesMutation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := &InspectModePolicy{}
		toolName := genMutationToolName().Draw(t, "mutationTool")

		decision := policy.CheckTool(PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
		})

		if decision.Action != PolicyActionDeny {
			t.Fatalf("inspect mode should deny mutation tool %q, got action=%s",
				toolName, decision.Action)
		}
	})
}

// TestProperty13_PlanModeDeniesDirectMutation verifies that for any mutation tool name
// plan mode denies direct mutation execution.
func TestProperty13_PlanModeDeniesDirectMutation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := &PlanModePolicy{}
		toolName := genMutationToolName().Draw(t, "mutationTool")

		decision := policy.CheckTool(PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
		})

		if decision.Action != PolicyActionDeny {
			t.Fatalf("plan mode should deny direct mutation tool %q, got action=%s",
				toolName, decision.Action)
		}
	})
}

// TestProperty13_ExecuteModeAllowsMutationWithApproval verifies that for any mutation
// tool name, execute mode returns NeedApproval with a valid ApprovalRequest.
func TestProperty13_ExecuteModeAllowsMutationWithApproval(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := &ExecuteModePolicy{}
		toolName := genMutationToolName().Draw(t, "mutationTool")

		decision := policy.CheckTool(PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
		})

		if decision.Action != PolicyActionNeedApproval {
			t.Fatalf("execute mode should require approval for mutation tool %q, got action=%s",
				toolName, decision.Action)
		}
		if decision.Approval == nil {
			t.Fatalf("execute mode should provide ApprovalRequest for mutation tool %q", toolName)
		}
		if decision.Approval.ToolName != toolName {
			t.Fatalf("ApprovalRequest.ToolName should be %q, got %q", toolName, decision.Approval.ToolName)
		}
	})
}

// TestProperty13_ReadOnlyAllowedInAllModes verifies that for any read-only tool name
// with metadata/name traits, all four modes allow the operation.
func TestProperty13_ReadOnlyAllowedInAllModes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := genMode().Draw(t, "mode")
		toolName := genReadOnlyToolName().Draw(t, "readOnlyTool")

		policies := NewDefaultModePolicies()
		policy := policies[mode]

		decision := policy.CheckTool(PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
		})

		if decision.Action != PolicyActionAllow {
			t.Fatalf("mode %q should allow read-only tool %q, got action=%s reason=%s",
				mode, toolName, decision.Action, decision.Reason)
		}
	})
}

// TestProperty13_ModeEnforcementViaEngine verifies that the Engine correctly routes
// to the appropriate mode policy for any mode and mutation tool combination.
func TestProperty13_ModeEnforcementViaEngine(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		mode := genMode().Draw(t, "mode")
		toolName := genMutationToolName().Draw(t, "mutationTool")

		engine := &Engine{
			ModePolicy: NewDefaultModePolicies(),
		}

		input := PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
			Mode:     mode,
		}

		decision := engine.CheckToolCall(context.Background(), input)

		switch mode {
		case ModeChat, ModeInspect, ModePlan:
			if decision.Action != PolicyActionDeny {
				t.Fatalf("engine with mode %q should deny mutation tool %q, got action=%s",
					mode, toolName, decision.Action)
			}
		case ModeExecute:
			if decision.Action != PolicyActionNeedApproval {
				t.Fatalf("engine with mode execute should require approval for mutation tool %q, got action=%s",
					toolName, decision.Action)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Property 14: 三层策略检查管道
// ---------------------------------------------------------------------------

// mockPermissionEvaluator returns a configurable PolicyDecision.
type mockPermissionEvaluator struct {
	decision PolicyDecision
}

func (m *mockPermissionEvaluator) CheckPermission(_ context.Context, _ PolicyInput) PolicyDecision {
	return m.decision
}

// mockEvidenceEvaluator returns a configurable PolicyDecision.
type mockEvidenceEvaluator struct {
	decision PolicyDecision
}

func (m *mockEvidenceEvaluator) CheckEvidence(_ context.Context, _ PolicyInput) PolicyDecision {
	return m.decision
}

// genPolicyAction generates one of the four canonical policy actions.
func genPolicyAction() *rapid.Generator[PolicyAction] {
	return rapid.SampledFrom([]PolicyAction{
		PolicyActionAllow,
		PolicyActionDeny,
		PolicyActionNeedApproval,
		PolicyActionNeedEvidence,
	})
}

// TestProperty14_PipelineExecutesInOrder verifies that for any tool call request,
// the Engine executes CapabilityPolicy → PermissionPolicy → EvidencePolicy in sequence.
// The pipeline short-circuits: the first non-Allow result is the final decision.
// When permission allows, evidence's decision is the final result.
// When permission denies/restricts, that becomes the final result (evidence not reached).
func TestProperty14_PipelineExecutesInOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Use execute mode so mode policy allows everything (read-only tools pass through)
		toolName := genReadOnlyToolName().Draw(t, "readOnlyTool")
		permAction := genPolicyAction().Draw(t, "permAction")
		evidAction := genPolicyAction().Draw(t, "evidAction")

		engine := &Engine{
			ModePolicy: NewDefaultModePolicies(),
			PermissionPolicy: &mockPermissionEvaluator{
				decision: PolicyDecision{Action: permAction, Reason: "perm-reason"},
			},
			EvidencePolicy: &mockEvidenceEvaluator{
				decision: PolicyDecision{Action: evidAction, Reason: "evid-reason"},
			},
		}

		input := PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
			Mode:     ModeExecute,
		}

		decision := engine.CheckToolCall(context.Background(), input)

		// Pipeline short-circuits: first non-Allow layer wins.
		// If permission is non-Allow, it's the final decision (evidence not consulted).
		// If permission allows, evidence's decision is the final result.
		var expected PolicyAction
		if permAction != PolicyActionAllow {
			expected = permAction
		} else {
			expected = evidAction
		}

		if decision.Action != expected {
			t.Fatalf("pipeline should short-circuit: perm=%s, evid=%s, expected=%s, got=%s",
				permAction, evidAction, expected, decision.Action)
		}
	})
}

// TestProperty14_ModePolicyDenyShortCircuits verifies that when mode policy denies,
// the pipeline short-circuits and does not consult permission or evidence policies.
func TestProperty14_ModePolicyDenyShortCircuits(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Use chat mode with a mutation tool — mode policy will deny
		toolName := genMutationToolName().Draw(t, "mutationTool")

		// Permission and evidence would allow, but should never be reached
		permCalled := false
		evidCalled := false

		engine := &Engine{
			ModePolicy: NewDefaultModePolicies(),
			PermissionPolicy: &trackingPermissionEvaluator{
				decision: PolicyDecision{Action: PolicyActionAllow},
				called:   &permCalled,
			},
			EvidencePolicy: &trackingEvidenceEvaluator{
				decision: PolicyDecision{Action: PolicyActionAllow},
				called:   &evidCalled,
			},
		}

		input := PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
			Mode:     ModeChat,
		}

		decision := engine.CheckToolCall(context.Background(), input)

		if decision.Action != PolicyActionDeny {
			t.Fatalf("mode policy should deny mutation in chat mode, got %s", decision.Action)
		}
		if permCalled {
			t.Fatal("permission policy should not be called when mode policy denies")
		}
		if evidCalled {
			t.Fatal("evidence policy should not be called when mode policy denies")
		}
	})
}

// TestProperty14_PermissionDenyShortCircuitsEvidence verifies that when permission
// policy denies, the pipeline short-circuits and does not consult evidence policy.
func TestProperty14_PermissionDenyShortCircuitsEvidence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genReadOnlyToolName().Draw(t, "readOnlyTool")

		evidCalled := false

		engine := &Engine{
			ModePolicy: NewDefaultModePolicies(),
			PermissionPolicy: &mockPermissionEvaluator{
				decision: PolicyDecision{Action: PolicyActionDeny, Reason: "no permission"},
			},
			EvidencePolicy: &trackingEvidenceEvaluator{
				decision: PolicyDecision{Action: PolicyActionAllow},
				called:   &evidCalled,
			},
		}

		input := PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
			Mode:     ModeExecute,
		}

		decision := engine.CheckToolCall(context.Background(), input)

		if decision.Action != PolicyActionDeny {
			t.Fatalf("permission deny should propagate, got %s", decision.Action)
		}
		if evidCalled {
			t.Fatal("evidence policy should not be called when permission policy denies")
		}
	})
}

// TestProperty14_AllLayersAllowResultsInAllow verifies that when all three layers
// allow, the final decision is Allow.
func TestProperty14_AllLayersAllowResultsInAllow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genReadOnlyToolName().Draw(t, "readOnlyTool")
		mode := genMode().Draw(t, "mode")

		engine := &Engine{
			ModePolicy: NewDefaultModePolicies(),
			PermissionPolicy: &mockPermissionEvaluator{
				decision: PolicyDecision{Action: PolicyActionAllow},
			},
			EvidencePolicy: &mockEvidenceEvaluator{
				decision: PolicyDecision{Action: PolicyActionAllow},
			},
		}

		input := PolicyInput{
			ToolName: toolName,
			Tool:     tooling.ToolMetadata{Name: toolName},
			Mode:     mode,
		}

		decision := engine.CheckToolCall(context.Background(), input)

		if decision.Action != PolicyActionAllow {
			t.Fatalf("all layers allow should result in Allow, got %s (mode=%s, tool=%s)",
				decision.Action, mode, toolName)
		}
	})
}

// ---------------------------------------------------------------------------
// Tracking evaluators for short-circuit verification
// ---------------------------------------------------------------------------

type trackingPermissionEvaluator struct {
	decision PolicyDecision
	called   *bool
}

func (m *trackingPermissionEvaluator) CheckPermission(_ context.Context, _ PolicyInput) PolicyDecision {
	*m.called = true
	return m.decision
}

type trackingEvidenceEvaluator struct {
	decision PolicyDecision
	called   *bool
}

func (m *trackingEvidenceEvaluator) CheckEvidence(_ context.Context, _ PolicyInput) PolicyDecision {
	*m.called = true
	return m.decision
}

// ---------------------------------------------------------------------------
// Property 15: CompletionPolicy Final Gate
// **Validates: Requirements 4.7**
// ---------------------------------------------------------------------------

// genPendingApprovals generates a slice of 0-5 pending approval IDs.
func genPendingApprovals() *rapid.Generator[[]string] {
	return rapid.SliceOfN(rapid.StringMatching(`approval-[a-z0-9]{4}`), 0, 5)
}

// genPendingEvidence generates a slice of 0-5 pending evidence IDs.
func genPendingEvidence() *rapid.Generator[[]string] {
	return rapid.SliceOfN(rapid.StringMatching(`evidence-[a-z0-9]{4}`), 0, 5)
}

// TestProperty15_CompletionDeniesWhenPendingApprovals verifies that for any
// turn state with pending approvals, CompletionPolicy denies finalization.
func TestProperty15_CompletionDeniesWhenPendingApprovals(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		eval := &DefaultCompletionEvaluator{}
		approvals := rapid.SliceOfN(rapid.StringMatching(`approval-[a-z0-9]{4}`), 1, 5).Draw(t, "approvals")

		state := TurnState{
			SessionID:        "sess-1",
			TurnID:           "turn-1",
			PendingApprovals: approvals,
		}

		decision := eval.CheckCompletion(context.Background(), state)

		if decision.Action != PolicyActionDeny {
			t.Fatalf("CompletionPolicy should deny when pending approvals exist (count=%d), got %s",
				len(approvals), decision.Action)
		}
	})
}

// TestProperty15_CompletionNeedsEvidenceWhenPending verifies that for any
// turn state with pending evidence (but no pending approvals), CompletionPolicy
// returns NeedEvidence.
func TestProperty15_CompletionNeedsEvidenceWhenPending(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		eval := &DefaultCompletionEvaluator{}
		evidence := rapid.SliceOfN(rapid.StringMatching(`evidence-[a-z0-9]{4}`), 1, 5).Draw(t, "evidence")

		state := TurnState{
			SessionID:       "sess-1",
			TurnID:          "turn-1",
			PendingEvidence: evidence,
		}

		decision := eval.CheckCompletion(context.Background(), state)

		if decision.Action != PolicyActionNeedEvidence {
			t.Fatalf("CompletionPolicy should return NeedEvidence when pending evidence exists (count=%d), got %s",
				len(evidence), decision.Action)
		}
	})
}

// TestProperty15_CompletionAllowsWhenAllResolved verifies that for any
// turn state with no pending approvals and no pending evidence, CompletionPolicy
// allows finalization.
func TestProperty15_CompletionAllowsWhenAllResolved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		eval := &DefaultCompletionEvaluator{}
		toolCallCount := rapid.IntRange(0, 100).Draw(t, "toolCallCount")

		state := TurnState{
			SessionID:     "sess-1",
			TurnID:        "turn-1",
			ToolCallCount: toolCallCount,
			Completed:     true,
		}

		decision := eval.CheckCompletion(context.Background(), state)

		if decision.Action != PolicyActionAllow {
			t.Fatalf("CompletionPolicy should allow when no pending items (toolCalls=%d), got %s",
				toolCallCount, decision.Action)
		}
	})
}

// TestProperty15_ApprovalsPriorityOverEvidence verifies that when both pending
// approvals and evidence exist, CompletionPolicy prioritizes approvals (Deny)
// over evidence (NeedEvidence).
func TestProperty15_ApprovalsPriorityOverEvidence(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		eval := &DefaultCompletionEvaluator{}
		approvals := rapid.SliceOfN(rapid.StringMatching(`approval-[a-z0-9]{4}`), 1, 3).Draw(t, "approvals")
		evidence := rapid.SliceOfN(rapid.StringMatching(`evidence-[a-z0-9]{4}`), 1, 3).Draw(t, "evidence")

		state := TurnState{
			SessionID:        "sess-1",
			TurnID:           "turn-1",
			PendingApprovals: approvals,
			PendingEvidence:  evidence,
		}

		decision := eval.CheckCompletion(context.Background(), state)

		if decision.Action != PolicyActionDeny {
			t.Fatalf("CompletionPolicy should deny (approvals priority) when both pending, got %s",
				decision.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 23: 三层能力分类
// **Validates: Requirements 8.1**
// ---------------------------------------------------------------------------

// genStructuredReadTool generates a tool name from the structured read set.
func genStructuredReadTool() *rapid.Generator[string] {
	tools := make([]string, 0, len(structuredReadTools))
	for t := range structuredReadTools {
		tools = append(tools, t)
	}
	return rapid.SampledFrom(tools)
}

// genControlledMutationTool generates a tool name from the controlled mutation set.
func genControlledMutationTool() *rapid.Generator[string] {
	tools := make([]string, 0, len(controlledMutationTools))
	for t := range controlledMutationTools {
		tools = append(tools, t)
	}
	return rapid.SampledFrom(tools)
}

// genRawShellTool generates a tool name matching raw shell patterns.
func genRawShellTool() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"command_exec", "script_run", "shell_exec", "raw_exec",
	})
}

// TestProperty23_StructuredReadClassification verifies that all structured read
// tools are classified as LayerStructuredRead.
func TestProperty23_StructuredReadClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genStructuredReadTool().Draw(t, "structuredReadTool")

		layer := ClassifyTool(toolName)

		if layer != LayerStructuredRead {
			t.Fatalf("tool %q should be classified as structured_read, got %s", toolName, layer)
		}
	})
}

// TestProperty23_ControlledMutationClassification verifies that all controlled
// mutation tools are classified as LayerControlledMutation.
func TestProperty23_ControlledMutationClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genControlledMutationTool().Draw(t, "controlledMutationTool")

		layer := ClassifyTool(toolName)

		if layer != LayerControlledMutation {
			t.Fatalf("tool %q should be classified as controlled_mutation, got %s", toolName, layer)
		}
	})
}

// TestProperty23_RawShellClassification verifies that all raw shell tools
// are classified as LayerRawShell.
func TestProperty23_RawShellClassification(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genRawShellTool().Draw(t, "rawShellTool")

		layer := ClassifyTool(toolName)

		if layer != LayerRawShell {
			t.Fatalf("tool %q should be classified as raw_shell, got %s", toolName, layer)
		}
	})
}

// TestProperty23_ExactlyOneLayer verifies that for any tool name, ClassifyTool
// returns exactly one of the three layers (never empty or invalid).
func TestProperty23_ExactlyOneLayer(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate arbitrary tool names including known and unknown ones
		toolName := rapid.SampledFrom(append(append(append(
			[]string{"host_list", "file_read", "file_write", "service_restart",
				"command_exec", "script_run", "unknown_tool", "random_thing"},
			readOnlyToolNames...),
			mutationToolNames...),
			"shell_exec", "raw_exec",
		)).Draw(t, "anyTool")

		layer := ClassifyTool(toolName)

		validLayers := AllCapabilityLayers()
		found := false
		for _, valid := range validLayers {
			if layer == valid {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ClassifyTool(%q) returned invalid layer %q", toolName, layer)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 24: 层级审批执行
// **Validates: Requirements 8.2, 8.3**
// ---------------------------------------------------------------------------

// TestProperty24_ControlledMutationForcesApproval verifies that for any
// controlled mutation tool call, the gateway forces approval.
func TestProperty24_ControlledMutationForcesApproval(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genControlledMutationTool().Draw(t, "controlledMutationTool")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		gateway := &GatewayPolicy{Whitelist: NewWhitelistManager()}
		decision := gateway.CheckApproval(toolName, "", hostID, time.Now())

		if decision.Action != PolicyActionNeedApproval {
			t.Fatalf("controlled mutation tool %q should force approval, got %s",
				toolName, decision.Action)
		}
		if decision.Approval == nil {
			t.Fatalf("controlled mutation tool %q should have ApprovalRequest", toolName)
		}
		if decision.Approval.ToolName != toolName {
			t.Fatalf("ApprovalRequest.ToolName should be %q, got %q",
				toolName, decision.Approval.ToolName)
		}
	})
}

// TestProperty24_RawShellRequiresApproval verifies that for any raw shell
// tool call without whitelist authorization, the gateway requires approval.
func TestProperty24_RawShellRequiresApproval(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genRawShellTool().Draw(t, "rawShellTool")
		command := rapid.SampledFrom([]string{
			"ls -la", "df -h", "cat /etc/hosts", "systemctl status nginx",
		}).Draw(t, "command")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		gateway := &GatewayPolicy{Whitelist: NewWhitelistManager()}
		decision := gateway.CheckApproval(toolName, command, hostID, time.Now())

		if decision.Action != PolicyActionNeedApproval {
			t.Fatalf("raw shell tool %q with command %q should require approval, got %s",
				toolName, command, decision.Action)
		}
	})
}

// TestProperty24_StructuredReadNoApproval verifies that for any structured
// read tool call, the gateway allows without approval.
func TestProperty24_StructuredReadNoApproval(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genStructuredReadTool().Draw(t, "structuredReadTool")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		gateway := &GatewayPolicy{Whitelist: NewWhitelistManager()}
		decision := gateway.CheckApproval(toolName, "", hostID, time.Now())

		if decision.Action != PolicyActionAllow {
			t.Fatalf("structured read tool %q should be allowed without approval, got %s",
				toolName, decision.Action)
		}
	})
}

// TestProperty24_WhitelistBypassesApproval verifies that when a tool/host
// combination is whitelisted, approval is bypassed.
func TestProperty24_WhitelistBypassesApproval(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := genControlledMutationTool().Draw(t, "controlledMutationTool")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		wm := NewWhitelistManager()
		now := time.Now()
		_ = wm.Create(WhitelistEntry{
			ID:        "wl-1",
			HostID:    hostID,
			ToolName:  toolName,
			TTL:       time.Hour,
			CreatedAt: now,
		})

		gateway := &GatewayPolicy{Whitelist: wm}
		decision := gateway.CheckApproval(toolName, "", hostID, now)

		if decision.Action != PolicyActionAllow {
			t.Fatalf("whitelisted tool %q on host %q should be allowed, got %s",
				toolName, hostID, decision.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 25: 高风险命令 TTL 强制
// **Validates: Requirements 8.5**
// ---------------------------------------------------------------------------

// genHighRiskCommand generates a command matching high-risk patterns.
func genHighRiskCommand() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{
		"rm -rf /",
		"rm -rf /*",
		"sudo su",
		"sudo -i",
		"iptables -F",
		"iptables --flush",
		"dd if=/dev/zero of=/dev/sda",
		"mkfs.ext4 /dev/sda1",
		"shutdown -h now",
		"reboot",
		"kill -9 1",
		"chmod -R 777 /",
	})
}

// TestProperty25_HighRiskCommandRejectsNoTTL verifies that for any high-risk
// command, creating a whitelist entry without TTL is rejected.
func TestProperty25_HighRiskCommandRejectsNoTTL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		command := genHighRiskCommand().Draw(t, "highRiskCommand")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		wm := NewWhitelistManager()
		err := wm.Create(WhitelistEntry{
			ID:        "wl-test",
			HostID:    hostID,
			ToolName:  "command_exec",
			Command:   command,
			TTL:       0, // No TTL — should be rejected
			CreatedAt: time.Now(),
		})

		if err == nil {
			t.Fatalf("high-risk command %q should be rejected without TTL", command)
		}
		if _, ok := err.(*HighRiskNoTTLError); !ok {
			t.Fatalf("expected HighRiskNoTTLError, got %T: %v", err, err)
		}
	})
}

// TestProperty25_HighRiskCommandAcceptsWithTTL verifies that for any high-risk
// command, creating a whitelist entry with a TTL succeeds.
func TestProperty25_HighRiskCommandAcceptsWithTTL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		command := genHighRiskCommand().Draw(t, "highRiskCommand")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")
		ttlMinutes := rapid.IntRange(1, 60).Draw(t, "ttlMinutes")

		wm := NewWhitelistManager()
		err := wm.Create(WhitelistEntry{
			ID:        "wl-test",
			HostID:    hostID,
			ToolName:  "command_exec",
			Command:   command,
			TTL:       time.Duration(ttlMinutes) * time.Minute,
			CreatedAt: time.Now(),
		})

		if err != nil {
			t.Fatalf("high-risk command %q with TTL=%dm should be accepted, got error: %v",
				command, ttlMinutes, err)
		}
	})
}

// TestProperty25_TTLExpirationEnforced verifies that whitelist entries with TTL
// become inactive after expiration.
func TestProperty25_TTLExpirationEnforced(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")
		ttlMinutes := rapid.IntRange(1, 60).Draw(t, "ttlMinutes")
		toolName := genControlledMutationTool().Draw(t, "tool")

		wm := NewWhitelistManager()
		createdAt := time.Now().Add(-2 * time.Hour) // Created 2 hours ago
		ttl := time.Duration(ttlMinutes) * time.Minute

		_ = wm.Create(WhitelistEntry{
			ID:        "wl-expired",
			HostID:    hostID,
			ToolName:  toolName,
			TTL:       ttl,
			CreatedAt: createdAt,
		})

		// Check after TTL has expired
		checkTime := createdAt.Add(ttl + time.Second)
		authorized := wm.IsAuthorized(hostID, toolName, "", checkTime)

		if authorized {
			t.Fatalf("whitelist entry should be expired after TTL (created=%v, ttl=%v, check=%v)",
				createdAt, ttl, checkTime)
		}
	})
}

// TestProperty25_NonHighRiskAllowsNoTTL verifies that non-high-risk commands
// can be whitelisted without a TTL.
func TestProperty25_NonHighRiskAllowsNoTTL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		command := rapid.SampledFrom([]string{
			"ls -la", "df -h", "cat /etc/hosts", "systemctl status nginx",
			"docker ps", "kubectl get pods",
		}).Draw(t, "safeCommand")
		hostID := rapid.StringMatching(`host-[a-z0-9]{4}`).Draw(t, "hostID")

		wm := NewWhitelistManager()
		err := wm.Create(WhitelistEntry{
			ID:        "wl-safe",
			HostID:    hostID,
			ToolName:  "command_exec",
			Command:   command,
			TTL:       0, // No TTL — should be allowed for safe commands
			CreatedAt: time.Now(),
		})

		if err != nil {
			t.Fatalf("non-high-risk command %q should be allowed without TTL, got error: %v",
				command, err)
		}
	})
}
