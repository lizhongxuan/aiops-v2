package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/planning"
)

func appendAgentItem(snapshot *TurnSnapshot, item agentstate.TurnItem) {
	if snapshot == nil {
		return
	}
	state := agentStateFromSnapshot(snapshot)
	next, err := agentstate.AppendItem(state, item)
	if err != nil {
		return
	}
	snapshot.AgentItems = next.Items
	snapshot.UpdatedAt = time.Now()
}

func updateAgentItem(snapshot *TurnSnapshot, itemID string, status agentstate.ItemStatus, summary string) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	state := agentStateFromSnapshot(snapshot)
	next, err := agentstate.UpdateItem(state, itemID, func(item agentstate.TurnItem) (agentstate.TurnItem, error) {
		item.Status = status
		if strings.TrimSpace(summary) != "" {
			item.Payload.Summary = strings.TrimSpace(summary)
		}
		return item, nil
	})
	if err != nil {
		return
	}
	snapshot.AgentItems = next.Items
	snapshot.UpdatedAt = time.Now()
}

func updateAgentItemData(snapshot *TurnSnapshot, itemID string, data any) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" || data == nil {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	state := agentStateFromSnapshot(snapshot)
	next, err := agentstate.UpdateItem(state, itemID, func(item agentstate.TurnItem) (agentstate.TurnItem, error) {
		item.Payload.Data = raw
		return item, nil
	})
	if err != nil {
		return
	}
	snapshot.AgentItems = next.Items
	snapshot.UpdatedAt = time.Now()
}

func removeAgentItem(snapshot *TurnSnapshot, itemID string) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" {
		return
	}
	next := snapshot.AgentItems[:0]
	for _, item := range snapshot.AgentItems {
		if item.ID != itemID {
			next = append(next, item)
		}
	}
	snapshot.AgentItems = next
	snapshot.UpdatedAt = time.Now()
}

type approvalAgentItemData struct {
	ApprovalID    string   `json:"approvalId"`
	ToolCallID    string   `json:"toolCallId,omitempty"`
	ToolName      string   `json:"toolName,omitempty"`
	ArgumentsHash string   `json:"argumentsHash,omitempty"`
	TargetRefs    []string `json:"targetRefs,omitempty"`
	Decision      string   `json:"decision,omitempty"`
	Status        string   `json:"status"`
}

func syncPendingApprovalAgentItems(snapshot *TurnSnapshot) {
	if snapshot == nil {
		return
	}
	for _, approval := range snapshot.PendingApprovals {
		appendApprovalRequestedAgentItem(snapshot, approval)
	}
}

func appendApprovalRequestedAgentItem(snapshot *TurnSnapshot, approval PendingApproval) {
	approval.ID = strings.TrimSpace(approval.ID)
	if snapshot == nil || approval.ID == "" {
		return
	}
	itemID := approvalRequestedAgentItemID(snapshot.ID, approval.ID)
	if hasAgentItemID(snapshot.AgentItems, itemID) {
		return
	}
	appendAgentItem(snapshot, newAgentItem(
		itemID,
		agentstate.TurnItemTypeApprovalRequested,
		agentstate.ItemStatusPending,
		firstNonEmptyString(approval.ToolName, "approval requested"),
		approvalAgentItemDataFromApproval(approval, "", "pending"),
	))
}

func appendApprovalDecidedAgentItem(snapshot *TurnSnapshot, approval PendingApproval, decision, status string) {
	if snapshot == nil {
		return
	}
	approval.ID = strings.TrimSpace(approval.ID)
	if approval.ID == "" {
		return
	}
	requestedID := approvalRequestedAgentItemID(snapshot.ID, approval.ID)
	if hasAgentItemID(snapshot.AgentItems, requestedID) {
		updateAgentItem(snapshot, requestedID, agentstate.ItemStatusCompleted, firstNonEmptyString(approval.ToolName, "approval decided"))
	}
	itemID := approvalDecidedAgentItemID(snapshot.ID, approval.ID)
	data := approvalAgentItemDataFromApproval(approval, decision, status)
	if hasAgentItemID(snapshot.AgentItems, itemID) {
		updateAgentItem(snapshot, itemID, agentstate.ItemStatusCompleted, data.Status)
		updateAgentItemData(snapshot, itemID, data)
		return
	}
	appendAgentItem(snapshot, newAgentItem(
		itemID,
		agentstate.TurnItemTypeApprovalDecided,
		agentstate.ItemStatusCompleted,
		data.Status,
		data,
	))
}

func approvalAgentItemDataFromApproval(approval PendingApproval, decision, status string) approvalAgentItemData {
	return approvalAgentItemData{
		ApprovalID:    strings.TrimSpace(approval.ID),
		ToolCallID:    strings.TrimSpace(approval.ToolCallID),
		ToolName:      strings.TrimSpace(approval.ToolName),
		ArgumentsHash: strings.TrimSpace(firstNonEmptyString(approval.ArgumentsHash, approval.InputHash)),
		TargetRefs:    uniqueSortedHarnessStrings(approval.TargetRefs),
		Decision:      strings.TrimSpace(decision),
		Status:        strings.TrimSpace(status),
	}
}

