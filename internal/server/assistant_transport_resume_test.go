package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

func TestAssistantTransportResumeIdleReturnsCurrentStateWithoutStartingTurn(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-idle", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/assistant/resume", "application/json", bytes.NewReader(assistantTransportResumePayload(t, "sess-idle", "thread-idle")))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/resume error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if !strings.Contains(text, "aui-state:") {
		t.Fatalf("response = %q, want aui-state frame", text)
	}
	if !strings.Contains(text, "\"path\":[],\"value\":{\"schemaVersion\":\"aiops.transport.v1\",\"sessionId\":\"sess-idle\"") {
		t.Fatalf("response = %q, want full idle state set", text)
	}
	if session.CurrentTurn != nil {
		t.Fatalf("CurrentTurn = %+v, want nil; idle resume must not create a turn", session.CurrentTurn)
	}
}

func TestAssistantTransportResumeRunningTurnStreamsCurrentAndFutureState(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-running", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-running",
		SessionID:   "sess-running",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "investigate payment-api"}, CreatedAt: now},
			{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "partial"}, CreatedAt: now},
		},
	}
	sessions.Update(session)

	go func() {
		time.Sleep(25 * time.Millisecond)
		updated := sessions.Get("sess-running")
		completedAt := time.Now().UTC()
		updated.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCompleted
		updated.CurrentTurn.UpdatedAt = completedAt
		updated.CurrentTurn.CompletedAt = &completedAt
		updated.CurrentTurn.AgentItems[1].Status = agentstate.ItemStatusCompleted
		updated.CurrentTurn.AgentItems[1].Payload.Summary = "final answer"
		sessions.Update(updated)
	}()

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/assistant/resume", "application/json", bytes.NewReader(assistantTransportResumePayload(t, "sess-running", "thread-running")))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/resume error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if !strings.Contains(text, "partial") {
		t.Fatalf("response = %q, want current running state", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, want resumed final state", text)
	}
	if !strings.Contains(text, "\"path\":[\"status\"],\"value\":\"idle\"") {
		t.Fatalf("response = %q, want idle state after completed turn", text)
	}
}

func TestAssistantTransportResumeCanceledTurnReturnsCanceledState(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-canceled", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-canceled",
		SessionID:   "sess-canceled",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCanceled,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "stop the run"}, CreatedAt: now},
		},
	}
	sessions.Update(session)

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/assistant/resume", "application/json", bytes.NewReader(assistantTransportResumePayload(t, "sess-canceled", "thread-canceled")))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/resume error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if !strings.Contains(text, "\"status\":\"canceled\"") {
		t.Fatalf("response = %q, want canceled state", text)
	}
	if !strings.Contains(text, "\"turn-canceled\":{\"id\":\"turn-canceled\"") {
		t.Fatalf("response = %q, want canceled turn state", text)
	}
}

func assistantTransportResumePayload(t *testing.T, sessionID, threadID string) []byte {
	t.Helper()
	body := map[string]any{
		"threadId": threadID,
		"state": map[string]any{
			"schemaVersion":    "aiops.transport.v1",
			"sessionId":        sessionID,
			"threadId":         threadID,
			"status":           "idle",
			"turns":            map[string]any{},
			"turnOrder":        []any{},
			"pendingApprovals": map[string]any{},
			"mcpSurfaces":      map[string]any{},
			"artifacts":        map[string]any{},
			"runtimeLiveness":  map[string]any{},
			"seq":              0,
			"updatedAt":        time.Now().UTC().Format(time.RFC3339Nano),
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return payload
}
