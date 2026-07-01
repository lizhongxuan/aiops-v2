package appui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

type ChatArchiveRequest struct {
	OpsRunID  string `json:"opsRunId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type ChatArchiveCaseResult struct {
	OpsRunID string       `json:"opsRunId"`
	Case     IncidentView `json:"case"`
}

type ChatRunRecordResult struct {
	ID        string `json:"id"`
	OpsRunID  string `json:"opsRunId"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type ChatExperienceCandidateResult struct {
	Items []ChatExperienceCandidate `json:"items"`
}

type ChatExperienceCandidate struct {
	ID        string `json:"id"`
	OpsRunID  string `json:"opsRunId"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type ChatArchiveService interface {
	ArchiveCase(context.Context, ChatArchiveRequest) (ChatArchiveCaseResult, error)
	CreateRunRecord(context.Context, ChatArchiveRequest) (ChatRunRecordResult, error)
	CreateExperienceCandidates(context.Context, ChatArchiveRequest) (ChatExperienceCandidateResult, error)
}

type defaultChatArchiveService struct {
	sessions  SessionSource
	incidents IncidentService
	now       func() time.Time
}

func NewChatArchiveService(sessions SessionSource, incidents IncidentService) ChatArchiveService {
	return &defaultChatArchiveService{sessions: sessions, incidents: incidents, now: time.Now}
}

func (s *defaultChatArchiveService) ArchiveCase(ctx context.Context, req ChatArchiveRequest) (ChatArchiveCaseResult, error) {
	if s == nil || s.incidents == nil {
		return ChatArchiveCaseResult{}, fmt.Errorf("incident service is not configured")
	}
	resolved := s.resolve(req)
	if resolved.OpsRunID == "" {
		return ChatArchiveCaseResult{}, fmt.Errorf("opsRunId is required")
	}
	turn := s.findTurn(resolved.SessionID, resolved.TurnID, resolved.OpsRunID)
	incident, err := s.incidents.Create(ctx, IncidentCreateCommand{
		ExternalID: resolved.OpsRunID,
		Title:      firstNonEmptyString(resolved.Title, "Chat 运维处理记录"),
		Source:     "ai_chat",
		Severity:   "info",
	})
	if err != nil {
		return ChatArchiveCaseResult{}, err
	}
	for _, evidence := range archiveEvidenceRefsFromTurn(resolved, turn) {
		if _, err := s.incidents.AddEvidence(ctx, incident.ID, evidence); err != nil {
			return ChatArchiveCaseResult{}, err
		}
	}
	if archived, ok := s.incidents.Get(ctx, incident.ID); ok {
		incident = archived
	}
	return ChatArchiveCaseResult{OpsRunID: resolved.OpsRunID, Case: incident}, nil
}

func (s *defaultChatArchiveService) CreateRunRecord(_ context.Context, req ChatArchiveRequest) (ChatRunRecordResult, error) {
	resolved := s.resolve(req)
	if resolved.OpsRunID == "" {
		return ChatRunRecordResult{}, fmt.Errorf("opsRunId is required")
	}
	createdAt := s.nowTime().UTC().Format(time.RFC3339Nano)
	return ChatRunRecordResult{
		ID:        "run-record-" + safeArchiveIDPart(resolved.OpsRunID),
		OpsRunID:  resolved.OpsRunID,
		Status:    "candidate",
		Title:     firstNonEmptyString(resolved.Title, "Chat 运维处理记录"),
		Summary:   resolved.Summary,
		CreatedAt: createdAt,
	}, nil
}

func (s *defaultChatArchiveService) CreateExperienceCandidates(_ context.Context, req ChatArchiveRequest) (ChatExperienceCandidateResult, error) {
	resolved := s.resolve(req)
	if resolved.OpsRunID == "" {
		return ChatExperienceCandidateResult{}, fmt.Errorf("opsRunId is required")
	}
	createdAt := s.nowTime().UTC().Format(time.RFC3339Nano)
	return ChatExperienceCandidateResult{Items: []ChatExperienceCandidate{{
		ID:        "experience-candidate-" + safeArchiveIDPart(resolved.OpsRunID),
		OpsRunID:  resolved.OpsRunID,
		Status:    "candidate",
		Title:     firstNonEmptyString(resolved.Title, "Chat 运维经验候选"),
		Summary:   resolved.Summary,
		CreatedAt: createdAt,
	}}}, nil
}

func (s *defaultChatArchiveService) resolve(req ChatArchiveRequest) ChatArchiveRequest {
	req.OpsRunID = strings.TrimSpace(req.OpsRunID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	if req.OpsRunID == "" {
		req.OpsRunID = s.findOpsRunID(req.SessionID, req.TurnID)
	}
	if req.Title == "" {
		if turn := s.findTurn(req.SessionID, req.TurnID, req.OpsRunID); turn != nil {
			req.Title = chatRunTraceViewFromMetadata(turn.Metadata, firstUserMessageSummary(turn)).Title
			req.Summary = firstNonEmptyString(req.Summary, strings.TrimSpace(turn.FinalOutput))
		}
	}
	return req
}

func (s *defaultChatArchiveService) findOpsRunID(sessionID, turnID string) string {
	if turn := s.findTurn(sessionID, turnID, ""); turn != nil {
		return strings.TrimSpace(turn.Metadata[metadataOpsRunID])
	}
	return ""
}

func (s *defaultChatArchiveService) findTurn(sessionID, turnID, opsRunID string) *runtimekernel.TurnSnapshot {
	if s == nil || s.sessions == nil {
		return nil
	}
	for _, session := range s.sessions.List() {
		if session == nil || (sessionID != "" && session.ID != sessionID) {
			continue
		}
		if turnMatchesArchive(session.CurrentTurn, turnID, opsRunID) {
			return session.CurrentTurn
		}
		for i := range session.TurnHistory {
			if turnMatchesArchive(&session.TurnHistory[i], turnID, opsRunID) {
				return &session.TurnHistory[i]
			}
		}
	}
	return nil
}

func turnMatchesArchive(turn *runtimekernel.TurnSnapshot, turnID, opsRunID string) bool {
	if turn == nil {
		return false
	}
	if turnID != "" && strings.TrimSpace(turn.ID) != turnID {
		return false
	}
	if opsRunID != "" && strings.TrimSpace(turn.Metadata[metadataOpsRunID]) != opsRunID {
		return false
	}
	return turnID != "" || opsRunID != ""
}

func firstUserMessageSummary(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil {
		return ""
	}
	for _, item := range turn.AgentItems {
		if item.Type == "user_message" {
			return strings.TrimSpace(item.Payload.Summary)
		}
	}
	return ""
}

func archiveEvidenceRefsFromTurn(req ChatArchiveRequest, turn *runtimekernel.TurnSnapshot) []EvidenceRefView {
	if turn == nil {
		return nil
	}
	opsRunID := firstNonEmptyString(req.OpsRunID, strings.TrimSpace(turn.Metadata[metadataOpsRunID]))
	createdAt := turn.UpdatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	out := make([]EvidenceRefView, 0)
	if summary := firstUserMessageSummary(turn); summary != "" {
		out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "user", "ai_chat", "turn:"+turn.ID+"#user", "用户输入："+summary, createdAt))
	}
	if final := strings.TrimSpace(turn.FinalOutput); final != "" {
		out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "diagnosis", "diagnosis", "turn:"+turn.ID+"#final", "诊断/处理摘要："+final, createdAt))
	}
	for _, item := range turn.AgentItems {
		summary := strings.TrimSpace(item.Payload.Summary)
		if summary == "" {
			continue
		}
		switch item.Type {
		case agentstate.TurnItemTypeEvidence:
			out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "agent-evidence-"+item.ID, "agent_evidence", "turn:"+turn.ID+"#item:"+item.ID, summary, archiveItemTime(item, createdAt)))
		case agentstate.TurnItemTypeApproval:
			out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "approval-item-"+item.ID, "approval", "turn:"+turn.ID+"#item:"+item.ID, summary, archiveItemTime(item, createdAt)))
		}
	}
	for _, approval := range turn.PendingApprovals {
		summary := firstNonEmptyString(strings.TrimSpace(approval.Reason), strings.TrimSpace(approval.Command), strings.TrimSpace(approval.ToolName))
		if summary == "" {
			continue
		}
		if risk := strings.TrimSpace(approval.Risk); risk != "" {
			summary = "审批：" + summary + "；风险：" + risk
		} else {
			summary = "审批：" + summary
		}
		out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "approval-"+approval.ID, "approval", "turn:"+turn.ID+"#approval:"+approval.ID, summary, firstNonZeroTime(approval.UpdatedAt, approval.CreatedAt, createdAt)))
	}
	toolNames := toolNamesByCallID(turn)
	for _, iteration := range turn.Iterations {
		for _, result := range iteration.ToolResults {
			toolCallID := strings.TrimSpace(result.ToolCallID)
			summary := archiveToolResultSummary(toolNames[toolCallID], result)
			if summary != "" {
				out = append(out, archiveEvidenceRef(opsRunID, turn.ID, "tool-result-"+toolCallID, "execution_result", "turn:"+turn.ID+"#tool-result:"+toolCallID, summary, firstNonZeroTime(iteration.UpdatedAt, createdAt)))
			}
			for index, ref := range result.References {
				rawRef := firstNonEmptyString(strings.TrimSpace(ref.URI), strings.TrimSpace(ref.FilePath), strings.TrimSpace(ref.CardRef))
				refSummary := firstNonEmptyString(strings.TrimSpace(ref.Summary), strings.TrimSpace(ref.Title), rawRef)
				if rawRef == "" && refSummary == "" {
					continue
				}
				out = append(out, archiveEvidenceRef(opsRunID, turn.ID, fmt.Sprintf("tool-ref-%s-%d", toolCallID, index), "tool_reference", rawRef, refSummary, firstNonZeroTime(iteration.UpdatedAt, createdAt)))
			}
		}
	}
	return dedupeArchiveEvidence(out)
}

func archiveEvidenceRef(opsRunID, turnID, key, source, rawRef, summary string, createdAt time.Time) EvidenceRefView {
	return EvidenceRefView{
		ID:         archiveEvidenceID(opsRunID, turnID, key, rawRef, summary),
		Source:     strings.TrimSpace(source),
		RawRef:     strings.TrimSpace(rawRef),
		Summary:    strings.TrimSpace(summary),
		Confidence: "observed",
		CreatedAt:  createdAt,
	}
}

func archiveEvidenceID(parts ...string) string {
	hash := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return "archive_ev_" + hex.EncodeToString(hash[:])[:16]
}

func archiveItemTime(item agentstate.TurnItem, fallback time.Time) time.Time {
	return firstNonZeroTime(item.UpdatedAt, item.CreatedAt, fallback)
}

func toolNamesByCallID(turn *runtimekernel.TurnSnapshot) map[string]string {
	names := map[string]string{}
	if turn == nil {
		return names
	}
	for _, iteration := range turn.Iterations {
		for _, call := range iteration.ToolCalls {
			if id := strings.TrimSpace(call.ID); id != "" {
				names[id] = strings.TrimSpace(call.Name)
			}
		}
	}
	return names
}

func archiveToolResultSummary(toolName string, result runtimekernel.ToolResult) string {
	summary := firstNonEmptyString(strings.TrimSpace(result.Summary), strings.TrimSpace(result.Error), strings.TrimSpace(result.Content))
	if summary == "" {
		return ""
	}
	if toolName = strings.TrimSpace(toolName); toolName != "" {
		return toolName + "：" + summary
	}
	return summary
}

func dedupeArchiveEvidence(items []EvidenceRefView) []EvidenceRefView {
	seen := map[string]bool{}
	out := make([]EvidenceRefView, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
	}
	return out
}

func (s *defaultChatArchiveService) nowTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func safeArchiveIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", " ", "-", ":", "-")
	return replacer.Replace(value)
}
