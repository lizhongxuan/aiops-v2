package appui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	agentuipkg "aiops-v2/internal/agentui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

// SnapshotBuilder projects runtime sessions into first-party Web DTOs.
type SnapshotBuilder struct {
	hosts    HostRepository
	settings SettingsRepository
}

const serverLocalHostID = "server-local"

func NewSnapshotBuilder(hosts ...HostRepository) *SnapshotBuilder {
	var repo HostRepository
	if len(hosts) > 0 {
		repo = hosts[0]
	}
	return &SnapshotBuilder{hosts: repo}
}

func NewSnapshotBuilderWithSettings(hosts HostRepository, settings SettingsRepository) *SnapshotBuilder {
	return &SnapshotBuilder{hosts: hosts, settings: settings}
}

func (b *SnapshotBuilder) BuildStateSnapshot(session *runtimekernel.SessionState) StateSnapshot {
	snapshot := defaultStateSnapshot()
	b.applyLLMConfig(&snapshot)
	selectedHostID := serverLocalHostID
	if session != nil {
		snapshot.SessionID = session.ID
		snapshot.Kind = mapSessionKind(session.Type)
		snapshot.LastActivityAt = isoStamp(session.UpdatedAt)
		snapshot.Cards = buildCards(session.Messages)
		snapshot.Approvals = buildApprovals(session.PendingApprovals)
		if strings.TrimSpace(session.HostID) != "" {
			selectedHostID = strings.TrimSpace(session.HostID)
		}
		snapshot.Runtime = buildRuntimeSnapshot(session, selectedHostID)
		snapshot.ToolInvocations = buildToolInvocations(session.CurrentTurn)
		snapshot.EvidenceSummaries = buildEvidenceSummaries(session)
		snapshot.CurrentMode = deriveCurrentMode(session)
		snapshot.CurrentStage = deriveCurrentStage(session.CurrentTurn)
		snapshot.CurrentLane = deriveCurrentLane(session)
		snapshot.FinalGateStatus = "pending"
		snapshot.TurnPolicy = buildTurnPolicy(session, snapshot.CurrentLane)
		snapshot.PromptEnvelope = buildPromptEnvelope(session, snapshot.CurrentLane)
		if events := buildAgentItemEvents(session.CurrentTurn); len(events) > 0 {
			snapshot.Config["agentItemEvents"] = events
			if projection, ok := buildAgentItemProjection(session.ID, session.CurrentTurn, events); ok {
				snapshot.AgentEventProjection = &projection
			}
		}
	}
	snapshot.SelectedHostID = selectedHostID
	snapshot.Hosts = b.buildHostSummaries(selectedHostID)
	return snapshot
}

func (b *SnapshotBuilder) applyLLMConfig(snapshot *StateSnapshot) {
	if b == nil || snapshot == nil || b.settings == nil {
		return
	}
	cfg, err := b.settings.GetLLMConfig()
	if err != nil || cfg == nil {
		return
	}
	if model := strings.TrimSpace(cfg.Model); model != "" {
		snapshot.Config["model"] = model
	}
}

func buildAgentItemEvents(turn *runtimekernel.TurnSnapshot) []AgentEvent {
	if turn == nil || len(turn.AgentItems) == 0 {
		return nil
	}
	return agentuipkg.ProjectTurnItemsToAgentEvents(turn.SessionID, turn.ID, turn.AgentItems, 0)
}

func buildAgentItemProjection(sessionID string, turn *runtimekernel.TurnSnapshot, events []AgentEvent) (AgentEventProjection, bool) {
	if strings.TrimSpace(sessionID) == "" || len(events) == 0 {
		return AgentEventProjection{}, false
	}
	projection, err := NewAgentEventProjector().Replay(sessionID, events)
	if err != nil {
		return AgentEventProjection{}, false
	}
	projection = sanitizeAgentItemProjectionForTurn(projection, turn)
	return projection, true
}

func sanitizeAgentItemProjectionForTurn(proj AgentEventProjection, turn *runtimekernel.TurnSnapshot) AgentEventProjection {
	if turn == nil || !turn.Lifecycle.IsTerminal() {
		return proj
	}
	status := terminalAgentEventStatusForTurnLifecycle(turn.Lifecycle)
	updatedAt := isoStamp(turn.UpdatedAt)
	if updatedAt == "" {
		updatedAt = isoStamp(turn.StartedAt)
	}
	return sanitizeTerminalAgentEventProjection(proj, status, turn.Lifecycle == runtimekernel.TurnLifecycleFailed, updatedAt, turn.ID)
}

