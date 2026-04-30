package appui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
)

func NormalizeToolInvocationAgentEvent(inv projection.ToolInvocation) AgentEvent {
	events, _ := NormalizeToolInvocation(inv)
	if len(events) == 0 {
		return AgentEvent{}
	}
	return events[0]
}

func NormalizeToolInvocation(inv projection.ToolInvocation) ([]AgentEvent, error) {
	phase := AgentEventPhaseUpdated
	status := AgentEventStatusRunning
	source := AgentEventSourceTool
	createdAt := inv.StartedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	switch inv.Status {
	case projection.ToolInvocationStarted:
		phase = AgentEventPhaseStarted
		status = AgentEventStatusRunning
	case projection.ToolInvocationProgress:
		phase = AgentEventPhaseUpdated
		status = AgentEventStatusRunning
	case projection.ToolInvocationCompleted:
		phase = AgentEventPhaseCompleted
		status = AgentEventStatusCompleted
		if inv.EndedAt != nil {
			createdAt = *inv.EndedAt
		}
	case projection.ToolInvocationFailed:
		phase = AgentEventPhaseFailed
		status = AgentEventStatusFailed
		if inv.EndedAt != nil {
			createdAt = *inv.EndedAt
		}
	}
	displayKind := displayKindForAgentTool(inv.ToolName)
	inputSummary := summarizeAgentToolInput(inv.ToolName, inv.Args)
	outputSummary := summarizeAgentToolOutput(inv.ToolName, inv.Result, inv.Error)
	budgetSummary, outputPreview, rawRef := summarizeToolResultForEvent(inv.TurnID, inv.ID, inv.Result)
	if outputSummary == "" {
		outputSummary = budgetSummary
	}
	if len(inv.OutputPreview) > 0 {
		outputPreview = append(json.RawMessage(nil), inv.OutputPreview...)
	}
	if strings.TrimSpace(inv.OutputSummary) != "" {
		outputSummary = truncateAgentEventSummary(inv.OutputSummary, 180)
	}
	if strings.TrimSpace(inv.RawRef) != "" {
		rawRef = strings.TrimSpace(inv.RawRef)
	}
	durationMs := int64(0)
	completedAt := ""
	if inv.EndedAt != nil {
		durationMs = inv.EndedAt.Sub(inv.StartedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
		completedAt = inv.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	payload, _ := json.Marshal(ToolPayload{
		ToolCallID:    inv.ID,
		ToolName:      inv.ToolName,
		DisplayName:   displayNameForAgentTool(inv.ToolName),
		DisplayKind:   displayKind,
		Title:         titleForAgentTool(displayKind, phase, inputSummary),
		InputSummary:  inputSummary,
		OutputSummary: outputSummary,
		OutputPreview: outputPreview,
		Foldable:      true,
		AutoCollapse:  status == AgentEventStatusCompleted,
		RawRef:        rawRef,
		DurationMs:    durationMs,
		Error:         inv.Error,
	})
	event := AgentEvent{
		EventID:     stableToolEventID(inv.TurnID, inv.ID, phase, createdAt),
		SessionID:   inv.SessionID,
		TurnID:      inv.TurnID,
		Kind:        AgentEventTool,
		Phase:       phase,
		Status:      status,
		Visibility:  AgentEventVisibilitySecondary,
		Source:      source,
		CreatedAt:   createdAt.UTC().Format(time.RFC3339Nano),
		CompletedAt: completedAt,
		DurationMs:  durationMs,
		Payload:     payload,
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func NormalizeActivity(activity projection.ActivityStats) ([]AgentEvent, error) {
	stage := strings.TrimSpace(activity.Stage)
	if strings.TrimSpace(activity.SessionID) == "" || strings.TrimSpace(activity.TurnID) == "" || stage == "" {
		return nil, nil
	}
	createdAt := time.Now().UTC()
	rowID := fmt.Sprintf("%s:activity:%d:%s", activity.TurnID, activity.Iteration, stage)
	payload, _ := json.Marshal(SystemPayload{
		ID:          rowID,
		DisplayKind: "runtime.activity",
		Title:       runtimeActivityStageTitle(stage),
		Summary:     fmt.Sprintf("第 %d 轮", activity.Iteration+1),
		Stage:       stage,
		Iteration:   activity.Iteration,
	})
	event := AgentEvent{
		EventID:    rowID,
		SessionID:  activity.SessionID,
		TurnID:     activity.TurnID,
		Kind:       AgentEventSystem,
		Phase:      AgentEventPhaseUpdated,
		Status:     AgentEventStatusRunning,
		Visibility: AgentEventVisibilitySecondary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  createdAt.Format(time.RFC3339Nano),
		Payload:    payload,
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func NormalizeCard(card projection.Card) ([]AgentEvent, error) {
	if strings.TrimSpace(card.SessionID) == "" || strings.TrimSpace(card.TurnID) == "" {
		return nil, nil
	}
	createdAt := card.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	cardID := strings.TrimSpace(card.ID)
	if cardID == "" {
		cardID = fmt.Sprintf("%s:card:%d", card.TurnID, createdAt.UnixNano())
	}
	cardType := strings.TrimSpace(card.Type)
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = cardType
	}
	payload, _ := json.Marshal(SystemPayload{
		ID:          cardID,
		DisplayKind: "runtime.card",
		Title:       "生成卡片",
		Summary:     title,
		CardID:      cardID,
		CardType:    cardType,
	})
	event := AgentEvent{
		EventID:    fmt.Sprintf("%s:card:%s", card.TurnID, cardID),
		SessionID:  card.SessionID,
		TurnID:     card.TurnID,
		Kind:       AgentEventSystem,
		Phase:      AgentEventPhaseCompleted,
		Status:     AgentEventStatusCompleted,
		Visibility: AgentEventVisibilitySecondary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		Payload:    payload,
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func runtimeActivityStageTitle(stage string) string {
	switch strings.TrimSpace(stage) {
	case "context_pipeline":
		return "准备上下文"
	case "compile_prompt":
		return "编译提示词"
	case "assemble_tools":
		return "准备工具"
	case "call_model":
		return "调用模型"
	case "dispatch_tools":
		return "执行工具"
	case "finalize_iteration":
		return "整理工具结果"
	default:
		if stage == "" {
			return "处理运行阶段"
		}
		return stage
	}
}

func stableToolEventID(turnID, toolCallID string, phase AgentEventPhase, createdAt time.Time) string {
	base := fmt.Sprintf("%s:tool:%s:%s", turnID, toolCallID, phase)
	if phase == AgentEventPhaseDelta || phase == AgentEventPhaseUpdated {
		return fmt.Sprintf("%s:%d", base, createdAt.UnixNano())
	}
	return base
}

func displayKindForAgentTool(toolName string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "exec_command", "shell_command", "execute_command", "execute_readonly_query", "code_mode":
		return "host.command"
	case "web_search", "search_web":
		return "browser.search"
	case "browse_url", "open_page":
		return "browser.open"
	case "find_in_page":
		return "browser.find"
	case "read_file":
		return "file.read"
	case "write_file", "apply_patch":
		return "file.diff"
	case "list_dir", "list_files":
		return "file.list"
	case "search_files":
		return "file.search"
	case "get_current_model_config", "current_model_config", "get_model_config":
		return "system.inspect"
	default:
		return "mcp.tool"
	}
}

func displayNameForAgentTool(toolName string) string {
	switch displayKindForAgentTool(toolName) {
	case "host.command":
		return "执行主机命令"
	case "browser.search":
		return "搜索网页"
	case "browser.open":
		return "浏览网页"
	case "browser.find":
		return "检索页面"
	case "file.read":
		return "读取文件"
	case "file.diff":
		return "修改文件"
	case "file.list":
		return "浏览目录"
	case "file.search":
		return "搜索文件"
	case "system.inspect":
		return "读取系统配置"
	default:
		name := strings.TrimSpace(toolName)
		if name == "" {
			return "执行工具"
		}
		return name
	}
}

func titleForAgentTool(displayKind string, phase AgentEventPhase, inputSummary string) string {
	running := phase == AgentEventPhaseStarted || phase == AgentEventPhaseUpdated || phase == AgentEventPhaseDelta
	switch displayKind {
	case "host.command":
		if running {
			return "正在执行主机命令"
		}
		return "主机命令已完成"
	case "browser.search":
		if running {
			return "正在搜索网页"
		}
		return "网页搜索已完成"
	case "browser.open":
		if running {
			return "正在浏览网页"
		}
		return "网页浏览已完成"
	case "file.read":
		if running {
			return "正在读取文件"
		}
		return "文件读取已完成"
	case "file.diff":
		if running {
			return "正在修改文件"
		}
		return "文件修改已完成"
	}
	if inputSummary != "" && running {
		return "正在执行工具"
	}
	return "工具执行完成"
}

func rawRefForAgentTool(turnID, toolCallID string) string {
	turnID = strings.TrimSpace(turnID)
	toolCallID = strings.TrimSpace(toolCallID)
	if turnID == "" || toolCallID == "" {
		return ""
	}
	return fmt.Sprintf("tool-result://%s/%s", turnID, toolCallID)
}

func summarizeAgentToolInput(toolName string, raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return truncateAgentEventSummary(trimmed, 180)
	}
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch name {
	case "web_search", "search_web":
		return truncateAgentEventSummary(agentEventStringField(payload, "query"), 180)
	case "browse_url":
		return truncateAgentEventSummary(agentEventStringField(payload, "url"), 180)
	case "exec_command":
		command := agentEventStringField(payload, "command")
		if command == "" {
			command = agentEventStringField(payload, "cmd")
		}
		args := agentEventStringSliceField(payload, "args")
		if command != "" && len(args) > 0 {
			return truncateAgentEventSummary(strings.Join(append([]string{command}, args...), " "), 180)
		}
		return truncateAgentEventSummary(command, 180)
	default:
		for _, key := range []string{"query", "url", "path", "command", "cmd", "title", "name"} {
			if value := agentEventStringField(payload, key); value != "" {
				return truncateAgentEventSummary(value, 180)
			}
		}
		return truncateAgentEventSummary(trimmed, 180)
	}
}

func summarizeAgentToolOutput(toolName, result, errText string) string {
	if strings.TrimSpace(errText) != "" {
		return truncateAgentEventSummary(errText, 180)
	}
	result = strings.TrimSpace(result)
	if result == "" {
		return ""
	}
	var payload map[string]any
	if json.Unmarshal([]byte(result), &payload) == nil {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "web_search", "search_web":
			if summary := summarizeWebSearchOutput(payload); summary != "" {
				return summary
			}
		case "browse_url":
			if url := agentEventStringField(payload, "url"); url != "" {
				return truncateAgentEventSummary("已读取页面："+url, 180)
			}
		case "get_current_model_config", "current_model_config", "get_model_config":
			if model := agentEventStringField(payload, "model"); model != "" {
				provider := agentEventStringField(payload, "provider")
				if provider != "" {
					return truncateAgentEventSummary(provider+" / "+model, 180)
				}
				return truncateAgentEventSummary(model, 180)
			}
		}
		if content := agentEventStringField(payload, "content"); content != "" {
			return truncateAgentEventSummary(firstAgentSummaryLine(content), 180)
		}
	}
	return truncateAgentEventSummary(firstAgentSummaryLine(result), 180)
}

func summarizeWebSearchOutput(payload map[string]any) string {
	content := agentEventStringField(payload, "content")
	if content == "" {
		return ""
	}
	titles := numberedResultTitles(content)
	if len(titles) == 0 {
		return truncateAgentEventSummary(firstAgentSummaryLine(content), 180)
	}
	return truncateAgentEventSummary(fmt.Sprintf("找到 %d 条网页结果：%s", len(titles), titles[0]), 180)
}

func numberedResultTitles(content string) []string {
	var titles []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		dot := strings.Index(line, ".")
		if dot <= 0 || dot > 3 {
			continue
		}
		numberPart := line[:dot]
		isNumber := true
		for _, r := range numberPart {
			if r < '0' || r > '9' {
				isNumber = false
				break
			}
		}
		if !isNumber {
			continue
		}
		title := strings.TrimSpace(line[dot+1:])
		if title != "" {
			titles = append(titles, title)
		}
	}
	return titles
}

func firstAgentSummaryLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(value)
}

func agentEventStringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func agentEventStringSliceField(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" && text != "<nil>" {
			out = append(out, text)
		}
	}
	return out
}

