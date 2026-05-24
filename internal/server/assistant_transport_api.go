package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/runtimekernel"
)

const assistantTransportPollInterval = 10 * time.Millisecond

type assistantTransportSessionSourceProvider interface {
	SessionSource() appui.SessionSource
}

func (s *HTTPServer) handleAssistantTransport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source := s.assistantTransportSessionSource()
	if source == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "assistant transport session source is not configured"})
		return
	}

	req, err := decodeAssistantTransportRequest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	commands, err := assistantTransportCommandsFromRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	encoder := newAssistantTransportStreamEncoder(w)
	projector := appui.NewTransportProjector()
	handler := appui.NewTransportCommandHandler(s.ui.ChatService(), s.ui.ApprovalService(), s.ui.ChoiceService(), s.ui.MCPService())

	state := assistantTransportInitialState(req)
	prev := state
	for _, command := range commands {
		next, _, applyErr := handler.Apply(r.Context(), assistantTransportCloneState(state), command)
		if applyErr != nil {
			next.Status = appui.AiopsTransportStatusFailed
			next.LastError = strings.TrimSpace(applyErr.Error())
			next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := encoder.WriteStateOps(assistantTransportDiffStateOps(prev, next)); err != nil {
				return
			}
			_ = encoder.WriteError(next.LastError)
			return
		}
		next = s.decorateAssistantTransportAgentUIArtifacts(next, command)
		if err := encoder.WriteStateOps(assistantTransportDiffStateOps(prev, next)); err != nil {
			return
		}
		prev = next
		state = next
	}

	shouldPoll := strings.TrimSpace(state.SessionID) != "" && (state.CurrentTurnID != "" || state.Status == appui.AiopsTransportStatusWorking || state.Status == appui.AiopsTransportStatusBlocked)
	if !shouldPoll {
		return
	}

	if _, err := s.streamAssistantTransportState(r.Context(), encoder, source, projector, s.ui.ChatService(), state); err != nil {
		return
	}
}

func (s *HTTPServer) assistantTransportSessionSource() appui.SessionSource {
	if s == nil || s.ui == nil {
		return nil
	}
	provider, ok := s.ui.(assistantTransportSessionSourceProvider)
	if !ok {
		return nil
	}
	return provider.SessionSource()
}

func (s *HTTPServer) decorateAssistantTransportAgentUIArtifacts(state appui.AiopsTransportState, command appui.TransportCommand) appui.AiopsTransportState {
	if command.Type != appui.TransportCommandTypeAddMessage || command.AddMessage == nil {
		return state
	}
	turnID := strings.TrimSpace(state.CurrentTurnID)
	if turnID == "" {
		return state
	}
	turn := state.Turns[turnID]
	if turn.ID == "" {
		turn.ID = turnID
	}
	switch strings.TrimSpace(command.AddMessage.Metadata["opsManualAction"]) {
	case "":
		if !assistantTransportOpsManualSkipped(command.AddMessage.Metadata) {
			return state
		}
	case "generate_ops_manual_candidate", "generate_runner_workflow_candidate":
		return state
	}
	if assistantTransportOpsManualSkipped(command.AddMessage.Metadata) {
		s.recordAssistantTransportOpsManualSuppression(
			firstAssistantTransportValue(state.SessionID, command.AddMessage.SessionID),
			command.AddMessage,
		)
	}
	if assistantTransportOpsManualReference(command.AddMessage.Metadata) {
		s.recordAssistantTransportManualGuidedReference(
			firstAssistantTransportValue(state.SessionID, command.AddMessage.SessionID),
			command.AddMessage,
		)
	}
	state.Turns[turnID] = turn
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state
}

func (s *HTTPServer) recordAssistantTransportOpsManualSuppression(sessionID string, command *appui.TransportAddMessageCommand) {
	if s == nil || command == nil {
		return
	}
	service := s.opsManualService()
	if service == nil {
		return
	}
	_ = service.RecordSuppression(context.Background(), strings.TrimSpace(sessionID), command.Message.Text, command.Metadata)
}

func (s *HTTPServer) recordAssistantTransportManualGuidedReference(sessionID string, command *appui.TransportAddMessageCommand) {
	if s == nil || command == nil {
		return
	}
	service := s.opsManualService()
	if service == nil {
		return
	}
	_ = service.RecordManualGuidedReference(context.Background(), strings.TrimSpace(sessionID), command.Message.Text, command.Metadata)
}

func assistantTransportOpsManualSkipped(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "skip_ops_manual") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualSkipped"]), "true")
}

func assistantTransportOpsManualReference(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "reference_ops_manual")
}