func approvalRequestedAgentItemID(turnID, approvalID string) string {
	return fmt.Sprintf("%s-approval-requested-%s", strings.TrimSpace(turnID), strings.TrimSpace(approvalID))
}

func approvalDecidedAgentItemID(turnID, approvalID string) string {
	return fmt.Sprintf("%s-approval-decided-%s", strings.TrimSpace(turnID), strings.TrimSpace(approvalID))
}

func upsertAssistantMessageItem(snapshot *TurnSnapshot, itemID string, status agentstate.ItemStatus, text string, data assistantMessageData) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" || strings.TrimSpace(text) == "" {
		return
	}
	data.TextHash = firstNonEmptyString(data.TextHash, debugTextHash(text))
	payload := assistantMessageAgentItemData(data)
	if hasAgentItemID(snapshot.AgentItems, itemID) {
		updateAgentItem(snapshot, itemID, status, text)
		updateAgentItemData(snapshot, itemID, payload)
		return
	}
	appendAgentItem(snapshot, newAgentItem(
		itemID,
		agentstate.TurnItemTypeAssistantMessage,
		status,
		text,
		payload,
	))
}

func completeAssistantMessageItem(snapshot *TurnSnapshot, itemID string, text string, data assistantMessageData) {
	data.StreamState = AssistantMessageStreamStateComplete
	upsertAssistantMessageItem(snapshot, itemID, agentstate.ItemStatusCompleted, text, data)
}

func failAssistantMessageItem(snapshot *TurnSnapshot, itemID string, errorText string, data assistantMessageData) {
	data.StreamState = AssistantMessageStreamStateIncomplete
	upsertAssistantMessageItem(snapshot, itemID, agentstate.ItemStatusFailed, errorText, data)
}

func markAssistantMessageReplacedForRetry(snapshot *TurnSnapshot, itemID string, text string, messageID string, iteration int, generationDuration time.Duration, evidenceBoundary string, action FinalMessageBoundaryAction) {
	if snapshot == nil || strings.TrimSpace(itemID) == "" || strings.TrimSpace(text) == "" {
		return
	}
	nextMessageID := assistantMessageItemID(snapshot.ID, iteration+1)
	failAssistantMessageItem(snapshot, itemID, text, assistantMessageData{
		MessageID:           messageID,
		Iteration:           iteration,
		Phase:               AssistantMessagePhaseFinalAnswer,
		StreamState:         AssistantMessageStreamStateIncomplete,
		EvidenceBoundary:    evidenceBoundary,
		BoundaryAction:      action,
		ReplacedByMessageID: nextMessageID,
		TextHash:            debugTextHash(text),
		Duration:            generationDuration,
	})
}

func latestAssistantFinalMessageItem(snapshot *TurnSnapshot) (agentstate.TurnItem, bool) {
	if snapshot == nil {
		return agentstate.TurnItem{}, false
	}
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		if item.Type != agentstate.TurnItemTypeAssistantMessage || item.Status != agentstate.ItemStatusCompleted {
			continue
		}
		payload := agentItemPayloadMap(item)
		if strings.TrimSpace(anyString(payload["phase"])) == string(AssistantMessagePhaseFinalAnswer) {
			return item, true
		}
	}
	return agentstate.TurnItem{}, false
}

func FinalTextFromAssistantMessage(snapshot *TurnSnapshot) string {
	item, ok := latestAssistantFinalMessageItem(snapshot)
	if !ok {
		return ""
	}
	return strings.TrimSpace(item.Payload.Summary)
}

func cancelActiveAgentItems(snapshot *TurnSnapshot) {
	if snapshot == nil {
		return
	}
	for i := range snapshot.AgentItems {
		switch snapshot.AgentItems[i].Status {
		case agentstate.ItemStatusPending, agentstate.ItemStatusRunning, agentstate.ItemStatusBlocked:
			snapshot.AgentItems[i].Status = agentstate.ItemStatusCancelled
			snapshot.AgentItems[i].UpdatedAt = time.Now()
		}
	}
}

func hasAgentItemID(items []agentstate.TurnItem, itemID string) bool {
	for _, item := range items {
		if item.ID == itemID {
			return true
		}
	}
	return false
}

func findAgentItemByID(items []agentstate.TurnItem, itemID string) (agentstate.TurnItem, bool) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return agentstate.TurnItem{}, false
	}
	for _, item := range items {
		if item.ID == itemID {
			return item, true
		}
	}
	return agentstate.TurnItem{}, false
}

