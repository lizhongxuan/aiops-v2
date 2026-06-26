package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/workflowgen"
)

type WorkflowGenerationChatService struct {
	sessionStore SessionStore
	store        workflowgen.SessionStore
	builder      workflowgen.PlanBuilder
	generator    workflowgen.GraphGenerator
	agentEvents  AgentEventService

	mu                   sync.Mutex
	activeByConversation map[string]string
}

func NewWorkflowGenerationChatService(
	sessionStore SessionStore,
	store workflowgen.SessionStore,
	builder workflowgen.PlanBuilder,
	generator workflowgen.GraphGenerator,
	agentEvents AgentEventService,
) *WorkflowGenerationChatService {
	if store == nil {
		store = workflowgen.NewMemorySessionStore()
	}
	if builder == nil {
		builder = workflowgen.DeterministicPlanBuilder{}
	}
	if generator == nil {
		generator = workflowgen.RunnerGraphGenerator{}
	}
	return &WorkflowGenerationChatService{
		sessionStore:         sessionStore,
		store:                store,
		builder:              builder,
		generator:            generator,
		agentEvents:          agentEvents,
		activeByConversation: map[string]string{},
	}
}

func (s *WorkflowGenerationChatService) Handle(ctx context.Context, cmd ChatCommand, req runtimekernel.TurnRequest) (TurnResponse, bool, error) {
	if s == nil || s.sessionStore == nil || s.store == nil || s.builder == nil {
		return TurnResponse{}, false, nil
	}
	content := strings.TrimSpace(req.Input)
	if s.isGenerateWorkflowConfirmation(req.SessionID, cmd, content) {
		response, handled, err := s.handleGenerateConfirmation(ctx, req, content)
		return response, handled, err
	}
	requirement, isNew := parseAddWorkflowMention(content)
	if !isNew && s.activeSession(req.SessionID) == "" {
		requirement, isNew = parsePlainWorkflowWritingRequest(content)
	}
	if !isNew && !s.shouldRevisePlan(req.SessionID, content) {
		return TurnResponse{}, false, nil
	}

	var (
		session *workflowgen.WorkflowGenerationSession
		plan    *workflowgen.WorkflowGenerationPlan
		err     error
	)
	if isNew {
		plan, err = s.buildInitialWorkflowPlan(ctx, requirement)
		if err != nil {
			return TurnResponse{}, true, err
		}
		session, err = s.store.Create(ctx, workflowgen.WorkflowGenerationSession{
			ConversationID:      req.SessionID,
			UserID:              "local-user",
			Status:              statusForPlan(plan),
			Requirement:         requirement,
			PlanVersion:         plan.Version,
			Plan:                plan,
			ValidationProvider:  plan.ValidationStrategy.Provider,
			CreatedByUserPrompt: true,
		})
		if err != nil {
			return TurnResponse{}, true, err
		}
		s.setActiveSession(req.SessionID, session.ID)
		_, _ = s.store.AppendEvent(ctx, session.ID, workflowgen.WorkflowGenerationEvent{Type: workflowgen.EventPlanStarted, Status: string(workflowgen.SessionStatusPlanStarted), Message: "开始生成工作流计划"})
		_, _ = s.store.AppendEvent(ctx, session.ID, workflowgen.WorkflowGenerationEvent{Type: workflowgen.EventPlanReady, Status: string(session.Status), Message: "工作流计划已生成"})
	} else {
		activeID := s.activeSession(req.SessionID)
		session, err = s.store.Get(ctx, activeID)
		if err != nil {
			return TurnResponse{}, true, err
		}
		if session.Plan == nil {
			return TurnResponse{}, true, fmt.Errorf("workflow generation session %s has no plan", activeID)
		}
		plan, err = s.builder.RevisePlan(ctx, workflowgen.RevisePlanRequest{Previous: *session.Plan, Message: content})
		if err != nil {
			return TurnResponse{}, true, err
		}
		session, err = s.store.Update(ctx, activeID, func(next *workflowgen.WorkflowGenerationSession) error {
			next.Status = statusForPlan(plan)
			next.Plan = plan
			next.PlanVersion = plan.Version
			next.ValidationProvider = plan.ValidationStrategy.Provider
			return nil
		})
		if err != nil {
			return TurnResponse{}, true, err
		}
		_, _ = s.store.AppendEvent(ctx, session.ID, workflowgen.WorkflowGenerationEvent{Type: workflowgen.EventPlanReady, Status: string(session.Status), Message: "工作流计划已更新"})
	}

	final := workflowPlanFinalText(session, plan)
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
		AgentItems: []agentstate.TurnItem{
			workflowGenerationUserItem(req, content, now),
			workflowGenerationModelCallItem(req, plan, now),
			workflowGenerationPlanItem(req, plan, now),
			workflowGenerationEvidenceItem(req, plan, now),
			workflowGenerationArtifactItem(req, session, plan, now, false, nil),
			workflowGenerationFinalItem(req, final, now),
		},
	}
	s.writeTurn(req, content, final, turn)
	s.appendAgentEvents(req, final)
	return TurnResponse{
		SessionID:       req.SessionID,
		TurnID:          req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "completed",
		Output:          final,
	}, true, nil
}

