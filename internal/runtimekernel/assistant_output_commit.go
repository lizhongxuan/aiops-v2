package runtimekernel

import (
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
)

const (
	assistantCommentarySourceModelPrelude      = "model_prelude"
	assistantCommentarySourceRuntimeToolIntent = "runtime_tool_intent"
)

type assistantOutputCommitInput struct {
	TurnID           string
	Iteration        int
	ItemID           string
	MessageID        string
	UserInput        string
	AssistantText    string
	ToolCalls        []ToolCall
	Duration         time.Duration
	FinishReason     string
	EvidenceBoundary string
	BoundaryAction   FinalMessageBoundaryAction
	EvidenceRefs     []string
	FinalContract    *FinalContract
}

type assistantOutputCommitResult struct {
	ItemID             string
	Phase              AssistantMessagePhase
	Text               string
	CommentarySource   string
	Committed          bool
	SuppressedRawDraft bool
}

func commitAssistantOutputForIteration(snapshot *TurnSnapshot, input assistantOutputCommitInput) assistantOutputCommitResult {
	turnID := strings.TrimSpace(input.TurnID)
	if snapshot == nil || turnID == "" {
		return assistantOutputCommitResult{}
	}
	itemID := assistantMessageItemID(turnID, input.Iteration)
	if !hasTypedAssistantToolCalls(input.ToolCalls) {
		return assistantOutputCommitResult{
			ItemID:    itemID,
			Phase:     AssistantMessagePhaseUnclassified,
			Text:      strings.TrimSpace(input.AssistantText),
			Committed: false,
		}
	}

	text, source, suppressed := commentaryTextForToolCalls(input)
	if strings.TrimSpace(text) == "" {
		return assistantOutputCommitResult{
			ItemID:             itemID,
			Phase:              AssistantMessagePhaseCommentary,
			CommentarySource:   source,
			SuppressedRawDraft: suppressed,
		}
	}

	completeAssistantMessageItem(snapshot, itemID, text, assistantMessageData{
		MessageID:        input.MessageID,
		Iteration:        input.Iteration,
		Phase:            AssistantMessagePhaseCommentary,
		StreamState:      AssistantMessageStreamStateComplete,
		TextHash:         debugTextHash(text),
		Duration:         input.Duration,
		CommentarySource: source,
		ToolCallIDs:      toolCallIDsForAssistantCommentary(input.ToolCalls),
	})
	return assistantOutputCommitResult{
		ItemID:             itemID,
		Phase:              AssistantMessagePhaseCommentary,
		Text:               text,
		CommentarySource:   source,
		Committed:          true,
		SuppressedRawDraft: suppressed,
	}
}

func commitFinalAssistantOutput(snapshot *TurnSnapshot, input assistantOutputCommitInput) assistantOutputCommitResult {
	turnID := strings.TrimSpace(input.TurnID)
	text := strings.TrimSpace(input.AssistantText)
	if snapshot == nil || turnID == "" || text == "" {
		return assistantOutputCommitResult{}
	}
	itemID := firstNonEmptyString(strings.TrimSpace(input.ItemID), assistantMessageItemID(turnID, input.Iteration))
	if hasTypedAssistantToolCalls(input.ToolCalls) ||
		EvaluateFinalCompleteness(text, input.FinishReason).Action != "allow" ||
		containsRawToolCallMarkup(strings.ToLower(text)) ||
		!hasStructuredFinalCommitBoundary(input) {
		return assistantOutputCommitResult{
			ItemID:    itemID,
			Phase:     AssistantMessagePhaseUnclassified,
			Text:      text,
			Committed: false,
		}
	}
	data := assistantMessageData{
		MessageID:        input.MessageID,
		Iteration:        input.Iteration,
		Phase:            AssistantMessagePhaseFinalAnswer,
		StreamState:      AssistantMessageStreamStateComplete,
		EvidenceBoundary: input.EvidenceBoundary,
		BoundaryAction:   input.BoundaryAction,
		EvidenceRefs:     input.EvidenceRefs,
		TextHash:         debugTextHash(text),
		Duration:         input.Duration,
		FinalContract:    input.FinalContract,
	}
	completeAssistantMessageItem(snapshot, itemID, text, data)
	appendAgentItem(snapshot, newAgentItem(
		finalResponseItemID(turnID, input.Iteration),
		agentstate.TurnItemTypeFinalResponse,
		agentstate.ItemStatusCompleted,
		text,
		assistantMessageAgentItemData(data),
	))
	return assistantOutputCommitResult{
		ItemID:    itemID,
		Phase:     AssistantMessagePhaseFinalAnswer,
		Text:      text,
		Committed: true,
	}
}

func commentaryTextForToolCalls(input assistantOutputCommitInput) (string, string, bool) {
	raw := strings.TrimSpace(input.AssistantText)
	if hasTypedAssistantToolCalls(input.ToolCalls) &&
		assistantToolCallFinishReasonAllowsCommentary(input.FinishReason) &&
		isAssistantProgressContentAllowed(raw) {
		return raw, assistantCommentarySourceModelPrelude, false
	}
	intent := toolIntentPrelude(input.UserInput, Message{ToolCalls: input.ToolCalls})
	return strings.TrimSpace(intent), assistantCommentarySourceRuntimeToolIntent, raw != ""
}

func hasTypedAssistantToolCalls(calls []ToolCall) bool {
	return len(calls) > 0
}

func assistantToolCallFinishReasonAllowsCommentary(value string) bool {
	switch finalCompletenessFinishReason(value) {
	case "", "tool_calls", "function_call", "tool_use", "stop", "end_turn", "complete", "completed":
		return true
	default:
		return false
	}
}

func hasStructuredFinalCommitBoundary(input assistantOutputCommitInput) bool {
	if input.FinalContract != nil {
		return input.FinalContract.SchemaVersion == FinalContractSchemaVersion && input.FinalContract.Validate() == nil
	}
	action := FinalMessageBoundaryAction(strings.ToLower(strings.TrimSpace(string(input.BoundaryAction))))
	switch action {
	case FinalMessageBoundaryAllow, FinalMessageBoundaryConstrain, FinalMessageBoundaryBlock:
		return strings.TrimSpace(input.EvidenceBoundary) != ""
	default:
		return false
	}
}

func toolCallIDsForAssistantCommentary(calls []ToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		if id := strings.TrimSpace(call.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return compactStringList(ids)
}
