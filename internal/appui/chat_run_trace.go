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
	metadataCorootMCPHealthStatus   = "aiops.coroot.mcpHealthStatus"
	metadataCorootSkipReason        = "aiops.coroot.skipReason"
	metadataObservabilityProvider   = "aiops.mentions.observabilityProvider"
	metadataTurnFollowup            = "aiops.turn.followup_of_previous_turn"
	metadataTurnHasExistingEvidence = "aiops.turn.has_existing_evidence"
	metadataTurnNoNewEvidence       = "aiops.turn.no_new_evidence"
	opsRunSourceChat                = "chat"
	defaultOpsRunStatus             = "working"
)

var explicitCorootMentionPattern = regexp.MustCompile(`(?i)(^|[^\pL\pN_])@coroot([^\pL\pN_]|$)`)

type ChatRunTraceView struct {
	ID                 string        `json:"id"`
	SessionID          string        `json:"sessionId,omitempty"`
	TurnID             string        `json:"turnId,omitempty"`
	ClientTurnID       string        `json:"clientTurnId,omitempty"`
	Source             string        `json:"source"`
	Status             string        `json:"status"`
	Title              string        `json:"title,omitempty"`
	RouteMode          string        `json:"routeMode,omitempty"`
	TargetSummary      string        `json:"targetSummary,omitempty"`
	ToolSurfaceSummary string        `json:"toolSurfaceSummary,omitempty"`
	EvidenceCount      int           `json:"evidenceCount,omitempty"`
	CurrentStep        string        `json:"currentStep,omitempty"`
	AgentRun           *AgentRunView `json:"agentRun,omitempty"`
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
	setMetadataIfEmpty(req.Metadata, metadataObservabilityProvider, "coroot")
	if status, ok := explicitCorootMCPHealthStatus(req.Metadata); ok {
		req.Metadata[metadataCorootMCPHealthStatus] = status
		if status != "healthy" {
			req.Metadata[metadataCorootRCADisplayAllowed] = "false"
			req.Metadata[metadataCorootSkipReason] = "mcp_" + status
			return
		}
	}
	setMetadataIfEmpty(req.Metadata, metadataCorootRCADisplayAllowed, "true")
	delete(req.Metadata, metadataCorootSkipReason)
}

func hasExplicitCorootMention(input string) bool {
	return explicitCorootMentionPattern.MatchString(input)
}

func explicitCorootMCPHealthStatus(metadata map[string]string) (string, bool) {
	if len(metadata) == 0 {
		return "", false
	}
	for _, key := range []string{"mcpHealth.coroot", metadataCorootMCPHealthStatus} {
		status := strings.ToLower(strings.TrimSpace(metadata[key]))
		if status != "" {
			return status, true
		}
	}
	return "", false
}

func chatRunTraceViewFromMetadata(metadata map[string]string, fallbackTitle string) ChatRunTraceView {
	opsRunID := strings.TrimSpace(metadata[metadataOpsRunID])
	if opsRunID == "" {
		return ChatRunTraceView{}
	}
	view := ChatRunTraceView{
		ID:                 opsRunID,
		SessionID:          strings.TrimSpace(metadata[metadataSessionID]),
		TurnID:             strings.TrimSpace(metadata[metadataTurnID]),
		ClientTurnID:       strings.TrimSpace(metadata[metadataClientTurn]),
		Source:             firstNonEmptyString(strings.TrimSpace(metadata[metadataChatSource]), opsRunSourceChat),
		Status:             firstNonEmptyString(strings.TrimSpace(metadata["aiops.opsRun.status"]), defaultOpsRunStatus),
		Title:              firstNonEmptyString(strings.TrimSpace(metadata["aiops.opsRun.title"]), summarizeOpsRunTitle(fallbackTitle)),
		RouteMode:          strings.TrimSpace(metadata["aiops.route.mode"]),
		TargetSummary:      strings.TrimSpace(metadata["aiops.target.summary"]),
		ToolSurfaceSummary: toolSurfaceSummaryFromMetadata(metadata),
		CurrentStep:        strings.TrimSpace(metadata["aiops.opsRun.currentStep"]),
	}
	view.AgentRun = BuildAgentRunViewFromTrace(view)
	return view
}

func toolSurfaceSummaryFromMetadata(metadata map[string]string) string {
	if metadata == nil {
		return ""
	}
	var parts []string
	if strings.EqualFold(strings.TrimSpace(metadata["aiops.tool.execCommandAllowed"]), "true") {
		parts = append(parts, "主机执行可用")
	} else {
		parts = append(parts, "无主机执行")
	}
	if strings.Contains(","+strings.TrimSpace(metadata["enableToolPack"])+",", ",public_web,") ||
		strings.EqualFold(strings.TrimSpace(metadata["aiops.weblearn.enabled"]), "true") {
		parts = append(parts, "WebLearn")
	}
	if strings.EqualFold(strings.TrimSpace(metadata["aiops.tool.corootRCAAllowed"]), "true") ||
		strings.EqualFold(strings.TrimSpace(metadata["aiops.route.allowsCorootRCA"]), "true") {
		parts = append(parts, "Coroot RCA")
	}
	if strings.Contains(","+strings.TrimSpace(metadata["enableToolPack"])+",", ",host_ops,") {
		parts = append(parts, "HostOps")
	}
	return strings.Join(parts, " / ")
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