func (s *WorkflowGenerationChatService) buildInitialWorkflowPlan(ctx context.Context, requirement string) (*workflowgen.WorkflowGenerationPlan, error) {
	frame := opsmanual.BuildOperationFrame(requirement, nil)
	if shouldUseResourceWorkflowPlan(requirement, frame) {
		return workflowgen.ResourcePlanBuilder{}.BuildResourcePlan(ctx, workflowgen.BuildResourcePlanRequest{
			Requirement:    requirement,
			OperationFrame: frame,
		})
	}
	return s.builder.BuildPlan(ctx, workflowgen.BuildPlanRequest{Requirement: requirement})
}

func (s *WorkflowGenerationChatService) handleGenerateConfirmation(ctx context.Context, req runtimekernel.TurnRequest, content string) (TurnResponse, bool, error) {
	activeID := s.activeSession(req.SessionID)
	if activeID == "" {
		return TurnResponse{}, false, nil
	}
	session, err := s.store.Get(ctx, activeID)
	if err != nil {
		return TurnResponse{}, true, err
	}
	if session.Plan == nil {
		return TurnResponse{}, true, fmt.Errorf("workflow generation session %s has no plan", activeID)
	}
	agent := workflowgen.WorkflowBuilderAgent{
		Store:          s.store,
		GraphGenerator: s.generator,
		Validator:      workflowGenerationValidationProvider(session.Plan, req.Metadata),
	}
	result, err := agent.GenerateDraft(ctx, workflowgen.GenerateDraftRequest{SessionID: session.ID})
	if err != nil {
		return TurnResponse{}, true, err
	}
	session, err = s.store.Update(ctx, session.ID, func(next *workflowgen.WorkflowGenerationSession) error {
		next.DraftWorkflowID = result.Graph.Workflow.Name
		return nil
	})
	if err != nil {
		return TurnResponse{}, true, err
	}

	final := workflowGenerationGeneratedFinalText(result)
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
		AgentItems: []agentstate.TurnItem{
			workflowGenerationUserItem(req, content, now),
			workflowGenerationArtifactItem(req, session, session.Plan, now, true, result),
			workflowGenerationFinalItem(req, final, now),
		},
	}
	s.writeTurn(req, content, final, turn)
	s.appendAgentEvents(req, final)
	return TurnResponse{SessionID: req.SessionID, TurnID: req.TurnID, ClientTurnID: req.ClientTurnID, ClientMessageID: req.ClientMessageID, Status: "completed", Output: final}, true, nil
}

