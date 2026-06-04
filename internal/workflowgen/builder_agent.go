package workflowgen

import (
	"context"
	"fmt"
	"time"

	"runner/workflow/visual"
)

type WorkflowBuilderAgent struct {
	Store          SessionStore
	PlanBuilder    PlanBuilder
	GraphGenerator GraphGenerator
	Validator      WorkflowValidationProvider
	Now            func() time.Time
}

type GenerateDraftRequest struct {
	SessionID string `json:"session_id"`
}

type GenerateDraftResult struct {
	Session    *WorkflowGenerationSession `json:"session"`
	Graph      visual.Graph               `json:"graph"`
	Validation *ValidationResult          `json:"validation,omitempty"`
}

func (a WorkflowBuilderAgent) GenerateDraft(ctx context.Context, req GenerateDraftRequest) (*GenerateDraftResult, error) {
	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if a.Store == nil {
		return nil, fmt.Errorf("workflow generation store is required")
	}
	graphGenerator := a.GraphGenerator
	if graphGenerator == nil {
		graphGenerator = RunnerGraphGenerator{}
	}
	validator := a.Validator
	if validator == nil {
		validator = StaticValidationProvider{}
	}

	session, err := a.Store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if session.Plan == nil {
		_ = a.appendError(ctx, req.SessionID, "生成失败：缺少已确认的工作流 plan。")
		return nil, fmt.Errorf("workflow generation plan is required")
	}

	if _, err := a.Store.Update(ctx, req.SessionID, func(next *WorkflowGenerationSession) error {
		next.Status = SessionStatusGenerationStarted
		return nil
	}); err != nil {
		return nil, err
	}
	if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
		Type:    EventGenerationStarted,
		Status:  string(SessionStatusGenerationStarted),
		Message: "开始生成 Runner Workflow 草稿。",
	}); err != nil {
		return nil, err
	}
	for _, node := range session.Plan.Nodes {
		nodeID := sanitizeID(node.ID)
		if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
			Type:    EventNodeGenerating,
			NodeID:  nodeID,
			Status:  "generating",
			Message: firstNonEmpty(node.Title, nodeID) + " 生成中。",
			Payload: map[string]any{"kind": string(node.Kind)},
		}); err != nil {
			return nil, err
		}
		if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
			Type:    EventNodeGenerated,
			NodeID:  nodeID,
			Status:  "generated",
			Message: firstNonEmpty(node.Title, nodeID) + " 已生成。",
			Payload: map[string]any{"action": firstNonEmpty(node.Action, "script.python")},
		}); err != nil {
			return nil, err
		}
	}

	graph, err := graphGenerator.GenerateGraph(ctx, GenerateGraphRequest{SessionID: req.SessionID, Plan: *session.Plan})
	if err != nil {
		_ = a.appendError(ctx, req.SessionID, "生成 Runner Workflow 图失败："+err.Error())
		return nil, err
	}
	if _, err := a.Store.Update(ctx, req.SessionID, func(next *WorkflowGenerationSession) error {
		next.Status = SessionStatusGraphReady
		return nil
	}); err != nil {
		return nil, err
	}
	if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
		Type:    EventGraphPreviewReady,
		Status:  string(SessionStatusGraphReady),
		Message: "Runner Workflow 图预览已生成。",
		Payload: map[string]any{"node_count": len(graph.Nodes), "edge_count": len(graph.Edges)},
	}); err != nil {
		return nil, err
	}

	if _, err := a.Store.Update(ctx, req.SessionID, func(next *WorkflowGenerationSession) error {
		next.Status = SessionStatusValidationStarted
		return nil
	}); err != nil {
		return nil, err
	}
	if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
		Type:    EventValidationStarted,
		Status:  string(SessionStatusValidationStarted),
		Message: validationStartMessage(validator.Name()),
		Payload: map[string]any{"provider": string(validator.Name()), "scenario": session.Plan.ValidationStrategy.Scenario},
	}); err != nil {
		return nil, err
	}

	validation, err := validator.Validate(ctx, ValidationRequest{
		SessionID:     req.SessionID,
		Graph:         graph,
		Scenario:      session.Plan.ValidationStrategy.Scenario,
		NetworkPolicy: firstNonEmpty(session.Plan.ValidationStrategy.Network, "none"),
		AllowedImages: []string{"python:3.12-slim"},
	})
	if err != nil {
		_ = a.appendError(ctx, req.SessionID, "验证 Provider 执行失败："+err.Error())
		return nil, err
	}
	if validation == nil {
		validation = &ValidationResult{
			ID:       "validation-empty",
			Provider: validator.Name(),
			Status:   "failed",
			Summary:  "验证 Provider 没有返回结果。",
		}
	}

	nextStatus := SessionStatusValidationFailed
	eventType := EventValidationFailed
	if validation.Status == "passed" || validation.Status == "skipped" {
		nextStatus = SessionStatusValidationPassed
		eventType = EventValidationPassed
	}
	updated, err := a.Store.Update(ctx, req.SessionID, func(next *WorkflowGenerationSession) error {
		next.Status = nextStatus
		next.ValidationProvider = validation.Provider
		next.ValidationRuns = append(next.ValidationRuns, ValidationRunSummary{
			ID:        validation.ID,
			Provider:  validation.Provider,
			Status:    validation.Status,
			Scenario:  validation.Scenario,
			Summary:   validation.Summary,
			StartedAt: validation.StartedAt,
			EndedAt:   validation.EndedAt,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := a.Store.AppendEvent(ctx, req.SessionID, WorkflowGenerationEvent{
		Type:    eventType,
		Status:  validation.Status,
		Message: validation.Summary,
		Payload: map[string]any{
			"provider":        string(validation.Provider),
			"validation_id":   validation.ID,
			"failure_node_id": validation.FailureNodeID,
			"skipped_reason":  validation.SkippedReason,
		},
	}); err != nil {
		return nil, err
	}

	updated, err = a.Store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	return &GenerateDraftResult{Session: updated, Graph: graph, Validation: validation}, nil
}

func (a WorkflowBuilderAgent) appendError(ctx context.Context, sessionID, message string) error {
	_, _ = a.Store.Update(ctx, sessionID, func(next *WorkflowGenerationSession) error {
		next.Status = SessionStatusFailed
		return nil
	})
	_, err := a.Store.AppendEvent(ctx, sessionID, WorkflowGenerationEvent{
		Type:    EventError,
		Status:  string(SessionStatusFailed),
		Message: message,
	})
	return err
}

func validationStartMessage(provider ValidationProvider) string {
	if provider == ValidationProviderDocker {
		return "启动 Docker 受控验证 Provider。"
	}
	return "启动静态验证 Provider。"
}