func SanitizeAgentEventProjectionForSnapshot(proj AgentEventProjection, snapshot StateSnapshot) AgentEventProjection {
	if snapshot.Runtime.Turn.Active {
		return proj
	}
	status, failed, ok := terminalAgentEventStatusForRuntimePhase(snapshot.Runtime.Turn.Phase)
	if !ok {
		return proj
	}
	if projectionSessionID := strings.TrimSpace(proj.SessionID); projectionSessionID != "" && strings.TrimSpace(snapshot.SessionID) != "" && projectionSessionID != strings.TrimSpace(snapshot.SessionID) {
		return proj
	}
	return sanitizeTerminalAgentEventProjection(proj, status, failed, "", "")
}

func sanitizeTerminalAgentEventProjection(proj AgentEventProjection, status AgentEventStatus, terminalFailed bool, updatedAt string, turnID string) AgentEventProjection {
	proj = ensureAgentEventProjection(proj)
	if strings.TrimSpace(turnID) == "" {
		turnID = strings.TrimSpace(proj.CurrentTurnID)
	}
	pending := cloneBoolMap(proj.RuntimeLiveness.PendingApprovals)
	for id := range proj.RuntimeLiveness.PendingUserInputs {
		pending[id] = true
	}
	proj.RuntimeLiveness.ActiveTurns = map[string]bool{}
	proj.RuntimeLiveness.ActiveAgents = map[string]bool{}
	proj.RuntimeLiveness.PendingApprovals = map[string]bool{}
	proj.RuntimeLiveness.PendingUserInputs = map[string]bool{}
	proj.RuntimeLiveness.ActiveCommandStreams = map[string]bool{}
	for i := range proj.Approvals {
		if proj.Approvals[i].Status != AgentEventStatusBlocked && !pending[proj.Approvals[i].ID] {
			continue
		}
		proj.Approvals[i].Status = status
		if updatedAt != "" {
			proj.Approvals[i].UpdatedAt = updatedAt
		}
	}
	if turnID != "" && proj.FinalMessages != nil {
		if final, ok := proj.FinalMessages[turnID]; ok {
			final.Status = status
			if updatedAt != "" {
				final.UpdatedAt = updatedAt
			}
			proj.FinalMessages[turnID] = final
		}
	}
	proj.LastTerminalFailed = terminalFailed
	proj.Status = deriveProjectionStatus(proj.RuntimeLiveness, proj.Diff, proj.LastTerminalFailed)
	return proj
}

func terminalAgentEventStatusForTurnLifecycle(lifecycle runtimekernel.TurnLifecycleState) AgentEventStatus {
	switch lifecycle {
	case runtimekernel.TurnLifecycleFailed:
		return AgentEventStatusFailed
	case runtimekernel.TurnLifecycleCanceled:
		return AgentEventStatusCanceled
	default:
		return AgentEventStatusCompleted
	}
}

func terminalAgentEventStatusForRuntimePhase(phase string) (AgentEventStatus, bool, bool) {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "failed":
		return AgentEventStatusFailed, true, true
	case "aborted", "canceled", "cancelled":
		return AgentEventStatusCanceled, false, true
	case "completed":
		return AgentEventStatusCompleted, false, true
	default:
		return "", false, false
	}
}

func (b *SnapshotBuilder) BuildSessionSummary(session *runtimekernel.SessionState) SessionSummary {
	if session == nil {
		return SessionSummary{
			Kind:           "single_host",
			SelectedHostID: "server-local",
			Title:          "新建会话",
			Preview:        "暂无消息",
			Status:         "empty",
		}
	}
	snapshot := b.BuildStateSnapshot(session)
	title := "新建会话"
	preview := "暂无消息"
	messageCount := 0
	for _, card := range snapshot.Cards {
		if isUserCard(card) || isAssistantCard(card) {
			messageCount += 1
		}
		if title == "新建会话" && isUserCard(card) {
			if text := compactCardText(card); text != "" {
				title = truncateText(text, 24)
			}
		}
	}
	for i := len(snapshot.Cards) - 1; i >= 0; i-- {
		if text := compactCardText(snapshot.Cards[i]); text != "" {
			preview = truncateText(text, 60)
			break
		}
	}
	return SessionSummary{
		ID:             session.ID,
		Kind:           snapshot.Kind,
		Title:          title,
		Preview:        preview,
		SelectedHostID: snapshot.SelectedHostID,
		Status:         deriveSessionStatus(snapshot.Cards, snapshot.Runtime),
		MessageCount:   messageCount,
		LastActivityAt: snapshot.LastActivityAt,
	}
}