func workflowGenerationValidationProvider(_ *workflowgen.WorkflowGenerationPlan, metadata map[string]string) workflowgen.WorkflowValidationProvider {
	override := strings.ToLower(strings.TrimSpace(os.Getenv("AIOPS_WORKFLOW_VALIDATION_PROVIDER")))
	switch override {
	case "docker":
		return workflowgen.DockerValidator{Image: workflowGenerationValidationImages(metadata)[0]}
	}
	return workflowgen.StaticValidationProvider{}
}

func workflowGenerationValidationImages(metadata ...map[string]string) []string {
	for _, item := range metadata {
		if image := strings.TrimSpace(item["workflowValidationImage"]); image != "" {
			return []string{image}
		}
		if image := strings.TrimSpace(item["workflow_validation_image"]); image != "" {
			return []string{image}
		}
	}
	if image := strings.TrimSpace(os.Getenv("AIOPS_WORKFLOW_VALIDATION_IMAGE")); image != "" {
		return []string{image}
	}
	return []string{"python:3.12-slim"}
}

func workflowGenerationGeneratedFinalText(result *workflowgen.GenerateDraftResult) string {
	if result == nil {
		return "Runner Workflow 草稿生成流程已结束。"
	}
	status := "未验证"
	summary := ""
	if result.Validation != nil {
		status = result.Validation.Status
		summary = result.Validation.Summary
		if result.Validation.Provider == workflowgen.ValidationProviderNone && result.Validation.Status == "passed" {
			summary = "静态验证通过。"
		}
	}
	if summary == "" {
		summary = "验证结果未返回摘要。"
	}
	return fmt.Sprintf(
		"Runner Workflow 草稿已生成。\n\n- 草稿 ID：`%s`\n- 节点数：%d\n- 边数：%d\n- 验证状态：`%s`\n- 验证摘要：%s\n\n验证由后端受控 Provider 执行，不会把 Docker 原始命令或宿主机权限暴露给 LLM。",
		result.Graph.Workflow.Name,
		len(result.Graph.Nodes),
		len(result.Graph.Edges),
		status,
		summary,
	)
}

func (s *WorkflowGenerationChatService) writeTurn(req runtimekernel.TurnRequest, userText, assistantText string, turn runtimekernel.TurnSnapshot) {
	session := s.sessionStore.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
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
			Content:         userText,
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
	s.sessionStore.Update(session)
}

