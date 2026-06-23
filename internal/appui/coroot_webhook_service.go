package appui

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/integrations/coroot"
)

type CorootWebhookCommand struct {
	Payload json.RawMessage `json:"payload"`
}

type CorootWebhookResult struct {
	Incident           IncidentView      `json:"incident"`
	Evidence           []EvidenceRefView `json:"evidence,omitempty"`
	StartedRuntimeTurn bool              `json:"startedRuntimeTurn"`
}

type CorootWebhookStartChatCommand struct {
	IncidentID string `json:"incidentId"`
	SessionID  string `json:"sessionId,omitempty"`
}

type CorootWebhookStartChatResult struct {
	Incident           IncidentView      `json:"incident"`
	SessionID          string            `json:"sessionId"`
	TurnID             string            `json:"turnId"`
	ClientTurnID       string            `json:"clientTurnId,omitempty"`
	Status             string            `json:"status"`
	Prompt             string            `json:"prompt"`
	StartedRuntimeTurn bool              `json:"startedRuntimeTurn"`
	OpsRun             *ChatRunTraceView `json:"opsRun,omitempty"`
}

type CorootWebhookService interface {
	Handle(ctx context.Context, cmd CorootWebhookCommand) (CorootWebhookResult, error)
	StartChat(ctx context.Context, cmd CorootWebhookStartChatCommand) (CorootWebhookStartChatResult, error)
}

type defaultCorootWebhookService struct {
	incidents IncidentService
	chat      ChatService
}

func NewCorootWebhookService(incidents IncidentService, chat ...ChatService) CorootWebhookService {
	var chatService ChatService
	if len(chat) > 0 {
		chatService = chat[0]
	}
	return &defaultCorootWebhookService{incidents: incidents, chat: chatService}
}

func (s *defaultCorootWebhookService) Handle(ctx context.Context, cmd CorootWebhookCommand) (CorootWebhookResult, error) {
	if len(cmd.Payload) == 0 {
		return CorootWebhookResult{}, fmt.Errorf("payload is required")
	}
	event, err := coroot.DecodeWebhook(cmd.Payload)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	mapped, err := coroot.MapWebhookToIncident(event, cmd.Payload)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	incident, err := s.incidents.Create(ctx, mapped.Incident)
	if err != nil {
		return CorootWebhookResult{}, err
	}
	evidence := make([]EvidenceRefView, 0, len(mapped.Evidence))
	for _, item := range mapped.Evidence {
		created, err := s.incidents.AddEvidence(ctx, incident.ID, item)
		if err != nil {
			return CorootWebhookResult{}, err
		}
		evidence = append(evidence, created)
	}
	incident, _ = s.incidents.Get(ctx, incident.ID)
	return CorootWebhookResult{Incident: incident, Evidence: evidence}, nil
}

func (s *defaultCorootWebhookService) StartChat(ctx context.Context, cmd CorootWebhookStartChatCommand) (CorootWebhookStartChatResult, error) {
	if s == nil || s.incidents == nil {
		return CorootWebhookStartChatResult{}, fmt.Errorf("incident service is not configured")
	}
	if s.chat == nil {
		return CorootWebhookStartChatResult{}, fmt.Errorf("chat service is not configured")
	}
	incident, ok := s.incidents.Get(ctx, cmd.IncidentID)
	if !ok {
		return CorootWebhookStartChatResult{}, fmt.Errorf("incident %q not found", cmd.IncidentID)
	}
	prompt := corootWebhookStartChatPrompt(incident)
	turn, err := s.chat.SendMessage(ctx, ChatCommand{
		SessionID:   cmd.SessionID,
		SessionType: "host",
		Mode:        "chat",
		Content:     prompt,
		Role:        "user",
		Metadata: map[string]string{
			"aiops.chat.source":               "coroot_webhook",
			"aiops.coroot.webhookIncidentId":  incident.ID,
			"aiops.coroot.webhookExternalId":  incident.ExternalID,
			"aiops.coroot.webhookEnvironment": incident.Environment,
			"aiops.coroot.webhookService":     firstNonEmptyString(incident.AffectedServices...),
		},
	})
	if err != nil {
		return CorootWebhookStartChatResult{}, err
	}
	return CorootWebhookStartChatResult{
		Incident:           incident,
		SessionID:          turn.SessionID,
		TurnID:             turn.TurnID,
		ClientTurnID:       turn.ClientTurnID,
		Status:             turn.Status,
		Prompt:             prompt,
		StartedRuntimeTurn: true,
		OpsRun:             turn.OpsRun,
	}, nil
}

func corootWebhookStartChatPrompt(incident IncidentView) string {
	alert := firstNonEmptyString(incident.Title, incident.ExternalID, incident.ID)
	service := firstNonEmptyString(incident.AffectedServices...)
	if service == "" {
		service = "相关服务"
	}
	environment := firstNonEmptyString(incident.Environment, "当前环境")
	return fmt.Sprintf(
		"基于 Coroot webhook 收到的 %s，请围绕 %s 在 %s 的异常做只读排查，先采集 Coroot 服务指标、调用链、日志、OpsGraph 邻域和相关主机信息，不要执行修复。若用户后续显式点名 Coroot，再进入 RCA。",
		alert,
		service,
		environment,
	)
}