func (b *SnapshotBuilder) SortSessions(sessions []*runtimekernel.SessionState) []*runtimekernel.SessionState {
	cloned := append([]*runtimekernel.SessionState(nil), sessions...)
	sort.SliceStable(cloned, func(i, j int) bool {
		return cloned[i].UpdatedAt.After(cloned[j].UpdatedAt)
	})
	return cloned
}

func defaultStateSnapshot() StateSnapshot {
	return StateSnapshot{
		Kind:                "single_host",
		SelectedHostID:      serverLocalHostID,
		Auth:                AuthSummary{Connected: false},
		Hosts:               []HostSummary{defaultServerLocalHost()},
		Cards:               []CardView{},
		Approvals:           []ApprovalView{},
		ToolInvocations:     []ToolInvocationView{},
		EvidenceSummaries:   []EvidenceSummaryView{},
		MissingRequirements: []string{},
		TurnPolicy: TurnPolicyView{
			FinalGateStatus: "pending",
		},
		PromptEnvelope: PromptEnvelopeView{
			RuntimePolicy:       PromptEnvelopeSectionView{},
			CompressionState:    "inline",
			FinalGateStatus:     "pending",
			MissingRequirements: []string{},
		},
		Config: map[string]any{"codexAlive": true},
		Runtime: RuntimeSnapshot{
			Turn: RuntimeTurnSnapshot{
				Active: false,
				Phase:  "idle",
				HostID: serverLocalHostID,
			},
			Codex: map[string]any{
				"status":       "connected",
				"retryAttempt": 0,
				"retryMax":     5,
				"lastError":    "",
			},
			Activity: map[string]any{},
		},
	}
}

func mapSessionKind(sessionType runtimekernel.SessionType) string {
	if sessionType == runtimekernel.SessionTypeWorkspace {
		return "workspace"
	}
	return "single_host"
}

func buildCards(messages []runtimekernel.Message) []CardView {
	cards := make([]CardView, 0, len(messages))
	for _, message := range messages {
		text := strings.TrimSpace(message.Content)
		cardType := "MessageCard"
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "user":
			cardType = "UserMessageCard"
		case "assistant":
			cardType = "AssistantMessageCard"
		}
		card := CardView{
			ID:              message.ID,
			ClientMessageID: message.ClientMessageID,
			ClientTurnID:    message.ClientTurnID,
			Type:            cardType,
			Role:            message.Role,
			Text:            text,
			Message:         text,
			Timestamp:       isoStamp(message.Timestamp),
		}
		if message.ToolResult != nil {
			card.Summary = strings.TrimSpace(firstNonEmpty(message.ToolResult.Summary, message.ToolResult.Content, message.ToolResult.Error))
			if card.Text == "" {
				card.Text = card.Summary
				card.Message = card.Summary
			}
		}
		if card.ID == "" {
			card.ID = fmt.Sprintf("%s-%s", strings.TrimSpace(message.Role), card.Timestamp)
		}
		cards = append(cards, card)
	}
	return cards
}

func buildApprovals(pending []runtimekernel.PendingApproval) []ApprovalView {
	approvals := make([]ApprovalView, 0, len(pending))
	for _, approval := range pending {
		approvals = append(approvals, ApprovalView{
			ID:        approval.ID,
			SessionID: approval.SessionID,
			TurnID:    approval.TurnID,
			ToolName:  approval.ToolName,
			Command:   strings.TrimSpace(firstNonEmpty(approval.Command, approval.Reason)),
			Reason:    strings.TrimSpace(approval.Reason),
			HostID:    approval.HostID,
			Status:    "pending",
			CreatedAt: isoStamp(approval.CreatedAt),
		})
	}
	return approvals
}

