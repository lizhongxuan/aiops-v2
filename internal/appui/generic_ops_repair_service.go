package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/opsrepair"
	"aiops-v2/internal/runtimekernel"
)

func (s *defaultChatService) handleGenericOpsRepair(ctx context.Context, _ ChatCommand, req runtimekernel.TurnRequest) (TurnResponse, bool, error) {
	sessionStore, ok := s.sessions.(SessionStore)
	if !ok {
		return TurnResponse{}, false, nil
	}
	if !genericOpsRepairDraftOnly(req.Metadata) {
		return TurnResponse{}, false, nil
	}
	frame := opsmanual.BuildOperationFrame(req.Input, nil)
	if !shouldHandleGenericOpsRepair(req.Input, frame) {
		return TurnResponse{}, false, nil
	}
	search, err := opsmanual.SearchOpsManuals(opsmanual.NewMemoryStore(), opsmanual.SearchOpsManualsRequest{Text: req.Input, OperationFrame: frame})
	if err != nil {
		return TurnResponse{}, true, err
	}
	repairPlan, err := opsrepair.PlanStatefulRepair(ctx, opsrepair.PlanRequest{Frame: frame})
	if err != nil {
		return TurnResponse{}, true, err
	}
	final := genericOpsRepairFinalText(frame, search, repairPlan)
	now := time.Now().UTC()
	completedAt := now
	turn := runtimekernel.TurnSnapshot{
		ID:              req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionID:       req.SessionID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Lifecycle:       runtimekernel.TurnLifecycleCompleted,
		ResumeState:     runtimekernel.TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
		CompletedAt:     &completedAt,
		FinalOutput:     final,
		AgentItems:      genericOpsRepairTurnItems(req, frame, search, repairPlan, final, now),
	}
	writeGenericOpsRepairTurn(sessionStore, req, final, turn)
	appendGenericOpsRepairEvents(ctx, s.agentEvents, req, final)
	return TurnResponse{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "completed",
		Output:          final,
	}, true, nil
}

func genericOpsRepairDraftOnly(metadata map[string]string) bool {
	for _, key := range []string{
		"aiops.genericOpsRepairDraftOnly",
		"genericOpsRepairDraftOnly",
		"generic_ops_repair_draft_only",
	} {
		if strings.EqualFold(strings.TrimSpace(metadata[key]), "true") ||
			strings.TrimSpace(metadata[key]) == "1" {
			return true
		}
	}
	return false
}

func shouldHandleGenericOpsRepair(input string, frame opsmanual.OperationFrame) bool {
	if strings.Contains(strings.ToLower(input), "workflow") || strings.Contains(input, "工作流") {
		return false
	}
	if strings.TrimSpace(frame.Target.Type) == "" || !frame.Operation.Stateful {
		return false
	}
	if len(frame.Roles) == 0 && len(frame.ObservationPoints) == 0 {
		return false
	}
	switch strings.TrimSpace(frame.Operation.Action) {
	case "rca_or_repair", "restore", "repair", "recover":
		return true
	default:
		return frame.Risk.DataMutation || frame.RiskPreference.DataLossAcceptable
	}
}

func genericOpsRepairTurnItems(req runtimekernel.TurnRequest, frame opsmanual.OperationFrame, search opsmanual.SearchOpsManualsResult, plan *opsrepair.RepairPlan, final string, now time.Time) []agentstate.TurnItem {
	items := []agentstate.TurnItem{
		genericOpsRepairUserItem(req, now),
		genericOpsRepairModelItem(req, frame, plan, now),
	}
	items = append(items, genericOpsRepairToolItems(req, "search_ops_manuals", "检索通用运维手册与 capability fallback", genericOpsRepairSearchPreview(search), now)...)
	items = append(items, genericOpsRepairToolItems(req, "run_ops_manual_preflight", "生成只读预检计划，未执行破坏性动作", genericOpsRepairPreflightPreview(frame, search), now)...)
	items = append(items, genericOpsRepairToolItems(req, "host_command", "规划主机只读探测命令，由 host-bound agent 执行", genericOpsRepairHostCommandPreview(frame), now)...)
	items = append(items,
		genericOpsRepairEvidenceItem(req, frame, search, now),
		genericOpsRepairPlanItem(req, plan, now),
		genericOpsRepairFinalItem(req, final, now),
	)
	return items
}