func (s *WorkflowGenerationChatService) appendAgentEvents(req runtimekernel.TurnRequest, summary string) {
	if s == nil || s.agentEvents == nil {
		return
	}
	ctx := context.Background()
	payload, _ := json.Marshal(TurnPayload{Prompt: req.Input, Title: req.Input, Summary: summary})
	_, _ = s.agentEvents.Append(ctx, AgentEvent{
		EventID:      fmt.Sprintf("%s:turn.workflow_generation.completed", req.TurnID),
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
	appendMainAgentEvent(ctx, s.agentEvents, req, AgentEventPhaseCompleted, AgentEventStatusCompleted, "", "工作流计划已生成")
}

func parseAddWorkflowMention(content string) (string, bool) {
	value := strings.TrimSpace(content)
	for _, mention := range []string{"@add_workflow", "＠add_workflow"} {
		if strings.HasPrefix(strings.ToLower(value), mention) {
			requirement := strings.TrimSpace(value[len(mention):])
			return requirement, requirement != ""
		}
	}
	return "", false
}

func parsePlainWorkflowWritingRequest(content string) (string, bool) {
	value := strings.TrimSpace(content)
	if value == "" {
		return "", false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(value, "确认生成工作流候选") || strings.HasPrefix(lower, "confirm workflow generation") {
		return "", false
	}
	if !containsAnyWorkflowKeyword(lower, []string{"workflow", "工作流"}) {
		return "", false
	}
	if !containsAnyWorkflowKeyword(lower, []string{
		"写", "生成", "创建", "新建", "设计", "编排", "搭建",
		"write", "generate", "create", "build", "design",
	}) {
		return "", false
	}
	return value, true
}

func containsAnyWorkflowKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(strings.ToLower(keyword))
		if keyword != "" && strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func (s *WorkflowGenerationChatService) shouldRevisePlan(conversationID, content string) bool {
	if strings.TrimSpace(content) == "" || s.activeSession(conversationID) == "" {
		return false
	}
	frame := opsmanual.BuildOperationFrame(content, nil)
	if shouldHandleGenericOpsRepair(content, frame) {
		return false
	}
	keywords := []string{"改成", "不要", "直接返回", "飞书", "邮件", "webhook", "调度", "每天", "手动", "推送"}
	for _, keyword := range keywords {
		if strings.Contains(strings.ToLower(content), strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (s *WorkflowGenerationChatService) isGenerateWorkflowConfirmation(sessionID string, cmd ChatCommand, content string) bool {
	action := strings.TrimSpace(cmd.Metadata["opsManualAction"])
	if action == "generate_runner_workflow_candidate" || action == "generate_workflow" {
		return s.activeSession(sessionID) != ""
	}
	return strings.HasPrefix(strings.TrimSpace(content), "确认生成工作流候选") && s.activeSession(sessionID) != ""
}

func (s *WorkflowGenerationChatService) setActiveSession(conversationID, workflowSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeByConversation[strings.TrimSpace(conversationID)] = strings.TrimSpace(workflowSessionID)
}

func (s *WorkflowGenerationChatService) activeSession(conversationID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeByConversation[strings.TrimSpace(conversationID)]
}

func statusForPlan(plan *workflowgen.WorkflowGenerationPlan) workflowgen.SessionStatus {
	if plan != nil && len(plan.RequiredSlots) > 0 {
		return workflowgen.SessionStatusSlotRequired
	}
	return workflowgen.SessionStatusPlanReady
}

func workflowGenerationUserItem(req runtimekernel.TurnRequest, content string, now time.Time) agentstate.TurnItem {
	payload := map[string]string{"prompt": content}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-user",
		Type:   agentstate.TurnItemTypeUserMessage,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Summary: content,
			Data:    mustJSON(payload),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowGenerationModelCallItem(req runtimekernel.TurnRequest, plan *workflowgen.WorkflowGenerationPlan, now time.Time) agentstate.TurnItem {
	return agentstate.TurnItem{
		ID:     req.TurnID + "-model-call",
		Type:   agentstate.TurnItemTypeModelCall,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Summary: "生成资源型 workflow 计划并检查通用 contract 信号",
			Data: mustJSON(map[string]any{
				"capabilityPath":     plan.Intent,
				"reviewStatus":       plan.ReviewStatus,
				"genericOpsContract": workflowGenerationGenericOpsContracts(plan),
			}),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowGenerationPlanItem(req runtimekernel.TurnRequest, plan *workflowgen.WorkflowGenerationPlan, now time.Time) agentstate.TurnItem {
	steps := make([]map[string]string, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		steps = append(steps, map[string]string{
			"id":      node.ID,
			"text":    node.Title,
			"status":  "waiting",
			"summary": node.Description,
		})
	}
	payload := map[string]any{
		"title":              plan.Title,
		"steps":              steps,
		"capabilityPath":     plan.Intent,
		"reviewStatus":       plan.ReviewStatus,
		"resourceRoles":      workflowGenerationResourceRoleSignals(plan),
		"genericOpsContract": workflowGenerationGenericOpsContracts(plan),
	}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-plan",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "workflow_generation_plan",
			Summary: plan.Title,
			Data:    mustJSON(payload),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowGenerationEvidenceItem(req runtimekernel.TurnRequest, plan *workflowgen.WorkflowGenerationPlan, now time.Time) agentstate.TurnItem {
	data := map[string]any{
		"evidenceKind":        "workflow_generation_preflight_requirements",
		"capabilityPath":      plan.Intent,
		"reviewStatus":        plan.ReviewStatus,
		"resourceRoles":       workflowGenerationResourceRoleSignals(plan),
		"genericOpsContract":  workflowGenerationGenericOpsContracts(plan),
		"evidenceLimitations": workflowGenerationEvidenceLimitations(plan),
	}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-workflow-generation-evidence",
		Type:   agentstate.TurnItemTypeEvidence,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "workflow_generation_evidence",
			Summary: "workflow generation evidence: draft_until_reviewed secret_ref_only pending_review",
			Data:    mustJSON(data),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowGenerationArtifactItem(req runtimekernel.TurnRequest, session *workflowgen.WorkflowGenerationSession, plan *workflowgen.WorkflowGenerationPlan, now time.Time, generated bool, result *workflowgen.GenerateDraftResult) agentstate.TurnItem {
	preview := workflowGenerationArtifactPayload(session, plan, generated, result)
	payload := transportToolPayload{
		ToolCallID:    "workflow-generation-" + session.ID,
		ToolName:      "workflow_generation.plan",
		DisplayKind:   "runner_workflow_generation",
		InputSummary:  session.Requirement,
		OutputSummary: "初始生成大纲已生成，等待用户确认生成草稿。",
		OutputPreview: mustJSON(preview),
	}
	return agentstate.TurnItem{
		ID:     req.TurnID + "-workflow-generation-artifact",
		Type:   agentstate.TurnItemTypeToolResult,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "tool",
			Summary: payload.OutputSummary,
			Data:    mustJSON(payload),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func workflowGenerationFinalItem(req runtimekernel.TurnRequest, final string, now time.Time) agentstate.TurnItem {
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

func workflowGenerationArtifactPayload(session *workflowgen.WorkflowGenerationSession, plan *workflowgen.WorkflowGenerationPlan, generated bool, result *workflowgen.GenerateDraftResult) map[string]any {
	steps := make([]map[string]any, 0, len(plan.Nodes)+2)
	generatedNodeDetails := workflowGenerationGeneratedNodeDetails(result)
	validationNodeResults := workflowGenerationNodeValidationResults(result)
	for _, node := range plan.Nodes {
		status := "planned"
		if generated {
			status = "passed"
		}
		step := map[string]any{
			"id":          node.ID,
			"title":       node.Title,
			"summary":     node.Description,
			"status":      status,
			"kind":        string(node.Kind),
			"action":      node.Action,
			"description": node.Description,
		}
		for key, value := range generatedNodeDetails[sanitizeWorkflowGenerationStepID(node.ID)] {
			step[key] = value
		}
		if validation, ok := validationNodeResults[sanitizeWorkflowGenerationStepID(node.ID)]; ok {
			if validation.Status != "" {
				step["status"] = validation.Status
			}
			step["validationStatus"] = validation.Status
			step["validationSummary"] = validation.Summary
			step["validationExitCode"] = validation.ExitCode
			step["validationStdout"] = limitWorkflowGenerationString(validation.StdoutSummary, 2000)
			step["validationStderr"] = limitWorkflowGenerationString(validation.StderrSummary, 2000)
			step["validationDurationMs"] = validation.DurationMs
		} else if generated && result != nil && result.Validation != nil && result.Validation.Provider == workflowgen.ValidationProviderNone {
			step["validationStatus"] = "not_run"
			step["validationSummary"] = "当前使用静态验证 Provider，只校验 Runner 图结构；未启动容器执行节点脚本。"
		} else if generated && result != nil && result.Validation != nil && result.Validation.Provider == workflowgen.ValidationProviderDocker && result.Validation.Status == "failed" {
			step["status"] = "skipped"
			step["validationStatus"] = "skipped"
			step["validationSummary"] = "Docker 验证入口失败，节点脚本未执行；请查看 Docker 验证详情。"
		}
		steps = append(steps, step)
	}
	validationStatus := "waiting"
	validationSummary := "用户确认生成后，在受控 Provider 中进行 mock 验证。"
	validationProvider := plan.ValidationStrategy.Provider
	if generated {
		validationStatus = "passed"
		validationSummary = "静态验证已通过；如开启容器验证配置，会由后端受控 Docker Provider 执行。"
		if len(session.ValidationRuns) > 0 {
			last := session.ValidationRuns[len(session.ValidationRuns)-1]
			validationProvider = last.Provider
			validationStatus = last.Status
			if last.Summary != "" {
				validationSummary = last.Summary
			}
			if last.Provider == workflowgen.ValidationProviderNone && last.Status == "passed" {
				validationSummary = "静态验证通过。"
			}
		}
	}
	validationTitle := "Docker 验证"
	if generated && validationProvider == workflowgen.ValidationProviderNone {
		validationTitle = "静态验证"
	}
	steps = append(steps, map[string]any{"id": "docker-validation", "title": validationTitle, "status": validationStatus, "summary": validationSummary})
	outputs := make([]map[string]any, 0, len(plan.Outputs))
	for _, output := range plan.Outputs {
		outputs = append(outputs, map[string]any{"id": output.ID, "target": string(output.Target), "description": output.Description, "secretRef": output.SecretRef})
	}
	slots := make([]map[string]any, 0, len(plan.RequiredSlots))
	for _, slot := range plan.RequiredSlots {
		slots = append(slots, map[string]any{"id": slot.ID, "label": slot.Label, "question": slot.Question, "type": slot.Type, "required": slot.Required, "sensitive": slot.Sensitive, "options": slot.Options})
	}
	return map[string]any{
		"schemaVersion":       "aiops.runner_workflow_generation/v1",
		"workflowTitle":       plan.Title,
		"workflowId":          session.ID,
		"workflowSessionId":   session.ID,
		"status":              string(session.Status),
		"planVersion":         plan.Version,
		"capabilityPath":      plan.Intent,
		"reviewStatus":        plan.ReviewStatus,
		"resourceKind":        plan.ResourceKind,
		"resourceRoles":       workflowGenerationResourceRoleSignals(plan),
		"genericOpsContract":  workflowGenerationGenericOpsContracts(plan),
		"evidenceLimitations": workflowGenerationEvidenceLimitations(plan),
		"requirement":         session.Requirement,
		"trigger":             plan.Trigger,
		"outputs":             outputs,
		"steps":               steps,
		"requiredSlots":       slots,
		"planIsProvisional":   !generated,
		"validationProvider":  string(validationProvider),
		"validationScenario":  plan.ValidationStrategy.Scenario,
		"validationDetails":   workflowGenerationValidationDetails(plan, result, validationProvider, validationStatus, validationSummary),
		"generationAvailable": len(plan.RequiredSlots) == 0 && !generated,
		"draftWorkflowId":     session.DraftWorkflowID,
		"actions": []map[string]any{
			{"id": "generate_workflow", "label": "生成", "kind": "confirm"},
		},
	}
}

func workflowGenerationGeneratedNodeDetails(result *workflowgen.GenerateDraftResult) map[string]map[string]any {
	details := map[string]map[string]any{}
	if result == nil {
		return details
	}
	for _, node := range result.Graph.Nodes {
		if node.Step == nil {
			continue
		}
		action := strings.TrimSpace(node.Step.Action)
		script, _ := node.Step.Args["script"].(string)
		nodeID := sanitizeWorkflowGenerationStepID(firstNonEmptyString(node.ID, node.Step.ID, node.Step.Name))
		if nodeID == "" {
			continue
		}
		details[nodeID] = map[string]any{
			"action":              action,
			"scriptLanguage":      workflowGenerationScriptLanguage(action),
			"scriptLanguageLabel": workflowGenerationScriptLanguageLabel(action),
			"scriptPreview":       limitWorkflowGenerationString(script, 6000),
			"scriptTruncated":     len(script) > 6000,
		}
	}
	return details
}

func workflowGenerationNodeValidationResults(result *workflowgen.GenerateDraftResult) map[string]workflowgen.NodeValidationSummary {
	results := map[string]workflowgen.NodeValidationSummary{}
	if result == nil || result.Validation == nil {
		return results
	}
	for _, item := range result.Validation.NodeResults {
		nodeID := sanitizeWorkflowGenerationStepID(item.NodeID)
		if nodeID == "" {
			continue
		}
		results[nodeID] = item
	}
	return results
}

func workflowGenerationValidationDetails(plan *workflowgen.WorkflowGenerationPlan, result *workflowgen.GenerateDraftResult, provider workflowgen.ValidationProvider, status string, summary string) map[string]any {
	mode := "docker"
	if provider == workflowgen.ValidationProviderNone {
		mode = "static"
	}
	details := map[string]any{
		"provider":       string(provider),
		"mode":           mode,
		"status":         status,
		"summary":        summary,
		"scenario":       plan.ValidationStrategy.Scenario,
		"networkPolicy":  firstNonEmptyString(plan.ValidationStrategy.Network, "none"),
		"allowedImages":  workflowGenerationValidationImages(),
		"sandboxSummary": "Docker Provider 会创建临时工作区，把生成的节点脚本只读挂载到容器内，使用 CPU、内存和网络策略限制执行验证脚本。",
	}
	if result != nil && result.Validation != nil {
		details["validationId"] = result.Validation.ID
		details["durationMs"] = result.Validation.DurationMs
		if strings.TrimSpace(result.Validation.Image) != "" {
			details["selectedImage"] = strings.TrimSpace(result.Validation.Image)
			details["allowedImages"] = []string{strings.TrimSpace(result.Validation.Image)}
		}
		details["stdoutSummary"] = limitWorkflowGenerationString(result.Validation.StdoutSummary, 2000)
		details["stderrSummary"] = limitWorkflowGenerationString(result.Validation.StderrSummary, 2000)
		if result.Validation.SkippedReason != "" {
			details["skippedReason"] = result.Validation.SkippedReason
		}
		if len(result.Validation.NodeResults) > 0 {
			details["nodeResults"] = result.Validation.NodeResults
		}
	}
	return details
}

func workflowGenerationScriptLanguage(action string) string {
	switch strings.TrimSpace(action) {
	case "script.python":
		return "python"
	case "script.shell", "shell", "script.bash":
		return "shell"
	default:
		return "unknown"
	}
}

func workflowGenerationScriptLanguageLabel(action string) string {
	switch workflowGenerationScriptLanguage(action) {
	case "python":
		return "Python 脚本"
	case "shell":
		return "Shell 脚本"
	default:
		return firstNonEmptyString(strings.TrimSpace(action), "未知脚本")
	}
}

func sanitizeWorkflowGenerationStepID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}

func limitWorkflowGenerationString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "\n... truncated ..."
}

func workflowPlanFinalText(session *workflowgen.WorkflowGenerationSession, plan *workflowgen.WorkflowGenerationPlan) string {
	var b strings.Builder
	b.WriteString("已生成工作流计划，先不创建生产工作流。\n\n")
	if plan.Intent != "" {
		b.WriteString("- capability_path：" + plan.Intent + "\n")
	}
	if plan.ReviewStatus != "" {
		b.WriteString("- review_status：" + string(plan.ReviewStatus) + "\n")
	}
	if roles := workflowGenerationResourceRoleSignals(plan); len(roles) > 0 {
		b.WriteString("- resource_roles：" + strings.Join(roles, ", ") + "\n")
	}
	if contracts := workflowGenerationGenericOpsContracts(plan); len(contracts) > 0 {
		b.WriteString("- generic_ops_contract：" + strings.Join(contracts, ", ") + "\n")
	}
	b.WriteString("**工作流计划**\n")
	b.WriteString("- 名称：" + plan.Title + "\n")
	b.WriteString("- 触发：" + firstNonEmptyString(plan.Trigger.Summary, string(plan.Trigger.Type)) + "\n")
	if len(plan.Nodes) > 0 {
		b.WriteString("- 初始生成大纲：")
		for i, node := range plan.Nodes {
			if i > 0 {
				b.WriteString(" -> ")
			}
			b.WriteString(node.Title)
			if stage, ok := node.Config["stage"].(string); ok && stage != "" {
				b.WriteString("(" + stage + ")")
			}
		}
		b.WriteString("（生成过程中可以拆分、合并或调整节点）\n")
	}
	if len(plan.Outputs) > 0 {
		b.WriteString("- 输出：" + plan.Outputs[0].Description + "\n")
	}
	if len(plan.RequiredSlots) > 0 {
		b.WriteString("\n**需要确认**\n")
		for _, slot := range plan.RequiredSlots {
			b.WriteString("- " + slot.Question + "\n")
		}
		b.WriteString("\n补齐后我再生成 Runner 草稿并进入 Docker 验证。\n")
		return b.String()
	}
	b.WriteString("\n确认后点击卡片里的“生成”，我会按这个初始大纲继续探索、生成节点、验证结果；实际 Runner 草稿以生成与验证后的节点为准，并按受控 Docker Provider 做 mock 验证。\n")
	_ = session
	return b.String()
}

func shouldUseResourceWorkflowPlan(requirement string, frame opsmanual.OperationFrame) bool {
	if len(frame.Roles) > 0 || len(frame.Relationships) > 0 || len(frame.ObservationPoints) > 0 {
		return true
	}
	lower := strings.ToLower(requirement)
	resourceTerms := []string{"主机", "节点", "集群", "部署", "形成", "host", "node", "cluster", "service", "resource"}
	for _, term := range resourceTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func workflowGenerationResourceRoleSignals(plan *workflowgen.WorkflowGenerationPlan) []string {
	if plan == nil || len(plan.OperationFrame) == 0 {
		return nil
	}
	rawRoles, ok := plan.OperationFrame["roles"].([]any)
	if !ok {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, raw := range rawRoles {
		role, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := workflowGenerationMapString(role, "kind")
		ref := firstNonEmptyString(
			workflowGenerationMapString(role, "user_label"),
			workflowGenerationMapString(role, "runtime_name"),
			workflowGenerationMapString(role, "resource_ref"),
			workflowGenerationMapString(role, "id"),
		)
		if kind == "" {
			continue
		}
		signal := kind
		if ref != "" && ref != "<nil>" {
			signal += ":" + ref
		}
		runtimeName := workflowGenerationMapString(role, "runtime_name")
		if runtimeName != "" && runtimeName != ref {
			signal += ":" + runtimeName
		}
		if _, exists := seen[signal]; exists {
			continue
		}
		seen[signal] = struct{}{}
		out = append(out, signal)
	}
	return out
}

func workflowGenerationMapString(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func workflowGenerationGenericOpsContracts(plan *workflowgen.WorkflowGenerationPlan) []string {
	if plan == nil || plan.Intent != "generate_resource_ops_workflow" {
		return nil
	}
	return []string{"draft_until_reviewed", "secret_ref_only"}
}

func workflowGenerationEvidenceLimitations(plan *workflowgen.WorkflowGenerationPlan) []string {
	if plan == nil || plan.Intent != "generate_resource_ops_workflow" {
		return nil
	}
	return []string{
		"需要在 preflight 阶段确认目标资源、执行面和观察点可访问。",
		"需要用 secret_ref 提供凭据引用，不能把密码或 token 写入 workflow。",
		"pending_review 状态下不能声明生产 verified。",
	}
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