func buildToolInvocations(turn *runtimekernel.TurnSnapshot) []ToolInvocationView {
	if turn == nil {
		return []ToolInvocationView{}
	}
	evidenceByToolCall := map[string]string{}
	approvalByToolCall := map[string]string{}
	for _, evidence := range turn.PendingEvidence {
		if toolCallID := strings.TrimSpace(evidence.ToolCallID); toolCallID != "" {
			evidenceByToolCall[toolCallID] = evidence.ID
		}
	}
	for _, approval := range turn.PendingApprovals {
		if toolCallID := strings.TrimSpace(approval.ToolCallID); toolCallID != "" {
			approvalByToolCall[toolCallID] = approval.ID
		}
	}
	items := make([]ToolInvocationView, 0)
	for _, iteration := range turn.Iterations {
		resultsByCall := map[string]runtimekernel.ToolResult{}
		for _, result := range iteration.ToolResults {
			if toolCallID := strings.TrimSpace(result.ToolCallID); toolCallID != "" {
				resultsByCall[toolCallID] = result
			}
		}
		for _, call := range iteration.ToolCalls {
			inputJSON := compactJSON(call.Arguments)
			result, hasResult := resultsByCall[call.ID]
			status := deriveInvocationStatus(call.ID, hasResult, result, evidenceByToolCall, approvalByToolCall)
			outputJSON := ""
			outputSummary := ""
			if hasResult {
				outputJSON = compactJSON(marshalResultSummary(result))
				outputSummary = strings.TrimSpace(firstNonEmpty(result.Summary, result.Content, result.Error))
			}
			inputSummary := summarizeToolArguments(inputJSON)
			hostID := hostIDFromToolInput(inputJSON)
			completedAt := ""
			if hasResult {
				completedAt = isoStamp(iteration.UpdatedAt)
			}
			items = append(items, ToolInvocationView{
				ID:            call.ID,
				Name:          call.Name,
				DisplayName:   call.Name,
				Status:        status,
				InputJSON:     inputJSON,
				OutputJSON:    outputJSON,
				InputSummary:  inputSummary,
				OutputSummary: outputSummary,
				HostID:        hostID,
				ApprovalID:    approvalByToolCall[call.ID],
				EvidenceID:    evidenceByToolCall[call.ID],
				StartedAt:     isoStamp(iteration.StartedAt),
				CompletedAt:   completedAt,
			})
		}
	}
	return items
}

func buildEvidenceSummaries(session *runtimekernel.SessionState) []EvidenceSummaryView {
	if session == nil {
		return []EvidenceSummaryView{}
	}
	items := make([]EvidenceSummaryView, 0, len(session.PendingEvidence))
	for _, evidence := range session.PendingEvidence {
		items = append(items, EvidenceSummaryView{
			ID:           evidence.ID,
			InvocationID: evidence.ToolCallID,
			SourceKind:   "tool_invocation",
			SourceRef:    evidence.ToolCallID,
			Kind:         "pending_evidence",
			Title:        strings.TrimSpace(firstNonEmpty(evidence.ToolName, evidence.ID)),
			Summary:      strings.TrimSpace(firstNonEmpty(evidence.Reason, evidence.ToolName)),
			Content:      evidence.Reason,
			HostID:       session.HostID,
			HostName:     session.HostID,
			Metadata: map[string]any{
				"toolName":     evidence.ToolName,
				"status":       evidence.Status,
				"hostId":       session.HostID,
				"invocationId": evidence.ToolCallID,
			},
			CreatedAt: isoStamp(evidence.CreatedAt),
		})
	}
	return items
}

func buildRuntimeSnapshot(session *runtimekernel.SessionState, selectedHostID string) RuntimeSnapshot {
	snapshot := defaultStateSnapshot().Runtime
	snapshot.Turn.HostID = selectedHostID
	if session == nil || session.CurrentTurn == nil {
		return snapshot
	}
	snapshot.Turn.Active = !session.CurrentTurn.Lifecycle.IsTerminal()
	snapshot.Turn.Phase = mapTurnPhase(session.CurrentTurn)
	snapshot.Turn.ClientTurnID = session.CurrentTurn.ClientTurnID
	snapshot.Turn.ClientMessageID = session.CurrentTurn.ClientMessageID
	return snapshot
}

