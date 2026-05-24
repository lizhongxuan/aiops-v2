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
	if !strings.Contains(text, "\"path\":[],\"value\":{\"schemaVersion\":\"aiops.transport.v2\",\"sessionId\":\"sess-idle\"") {
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

func TestAssistantTransportResumeProjectsCorootChartArtifact(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-coroot-chart-resume", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-chart",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"aiops-host-agent cpu",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
			"metrics":[],
			"chartReports":[
				{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: aiops-host-agent","series":[{"name":"aiops-host-agent@node-1","data":[0.0004,0.0006]}]}]}}]}
			]
		}
	}`)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-chart-resume",
		SessionID:   "sess-coroot-chart-resume",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看 aiops-host-agent CPU 图表"}, CreatedAt: now},
			{ID: "tool-result-coroot-chart", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}
	sessions.Update(session)

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/assistant/resume", "application/json", bytes.NewReader(assistantTransportResumePayload(t, session.ID, session.ID)))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/resume error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if !strings.Contains(text, "\"type\":\"coroot_chart\"") {
		t.Fatalf("response = %q, want coroot_chart artifact", text)
	}
	if !strings.Contains(text, "\"kind\":\"coroot_report_charts\"") {
		t.Fatalf("response = %q, want native Coroot chart visual kind", text)
	}
}

func TestAssistantTransportResumeRestoresAllPersistedTurnsWithProcess(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-resume-history", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	firstTurn := assistantTransportCompletedResumeTurn(session.ID, "turn-history-1", "第一轮问题", "第一轮处理过程", "第一轮最终回答", now)
	secondTurn := assistantTransportCompletedResumeTurn(session.ID, "turn-history-2", "第二轮问题", "第二轮处理过程", "第二轮最终回答", now.Add(5*time.Second))
	session.TurnHistory = []runtimekernel.TurnSnapshot{firstTurn, secondTurn}
	session.CurrentTurn = &secondTurn
	sessions.Update(session)

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/assistant/resume", "application/json", bytes.NewReader(assistantTransportResumePayload(t, session.ID, session.ID)))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/resume error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	for _, want := range []string{
		"\"turnOrder\":[\"turn-history-1\",\"turn-history-2\"]",
		"第一轮问题",
		"第一轮处理过程",
		"第一轮最终回答",
		"第二轮问题",
		"第二轮处理过程",
		"第二轮最终回答",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("response = %q, want %q", text, want)
		}
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
			"schemaVersion":    "aiops.transport.v2",
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

func assistantTransportCompletedResumeTurn(sessionID, turnID, userText, processText, finalText string, startedAt time.Time) runtimekernel.TurnSnapshot {
	completedAt := startedAt.Add(time.Second)
	return runtimekernel.TurnSnapshot{
		ID:          turnID,
		SessionID:   sessionID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   startedAt,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
		FinalOutput: finalText,
		AgentItems: []agentstate.TurnItem{
			{ID: turnID + "-user", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: userText}, CreatedAt: startedAt},
			{ID: turnID + "-reasoning", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: processText}, CreatedAt: startedAt.Add(500 * time.Millisecond)},
			{ID: turnID + "-final", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: finalText}, CreatedAt: completedAt},
		},
	}
}
