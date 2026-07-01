package runtimekernel

import (
	"strings"
	"time"
)

type AssistantMessagePhase string

const (
	AssistantMessagePhaseCommentary  AssistantMessagePhase = "commentary"
	AssistantMessagePhaseFinalAnswer AssistantMessagePhase = "final_answer"
)

type AssistantMessageStreamState string

const (
	AssistantMessageStreamStateStreaming  AssistantMessageStreamState = "streaming"
	AssistantMessageStreamStateComplete   AssistantMessageStreamState = "complete"
	AssistantMessageStreamStateIncomplete AssistantMessageStreamState = "incomplete"
)

type FinalMessageBoundaryAction string

const (
	FinalMessageBoundaryAllow     FinalMessageBoundaryAction = "allow"
	FinalMessageBoundaryConstrain FinalMessageBoundaryAction = "constrain"
	FinalMessageBoundaryBlock     FinalMessageBoundaryAction = "block"
	FinalMessageBoundaryRetryOnce FinalMessageBoundaryAction = "retry_once"
)

type assistantMessageData struct {
	MessageID           string
	Iteration           int
	Phase               AssistantMessagePhase
	StreamState         AssistantMessageStreamState
	EvidenceBoundary    string
	BoundaryAction      FinalMessageBoundaryAction
	ReplacedByMessageID string
	EvidenceRefs        []string
	TextHash            string
	Duration            time.Duration
	GenerationDuration  time.Duration
	CommentarySource    string
	ToolCallIDs         []string
}

type finalMessageBoundaryInput struct {
	Text                   string
	FinishReason           string
	PendingToolIntent      bool
	FinalEvidenceAction    string
	EvidenceCoverageAction string
	RequiresEvidence       bool
	RequiresPlan           bool
}

type finalMessageBoundaryDecision struct {
	Action            FinalMessageBoundaryAction
	EvidenceBoundary  string
	Retry             bool
	Reasons           []string
	UserVisiblePrefix string
}

func assistantMessageAgentItemData(data assistantMessageData) map[string]any {
	payload := map[string]any{
		"displayKind": "assistant.message",
		"iteration":   data.Iteration,
	}
	if messageID := strings.TrimSpace(data.MessageID); messageID != "" {
		payload["messageId"] = messageID
	}
	if phase := strings.TrimSpace(string(data.Phase)); phase != "" {
		payload["phase"] = phase
	}
	if streamState := strings.TrimSpace(string(data.StreamState)); streamState != "" {
		payload["streamState"] = streamState
	}
	if evidenceBoundary := strings.TrimSpace(data.EvidenceBoundary); evidenceBoundary != "" {
		payload["evidenceBoundary"] = evidenceBoundary
	}
	if boundaryAction := strings.TrimSpace(string(data.BoundaryAction)); boundaryAction != "" {
		payload["boundaryAction"] = boundaryAction
	}
	if replacedByMessageID := strings.TrimSpace(data.ReplacedByMessageID); replacedByMessageID != "" {
		payload["replacedByMessageId"] = replacedByMessageID
	}
	if refs := compactStringList(data.EvidenceRefs); len(refs) > 0 {
		payload["evidenceRefs"] = refs
	}
	if textHash := strings.TrimSpace(data.TextHash); textHash != "" {
		payload["textHash"] = textHash
	}
	if source := strings.TrimSpace(data.CommentarySource); source != "" {
		payload["commentarySource"] = source
	}
	if ids := compactStringList(data.ToolCallIDs); len(ids) > 0 {
		payload["toolCallIds"] = ids
	}
	duration := data.GenerationDuration
	if duration <= 0 {
		duration = data.Duration
	}
	if durationMs := durationMilliseconds(duration); durationMs > 0 {
		payload["durationMs"] = durationMs
	}
	return payload
}

func evaluateFinalMessageBoundary(input finalMessageBoundaryInput) finalMessageBoundaryDecision {
	decision := finalMessageBoundaryDecision{
		Action:           FinalMessageBoundaryAllow,
		EvidenceBoundary: "sufficient",
	}
	finishReason := strings.ToLower(strings.TrimSpace(input.FinishReason))
	if finishReason != "" && finishReason != "stop" {
		decision.Action = FinalMessageBoundaryBlock
		decision.EvidenceBoundary = "blocked"
		decision.Reasons = append(decision.Reasons, "finish_reason_not_stop")
		return decision
	}
	if input.PendingToolIntent {
		decision.Action = FinalMessageBoundaryBlock
		decision.EvidenceBoundary = "blocked"
		decision.Reasons = append(decision.Reasons, "pending_tool_or_process_intent")
		return decision
	}
	switch strings.ToLower(strings.TrimSpace(input.FinalEvidenceAction)) {
	case strings.ToLower(FinalEvidenceActionBlock):
		decision.Action = FinalMessageBoundaryBlock
		decision.EvidenceBoundary = "blocked"
		decision.Reasons = append(decision.Reasons, "final_evidence_block")
		return decision
	case strings.ToLower(FinalEvidenceActionDowngrade):
		decision.Action = FinalMessageBoundaryConstrain
		decision.EvidenceBoundary = "limited"
		decision.Reasons = append(decision.Reasons, "final_evidence_limited")
	}
	if input.RequiresEvidence && strings.TrimSpace(input.EvidenceCoverageAction) != "" && !strings.EqualFold(strings.TrimSpace(input.EvidenceCoverageAction), "allow") {
		decision.Action = FinalMessageBoundaryConstrain
		decision.EvidenceBoundary = "limited"
		decision.Reasons = append(decision.Reasons, "evidence_coverage_limited")
	}
	if input.RequiresPlan && decision.Action == FinalMessageBoundaryAllow {
		decision.Action = FinalMessageBoundaryConstrain
		decision.EvidenceBoundary = "limited"
		decision.Reasons = append(decision.Reasons, "plan_boundary_limited")
	}
	return decision
}