func genericOpsRepairUserItem(req runtimekernel.TurnRequest, now time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:     req.TurnID + "-user",
		Type:   agentstate.TurnItemTypeUserMessage,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "turn",
			Summary: req.Input,
			Data:    mustJSON(map[string]any{"prompt": req.Input, "summary": req.Input}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genericOpsRepairModelItem(req runtimekernel.TurnRequest, frame opsmanual.OperationFrame, plan *opsrepair.RepairPlan, now time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:     req.TurnID + "-model-call",
		Type:   agentstate.TurnItemTypeModelCall,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "system",
			Summary: "生成通用有状态集群恢复计划",
			Data: mustJSON(map[string]any{
				"capabilityPath": plan.Capability,
				"resourceRoles":  genericOpsRepairRoleSignals(frame),
				"genericOpsContract": []string{
					"read_only_evidence_first",
					"approval_before_mutation",
				},
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genericOpsRepairToolItems(req runtimekernel.TurnRequest, toolName, summary string, preview map[string]any, now time.Time) []agentstate.TurnItem {
	toolCallID := req.TurnID + "-" + strings.ReplaceAll(toolName, "_", "-")
	payload := transportToolPayload{
		ID:            toolCallID,
		ToolCallID:    toolCallID,
		ToolName:      toolName,
		Name:          toolName,
		DisplayKind:   "generic_ops_repair_evidence",
		InputSummary:  summary,
		OutputSummary: summary,
		OutputPreview: mustJSON(preview),
		Mock:          true,
	}
	return []agentstate.TurnItem{
		{
			ID:     toolCallID + "-call",
			Type:   agentstate.TurnItemTypeToolCall,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "tool",
				Summary: toolName,
				Data:    mustJSON(payload),
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:     toolCallID,
			Type:   agentstate.TurnItemTypeToolResult,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "tool",
				Summary: toolName,
				Data:    mustJSON(payload),
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

func genericOpsRepairEvidenceItem(req runtimekernel.TurnRequest, frame opsmanual.OperationFrame, search opsmanual.SearchOpsManualsResult, now time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:     req.TurnID + "-evidence",
		Type:   agentstate.TurnItemTypeEvidence,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "generic_ops_repair_evidence",
			Summary: "只读证据优先，当前为预检证据需求与限制",
			Data: mustJSON(map[string]any{
				"capabilityPath":       "stateful_middleware_cluster_repair",
				"resourceRoles":        genericOpsRepairRoleSignals(frame),
				"genericOpsContract":   []string{"read_only_evidence_first", "approval_before_mutation"},
				"evidenceRequirements": frame.EvidenceRequirements,
				"evidenceLimitations": []string{
					"当前仅完成通用预检计划，真实主机证据需要 host-bound agent 执行只读探测。",
					"数据可丢失只是风险偏好，不等于审批通过。",
				},
				"searchDecision": search.Decision,
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genericOpsRepairPlanItem(req runtimekernel.TurnRequest, plan *opsrepair.RepairPlan, now time.Time) agentstate.TurnItem {
	var steps []map[string]string
	if plan != nil && len(plan.Options) > 0 {
		for _, step := range plan.Options[0].Steps {
			steps = append(steps, map[string]string{
				"id":      step.ID,
				"text":    step.Phase,
				"status":  "waiting",
				"summary": step.ActionRef,
			})
		}
	}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-plan",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "plan",
			Summary: "通用有状态集群恢复方案",
			Data: mustJSON(map[string]any{
				"title": "通用有状态集群恢复方案",
				"steps": steps,
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genericOpsRepairFinalItem(req runtimekernel.TurnRequest, final string, now time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:     req.TurnID + "-final",
		Type:   agentstate.TurnItemTypeAssistantMessage,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "assistant_message",
			Summary: final,
			Data: mustJSON(map[string]any{
				"displayKind":    "assistant.message",
				"phase":          "final_answer",
				"streamState":    "complete",
				"boundaryAction": "allow",
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func genericOpsRepairSearchPreview(search opsmanual.SearchOpsManualsResult) map[string]any {
	var manuals []string
	for _, hit := range search.Manuals {
		manuals = append(manuals, hit.Manual.ID)
	}
	return map[string]any{
		"decision":          search.Decision,
		"capabilityPath":    "stateful_middleware_cluster_repair",
		"manuals":           manuals,
		"recommendedAction": search.RecommendedNextAction,
	}
}

func genericOpsRepairPreflightPreview(frame opsmanual.OperationFrame, search opsmanual.SearchOpsManualsResult) map[string]any {
	outputs := append([]string(nil), frame.EvidenceRequirements...)
	if len(outputs) == 0 && len(search.Manuals) > 0 {
		outputs = append(outputs, search.Manuals[0].Manual.PreflightProbe.RequiredOutputs...)
	}
	return map[string]any{
		"status":          "planned",
		"readOnly":        true,
		"requiredOutputs": outputs,
		"resourceRoles":   genericOpsRepairRoleSignals(frame),
	}
}

func genericOpsRepairHostCommandPreview(frame opsmanual.OperationFrame) map[string]any {
	return map[string]any{
		"status":    "planned",
		"readOnly":  true,
		"resources": frame.ExecutionSurfaceV2.Resources,
		"commands": []string{
			"collect member_health",
			"collect storage_health",
			"collect sync_status",
			"collect observer_health",
		},
	}
}

func genericOpsRepairFinalText(frame opsmanual.OperationFrame, search opsmanual.SearchOpsManualsResult, plan *opsrepair.RepairPlan) string {
	target := firstNonEmptyString(frame.Target.Type, "有状态中间件")
	var b strings.Builder
	b.WriteString("已进入通用有状态集群恢复流程，当前只生成诊断与恢复方案，尚未执行破坏性动作。\n\n")
	b.WriteString("- capability_path：stateful_middleware_cluster_repair\n")
	b.WriteString("- generic_ops_contract：read_only_evidence_first, approval_before_mutation\n")
	if roles := genericOpsRepairRoleSignals(frame); len(roles) > 0 {
		b.WriteString("- resource_roles：" + strings.Join(roles, ", ") + "\n")
	}
	b.WriteString("\n**诊断**\n")
	b.WriteString("- 目标：" + target)
	b.WriteString(" 集群异常。\n")
	b.WriteString("- 已识别数据节点和 monitor/observer 角色；monitor 仅作为观察点，不作为数据节点。\n")
	b.WriteString("- 证据限制：当前只有用户输入和通用预检计划，真实主机状态、同步状态、存储空间和 observer 健康需要 host-bound agent 执行只读证据采集。\n")
	b.WriteString("\n**恢复方案**\n")
	if plan != nil && len(plan.Options) > 0 {
		for _, option := range plan.Options {
			b.WriteString("- " + option.Title + "：先运行只读 preflight，再在审批后执行受治理修复；")
			if option.DataLoss {
				b.WriteString("用户允许数据可丢失会作为风险偏好记录，但不跳过审批。")
			} else {
				b.WriteString("未确认数据可丢失时优先保守恢复。")
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("- 先收集 member_health、storage_health、sync_status、observer_health，再选择重建异常成员或人工接管。\n")
	}
	b.WriteString("\n**只读证据**\n")
	b.WriteString("- search_ops_manuals：" + string(search.Decision) + "\n")
	b.WriteString("- run_ops_manual_preflight：planned，只读采集 resource_roles、member_health、storage_health、sync_status、observer_health。\n")
	b.WriteString("- host_command：planned，每台主机由独立 host-bound agent 执行只读探测。\n")
	b.WriteString("\n**验证**\n")
	b.WriteString("- 恢复后必须独立验证成员健康、同步状态和 observer 健康，验证通过前不能声明恢复完成。\n")
	return b.String()
}

func genericOpsRepairRoleSignals(frame opsmanual.OperationFrame) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, role := range frame.Roles {
		signal := string(role.Kind)
		ref := firstNonEmptyString(role.UserLabel, role.ResourceRef, role.ID)
		if ref != "" {
			signal += ":" + ref
		}
		if role.RuntimeName != "" {
			signal += ":" + role.RuntimeName
		}
		if _, ok := seen[signal]; ok {
			continue
		}
		seen[signal] = struct{}{}
		out = append(out, signal)
	}
	return out
}

func writeGenericOpsRepairTurn(store SessionStore, req runtimekernel.TurnRequest, assistantText string, turn runtimekernel.TurnSnapshot) {
	session := store.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
	if session.HostID == "" {
		session.HostID = req.HostID
	}
	now := time.Now().UTC()
	if session.CurrentTurn != nil {
		session.TurnHistory = append(session.TurnHistory, *session.CurrentTurn)
	}
	session.Messages = append(session.Messages,
		runtimekernel.Message{
			ID:              firstNonEmptyString(req.ClientMessageID, req.TurnID+":user"),
			ClientMessageID: req.ClientMessageID,
			ClientTurnID:    req.ClientTurnID,
			Role:            "user",
			Content:         req.Input,
			Timestamp:       now,
		},
		runtimekernel.Message{
			ID:           req.TurnID + ":assistant",
			ClientTurnID: req.ClientTurnID,
			Role:         "assistant",
			Content:      assistantText,
			Timestamp:    now,
		},
	)
	session.CurrentTurn = &turn
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	store.Update(session)
}

func appendGenericOpsRepairEvents(ctx context.Context, events AgentEventService, req runtimekernel.TurnRequest, summary string) {
	if events == nil {
		return
	}
	payload, _ := json.Marshal(TurnPayload{Prompt: req.Input, Title: "通用有状态集群恢复", Summary: summary})
	_, _ = events.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.generic_ops_repair.completed", req.TurnID),
		SessionID:    req.SessionID,
		TurnID:       req.TurnID,
		ClientTurnID: req.ClientTurnID,
		Kind:         AgentEventTurn,
		Phase:        AgentEventPhaseCompleted,
		Status:       AgentEventStatusCompleted,
		Visibility:   AgentEventVisibilityPrimary,
		Source:       AgentEventSourceSystem,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Payload:      payload,
	})
	appendMainAgentEvent(ctx, events, req, AgentEventPhaseCompleted, AgentEventStatusCompleted, "", "通用有状态集群恢复计划已生成")
}