func (b *SnapshotBuilder) buildHostSummaries(selectedHostID string) []HostSummary {
	hosts := []HostSummary{defaultServerLocalHost()}
	seen := map[string]struct{}{serverLocalHostID: {}}

	if b != nil && b.hosts != nil {
		records, err := b.hosts.ListHosts()
		if err == nil {
			sort.SliceStable(records, func(i, j int) bool {
				return records[i].ID < records[j].ID
			})
			for _, record := range records {
				id := strings.TrimSpace(record.ID)
				if id == "" {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				hosts = append(hosts, mapHostRecord(record))
				seen[id] = struct{}{}
			}
		}
	}

	selectedHostID = strings.TrimSpace(selectedHostID)
	if selectedHostID != "" {
		if _, ok := seen[selectedHostID]; !ok {
			hosts = append(hosts, fallbackSelectedHost(selectedHostID))
		}
	}
	return hosts
}

func defaultServerLocalHost() HostSummary {
	return HostSummary{
		ID:              serverLocalHostID,
		Name:            serverLocalHostID,
		Status:          "online",
		Kind:            "local",
		Address:         serverLocalHostID,
		Transport:       "local",
		Executable:      true,
		TerminalCapable: true,
		ControlMode:     "local",
	}
}

func fallbackSelectedHost(hostID string) HostSummary {
	record := store.HostRecord{
		ID:            hostID,
		Name:          hostID,
		Status:        "offline",
		Address:       hostID,
		Transport:     "inventory",
		InstallState:  "inventory",
		ControlMode:   "inventory",
		LastHeartbeat: "offline",
	}
	return mapHostRecord(record)
}

func mapTurnPhase(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return "idle"
	}
	if len(turn.PendingApprovals) > 0 {
		return "waiting_approval"
	}
	switch turn.Lifecycle {
	case runtimekernel.TurnLifecycleCompleted:
		return "completed"
	case runtimekernel.TurnLifecycleFailed:
		return "failed"
	case runtimekernel.TurnLifecycleCanceled:
		return "aborted"
	case runtimekernel.TurnLifecycleSuspended, runtimekernel.TurnLifecycleResumable:
		return "waiting_input"
	case runtimekernel.TurnLifecyclePending:
		return "thinking"
	default:
		return "executing"
	}
}

func deriveCurrentMode(session *runtimekernel.SessionState) string {
	if session == nil {
		return ""
	}
	switch session.Mode {
	case runtimekernel.ModeExecute:
		return "execute"
	default:
		return "analysis"
	}
}

func deriveCurrentStage(turn *runtimekernel.TurnSnapshot) string {
	phase := mapTurnPhase(turn)
	switch phase {
	case "waiting_approval":
		return "waiting_action_approval"
	case "waiting_input":
		return "waiting_input"
	case "completed":
		return "completed"
	case "failed":
		return "failed"
	case "aborted":
		return "canceled"
	case "thinking":
		return "understanding"
	case "executing":
		return "executing"
	default:
		return ""
	}
}

func deriveCurrentLane(session *runtimekernel.SessionState) string {
	if session == nil {
		return ""
	}
	switch session.Mode {
	case runtimekernel.ModeExecute:
		return "execute"
	case runtimekernel.ModePlan:
		return "plan"
	case runtimekernel.ModeInspect:
		return "readonly"
	default:
		return "answer"
	}
}

func buildTurnPolicy(session *runtimekernel.SessionState, lane string) TurnPolicyView {
	if session == nil || session.CurrentTurn == nil {
		return defaultStateSnapshot().TurnPolicy
	}
	return TurnPolicyView{
		IntentClass:           deriveIntentClass(session.Mode),
		Lane:                  lane,
		NeedsApproval:         len(session.PendingApprovals) > 0,
		RequiresExternalFacts: strings.TrimSpace(session.CurrentTurn.GovernanceSnapshot) != "",
		FinalGateStatus:       "pending",
		ClassificationReason:  strings.TrimSpace(session.CurrentTurn.GovernanceSnapshot),
		UpdatedAt:             isoStamp(session.CurrentTurn.UpdatedAt),
	}
}

