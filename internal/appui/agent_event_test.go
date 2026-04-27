package appui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentEventValidateAcceptsCanonicalEnvelope(t *testing.T) {
	event := AgentEvent{
		EventID:    "evt-1",
		Seq:        1,
		SessionID:  "session-1",
		ThreadID:   "thread-1",
		TurnID:     "turn-1",
		Kind:       AgentEventTurn,
		Phase:      AgentEventPhaseStarted,
		Status:     AgentEventStatusRunning,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  "2026-04-24T00:00:00Z",
		Payload:    json.RawMessage(`{"text":"working"}`),
	}

	if err := event.Validate(); err != nil {
		t.Fatalf("valid AgentEvent.Validate() = %v", err)
	}
}

func TestAgentEventValidateRequiresStableEnvelopeFields(t *testing.T) {
	valid := AgentEvent{
		EventID:    "evt-1",
		SessionID:  "session-1",
		Kind:       AgentEventTurn,
		Phase:      AgentEventPhaseStarted,
		Status:     AgentEventStatusRunning,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  "2026-04-24T00:00:00Z",
	}

	tests := []struct {
		name string
		edit func(*AgentEvent)
		want string
	}{
		{name: "event id", edit: func(e *AgentEvent) { e.EventID = "" }, want: "event id is required"},
		{name: "session id", edit: func(e *AgentEvent) { e.SessionID = "" }, want: "session id is required"},
		{name: "kind", edit: func(e *AgentEvent) { e.Kind = "" }, want: "kind is required"},
		{name: "phase", edit: func(e *AgentEvent) { e.Phase = "" }, want: "phase is required"},
		{name: "status", edit: func(e *AgentEvent) { e.Status = "" }, want: "status is required"},
		{name: "visibility", edit: func(e *AgentEvent) { e.Visibility = "" }, want: "visibility is required"},
		{name: "source", edit: func(e *AgentEvent) { e.Source = "" }, want: "source is required"},
		{name: "created at", edit: func(e *AgentEvent) { e.CreatedAt = "" }, want: "createdAt is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.edit(&event)
			err := event.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want containing %q", err.Error(), tt.want)
			}
		})
	}
}

func TestAgentEventValidateRejectsNonCanonicalEnums(t *testing.T) {
	valid := AgentEvent{
		EventID:    "evt-1",
		SessionID:  "session-1",
		Kind:       AgentEventTurn,
		Phase:      AgentEventPhaseStarted,
		Status:     AgentEventStatusRunning,
		Visibility: AgentEventVisibilityPrimary,
		Source:     AgentEventSourceRuntime,
		CreatedAt:  "2026-04-24T00:00:00Z",
	}

	tests := []struct {
		name string
		edit func(*AgentEvent)
		want string
	}{
		{name: "kind", edit: func(e *AgentEvent) { e.Kind = "chat_event" }, want: "invalid kind"},
		{name: "phase", edit: func(e *AgentEvent) { e.Phase = "streaming" }, want: "invalid phase"},
		{name: "status", edit: func(e *AgentEvent) { e.Status = "busy" }, want: "invalid status"},
		{name: "visibility", edit: func(e *AgentEvent) { e.Visibility = "visible" }, want: "invalid visibility"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := valid
			tt.edit(&event)
			err := event.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %q, want containing %q", err.Error(), tt.want)
			}
		})
	}
}

func TestAgentEventCanonicalEnumSets(t *testing.T) {
	for _, kind := range []AgentEventKind{
		AgentEventTurn,
		AgentEventAgent,
		AgentEventAssistant,
		AgentEventTool,
		AgentEventApproval,
		AgentEventArtifact,
		AgentEventDiff,
		AgentEventBrowser,
		AgentEventPlan,
		AgentEventEvidence,
		AgentEventSystem,
	} {
		if !kind.IsValid() {
			t.Fatalf("kind %q should be valid", kind)
		}
	}

	for _, phase := range []AgentEventPhase{
		AgentEventPhaseRequested,
		AgentEventPhaseStarted,
		AgentEventPhaseDelta,
		AgentEventPhaseUpdated,
		AgentEventPhaseCompleted,
		AgentEventPhaseFailed,
		AgentEventPhaseCanceled,
		AgentEventPhaseBlocked,
		AgentEventPhaseResolved,
	} {
		if !phase.IsValid() {
			t.Fatalf("phase %q should be valid", phase)
		}
	}

	for _, status := range []AgentEventStatus{
		AgentEventStatusQueued,
		AgentEventStatusRunning,
		AgentEventStatusWaiting,
		AgentEventStatusBlocked,
		AgentEventStatusCompleted,
		AgentEventStatusFailed,
		AgentEventStatusCanceled,
		AgentEventStatusSkipped,
	} {
		if !status.IsValid() {
			t.Fatalf("status %q should be valid", status)
		}
	}

	for _, visibility := range []AgentEventVisibility{
		AgentEventVisibilityPrimary,
		AgentEventVisibilitySecondary,
		AgentEventVisibilityDebug,
		AgentEventVisibilityHidden,
	} {
		if !visibility.IsValid() {
			t.Fatalf("visibility %q should be valid", visibility)
		}
	}
}