func truncateAgentEventSummary(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func NormalizeRuntimeLifecycleAgentEvent(event runtimekernel.LifecycleEvent) (AgentEvent, bool) {
	events, err := NormalizeRuntimeLifecycleEvent(event)
	if err != nil || len(events) == 0 {
		return AgentEvent{}, false
	}
	return events[0], true
}

func NormalizeRuntimeLifecycleEvent(event runtimekernel.LifecycleEvent) ([]AgentEvent, error) {
	createdAt := event.Timestamp
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	normalized := AgentEvent{
		EventID:    fmt.Sprintf("%s:%s:%d", event.TurnID, event.Type, createdAt.UnixNano()),
		SessionID:  event.SessionID,
		TurnID:     event.TurnID,
		Visibility: AgentEventVisibilitySecondary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		Payload:    normalizePayload(event.Payload),
	}
	switch event.Type {
	case runtimekernel.EventTurnStarted:
		normalized.Kind = AgentEventTurn
		normalized.Phase = AgentEventPhaseStarted
		normalized.Status = AgentEventStatusRunning
		normalized.Visibility = AgentEventVisibilityPrimary
		var payload TurnPayload
		decodeAgentEventPayload(normalized.Payload, &payload)
		normalized.ClientTurnID = payload.ClientTurnID
	case runtimekernel.EventAssistantIntent:
		normalized.Kind = AgentEventAssistant
		normalized.Phase = AgentEventPhaseDelta
		normalized.Status = AgentEventStatusRunning
		normalized.Payload = normalizePayloadWithChannel(event.Payload, "intent")
	case runtimekernel.EventAssistantFinalDelta:
		normalized.Kind = AgentEventAssistant
		normalized.Phase = AgentEventPhaseDelta
		normalized.Status = AgentEventStatusRunning
		normalized.Visibility = AgentEventVisibilityPrimary
		normalized.Payload = normalizePayloadWithChannel(event.Payload, "final")
	case runtimekernel.EventReasoningSummaryDelta:
		normalized.Kind = AgentEventReasoning
		normalized.Phase = AgentEventPhaseDelta
		normalized.Status = AgentEventStatusRunning
		normalized.Visibility = AgentEventVisibilitySecondary
		normalized.Payload = normalizeReasoningSummaryPayload(event.Payload, false)
	case runtimekernel.EventReasoningSummaryCompleted:
		normalized.Kind = AgentEventReasoning
		normalized.Phase = AgentEventPhaseCompleted
		normalized.Status = AgentEventStatusCompleted
		normalized.Visibility = AgentEventVisibilitySecondary
		normalized.Payload = normalizeReasoningSummaryPayload(event.Payload, true)
	case runtimekernel.EventPhaseEnd:
		normalized.Kind = AgentEventSystem
		normalized.Phase = AgentEventPhaseCompleted
		normalized.Status = AgentEventStatusCompleted
	case runtimekernel.EventProcessSummary:
		normalized.Kind = AgentEventAssistant
		normalized.Phase = AgentEventPhaseCompleted
		normalized.Status = AgentEventStatusCompleted
		normalized.Payload = normalizePayloadWithChannel(event.Payload, "summary")
	case runtimekernel.EventTurnComplete:
		normalized.Kind = AgentEventTurn
		normalized.Phase = AgentEventPhaseCompleted
		normalized.Status = AgentEventStatusCompleted
		normalized.Visibility = AgentEventVisibilityPrimary
	case runtimekernel.EventTurnError:
		normalized.Kind = AgentEventTurn
		normalized.Phase = AgentEventPhaseFailed
		normalized.Status = AgentEventStatusFailed
		normalized.Visibility = AgentEventVisibilityPrimary
	case runtimekernel.EventTurnAborted:
		normalized.Kind = AgentEventTurn
		normalized.Phase = AgentEventPhaseCanceled
		normalized.Status = AgentEventStatusCanceled
		normalized.Visibility = AgentEventVisibilityPrimary
	default:
		return nil, nil
	}
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{normalized}, nil
}

func normalizeReasoningSummaryPayload(raw json.RawMessage, completed bool) json.RawMessage {
	var payload ReasoningPayload
	decodeAgentEventPayload(raw, &payload)
	payload.Foldable = true
	if completed {
		payload.AutoCollapse = true
	}
	normalized, _ := json.Marshal(payload)
	return normalized
}

func NormalizeSnapshotCompletedAgentEvent(snapshot projection.Snapshot) AgentEvent {
	events, _ := NormalizeSnapshot(snapshot)
	if len(events) == 0 {
		return AgentEvent{}
	}
	return events[0]
}

func NormalizeSnapshot(snapshot projection.Snapshot) ([]AgentEvent, error) {
	createdAt := snapshot.Timestamp
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	event := AgentEvent{
		EventID:    fmt.Sprintf("%s:turn.completed:%d", snapshot.TurnID, createdAt.UnixNano()),
		SessionID:  snapshot.SessionID,
		TurnID:     snapshot.TurnID,
		Kind:       AgentEventTurn,
		Phase:      AgentEventPhaseCompleted,
		Status:     AgentEventStatusCompleted,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceProjection,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func NormalizeApprovalAgentEvent(approval projection.Approval) AgentEvent {
	events, _ := NormalizeApproval(approval)
	if len(events) == 0 {
		return AgentEvent{}
	}
	return events[0]
}

func NormalizeApproval(approval projection.Approval) ([]AgentEvent, error) {
	createdAt := approval.CreatedAt
	if approval.DecidedAt != nil {
		createdAt = *approval.DecidedAt
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	phase := AgentEventPhaseRequested
	status := AgentEventStatusBlocked
	if approval.Status == projection.ApprovalApproved || approval.Status == projection.ApprovalDenied {
		phase = AgentEventPhaseResolved
		status = AgentEventStatusCompleted
	}
	payload, _ := json.Marshal(ApprovalPayload{
		ApprovalID:   approval.ID,
		ApprovalType: "tool",
		Title:        approval.ToolName,
		Command:      approval.Command,
		Decision:     approval.Decision,
		Targets:      []string{approval.HostID},
	})
	event := AgentEvent{
		EventID:    fmt.Sprintf("%s:approval:%s:%s", approval.TurnID, approval.ID, phase),
		SessionID:  approval.SessionID,
		TurnID:     approval.TurnID,
		Kind:       AgentEventApproval,
		Phase:      phase,
		Status:     status,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceApproval,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		Payload:    payload,
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func NormalizeEvidenceAgentEvent(evidence projection.Evidence) AgentEvent {
	events, _ := NormalizeEvidence(evidence)
	if len(events) == 0 {
		return AgentEvent{}
	}
	return events[0]
}

func NormalizeEvidence(evidence projection.Evidence) ([]AgentEvent, error) {
	createdAt := evidence.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	payload, _ := json.Marshal(EvidencePayload{
		ID:         evidence.ID,
		Kind:       evidence.Type,
		Title:      firstNonEmptyString(evidence.Title, evidence.Type, evidence.ID),
		Summary:    strings.TrimSpace(evidence.Summary),
		Source:     strings.TrimSpace(evidence.Source),
		Confidence: strings.TrimSpace(evidence.Confidence),
		RawRef:     firstNonEmptyString(evidence.RawRef, rawRefForAgentEvidence(evidence.TurnID, evidence.ID)),
		Data:       evidence.Data,
	})
	event := AgentEvent{
		EventID:    fmt.Sprintf("%s:evidence:%s:completed", evidence.TurnID, evidence.ID),
		SessionID:  evidence.SessionID,
		TurnID:     evidence.TurnID,
		Kind:       AgentEventEvidence,
		Phase:      AgentEventPhaseCompleted,
		Status:     AgentEventStatusCompleted,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceProjection,
		CreatedAt:  createdAt.UTC().Format(time.RFC3339Nano),
		Payload:    payload,
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return []AgentEvent{event}, nil
}

func rawRefForAgentEvidence(turnID, evidenceID string) string {
	turnID = strings.TrimSpace(turnID)
	evidenceID = strings.TrimSpace(evidenceID)
	if turnID == "" || evidenceID == "" {
		return ""
	}
	return fmt.Sprintf("evidence://%s/%s", turnID, evidenceID)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizePayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

func normalizePayloadWithChannel(raw json.RawMessage, channel string) json.RawMessage {
	payload := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	payload["channel"] = channel
	encoded, _ := json.Marshal(payload)
	return encoded
}
