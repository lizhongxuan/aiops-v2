package appui

import (
	"regexp"
	"strings"

	"aiops-v2/internal/runtimekernel"
)

const (
	metadataOpsRunID                = "aiops.opsRunId"
	metadataChatSource              = "aiops.chat.source"
	metadataSessionID               = "aiops.sessionId"
	metadataTurnID                  = "aiops.turnId"
	metadataClientTurn              = "aiops.clientTurnId"
	metadataCorootExplicitRCA       = "aiops.coroot.explicitRCA"
	metadataCorootRCADisplayAllowed = "aiops.coroot.rcaDisplayAllowed"
	opsRunSourceChat                = "chat"
	defaultOpsRunStatus             = "working"
)

var explicitCorootMentionPattern = regexp.MustCompile(`(?i)(^|[^\pL\pN_])@coroot([^\pL\pN_]|$)`)

type ChatRunTraceView struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId,omitempty"`
	TurnID        string `json:"turnId,omitempty"`
	ClientTurnID  string `json:"clientTurnId,omitempty"`
	Source        string `json:"source"`
	Status        string `json:"status"`
	Title         string `json:"title,omitempty"`
	TargetSummary string `json:"targetSummary,omitempty"`
	EvidenceCount int    `json:"evidenceCount,omitempty"`
	CurrentStep   string `json:"currentStep,omitempty"`
}

func ensureOpsRunMetadata(req *runtimekernel.TurnRequest) ChatRunTraceView {
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	setMetadataIfEmpty(req.Metadata, metadataOpsRunID, defaultOpsRunID(req.TurnID))
	setMetadataIfEmpty(req.Metadata, metadataChatSource, opsRunSourceChat)
	setMetadataIfEmpty(req.Metadata, metadataSessionID, req.SessionID)
	setMetadataIfEmpty(req.Metadata, metadataTurnID, req.TurnID)
	setMetadataIfEmpty(req.Metadata, metadataClientTurn, req.ClientTurnID)
	return chatRunTraceViewFromMetadata(req.Metadata, req.Input)
}

func ensureCorootRCAMetadata(req *runtimekernel.TurnRequest) {
	if req == nil || !hasExplicitCorootMention(req.Input) {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	setMetadataIfEmpty(req.Metadata, metadataCorootExplicitRCA, "true")
	setMetadataIfEmpty(req.Metadata, metadataCorootRCADisplayAllowed, "true")
}

func hasExplicitCorootMention(input string) bool {
	return explicitCorootMentionPattern.MatchString(input)
}

func chatRunTraceViewFromMetadata(metadata map[string]string, fallbackTitle string) ChatRunTraceView {
	opsRunID := strings.TrimSpace(metadata[metadataOpsRunID])
	if opsRunID == "" {
		return ChatRunTraceView{}
	}
	return ChatRunTraceView{
		ID:            opsRunID,
		SessionID:     strings.TrimSpace(metadata[metadataSessionID]),
		TurnID:        strings.TrimSpace(metadata[metadataTurnID]),
		ClientTurnID:  strings.TrimSpace(metadata[metadataClientTurn]),
		Source:        firstNonEmptyString(strings.TrimSpace(metadata[metadataChatSource]), opsRunSourceChat),
		Status:        firstNonEmptyString(strings.TrimSpace(metadata["aiops.opsRun.status"]), defaultOpsRunStatus),
		Title:         firstNonEmptyString(strings.TrimSpace(metadata["aiops.opsRun.title"]), summarizeOpsRunTitle(fallbackTitle)),
		TargetSummary: strings.TrimSpace(metadata["aiops.target.summary"]),
		CurrentStep:   strings.TrimSpace(metadata["aiops.opsRun.currentStep"]),
	}
}

func defaultOpsRunID(turnID string) string {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return ""
	}
	if strings.HasPrefix(turnID, "opsrun-") {
		return turnID
	}
	return "opsrun-" + turnID
}

func summarizeOpsRunTitle(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "本次处理"
	}
	const limit = 48
	runes := []rune(input)
	if len(runes) <= limit {
		return input
	}
	return string(runes[:limit]) + "..."
}
