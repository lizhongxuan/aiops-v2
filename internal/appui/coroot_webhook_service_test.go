package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/incidents"
)

func TestCorootWebhookServiceCreatesIncidentWithoutRuntimeExecution(t *testing.T) {
	incidentService := NewIncidentService(incidents.NewService(incidents.NewInMemoryStore(), nil))
	service := NewCorootWebhookService(incidentService)

	result, err := service.Handle(context.Background(), CorootWebhookCommand{
		Payload: []byte(`{
			"event":"incident",
			"project":"prod",
			"environment":"prod",
			"incident":{"id":"coroot-inc-1","title":"order-api latency spike"},
			"application":{"name":"order-api"}
		}`),
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Incident.ID == "" || len(result.Incident.EvidenceRefs) != 1 {
		t.Fatalf("result = %#v, want incident with raw evidence", result)
	}
	if result.StartedRuntimeTurn {
		t.Fatalf("StartedRuntimeTurn = true, webhook must only create incident/evidence")
	}
}

func TestCorootWebhookServiceStartChatSendsReadonlyPromptWithoutRCAFlag(t *testing.T) {
	incidentService := NewIncidentService(incidents.NewService(incidents.NewInMemoryStore(), nil))
	created, err := incidentService.Create(context.Background(), IncidentCreateCommand{
		ExternalID:       "coroot-inc-2",
		Title:            "order-api SLO burn",
		Source:           "coroot",
		Environment:      "prod",
		AffectedServices: []string{"order-api"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	chat := &corootWebhookChatStub{response: TurnResponse{SessionID: "sess-coroot", TurnID: "turn-coroot", Status: "accepted"}}
	service := NewCorootWebhookService(incidentService, chat)

	result, err := service.StartChat(context.Background(), CorootWebhookStartChatCommand{IncidentID: created.ID})
	if err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	if !result.StartedRuntimeTurn || result.SessionID != "sess-coroot" {
		t.Fatalf("result = %#v, want started chat turn", result)
	}
	if chat.last.Content == "" || chat.last.Content != result.Prompt {
		t.Fatalf("chat prompt was not sent: %#v", chat.last)
	}
	if chat.last.Metadata[metadataCorootExplicitRCA] == "true" || hasExplicitCorootMention(chat.last.Content) {
		t.Fatalf("start-chat prompt must not trigger explicit RCA: prompt=%q metadata=%#v", chat.last.Content, chat.last.Metadata)
	}
	if chat.last.Metadata["aiops.coroot.webhookIncidentId"] != created.ID {
		t.Fatalf("metadata = %#v, want webhook incident id", chat.last.Metadata)
	}
}

type corootWebhookChatStub struct {
	last     ChatCommand
	response TurnResponse
}

func (s *corootWebhookChatStub) SendMessage(_ context.Context, cmd ChatCommand) (TurnResponse, error) {
	s.last = cmd
	return s.response, nil
}

func (s *corootWebhookChatStub) ResumeTurn(context.Context, ResumeCommand) (TurnResponse, error) {
	return TurnResponse{}, nil
}

func (s *corootWebhookChatStub) CancelTurn(context.Context, CancelCommand) (TurnResponse, error) {
	return TurnResponse{}, nil
}

func (s *corootWebhookChatStub) StopTurn(context.Context, StopCommand) (TurnResponse, error) {
	return TurnResponse{}, nil
}
