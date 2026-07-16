package appui

import (
	"strings"

	"aiops-v2/internal/agentstate"
)

type assistantChatPresentation string

const (
	assistantChatPresentationHidden     assistantChatPresentation = "hidden"
	assistantChatPresentationCommentary assistantChatPresentation = "commentary"
	assistantChatPresentationFinal      assistantChatPresentation = "final"
)

func assistantMessageChatPresentation(item agentstate.TurnItem, message assistantMessageProjectionPayload) assistantChatPresentation {
	if strings.TrimSpace(message.ReplacedByMessageID) != "" || strings.EqualFold(strings.TrimSpace(message.BoundaryAction), "retry_once") {
		return assistantChatPresentationHidden
	}
	phase := strings.ToLower(strings.TrimSpace(message.Phase))
	switch phase {
	case "unclassified":
		return assistantChatPresentationHidden
	case "final_answer":
		if item.Status == agentstate.ItemStatusCompleted && strings.EqualFold(strings.TrimSpace(message.StreamState), "complete") {
			return assistantChatPresentationFinal
		}
		return assistantChatPresentationHidden
	case "commentary":
		if item.Status == agentstate.ItemStatusCompleted {
			return assistantChatPresentationCommentary
		}
		return assistantChatPresentationHidden
	case "":
		// Compatibility boundary for historical completed assistant items that
		// predate the typed phase. New runtime writes must always set a phase.
		if item.Status == agentstate.ItemStatusCompleted {
			return assistantChatPresentationCommentary
		}
	}
	return assistantChatPresentationHidden
}

func shouldProjectLegacyFinalResponse(turn AiopsTransportTurn, item agentstate.TurnItem) bool {
	return turn.Final == nil && item.Status == agentstate.ItemStatusCompleted
}