func assistantTransportOpsManualSearchArtifactFromToolResult(turnID string, itemID string, tool runtimekernel.ToolResult) (appui.AiopsTransportAgentUIArtifact, bool) {
	if tool.Display == nil || strings.TrimSpace(tool.Display.Type) != "ops_manual_search_result" {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	data := tool.Display.Data
	if len(data) == 0 && strings.TrimSpace(tool.Content) != "" {
		data = json.RawMessage(tool.Content)
	}
	if len(data) == 0 {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	decision := strings.TrimSpace(fmt.Sprint(payload["decision"]))
	if decision == "" {
		decision = "unknown"
	}
	if !assistantTransportActionableOpsManualSearchPayload(payload) {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	idPart := firstAssistantTransportValue(strings.TrimSpace(itemID), "search")
	return appui.AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-search:" + turnID + ":" + idPart,
		Type:            "ops_manual_search_result",
		Title:           "Ops manual search result",
		TitleZh:         "运维手册检索结果",
		Summary:         decision,
		SummaryZh:       assistantTransportOpsManualSummary(decision),
		Status:          decision,
		Severity:        assistantTransportOpsManualSeverity(decision),
		Source:          "tool:search_ops_manuals",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         assistantTransportOpsManualActions(decision),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func assistantTransportOpsManualPreflightArtifactFromToolResult(turnID string, itemID string, tool runtimekernel.ToolResult) (appui.AiopsTransportAgentUIArtifact, bool) {
	if tool.Display == nil || strings.TrimSpace(tool.Display.Type) != "ops_manual_preflight_result" {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	data := tool.Display.Data
	if len(data) == 0 && strings.TrimSpace(tool.Content) != "" {
		data = json.RawMessage(tool.Content)
	}
	if len(data) == 0 {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	status := strings.TrimSpace(firstAssistantTransportValue(assistantTransportStringValueFromMap(payload, "status"), assistantTransportStringValueFromMap(payload, "preflight_status"), "unknown"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	idPart := firstAssistantTransportValue(strings.TrimSpace(itemID), "preflight")
	return appui.AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-preflight:" + turnID + ":" + idPart,
		Type:            "ops_manual_preflight_result",
		Title:           "Ops manual preflight result",
		TitleZh:         "运维手册预检结果",
		Summary:         status,
		SummaryZh:       assistantTransportOpsManualPreflightSummary(status),
		Status:          status,
		Severity:        assistantTransportOpsManualPreflightSeverity(status),
		Source:          "tool:run_ops_manual_preflight",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         assistantTransportOpsManualPreflightActions(status, assistantTransportStringValueFromMap(payload, "next_action")),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func assistantTransportOpsManualParamResolutionArtifactFromToolResult(turnID string, itemID string, tool runtimekernel.ToolResult) (appui.AiopsTransportAgentUIArtifact, bool) {
	if tool.Display == nil || (strings.TrimSpace(tool.Display.Type) != "ops_manual_param_resolution" && strings.TrimSpace(tool.Display.Type) != "ops_manual_param_form") {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	data := tool.Display.Data
	if len(data) == 0 && strings.TrimSpace(tool.Content) != "" {
		data = json.RawMessage(tool.Content)
	}
	if len(data) == 0 {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	status := strings.TrimSpace(firstAssistantTransportValue(assistantTransportStringValueFromMap(payload, "status"), "unknown"))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	idPart := firstAssistantTransportValue(strings.TrimSpace(itemID), "param-resolution")
	return appui.AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-param-resolution:" + turnID + ":" + idPart,
		Type:            strings.TrimSpace(tool.Display.Type),
		Title:           "Ops manual parameter resolution",
		TitleZh:         "运维手册参数解析",
		Summary:         status,
		SummaryZh:       assistantTransportOpsManualParamResolutionSummary(status),
		Status:          status,
		Severity:        assistantTransportOpsManualParamResolutionSeverity(status),
		Source:          "tool:resolve_ops_manual_params",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData:      payload,
		Actions:         assistantTransportOpsManualParamResolutionActions(status),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func assistantTransportOpsManualParamResolutionSummary(status string) string {
	switch strings.TrimSpace(status) {
	case "resolved":
		return "参数已自动补齐，可进入预检。"
	case "ambiguous":
		return "发现多个候选，需要用户选择。"
	case "need_user_input":
		return "仍缺少少量无法自动获取的参数。"
	default:
		return "已完成运维手册参数解析。"
	}
}

func assistantTransportOpsManualParamResolutionSeverity(status string) string {
	switch strings.TrimSpace(status) {
	case "resolved":
		return "success"
	case "ambiguous", "need_user_input":
		return "warning"
	default:
		return "neutral"
	}
}

func assistantTransportOpsManualParamResolutionActions(status string) []map[string]any {
	switch strings.TrimSpace(status) {
	case "resolved":
		return []map[string]any{{"id": "run_preflight", "label": "运行预检", "kind": "panel"}}
	case "ambiguous", "need_user_input":
		return []map[string]any{{"id": "fill_params", "label": "补充参数", "kind": "form"}}
	default:
		return nil
	}
}

func (s *HTTPServer) assistantTransportOpsManualMatchArtifact(turnID string, command *appui.TransportAddMessageCommand) (appui.AiopsTransportAgentUIArtifact, bool) {
	if command == nil || strings.TrimSpace(command.Message.Text) == "" {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	if s == nil || !s.opsManualAutoRetrieval {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	service := s.opsManualService()
	if service == nil {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	result, err := service.RetrieveManuals(appui.OpsManualRetrieveRequest{
		Text:     command.Message.Text,
		Metadata: assistantTransportStringMetadataToAny(command.Metadata),
	})
	if err != nil || len(result.Matches) == 0 {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	match := result.Matches[0]
	if match.State == "no_match" || strings.TrimSpace(match.Manual.ID) == "" {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return appui.AiopsTransportAgentUIArtifact{
		ID:              "ops-manual-match:" + turnID,
		Type:            "ops_manual_match",
		Title:           "Ops manual decision",
		TitleZh:         "运维手册判定",
		Summary:         string(match.State),
		SummaryZh:       assistantTransportOpsManualSummary(string(match.State)),
		Status:          string(match.State),
		Severity:        assistantTransportOpsManualSeverity(string(match.State)),
		Source:          "ai-chat",
		PermissionScope: "read",
		RedactionStatus: "redacted",
		InlineData: map[string]any{
			"state":                  string(match.State),
			"operationFrame":         result.OperationFrame,
			"manual":                 match.Manual,
			"manualId":               match.Manual.ID,
			"manualTitle":            match.Manual.Title,
			"workflowRef":            match.Manual.WorkflowRef,
			"reasons":                match.Reasons,
			"missingContext":         match.MissingContext,
			"compatibilityGaps":      match.CompatibilityGaps,
			"recommendedNextActions": match.RecommendedNextActions,
			"runRecordSummary":       match.RunRecordSummary,
		},
		Actions:   assistantTransportOpsManualActions(string(match.State)),
		CreatedAt: now,
		UpdatedAt: now,
	}, true
}

func assistantTransportOpsManualSummary(state string) string {
	switch strings.TrimSpace(state) {
	case "direct_execute", "direct":
		return "已找到可直接使用的运维手册，仍需参数确认、环境预检和确认或审批。"
	case "adapt", "adapt_required":
		return "找到相似运维手册，但当前环境存在差异，需要先生成变体并校验。"
	case "reference_only":
		return "没有可直接运行的 Workflow，可继续只读自动化排查。"
	case "need_info", "need_more_info":
		return "识别到相关运维手册，但还缺少少量关键上下文。"
	case "no_match":
		return "没有找到合适的运维手册。"
	default:
		return "已完成运维手册检索判定。"
	}
}

func assistantTransportOpsManualSeverity(state string) string {
	switch strings.TrimSpace(state) {
	case "direct_execute", "direct":
		return "success"
	case "adapt", "adapt_required":
		return "warning"
	case "reference_only", "need_info", "need_more_info":
		return "info"
	default:
		return "neutral"
	}
}

func assistantTransportOpsManualActions(state string) []map[string]any {
	switch strings.TrimSpace(state) {
	case "direct_execute", "direct":
		return []map[string]any{
			{"id": "fill_parameters", "label": "填写参数", "kind": "panel"},
			{"id": "run_preflight", "label": "运行预检", "kind": "panel"},
		}
	case "adapt", "adapt_required":
		return []map[string]any{
			{"id": "generate_variant", "label": "生成适配工作流", "kind": "confirm"},
			{"id": "review_gaps", "label": "查看差异", "kind": "panel"},
		}
	case "reference_only":
		return nil
	case "need_info", "need_more_info":
		return nil
	default:
		return nil
	}
}

func assistantTransportOpsManualPreflightSummary(status string) string {
	switch strings.TrimSpace(status) {
	case "passed":
		return "预检已通过，可以确认或审批后执行。"
	case "blocked":
		return "预检被阻断，需要补充参数、权限或环境适配。"
	case "failed":
		return "预检失败，不能执行绑定工作流。"
	case "not_applicable":
		return "该手册没有预检探针，需要人工确认或审批后执行。"
	default:
		return "已完成运维手册预检。"
	}
}

func assistantTransportOpsManualPreflightSeverity(status string) string {
	switch strings.TrimSpace(status) {
	case "passed":
		return "success"
	case "blocked":
		return "warning"
	case "failed":
		return "error"
	case "not_applicable":
		return "info"
	default:
		return "neutral"
	}
}

func assistantTransportOpsManualPreflightActions(status string, nextAction string) []map[string]any {
	nextAction = strings.TrimSpace(nextAction)
	switch strings.TrimSpace(status) {
	case "passed", "not_applicable":
		if nextAction == "request_approval" {
			return []map[string]any{{"id": "request_approval", "label": "发起审批", "kind": "confirm"}}
		}
		if nextAction == "execute_workflow" {
			return []map[string]any{{"id": "execute_workflow", "label": "执行 Workflow", "kind": "confirm"}}
		}
		return []map[string]any{{"id": "confirm_execution", "label": "确认执行", "kind": "confirm"}}
	case "blocked":
		if nextAction == "request_permission" {
			return []map[string]any{{"id": "request_permission", "label": "申请权限", "kind": "panel"}}
		}
		if nextAction == "generate_workflow_variant" {
			return []map[string]any{{"id": "generate_variant", "label": "生成适配工作流", "kind": "confirm"}}
		}
		return []map[string]any{{"id": "collect_context", "label": "补充上下文", "kind": "form"}}
	case "failed":
		return []map[string]any{{"id": "fallback_guide", "label": "查看降级步骤", "kind": "panel"}}
	default:
		return nil
	}
}

func assistantTransportStringValueFromMap(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func assistantTransportStringMetadataToAny(metadata map[string]string) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func upsertAssistantTransportAgentUIArtifact(items []appui.AiopsTransportAgentUIArtifact, artifact appui.AiopsTransportAgentUIArtifact) []appui.AiopsTransportAgentUIArtifact {
	if strings.TrimSpace(artifact.ID) == "" {
		return items
	}
	for idx := range items {
		if items[idx].ID == artifact.ID {
			items[idx] = artifact
			return items
		}
	}
	return append(items, artifact)
}

func assistantTransportInitialState(req *assistantTransportRequest) appui.AiopsTransportState {
	if req == nil {
		return appui.NewAiopsTransportState("", "")
	}
	state := req.State
	if strings.TrimSpace(state.SchemaVersion) == "" {
		state = appui.NewAiopsTransportState(strings.TrimSpace(state.SessionID), strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.ThreadID)))
	}
	if strings.TrimSpace(state.ThreadID) == "" {
		state.ThreadID = strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.SessionID))
	}
	if strings.TrimSpace(state.SessionID) == "" {
		state.SessionID = strings.TrimSpace(req.State.SessionID)
	}
	if state.Turns == nil {
		state.Turns = map[string]appui.AiopsTransportTurn{}
	}
	if state.TurnOrder == nil {
		state.TurnOrder = []string{}
	}
	if state.PendingApprovals == nil {
		state.PendingApprovals = map[string]appui.AiopsTransportApproval{}
	}
	if state.McpSurfaces == nil {
		state.McpSurfaces = map[string]appui.AiopsTransportMcpSurface{}
	}
	if state.Artifacts == nil {
		state.Artifacts = map[string]appui.AiopsTransportArtifact{}
	}
	if state.RuntimeLiveness.ActiveTurns == nil {
		state.RuntimeLiveness.ActiveTurns = map[string]bool{}
	}
	if state.RuntimeLiveness.ActiveAgents == nil {
		state.RuntimeLiveness.ActiveAgents = map[string]bool{}
	}
	if state.RuntimeLiveness.PendingApprovals == nil {
		state.RuntimeLiveness.PendingApprovals = map[string]bool{}
	}
	if state.RuntimeLiveness.PendingUserInputs == nil {
		state.RuntimeLiveness.PendingUserInputs = map[string]bool{}
	}
	if state.RuntimeLiveness.ActiveCommandStreams == nil {
		state.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
	}
	if strings.TrimSpace(state.UpdatedAt) == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return state
}

func assistantTransportCommandsFromRequest(req *assistantTransportRequest) ([]appui.TransportCommand, error) {
	if req == nil || len(req.Commands) == 0 {
		return nil, nil
	}
	state := assistantTransportInitialState(req)
	commands := make([]appui.TransportCommand, 0, len(req.Commands))
	for _, command := range req.Commands {
		next, err := assistantTransportCommandFromDecoded(command, req, state)
		if err != nil {
			return nil, err
		}
		commands = append(commands, next)
	}
	return commands, nil
}

func assistantTransportCommandFromDecoded(raw assistantTransportCommand, req *assistantTransportRequest, state appui.AiopsTransportState) (appui.TransportCommand, error) {
	switch command := raw.(type) {
	case *assistantTransportAddMessageCommand:
		text, sourceID, hostID, metadata, err := decodeAssistantTransportMessage(command.Message)
		if err != nil {
			return appui.TransportCommand{}, err
		}
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeAddMessage,
			AddMessage: &appui.TransportAddMessageCommand{
				SessionID: state.SessionID,
				ThreadID:  strings.TrimSpace(firstAssistantTransportValue(req.ThreadID, state.ThreadID)),
				ParentID:  strings.TrimSpace(req.ParentID),
				SourceID:  sourceID,
				HostID:    strings.TrimSpace(hostID),
				Message:   appui.TransportUserMessage{Text: text},
				Metadata:  metadata,
			},
		}, nil
	case *assistantTransportRetryCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeRetry,
			Retry: &appui.TransportRetryCommand{
				SessionID: strings.TrimSpace(firstAssistantTransportValue(command.SessionID, state.SessionID)),
				TurnID:    strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
			},
		}, nil
	case *assistantTransportStopCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeStop,
			Stop: &appui.TransportStopCommand{
				SessionID: strings.TrimSpace(firstAssistantTransportValue(command.SessionID, state.SessionID)),
				TurnID:    strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
				Reason:    strings.TrimSpace(command.Reason),
			},
		}, nil
	case *assistantTransportApprovalDecisionCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeApprovalDecision,
			ApprovalDecision: &appui.TransportApprovalDecisionCommand{
				SessionID:  strings.TrimSpace(firstAssistantTransportValue(command.SessionID, state.SessionID)),
				TurnID:     strings.TrimSpace(firstAssistantTransportValue(command.TurnID, state.CurrentTurnID)),
				ApprovalID: strings.TrimSpace(command.ApprovalID),
				Decision:   strings.TrimSpace(command.Decision),
			},
		}, nil
	case *assistantTransportChoiceAnswerCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeChoiceAnswer,
			ChoiceAnswer: &appui.TransportChoiceAnswerCommand{
				RequestID: strings.TrimSpace(command.RequestID),
				Answer:    strings.TrimSpace(command.Answer),
			},
		}, nil
	case *assistantTransportMCPActionCommand:
		action := strings.TrimSpace(command.Action)
		if strings.EqualFold(action, "refresh") {
			return appui.TransportCommand{
				Type: appui.TransportCommandTypeMCPRefresh,
				MCPRefresh: &appui.TransportMCPRefreshCommand{
					SurfaceID: strings.TrimSpace(firstAssistantTransportValue(command.SurfaceID, command.Target)),
				},
			}, nil
		}
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPAction,
			MCPAction: &appui.TransportMCPActionCommand{
				SurfaceID: strings.TrimSpace(firstAssistantTransportValue(command.SurfaceID, command.Target)),
				ActionID:  action,
				Input:     cloneTransportAnyMap(command.Params),
			},
		}, nil
	case *assistantTransportMCPRefreshCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPRefresh,
			MCPRefresh: &appui.TransportMCPRefreshCommand{
				SurfaceID: strings.TrimSpace(command.SurfaceID),
			},
		}, nil
	case *assistantTransportMCPPinCommand:
		return appui.TransportCommand{
			Type: appui.TransportCommandTypeMCPPin,
			MCPPin: &appui.TransportMCPPinCommand{
				SurfaceID: strings.TrimSpace(command.SurfaceID),
				Pinned:    command.Pinned,
			},
		}, nil
	default:
		return appui.TransportCommand{}, errors.New("assistant transport command is not supported")
	}
}

func decodeAssistantTransportMessage(raw json.RawMessage) (text string, sourceID string, hostID string, metadata map[string]string, err error) {
	var payload struct {
		ID       string            `json:"id"`
		Role     string            `json:"role"`
		HostID   string            `json:"hostId"`
		Metadata map[string]string `json:"metadata"`
		Content  []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return "", "", "", nil, err
	}
	parts := make([]string, 0, len(payload.Content)+len(payload.Parts))
	for _, item := range payload.Content {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") {
			if value := strings.TrimSpace(item.Text); value != "" {
				parts = append(parts, value)
			}
		}
	}
	for _, item := range payload.Parts {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") {
			if value := strings.TrimSpace(item.Text); value != "" {
				parts = append(parts, value)
			}
		}
	}
	return strings.Join(parts, "\n"), strings.TrimSpace(payload.ID), strings.TrimSpace(payload.HostID), cloneTransportMetadata(payload.Metadata), nil
}

func (s *HTTPServer) streamAssistantTransportState(
	ctx context.Context,
	encoder *assistantTransportStreamEncoder,
	source appui.SessionSource,
	projector *appui.TransportProjector,
	chat appui.ChatService,
	state appui.AiopsTransportState,
) (appui.AiopsTransportState, error) {
	ticker := time.NewTicker(assistantTransportPollInterval)
	defer ticker.Stop()

	current := state
	lastFingerprint := ""
	for {
		session := source.Get(current.SessionID)
		waitingForAcceptedApproval := false
		if session != nil {
			latestTurn := assistantTransportLatestSessionTurn(session)
			waitingForAcceptedTurn := assistantTransportShouldWaitForAcceptedTurn(current, latestTurn)
			waitingForAcceptedApproval = assistantTransportShouldWaitForAcceptedApproval(current, latestTurn)
			fingerprint := assistantTransportTurnFingerprint(latestTurn)
			if !waitingForAcceptedTurn && !waitingForAcceptedApproval && fingerprint != "" && fingerprint != lastFingerprint {
				next, err := projectAssistantTransportSessionState(s, assistantTransportCloneState(current), session, projector)
				if err != nil {
					return current, err
				}
				if err := encoder.WriteStateOps(assistantTransportDiffStateOps(current, next)); err != nil {
					return current, err
				}
				if assistantTransportShouldWriteError(next) {
					if err := encoder.WriteError(next.LastError); err != nil {
						return next, err
					}
				}
				current = next
				lastFingerprint = fingerprint
			}
			if assistantTransportSessionTurnShouldCloseStream(session) {
				if !waitingForAcceptedTurn && !waitingForAcceptedApproval && current.Status != appui.AiopsTransportStatusWorking && current.Status != appui.AiopsTransportStatusBlocked {
					return current, nil
				}
				if !waitingForAcceptedTurn && !waitingForAcceptedApproval && current.Status == appui.AiopsTransportStatusBlocked {
					return current, nil
				}
			}
		}

		select {
		case <-ctx.Done():
			if !waitingForAcceptedApproval && shouldCancelAssistantTransportOnContextDone(current, session) {
				_ = cancelAssistantTransportTurn(context.Background(), chat, current, session)
			}
			return current, ctx.Err()
		case <-ticker.C:
		}
	}
}

func assistantTransportShouldWaitForAcceptedTurn(state appui.AiopsTransportState, latest *runtimekernel.TurnSnapshot) bool {
	if latest == nil {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking && state.Status != appui.AiopsTransportStatusBlocked {
		return false
	}
	currentTurnID := strings.TrimSpace(state.CurrentTurnID)
	latestTurnID := strings.TrimSpace(latest.ID)
	if currentTurnID == "" || latestTurnID == "" || currentTurnID == latestTurnID {
		return false
	}
	return latest.Lifecycle.IsTerminal()
}

func assistantTransportShouldWaitForAcceptedApproval(state appui.AiopsTransportState, latest *runtimekernel.TurnSnapshot) bool {
	if latest == nil {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking &&
		state.Status != appui.AiopsTransportStatusFailed &&
		state.Status != appui.AiopsTransportStatusCanceled {
		return false
	}
	currentTurnID := strings.TrimSpace(state.CurrentTurnID)
	latestTurnID := strings.TrimSpace(latest.ID)
	if currentTurnID == "" || latestTurnID == "" || currentTurnID != latestTurnID {
		return false
	}
	if state.Status == appui.AiopsTransportStatusWorking && !state.RuntimeLiveness.ActiveTurns[latestTurnID] {
		return false
	}
	if latest.Lifecycle != runtimekernel.TurnLifecycleSuspended && latest.Lifecycle != runtimekernel.TurnLifecycleResumable {
		return false
	}
	pendingIDs := assistantTransportPendingApprovalIDs(latest)
	if len(pendingIDs) == 0 {
		return false
	}
	if !assistantTransportHasLocalApprovalDecision(state, latestTurnID, pendingIDs) {
		return false
	}
	for approvalID := range pendingIDs {
		if _, ok := state.PendingApprovals[approvalID]; ok {
			return false
		}
		if state.RuntimeLiveness.PendingApprovals[approvalID] {
			return false
		}
	}
	return true
}

func assistantTransportHasLocalApprovalDecision(state appui.AiopsTransportState, turnID string, pendingIDs map[string]bool) bool {
	turn, ok := state.Turns[strings.TrimSpace(turnID)]
	if !ok {
		return false
	}
	for _, block := range turn.Process {
		approvalID := strings.TrimSpace(block.ApprovalID)
		if approvalID == "" || !pendingIDs[approvalID] {
			continue
		}
		switch block.Status {
		case appui.AiopsTransportProcessStatusRunning,
			appui.AiopsTransportProcessStatusRejected,
			appui.AiopsTransportProcessStatusFailed:
			return true
		}
	}
	return false
}

func assistantTransportPendingApprovalIDs(turn *runtimekernel.TurnSnapshot) map[string]bool {
	pending := map[string]bool{}
	if turn == nil {
		return pending
	}
	for _, approval := range turn.PendingApprovals {
		if id := strings.TrimSpace(approval.ID); id != "" {
			pending[id] = true
		}
	}
	for _, evidence := range turn.PendingEvidence {
		if id := strings.TrimSpace(evidence.ID); id != "" {
			pending[id] = true
		}
	}
	return pending
}

func assistantTransportShouldWriteError(state appui.AiopsTransportState) bool {
	if state.Status != appui.AiopsTransportStatusFailed {
		return false
	}
	message := strings.TrimSpace(state.LastError)
	if message == "" {
		return false
	}
	normalized := strings.ToLower(message)
	if normalized == "approval denied" || normalized == "approval rejected" || normalized == "user denied approval" {
		return false
	}
	return true
}

func shouldCancelAssistantTransportOnContextDone(state appui.AiopsTransportState, session *runtimekernel.SessionState) bool {
	if session == nil || session.CurrentTurn == nil {
		return false
	}
	if session.CurrentTurn.Lifecycle.IsTerminal() {
		return false
	}
	if state.Status != appui.AiopsTransportStatusWorking && state.Status != appui.AiopsTransportStatusBlocked {
		return false
	}
	return strings.TrimSpace(session.ID) != "" && strings.TrimSpace(session.CurrentTurn.ID) != ""
}

func cancelAssistantTransportTurn(ctx context.Context, chat appui.ChatService, state appui.AiopsTransportState, session *runtimekernel.SessionState) error {
	if chat == nil || session == nil || session.CurrentTurn == nil {
		return nil
	}
	_, err := chat.StopTurn(ctx, appui.StopCommand{
		SessionID: strings.TrimSpace(firstAssistantTransportValue(state.SessionID, session.ID)),
		TurnID:    strings.TrimSpace(firstAssistantTransportValue(state.CurrentTurnID, session.CurrentTurn.ID)),
		Reason:    "assistant transport client disconnected",
	})
	return err
}

func projectAssistantTransportSessionState(
	server *HTTPServer,
	state appui.AiopsTransportState,
	session *runtimekernel.SessionState,
	projector *appui.TransportProjector,
) (appui.AiopsTransportState, error) {
	if projector == nil {
		projector = appui.NewTransportProjector()
	}
	next := state
	if session == nil {
		return next, nil
	}
	next.SessionID = strings.TrimSpace(firstAssistantTransportValue(session.ID, next.SessionID))
	if strings.TrimSpace(next.ThreadID) == "" {
		next.ThreadID = strings.TrimSpace(firstAssistantTransportValue(next.SessionID, session.ID))
	}
	turns := assistantTransportSessionTurns(session)
	if len(turns) > 0 {
		for i := range turns {
			projected, err := projector.ProjectTurnSnapshot(next, &turns[i])
			if err != nil {
				return next, err
			}
			next = server.decorateAssistantTransportOpsManualFallback(projected, &turns[i])
		}
		return next, nil
	}
	next.Status = appui.AiopsTransportStatusIdle
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return next, nil
}

func assistantTransportSessionTurns(session *runtimekernel.SessionState) []runtimekernel.TurnSnapshot {
	if session == nil {
		return nil
	}
	turns := make([]runtimekernel.TurnSnapshot, 0, len(session.TurnHistory)+1)
	indexByID := make(map[string]int, len(session.TurnHistory)+1)
	appendTurn := func(turn *runtimekernel.TurnSnapshot) {
		if turn == nil {
			return
		}
		turnID := strings.TrimSpace(turn.ID)
		if turnID != "" {
			if idx, ok := indexByID[turnID]; ok {
				turns[idx] = *turn
				return
			}
			indexByID[turnID] = len(turns)
		}
		turns = append(turns, *turn)
	}
	for i := range session.TurnHistory {
		appendTurn(&session.TurnHistory[i])
	}
	appendTurn(session.CurrentTurn)
	return turns
}

func (s *HTTPServer) decorateAssistantTransportOpsManualFallback(state appui.AiopsTransportState, turn *runtimekernel.TurnSnapshot) appui.AiopsTransportState {
	if s == nil || turn == nil || !s.opsManualAutoRetrieval || !turn.Lifecycle.IsTerminal() {
		return state
	}
	turnID := strings.TrimSpace(turn.ID)
	if turnID == "" {
		return state
	}
	projectedTurn := state.Turns[turnID]
	if projectedTurn.ID == "" || assistantTransportHasOpsManualArtifact(projectedTurn.AgentUIArtifacts) {
		return state
	}
	userText := firstAssistantTransportValue(strings.TrimSpace(projectedTurnUserText(projectedTurn)), assistantTransportTurnUserText(turn))
	if strings.TrimSpace(userText) == "" {
		return state
	}
	if !opsmanual.ShouldSearchForOpsManuals(userText) {
		return state
	}
	artifact, ok := s.assistantTransportOpsManualSearchFallbackArtifact(turnID, userText)
	if !ok {
		return state
	}
	projectedTurn.AgentUIArtifacts = upsertAssistantTransportAgentUIArtifact(projectedTurn.AgentUIArtifacts, artifact)
	state.Turns[turnID] = projectedTurn
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return state
}

func assistantTransportHasOpsManualArtifact(artifacts []appui.AiopsTransportAgentUIArtifact) bool {
	for _, artifact := range artifacts {
		switch strings.TrimSpace(artifact.Type) {
		case "ops_manual_search_result", "ops_manual_match", "ops_manual_preflight_result", "ops_manual_param_resolution", "ops_manual_param_form":
			return true
		}
	}
	return false
}

func projectedTurnUserText(turn appui.AiopsTransportTurn) string {
	if turn.User == nil {
		return ""
	}
	return strings.TrimSpace(turn.User.Text)
}

func assistantTransportTurnUserText(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	for _, item := range turn.AgentItems {
		if item.Type != "user_message" {
			continue
		}
		return firstAssistantTransportValue(strings.TrimSpace(item.Payload.Summary), assistantTransportDecodeUserText(item.Payload.Data))
	}
	return ""
}

func assistantTransportDecodeUserText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	parts := make([]string, 0, len(payload.Content)+len(payload.Parts))
	for _, item := range payload.Content {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	for _, item := range payload.Parts {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func (s *HTTPServer) assistantTransportOpsManualSearchFallbackArtifact(turnID string, text string) (appui.AiopsTransportAgentUIArtifact, bool) {
	service := s.opsManualService()
	if service == nil || strings.TrimSpace(text) == "" {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	result, err := service.SearchOpsManuals(opsmanual.SearchOpsManualsRequest{Text: text, Limit: 3})
	if err != nil || len(result.Manuals) == 0 || result.Decision == opsmanual.DecisionNoMatch {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return appui.AiopsTransportAgentUIArtifact{}, false
	}
	return assistantTransportOpsManualSearchArtifactFromToolResult(turnID, "fallback-search", runtimekernel.ToolResult{
		ToolCallID: "fallback-search-ops-manuals",
		Content:    string(raw),
		Display: &runtimekernel.ToolDisplayPayload{
			Type:  "ops_manual_search_result",
			Title: "search_ops_manuals",
			Data:  raw,
		},
	})
}

func assistantTransportActionableOpsManualSearchPayload(payload map[string]any) bool {
	decision := strings.TrimSpace(fmt.Sprint(payload["decision"]))
	if decision == string(opsmanual.DecisionNoMatch) {
		return false
	}
	return assistantTransportOpsManualSearchPayloadHasManual(payload)
}

func assistantTransportOpsManualSearchPayloadHasManual(payload map[string]any) bool {
	for _, key := range []string{"manuals", "hits", "matches"} {
		if values, ok := payload[key].([]any); ok && len(values) > 0 {
			return true
		}
	}
	if manual, ok := payload["manual"]; ok && manual != nil {
		return true
	}
	if manualID := strings.TrimSpace(fmt.Sprint(payload["manual_id"])); manualID != "" && manualID != "<nil>" {
		return true
	}
	return false
}

func assistantTransportSessionTurnIsTerminal(session *runtimekernel.SessionState) bool {
	turn := assistantTransportLatestSessionTurn(session)
	if turn == nil {
		return false
	}
	return turn.Lifecycle.IsTerminal()
}

func assistantTransportSessionTurnShouldCloseStream(session *runtimekernel.SessionState) bool {
	turn := assistantTransportLatestSessionTurn(session)
	if turn == nil {
		return false
	}
	return turn.Lifecycle.IsTerminal() || turn.Lifecycle == runtimekernel.TurnLifecycleSuspended || turn.Lifecycle == runtimekernel.TurnLifecycleResumable
}

func assistantTransportLatestSessionTurn(session *runtimekernel.SessionState) *runtimekernel.TurnSnapshot {
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil {
		return session.CurrentTurn
	}
	if len(session.TurnHistory) == 0 {
		return nil
	}
	return &session.TurnHistory[len(session.TurnHistory)-1]
}

func assistantTransportTurnFingerprint(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(turn.ID),
		string(turn.Lifecycle),
		turn.UpdatedAt.UTC().Format(time.RFC3339Nano),
		strings.TrimSpace(turn.FinalOutput),
		strings.TrimSpace(turn.Error),
		assistantTransportTurnCompletedAtFingerprint(turn),
		fmt.Sprintf("%d", len(turn.ContextGovernanceEvents)),
		assistantTransportLatestGovernanceFingerprint(turn),
	}, "|")
}

func assistantTransportLatestGovernanceFingerprint(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil || len(turn.ContextGovernanceEvents) == 0 {
		return ""
	}
	event := turn.ContextGovernanceEvents[len(turn.ContextGovernanceEvents)-1]
	return strings.Join([]string{
		strings.TrimSpace(event.ID),
		string(event.Layer),
		strings.TrimSpace(event.Kind),
		event.CreatedAt.UTC().Format(time.RFC3339Nano),
	}, ":")
}

func assistantTransportTurnCompletedAtFingerprint(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil || turn.CompletedAt == nil {
		return ""
	}
	return turn.CompletedAt.UTC().Format(time.RFC3339Nano)
}

func assistantTransportDiffStateOps(prev, next appui.AiopsTransportState) []assistantTransportStreamStateOp {
	ops := make([]assistantTransportStreamStateOp, 0, 16)
	appendSet := func(path []any, value any) {
		ops = append(ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpSet,
			Path:  path,
			Value: value,
		})
	}
	appendText := func(path []any, value string) {
		if value == "" {
			return
		}
		ops = append(ops, assistantTransportStreamStateOp{
			Type:  assistantTransportStreamOpAppendText,
			Path:  path,
			Value: value,
		})
	}

	if prev.SchemaVersion != next.SchemaVersion {
		appendSet([]any{"schemaVersion"}, next.SchemaVersion)
	}
	if prev.SessionID != next.SessionID {
		appendSet([]any{"sessionId"}, next.SessionID)
	}
	if prev.ThreadID != next.ThreadID {
		appendSet([]any{"threadId"}, next.ThreadID)
	}
	if prev.CurrentTurnID != next.CurrentTurnID {
		appendSet([]any{"currentTurnId"}, next.CurrentTurnID)
	}
	if prev.Status != next.Status {
		appendSet([]any{"status"}, next.Status)
	}
	if prev.Seq != next.Seq {
		appendSet([]any{"seq"}, next.Seq)
	}
	if prev.UpdatedAt != next.UpdatedAt {
		appendSet([]any{"updatedAt"}, next.UpdatedAt)
	}
	if prev.LastError != next.LastError {
		appendSet([]any{"lastError"}, next.LastError)
	}
	if !reflect.DeepEqual(prev.TurnOrder, next.TurnOrder) {
		appendSet([]any{"turnOrder"}, next.TurnOrder)
	}
	if !reflect.DeepEqual(prev.PendingApprovals, next.PendingApprovals) {
		appendSet([]any{"pendingApprovals"}, next.PendingApprovals)
	}
	if !reflect.DeepEqual(prev.McpSurfaces, next.McpSurfaces) {
		appendSet([]any{"mcpSurfaces"}, next.McpSurfaces)
	}
	if !reflect.DeepEqual(prev.Artifacts, next.Artifacts) {
		appendSet([]any{"artifacts"}, next.Artifacts)
	}
	if !reflect.DeepEqual(prev.RuntimeLiveness, next.RuntimeLiveness) {
		appendSet([]any{"runtimeLiveness"}, next.RuntimeLiveness)
	}

	seenTurns := map[string]struct{}{}
	for _, turnID := range next.TurnOrder {
		seenTurns[turnID] = struct{}{}
		nextTurn := next.Turns[turnID]
		prevTurn := prev.Turns[turnID]

		nextTurnForSet := nextTurn
		prevTurnForSet := prevTurn
		nextFinalText := ""
		prevFinalText := ""
		if prevTurnForSet.Final != nil {
			prevFinalText = prevTurnForSet.Final.Text
			finalCopy := *prevTurnForSet.Final
			finalCopy.Text = prevFinalText
			prevTurnForSet.Final = &finalCopy
		}
		if nextTurnForSet.Final != nil {
			nextFinalText = nextTurnForSet.Final.Text
			finalCopy := *nextTurnForSet.Final
			finalCopy.Text = prevFinalText
			nextTurnForSet.Final = &finalCopy
		}
		if !reflect.DeepEqual(prevTurnForSet, nextTurnForSet) {
			appendSet([]any{"turns", turnID}, nextTurnForSet)
		}
		if nextFinalText != prevFinalText {
			if strings.HasPrefix(nextFinalText, prevFinalText) {
				appendText([]any{"turns", turnID, "final", "text"}, nextFinalText[len(prevFinalText):])
			} else {
				appendSet([]any{"turns", turnID, "final", "text"}, "")
				appendText([]any{"turns", turnID, "final", "text"}, nextFinalText)
			}
		}
	}
	for turnID := range next.Turns {
		if _, ok := seenTurns[turnID]; ok {
			continue
		}
		nextTurn := next.Turns[turnID]
		prevTurn := prev.Turns[turnID]
		if !reflect.DeepEqual(prevTurn, nextTurn) {
			appendSet([]any{"turns", turnID}, nextTurn)
		}
	}

	return ops
}

func firstAssistantTransportValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneTransportMetadata(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTransportAnyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func assistantTransportCloneState(state appui.AiopsTransportState) appui.AiopsTransportState {
	cloned := state
	if len(state.TurnOrder) > 0 {
		cloned.TurnOrder = append([]string(nil), state.TurnOrder...)
	}
	cloned.Turns = make(map[string]appui.AiopsTransportTurn, len(state.Turns))
	for key, turn := range state.Turns {
		cloned.Turns[key] = assistantTransportCloneTurn(turn)
	}
	cloned.PendingApprovals = make(map[string]appui.AiopsTransportApproval, len(state.PendingApprovals))
	for key, approval := range state.PendingApprovals {
		cloned.PendingApprovals[key] = approval
	}
	cloned.McpSurfaces = make(map[string]appui.AiopsTransportMcpSurface, len(state.McpSurfaces))
	for key, surface := range state.McpSurfaces {
		cloned.McpSurfaces[key] = surface
	}
	cloned.Artifacts = make(map[string]appui.AiopsTransportArtifact, len(state.Artifacts))
	for key, artifact := range state.Artifacts {
		cloned.Artifacts[key] = artifact
	}
	cloned.RuntimeLiveness = appui.AiopsRuntimeLiveness{
		ActiveTurns:          cloneTransportBoolMap(state.RuntimeLiveness.ActiveTurns),
		ActiveAgents:         cloneTransportBoolMap(state.RuntimeLiveness.ActiveAgents),
		PendingApprovals:     cloneTransportBoolMap(state.RuntimeLiveness.PendingApprovals),
		PendingUserInputs:    cloneTransportBoolMap(state.RuntimeLiveness.PendingUserInputs),
		ActiveCommandStreams: cloneTransportBoolMap(state.RuntimeLiveness.ActiveCommandStreams),
	}
	return cloned
}

func assistantTransportCloneTurn(turn appui.AiopsTransportTurn) appui.AiopsTransportTurn {
	cloned := turn
	if turn.User != nil {
		userCopy := *turn.User
		cloned.User = &userCopy
	}
	if turn.Intent != nil {
		intentCopy := *turn.Intent
		cloned.Intent = &intentCopy
	}
	if turn.Final != nil {
		finalCopy := *turn.Final
		cloned.Final = &finalCopy
	}
	if len(turn.Process) > 0 {
		cloned.Process = make([]appui.AiopsProcessBlock, len(turn.Process))
		for idx, block := range turn.Process {
			cloned.Process[idx] = assistantTransportCloneProcessBlock(block)
		}
	}
	if len(turn.ContextGovernance) > 0 {
		cloned.ContextGovernance = make([]appui.AiopsContextGovernanceEvent, len(turn.ContextGovernance))
		for idx, event := range turn.ContextGovernance {
			cloned.ContextGovernance[idx] = assistantTransportCloneContextGovernanceEvent(event)
		}
	}
	if len(turn.AgentUIArtifacts) > 0 {
		cloned.AgentUIArtifacts = make([]appui.AiopsTransportAgentUIArtifact, len(turn.AgentUIArtifacts))
		for idx, artifact := range turn.AgentUIArtifacts {
			cloned.AgentUIArtifacts[idx] = assistantTransportCloneAgentUIArtifact(artifact)
		}
	}
	return cloned
}

func assistantTransportCloneAgentUIArtifact(artifact appui.AiopsTransportAgentUIArtifact) appui.AiopsTransportAgentUIArtifact {
	cloned := artifact
	cloned.InlineData = cloneTransportAnyMap(artifact.InlineData)
	cloned.Payload = cloneTransportAnyMap(artifact.Payload)
	cloned.Metadata = cloneTransportAnyMap(artifact.Metadata)
	if len(artifact.Actions) > 0 {
		cloned.Actions = make([]map[string]any, len(artifact.Actions))
		for idx, action := range artifact.Actions {
			cloned.Actions[idx] = cloneTransportAnyMap(action)
		}
	}
	return cloned
}

func assistantTransportCloneProcessBlock(block appui.AiopsProcessBlock) appui.AiopsProcessBlock {
	cloned := block
	if len(block.Steps) > 0 {
		cloned.Steps = append([]appui.AiopsTransportPlanStep(nil), block.Steps...)
	}
	if len(block.Queries) > 0 {
		cloned.Queries = append([]string(nil), block.Queries...)
	}
	if len(block.Results) > 0 {
		cloned.Results = append([]appui.AiopsSearchResult(nil), block.Results...)
	}
	if len(block.ExternalReferences) > 0 {
		cloned.ExternalReferences = append([]appui.AiopsExternalReference(nil), block.ExternalReferences...)
	}
	if block.ExitCode != nil {
		exitCode := *block.ExitCode
		cloned.ExitCode = &exitCode
	}
	return cloned
}

func assistantTransportCloneContextGovernanceEvent(event appui.AiopsContextGovernanceEvent) appui.AiopsContextGovernanceEvent {
	cloned := event
	if len(event.Budget) > 0 {
		cloned.Budget = make(map[string]any, len(event.Budget))
		for key, value := range event.Budget {
			cloned.Budget[key] = value
		}
	}
	if len(event.ReferenceIDs) > 0 {
		cloned.ReferenceIDs = append([]string(nil), event.ReferenceIDs...)
	}
	if len(event.CompactedIDs) > 0 {
		cloned.CompactedIDs = append([]string(nil), event.CompactedIDs...)
	}
	if len(event.DroppedGroupIDs) > 0 {
		cloned.DroppedGroupIDs = append([]string(nil), event.DroppedGroupIDs...)
	}
	return cloned
}

func cloneTransportBoolMap(value map[string]bool) map[string]bool {
	if len(value) == 0 {
		return map[string]bool{}
	}
	cloned := make(map[string]bool, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
