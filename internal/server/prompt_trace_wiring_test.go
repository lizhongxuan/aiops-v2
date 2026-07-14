package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/runtimekernel"
)

type promptTraceWiringRuntime struct {
	events []modeltrace.CanonicalRolloutEvent
}

func (r promptTraceWiringRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r promptTraceWiringRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r promptTraceWiringRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r promptTraceWiringRuntime) CanonicalRolloutEvents(context.Context, string, string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return append([]modeltrace.CanonicalRolloutEvent(nil), r.events...), nil
}

func TestPromptTraceDefaultServerWiringReadsRuntimeCanonicalRollout(t *testing.T) {
	event, err := modeltrace.FreezeCanonicalRolloutEvent(modeltrace.CanonicalRolloutEvent{
		Sequence: 1, SessionID: "session-wired", TurnID: "turn-wired",
		Kind:    modeltrace.CanonicalRolloutKindAdmission,
		Payload: map[string]any{"factsHash": "sha256:facts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	services := appui.NewServices(promptTraceWiringRuntime{events: []modeltrace.CanonicalRolloutEvent{event}}, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/debug/model-input-traces?includeControlChain=true&sessionId=session-wired&turnId=turn-wired", nil)
	response := httptest.NewRecorder()
	NewHTTPServer(services).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", response.Code, response.Body.String())
	}
	var payload appui.PromptTraceListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ControlChain == nil || !payload.ControlChain.Available || len(payload.ControlChain.Events) != 1 || payload.ControlChain.Events[0].Kind != modeltrace.CanonicalRolloutKindAdmission {
		t.Fatalf("controlChain = %#v", payload.ControlChain)
	}
}