func agentItemPayloadMap(item agentstate.TurnItem) map[string]any {
	payload := map[string]any{}
	if len(item.Payload.Data) == 0 {
		return payload
	}
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func agentStateFromSnapshot(snapshot *TurnSnapshot) agentstate.AgentState {
	return agentstate.AgentState{
		SessionID: snapshot.SessionID,
		TurnID:    snapshot.ID,
		Phase:     agentPhaseFromLifecycle(snapshot.Lifecycle),
		Items:     append([]agentstate.TurnItem(nil), snapshot.AgentItems...),
	}
}

func agentPhaseFromLifecycle(lifecycle TurnLifecycleState) agentstate.AgentPhase {
	switch lifecycle {
	case TurnLifecycleCompleted:
		return agentstate.AgentPhaseFinished
	case TurnLifecycleFailed, TurnLifecycleCanceled:
		return agentstate.AgentPhaseFailed
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		return agentstate.AgentPhaseObserving
	default:
		return agentstate.AgentPhaseActing
	}
}

func newAgentItem(id string, typ agentstate.TurnItemType, status agentstate.ItemStatus, summary string, data any) agentstate.TurnItem {
	item := agentstate.TurnItem{
		ID:     id,
		Type:   typ,
		Status: status,
		Payload: agentstate.PayloadEnvelope{
			Summary: strings.TrimSpace(summary),
		},
	}
	if data != nil {
		if raw, err := json.Marshal(data); err == nil {
			item.Payload.Data = raw
		}
	}
	return item
}

func modelCallItemID(turnID string, iteration int) string {
	return fmt.Sprintf("%s-model-%d", strings.TrimSpace(turnID), iteration)
}

func assistantMessageItemID(turnID string, iteration int) string {
	return fmt.Sprintf("%s-assistant-message-%d", strings.TrimSpace(turnID), iteration)
}

func toolCallItemID(turnID string, tc ToolCall) string {
	suffix := strings.TrimSpace(tc.ID)
	if suffix == "" {
		suffix = strings.TrimSpace(tc.Name)
	}
	if suffix == "" {
		suffix = "unknown"
	}
	return fmt.Sprintf("%s-tool-call-%s", strings.TrimSpace(turnID), suffix)
}

func toolResultItemID(turnID string, tc ToolCall) string {
	return toolCallItemID(turnID, tc) + "-result"
}

func errorItemID(turnID string, iteration int) string {
	if iteration >= 0 {
		return fmt.Sprintf("%s-error-%d", strings.TrimSpace(turnID), iteration)
	}
	return fmt.Sprintf("%s-error", strings.TrimSpace(turnID))
}

func finalResponseItemID(turnID string, iteration int) string {
	if iteration >= 0 {
		return fmt.Sprintf("%s-final-response-%d", strings.TrimSpace(turnID), iteration)
	}
	return fmt.Sprintf("%s-final-response", strings.TrimSpace(turnID))
}

func planItemFromToolCall(turnID string, tc ToolCall) (agentstate.TurnItem, bool) {
	if !isUpdatePlanToolName(tc.Name) {
		return agentstate.TurnItem{}, false
	}
	plan, err := planning.DecodeUpdatePlan(tc.Arguments)
	if err != nil {
		return agentstate.TurnItem{}, false
	}
	return newAgentItem(
		planItemID(turnID, tc),
		agentstate.TurnItemTypePlan,
		agentstate.ItemStatusCompleted,
		planning.CompactSummary(plan),
		plan,
	), true
}

func planItemID(turnID string, tc ToolCall) string {
	suffix := strings.TrimSpace(tc.ID)
	if suffix == "" {
		suffix = "update_plan"
	}
	return fmt.Sprintf("%s-plan-%s", strings.TrimSpace(turnID), suffix)
}

func userEvidenceAgentItemFromMetadata(turnID string, metadata map[string]string) (agentstate.TurnItem, bool) {
	if !metadataBool(metadata["aiops.userEvidence.present"]) {
		return agentstate.TurnItem{}, false
	}
	kinds := strings.TrimSpace(metadata["aiops.userEvidence.kinds"])
	signals := strings.TrimSpace(metadata["aiops.userEvidence.signals"])
	excerpt := strings.TrimSpace(metadata["aiops.userEvidence.rawExcerpt"])
	if kinds == "" && signals == "" && excerpt == "" {
		return agentstate.TurnItem{}, false
	}
	ref := fmt.Sprintf("user-evidence:%s", strings.TrimSpace(turnID))
	data := map[string]string{
		"source": "user",
		"ref":    ref,
	}
	parts := []string{"user-provided evidence"}
	if kinds != "" {
		data["kinds"] = kinds
		parts = append(parts, "kinds="+kinds)
	}
	if signals != "" {
		data["signals"] = signals
		parts = append(parts, "signals="+signals)
	}
	if excerpt != "" {
		data["excerpt"] = excerpt
	}
	item := newAgentItem(
		fmt.Sprintf("%s-user-evidence", strings.TrimSpace(turnID)),
		agentstate.TurnItemTypeEvidence,
		agentstate.ItemStatusCompleted,
		strings.Join(parts, "; "),
		data,
	)
	item.Payload.Kind = "user_provided"
	return item, true
}

func completedEvidenceItemIDs(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range snapshot.AgentItems {
		if item.Type != agentstate.TurnItemTypeEvidence || item.Status != agentstate.ItemStatusCompleted {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func isUpdatePlanToolName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "update_plan", "plan_update", "set_plan":
		return true
	default:
		return false
	}
}

func toolResultItemStatus(result ToolResult) agentstate.ItemStatus {
	if strings.TrimSpace(result.Error) != "" {
		return agentstate.ItemStatusFailed
	}
	return agentstate.ItemStatusCompleted
}