func buildPromptEnvelope(session *runtimekernel.SessionState, lane string) PromptEnvelopeView {
	envelope := defaultStateSnapshot().PromptEnvelope
	if session == nil || session.CurrentTurn == nil {
		return envelope
	}
	turn := session.CurrentTurn
	envelope.CurrentLane = lane
	envelope.IntentClass = deriveIntentClass(session.Mode)
	envelope.UpdatedAt = isoStamp(turn.UpdatedAt)
	envelope.RuntimePolicy = PromptEnvelopeSectionView{
		Name:    "Runtime Policy",
		Content: strings.TrimSpace(turn.GovernanceSnapshot),
	}
	for _, name := range turn.PromptSections {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		envelope.StaticSections = append(envelope.StaticSections, PromptEnvelopeSectionView{Name: name})
	}
	if latest := latestTurnIteration(turn); latest != nil {
		envelope.VisibleTools = buildPromptEnvelopeTools(latest.VisibleTools)
		if len(turn.CompactedSegments) > 0 || len(session.CompactedSegments) > 0 {
			envelope.CompressionState = "compacted"
		}
	}
	envelope.HiddenTools = buildPromptEnvelopeTools(turn.HiddenTools)
	return envelope
}

func buildPromptEnvelopeTools(names []string) []PromptEnvelopeToolView {
	items := make([]PromptEnvelopeToolView, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		items = append(items, PromptEnvelopeToolView{
			Name:        name,
			DisplayName: name,
		})
	}
	return items
}

func latestTurnIteration(turn *runtimekernel.TurnSnapshot) *runtimekernel.IterationState {
	if turn == nil || len(turn.Iterations) == 0 {
		return nil
	}
	return &turn.Iterations[len(turn.Iterations)-1]
}

func deriveIntentClass(mode runtimekernel.Mode) string {
	switch mode {
	case runtimekernel.ModeExecute:
		return "risky_exec"
	case runtimekernel.ModePlan:
		return "design"
	case runtimekernel.ModeInspect:
		return "research"
	default:
		return "factual"
	}
}

func deriveInvocationStatus(toolCallID string, hasResult bool, result runtimekernel.ToolResult, evidenceByToolCall, approvalByToolCall map[string]string) string {
	if toolCallID != "" {
		if approvalByToolCall[toolCallID] != "" {
			return "waiting_approval"
		}
		if evidenceByToolCall[toolCallID] != "" {
			return "waiting_user"
		}
	}
	if hasResult {
		if strings.TrimSpace(result.Error) != "" {
			return "failed"
		}
		return "completed"
	}
	return "started"
}

func marshalResultSummary(result runtimekernel.ToolResult) any {
	return map[string]any{
		"content": result.Content,
		"summary": result.Summary,
		"error":   result.Error,
	}
}

func compactJSON(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case json.RawMessage:
		return strings.TrimSpace(string(typed))
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

func summarizeToolArguments(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 180 {
		return raw
	}
	return raw[:180]
}

func hostIDFromToolInput(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ""
	}
	hostID := strings.TrimSpace(fmt.Sprint(payload["hostId"]))
	if hostID == "<nil>" {
		return ""
	}
	return hostID
}

func compactCardText(card CardView) string {
	return strings.TrimSpace(firstNonEmpty(card.Text, card.Message, card.Summary))
}

func deriveSessionStatus(cards []CardView, runtime RuntimeSnapshot) string {
	if runtime.Turn.Active {
		if runtime.Turn.Phase == "waiting_approval" {
			return "waiting_approval"
		}
		return "running"
	}
	if len(cards) == 0 {
		return "empty"
	}
	for i := len(cards) - 1; i >= 0; i-- {
		card := cards[i]
		if card.Type == "ErrorCard" {
			return "failed"
		}
		if isUserCard(card) || isAssistantCard(card) || card.Type == "NoticeCard" {
			break
		}
	}
	return "completed"
}

func isUserCard(card CardView) bool {
	return card.Type == "UserMessageCard" || (card.Type == "MessageCard" && card.Role == "user")
}

func isAssistantCard(card CardView) bool {
	return card.Type == "AssistantMessageCard" || (card.Type == "MessageCard" && card.Role == "assistant")
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
