package main

import (
	"context"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/modeltrace"
)

type aiServerPromptTraceReaderStub struct {
	events []modeltrace.CanonicalRolloutEvent
}

func (s aiServerPromptTraceReaderStub) CanonicalRolloutEvents(context.Context, string, string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return append([]modeltrace.CanonicalRolloutEvent(nil), s.events...), nil
}

func TestAIServerPromptTraceServiceKeepsCanonicalRuntimeReader(t *testing.T) {
	event, err := modeltrace.FreezeCanonicalRolloutEvent(modeltrace.CanonicalRolloutEvent{
		Sequence: 1, SessionID: "session-main", TurnID: "turn-main",
		Kind:    modeltrace.CanonicalRolloutKindAdmission,
		Payload: map[string]any{"factsHash": "sha256:facts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := newAIServerPromptTraceService(t.TempDir(), aiServerPromptTraceReaderStub{events: []modeltrace.CanonicalRolloutEvent{event}})
	response, err := service.ListModelInputTraces(context.Background(), appui.PromptTraceListRequest{
		SessionID: "session-main", TurnID: "turn-main", IncludeControlChain: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ControlChain == nil || !response.ControlChain.Available || len(response.ControlChain.Events) != 1 {
		t.Fatalf("production prompt trace control chain = %#v", response.ControlChain)
	}
}
