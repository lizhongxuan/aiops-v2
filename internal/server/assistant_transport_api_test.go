package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

type assistantTransportCaptureWriter struct {
	bytes.Buffer
}

func (w *assistantTransportCaptureWriter) Flush() {}

func firstAssistantTransportStreamFrame(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "\n"); idx >= 0 {
		return text[:idx]
	}
	return text
}

func TestAssistantTransportShouldPollHostOpsOnlyTurn(t *testing.T) {
	state := appui.NewAiopsTransportState("sess-hostops", "thread-hostops")
	state.CurrentTurnID = "turn-hostops"
	state.Status = appui.AiopsTransportStatusIdle
	state.ActiveHostMissionID = "hostops:turn-hostops"
	state.HostMissions["hostops:turn-hostops"] = appui.AiopsTransportHostMission{
		ID:     "hostops:turn-hostops",
		TurnID: "turn-hostops",
		Status: "waiting_plan_acceptance",
	}

	if !assistantTransportShouldPoll(state) {
		t.Fatal("host-ops turn should keep polling runtime after mission projection")
	}
}

func TestAssistantTransportShouldPollHostOpsTurnWithActiveChildAgent(t *testing.T) {
	state := appui.NewAiopsTransportState("sess-hostops-active-child", "thread-hostops-active-child")
	state.CurrentTurnID = "turn-hostops-active-child"
	state.Status = appui.AiopsTransportStatusIdle
	state.ActiveHostMissionID = "hostops:turn-hostops-active-child"
	state.HostMissions["hostops:turn-hostops-active-child"] = appui.AiopsTransportHostMission{
		ID:            "hostops:turn-hostops-active-child",
		TurnID:        "turn-hostops-active-child",
		Status:        "running",
		ChildAgentIDs: []string{"child-active"},
	}
	state.ChildAgents["child-active"] = appui.AiopsTransportChildAgent{
		ID:        "child-active",
		MissionID: "hostops:turn-hostops-active-child",
		SessionID: "host-child:hostops:turn-hostops-active-child:host-a",
		HostID:    "host-a",
		Status:    "running",
	}

	if !assistantTransportShouldPoll(state) {
		t.Fatal("host-ops turn with active child agent should keep polling child session state")
	}
}

func TestAssistantTransportAPIRoutesResolvedPGRecoveryMentionsToHostOpsMission(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	missions := hostops.NewInMemoryMissionStore()
	transcripts := hostops.NewInMemoryTranscriptStore()
	orchestrator := hostops.NewOrchestrator(missions, transcripts, nil)
	hosts := newAssistantTransportHostRepoStub(
		store.HostRecord{ID: "accept-host-a", Name: "@pg-a", Address: "aiops-accept-host-a", Status: "online"},
		store.HostRecord{ID: "accept-host-b", Name: "@pg-b", Address: "aiops-accept-host-b", Status: "online"},
		store.HostRecord{ID: "accept-host-c", Name: "@pg-mon", Address: "aiops-accept-host-c", Status: "online"},
	)
	services := appui.NewServices(
		runtime,
		sessions,
		appui.WithHostRepository(hosts),
		appui.WithHostOpsService(appui.NewHostOpsService(missions, transcripts, orchestrator)),
	)
	server := NewHTTPServer(services)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	mentions := []map[string]any{
		{"raw": "@pg-a", "hostId": "accept-host-a", "address": "aiops-accept-host-a", "displayName": "@pg-a", "source": "inventory", "resolved": true},
		{"raw": "@pg-b", "hostId": "accept-host-b", "address": "aiops-accept-host-b", "displayName": "@pg-b", "source": "inventory", "resolved": true},
		{"raw": "@pg-mon", "hostId": "accept-host-c", "address": "aiops-accept-host-c", "displayName": "@pg-mon", "source": "inventory", "resolved": true},
	}
	mentionsJSON, err := json.Marshal(mentions)
	if err != nil {
		t.Fatalf("marshal mentions: %v", err)
	}
	payload := map[string]any{
		"state": map[string]any{
			"schemaVersion":    "aiops.transport.v2",
			"threadId":         "thread-pg-recovery",
			"status":           "idle",
			"turns":            map[string]any{},
			"turnOrder":        []string{},
			"pendingApprovals": map[string]any{},
			"mcpSurfaces":      map[string]any{},
			"artifacts":        map[string]any{},
			"hostMissions":     map[string]any{},
			"childAgents":      map[string]any{},
			"runtimeLiveness": map[string]any{
				"activeTurns":          map[string]any{},
				"activeAgents":         map[string]any{},
				"pendingApprovals":     map[string]any{},
				"pendingUserInputs":    map[string]any{},
				"activeCommandStreams": map[string]any{},
			},
		},
		"threadId": "thread-pg-recovery",
		"commands": []map[string]any{
			{
				"type": "add-message",
				"message": map[string]any{
					"role": "user",
					"metadata": map[string]string{
						"aiops.hostops.mentions":                string(mentionsJSON),
						"aiops.hostops.clientDetectedMultiHost": "true",
					},
					"parts": []map[string]string{{
						"type": "text",
						"text": "主机A=@pg-a和主机B=@pg-b的PG主从集群异常，请帮忙恢复，数据可以不要，只需要PG主从集群可以正常运行，他们的pg_mon部署在主机C=@pg-mon。",
					}},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	decodedReq, err := decodeAssistantTransportRequest(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	decodedCommands, err := assistantTransportCommandsFromRequest(decodedReq)
	if err != nil {
		t.Fatalf("decode transport commands: %v", err)
	}
	if len(decodedCommands) != 1 || decodedCommands[0].AddMessage == nil {
		t.Fatalf("decoded commands = %#v, want one add-message", decodedCommands)
	}
	if decodedCommands[0].AddMessage.Metadata["aiops.hostops.mentions"] == "" {
		t.Fatalf("decoded add-message metadata = %#v, want hostops mentions", decodedCommands[0].AddMessage.Metadata)
	}
	directHandler := appui.NewTransportCommandHandler(services.ChatService(), services.ApprovalService(), services.ChoiceService(), services.MCPService()).WithHostOpsService(services.HostOpsService())
	directState, _, err := directHandler.Apply(context.Background(), appui.NewAiopsTransportState("", "thread-direct-pg-recovery"), decodedCommands[0])
	if err != nil {
		t.Fatalf("direct handler Apply() error = %v", err)
	}
	if directState.ActiveHostMissionID == "" {
		t.Fatalf("direct handler state has no host mission: %#v", directState)
	}
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(text))
	}
	for _, want := range []string{`"path":["hostMissions"`, `"accept-host-a"`, `"accept-host-b"`, `"accept-host-c"`, `"waiting_plan_acceptance"`} {
		if !bytes.Contains(text, []byte(want)) {
			t.Fatalf("transport response missing %q:\n%s", want, string(text))
		}
	}
	if !bytes.Contains(text, []byte(`"path":["status"],"value":"working"`)) {
		t.Fatalf("transport response should show runtime-owned host-ops turn working before projection:\n%s", string(text))
	}
	if !bytes.Contains(text, []byte(`"path":["status"],"value":"idle"`)) {
		t.Fatalf("transport response should poll runtime projection back to idle after completion:\n%s", string(text))
	}
}

type assistantTransportAPITestRuntime struct {
	sessions *runtimekernel.SessionManager
	runErr   error
	delay    time.Duration
	runReq   runtimekernel.TurnRequest
	runCh    chan runtimekernel.TurnRequest
}

func waitForAssistantTransportRunTurn(t *testing.T, runtime *assistantTransportAPITestRuntime) runtimekernel.TurnRequest {
	t.Helper()
	if runtime.runCh == nil {
		t.Fatal("runtime.runCh is nil")
	}
	select {
	case req := <-runtime.runCh:
		return req
	case <-time.After(time.Second):
		t.Fatal("RunTurn was not called")
		return runtimekernel.TurnRequest{}
	}
}

type assistantTransportHostRepoStub struct {
	items map[string]store.HostRecord
}

func newAssistantTransportHostRepoStub(records ...store.HostRecord) *assistantTransportHostRepoStub {
	repo := &assistantTransportHostRepoStub{items: map[string]store.HostRecord{}}
	for _, record := range records {
		repo.items[record.ID] = record
	}
	return repo
}

func (r *assistantTransportHostRepoStub) GetHost(id string) (*store.HostRecord, error) {
	record, ok := r.items[id]
	if !ok {
		return nil, errors.New("host not found")
	}
	cp := record
	return &cp, nil
}

func (r *assistantTransportHostRepoStub) ListHosts() ([]store.HostRecord, error) {
	items := make([]store.HostRecord, 0, len(r.items))
	for _, record := range r.items {
		items = append(items, record)
	}
	return items, nil
}

func (r *assistantTransportHostRepoStub) SaveHost(host *store.HostRecord) error {
	cp := *host
	r.items[cp.ID] = cp
	return nil
}

func (r *assistantTransportHostRepoStub) DeleteHost(id string) error {
	delete(r.items, id)
	return nil
}

func (r *assistantTransportAPITestRuntime) RunTurn(_ context.Context, req runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	r.runReq = req
	if r.runCh != nil {
		select {
		case r.runCh <- req:
		default:
		}
	}
	session := r.sessions.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:              req.TurnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionID:       req.SessionID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Lifecycle:       runtimekernel.TurnLifecycleRunning,
		ResumeState:     runtimekernel.TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: req.Input}, CreatedAt: now},
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "analyzing service state"}, CreatedAt: now},
			assistantMessageFinalItemForServerTest("final-1", agentstate.ItemStatusRunning, "partial", now),
		},
	}
	r.sessions.Update(session)

	if r.delay > 0 {
		time.Sleep(r.delay)
	}

	if r.runErr != nil {
		session = r.sessions.Get(req.SessionID)
		session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleFailed
		session.CurrentTurn.Error = r.runErr.Error()
		session.CurrentTurn.UpdatedAt = time.Now().UTC()
		session.CurrentTurn.AgentItems = append(session.CurrentTurn.AgentItems,
			agentstate.TurnItem{ID: "err-1", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: r.runErr.Error()}, CreatedAt: time.Now().UTC()},
		)
		r.sessions.Update(session)
		return runtimekernel.TurnResult{
			SessionType: req.SessionType,
			Mode:        req.Mode,
			SessionID:   req.SessionID,
			TurnID:      req.TurnID,
			Status:      "failed",
			Error:       r.runErr.Error(),
		}, r.runErr
	}

	session = r.sessions.Get(req.SessionID)
	session.Messages = append(session.Messages,
		runtimekernel.Message{ID: "msg-user-1", Role: "user", Content: req.Input, Timestamp: now},
		runtimekernel.Message{ID: "msg-assistant-1", Role: "assistant", Content: "final answer", Timestamp: time.Now().UTC()},
	)
	session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCompleted
	session.CurrentTurn.UpdatedAt = time.Now().UTC()
	session.CurrentTurn.AgentItems[1].Status = agentstate.ItemStatusCompleted
	session.CurrentTurn.AgentItems[2].Status = agentstate.ItemStatusCompleted
	session.CurrentTurn.AgentItems[2].Payload.Summary = "final answer"
	r.sessions.Update(session)

	return runtimekernel.TurnResult{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		Status:      "completed",
	}, nil
}

func (r *assistantTransportAPITestRuntime) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "completed"}, nil
}

func (r *assistantTransportAPITestRuntime) CancelTurn(_ context.Context, req runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	session := r.sessions.Get(req.SessionID)
	if session != nil && session.CurrentTurn != nil && session.CurrentTurn.ID == req.TurnID {
		now := time.Now().UTC()
		session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCanceled
		session.CurrentTurn.UpdatedAt = now
		session.CurrentTurn.CompletedAt = &now
		r.sessions.Update(session)
	}
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "cancelled"}, nil
}

type assistantTransportBlockingResumeRuntime struct {
	sessions *runtimekernel.SessionManager
	started  chan runtimekernel.ResumeRequest
	release  chan struct{}
}

func (r *assistantTransportBlockingResumeRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (r *assistantTransportBlockingResumeRuntime) ResumeTurn(_ context.Context, req runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	r.started <- req
	<-r.release
	if session := r.sessions.Get(req.SessionID); session != nil && session.CurrentTurn != nil {
		now := time.Now().UTC()
		session.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCompleted
		session.CurrentTurn.ResumeState = runtimekernel.TurnResumeStateNone
		session.CurrentTurn.PendingApprovals = nil
		session.CurrentTurn.PendingEvidence = nil
		session.CurrentTurn.UpdatedAt = now
		session.CurrentTurn.CompletedAt = &now
		session.CurrentTurn.FinalOutput = "approved command finished"
		session.PendingApprovals = nil
		session.PendingEvidence = nil
		r.sessions.Update(session)
	}
	return runtimekernel.TurnResult{SessionID: req.SessionID, TurnID: req.TurnID, Status: "completed"}, nil
}

func (r *assistantTransportBlockingResumeRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type assistantTransportApprovalServices struct {
	*appui.Services
	approval appui.ApprovalService
}

func (s assistantTransportApprovalServices) ApprovalService() appui.ApprovalService {
	return s.approval
}

type assistantTransportBlockingHostCommandExecutor struct {
	started chan hostops.HostCommandRequest
	done    chan assistantTransportBlockingHostCommandExecutorResult
}

type assistantTransportBlockingHostCommandExecutorResult struct {
	result hostops.HostCommandResult
	err    error
}

func newAssistantTransportBlockingHostCommandExecutor() *assistantTransportBlockingHostCommandExecutor {
	return &assistantTransportBlockingHostCommandExecutor{
		started: make(chan hostops.HostCommandRequest, 1),
		done:    make(chan assistantTransportBlockingHostCommandExecutorResult, 1),
	}
}

func (e *assistantTransportBlockingHostCommandExecutor) RunShell(_ context.Context, _ hostops.ToolContext, req hostops.HostCommandRequest) (hostops.HostCommandResult, error) {
	e.started <- req
	next := <-e.done
	return next.result, next.err
}

func (e *assistantTransportBlockingHostCommandExecutor) release(result hostops.HostCommandResult, err error) {
	e.done <- assistantTransportBlockingHostCommandExecutorResult{result: result, err: err}
}

func TestAssistantTransportAPIAddMessageStreamsTransportState(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, delay: 25 * time.Millisecond}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := map[string]any{
		"state": map[string]any{
			"schemaVersion":    "aiops.transport.v2",
			"sessionId":        "",
			"threadId":         "thread-1",
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
		"threadId": "thread-1",
		"commands": []map[string]any{
			{
				"type": "add-message",
				"message": map[string]any{
					"role": "user",
					"content": []map[string]any{
						{"type": "text", "text": "investigate payment-api"},
					},
				},
			},
		},
	}
	payload, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
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
	if !strings.Contains(text, "\"path\":[\"status\"],\"value\":\"working\"") && !strings.Contains(text, "\"path\":[\"status\"],\"value\":\"idle\"") {
		t.Fatalf("response = %q, want working or idle state update", text)
	}
	if !strings.Contains(text, "append-text") {
		t.Fatalf("response = %q, want append-text for final text", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, want streamed final answer", text)
	}
}

func TestAssistantTransportAPIRuntimeControlsAndClientIDsReachTurnRequest(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, runCh: make(chan runtimekernel.TurnRequest, 1)}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportRuntimeControlPayload(t,
		map[string]any{"sessionType": "host", "mode": "execute"},
		map[string]any{
			"id":       "client-message-controls",
			"role":     "user",
			"metadata": map[string]string{"clientTurnId": "client-turn-controls"},
			"content":  []map[string]any{{"type": "text", "text": "@local inspect runtime controls"}},
		},
	)
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST AssistantTransport: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeHost || runReq.Mode != runtimekernel.ModeExecute {
		t.Fatalf("TurnRequest runtime controls = %q/%q, want host/execute", runReq.SessionType, runReq.Mode)
	}
	if runReq.ClientMessageID != "client-message-controls" || runReq.ClientTurnID != "client-turn-controls" {
		t.Fatalf("TurnRequest client ids = %q/%q", runReq.ClientMessageID, runReq.ClientTurnID)
	}
}

func TestAssistantTransportAPIMissingRuntimeControlsUseExistingDefaults(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, runCh: make(chan runtimekernel.TurnRequest, 1)}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportRuntimeControlPayload(t, nil, map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "text", "text": "use existing defaults"}},
	})
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST AssistantTransport: %v", err)
	}
	defer resp.Body.Close()
	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace || runReq.Mode != runtimekernel.ModeChat {
		t.Fatalf("TurnRequest defaults = %q/%q, want workspace/chat", runReq.SessionType, runReq.Mode)
	}
	if runReq.ClientMessageID != "" || runReq.ClientTurnID != "" {
		t.Fatalf("missing client ids were synthesized: %q/%q", runReq.ClientMessageID, runReq.ClientTurnID)
	}
}

func TestAssistantTransportAPIRejectsInvalidRuntimeControlConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
	}{
		{name: "unknown session type", config: map[string]any{"sessionType": "tenant"}},
		{name: "non-string session type", config: map[string]any{"sessionType": 7}},
		{name: "unknown mode", config: map[string]any{"mode": "destroy"}},
		{name: "non-string mode", config: map[string]any{"mode": true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := runtimekernel.NewSessionManager()
			runtime := &assistantTransportAPITestRuntime{sessions: sessions, runCh: make(chan runtimekernel.TurnRequest, 1)}
			server := NewHTTPServer(appui.NewServices(runtime, sessions))
			ts := httptest.NewServer(server.Handler())
			defer ts.Close()

			payload := assistantTransportRuntimeControlPayload(t, tt.config, map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "reject invalid config"}},
			})
			resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("POST AssistantTransport: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d body=%s, want 400", resp.StatusCode, body)
			}
			select {
			case req := <-runtime.runCh:
				t.Fatalf("runtime started for invalid config: %+v", req)
			default:
			}
		})
	}
}

func assistantTransportRuntimeControlPayload(t *testing.T, config map[string]any, message map[string]any) []byte {
	t.Helper()
	body := map[string]any{
		"state": map[string]any{
			"schemaVersion": "aiops.transport.v2", "sessionId": "", "threadId": "thread-controls", "status": "idle",
			"turns": map[string]any{}, "turnOrder": []any{}, "pendingApprovals": map[string]any{},
			"mcpSurfaces": map[string]any{}, "artifacts": map[string]any{}, "runtimeLiveness": map[string]any{},
			"seq": 0, "updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
		"threadId": "thread-controls",
		"commands": []map[string]any{{"type": "add-message", "message": message}},
	}
	if config != nil {
		body["config"] = config
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal AssistantTransport payload: %v", err)
	}
	return payload
}

func TestAssistantTransportAPIStopReturnsReprojectedCanceledProcess(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-stop-reproject", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-stop-reproject",
		SessionID:   session.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-stop", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "分析 Kubernetes CrashLoopBackOff"}, CreatedAt: now},
			{ID: "model-stop", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "calling model"}, CreatedAt: now},
		},
	}
	sessions.Update(session)

	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := map[string]any{
		"state": map[string]any{
			"schemaVersion": "aiops.transport.v2",
			"sessionId":     session.ID,
			"threadId":      session.ID,
			"status":        "working",
			"currentTurnId": "turn-stop-reproject",
			"turns": map[string]any{
				"turn-stop-reproject": map[string]any{
					"id":     "turn-stop-reproject",
					"status": "working",
					"process": []map[string]any{
						{
							"id":     "block:turn-stop-reproject:reasoning:model-stop",
							"kind":   "reasoning",
							"status": "running",
							"text":   "正在等待模型返回",
						},
					},
				},
			},
			"turnOrder":        []string{"turn-stop-reproject"},
			"pendingApprovals": map[string]any{},
			"mcpSurfaces":      map[string]any{},
			"artifacts":        map[string]any{},
			"hostMissions":     map[string]any{},
			"childAgents":      map[string]any{},
			"runtimeLiveness": map[string]any{
				"activeTurns":          map[string]any{"turn-stop-reproject": true},
				"activeAgents":         map[string]any{},
				"pendingApprovals":     map[string]any{},
				"pendingUserInputs":    map[string]any{},
				"activeCommandStreams": map[string]any{},
			},
			"seq":       0,
			"updatedAt": now.Format(time.RFC3339Nano),
		},
		"threadId": session.ID,
		"commands": []map[string]any{
			{
				"type":      "aiops.stop",
				"sessionId": session.ID,
				"turnId":    "turn-stop-reproject",
				"reason":    "user requested stop",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.StatusCode, text)
	}
	if !strings.Contains(text, "模型调用已取消") {
		t.Fatalf("response = %q, want canceled model process projection", text)
	}
	if strings.Contains(text, "正在等待模型返回") {
		t.Fatalf("response = %q, should not re-emit stale waiting process after stop", text)
	}
}

func TestAssistantTransportAPIPlainPGQuestionDoesNotBindServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{
		sessions: sessions,
		runCh:    make(chan runtimekernel.TurnRequest, 1),
	}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-v2-pg-advisory", "pgBackRest 恢复后，pg_auto_failover 从节点 timeline 比主库高，这是为什么？")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(raw))
	}

	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn hostId = %q, want empty for advisory", runReq.HostID)
	}
	if runReq.SessionType != runtimekernel.SessionTypeWorkspace {
		t.Fatalf("RunTurn sessionType = %q, want workspace", runReq.SessionType)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(appui.ChatRouteAdvisory) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.target.binding"]; got != "none" {
		t.Fatalf("target binding = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestAssistantTransportAPILocalMentionBindsServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{
		sessions: sessions,
		runCh:    make(chan runtimekernel.TurnRequest, 1),
	}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-v2-local-mention", "@local 帮我只读检查 PostgreSQL 状态")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(raw))
	}

	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runReq.HostID)
	}
	if runReq.SessionType != runtimekernel.SessionTypeHost {
		t.Fatalf("RunTurn sessionType = %q, want host", runReq.SessionType)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(appui.ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.target.binding"]; got != "host" {
		t.Fatalf("target binding = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "true" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestAssistantTransportAPIServerLocalMentionBindsServerLocal(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{
		sessions: sessions,
		runCh:    make(chan runtimekernel.TurnRequest, 1),
	}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-v2-server-local-mention", "@server-local 查看 CPU 情况")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, string(raw))
	}

	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.HostID != "server-local" {
		t.Fatalf("RunTurn hostId = %q, want server-local", runReq.HostID)
	}
	if runReq.SessionType != runtimekernel.SessionTypeHost {
		t.Fatalf("RunTurn sessionType = %q, want host", runReq.SessionType)
	}
	if got := runReq.Metadata["aiops.route.mode"]; got != string(appui.ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.target.binding"]; got != "host" {
		t.Fatalf("target binding = %q; metadata=%#v", got, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "true" {
		t.Fatalf("exec allowed = %q; metadata=%#v", got, runReq.Metadata)
	}
}

func TestAssistantTransportAPIStructuredHostMentionBindsAfterServerResolution(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{
		sessions: sessions,
		runCh:    make(chan runtimekernel.TurnRequest, 1),
	}
	hosts := newAssistantTransportHostRepoStub(store.HostRecord{
		ID:         "host-a",
		Name:       "pg-primary",
		Address:    "120.77.239.90",
		Status:     "online",
		Executable: true,
		AgentURL:   "http://host-a:7072",
	})
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithHostRepository(hosts)))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayloadWithMetadata(t, "", "thread-structured-host", "@120.77.239.90 检查状态", map[string]string{
		"aiops.input.mentions.v1": `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-host-a","sigil":"@","display":"@120.77.239.90","rawText":"@120.77.239.90","kind":"host","path":"host://host-a","source":"selection","range":{"start":0,"end":14},"payload":{"hostId":"host-a","address":"120.77.239.90","displayName":"pg-primary"}}]}`,
	})
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()

	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.HostID != "host-a" {
		t.Fatalf("RunTurn HostID = %q, want host-a; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.input.mentionSource"]; got != "structured" {
		t.Fatalf("mentionSource = %q, want structured; metadata=%#v", got, runReq.Metadata)
	}
}

func TestAssistantTransportAPIStructuredStaleMentionFailsClosed(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{
		sessions: sessions,
		runCh:    make(chan runtimekernel.TurnRequest, 1),
	}
	hosts := newAssistantTransportHostRepoStub(store.HostRecord{ID: "host-a", Name: "pg-primary", Address: "120.77.239.90", Status: "online", Executable: true, AgentURL: "http://host-a:7072"})
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithHostRepository(hosts)))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayloadWithMetadata(t, "", "thread-structured-stale", "@host-b 检查状态", map[string]string{
		"aiops.input.mentions.v1": `{"version":1,"mentions":[{"version":1,"tokenId":"mention-0-host-a","sigil":"@","display":"@120.77.239.90","rawText":"@120.77.239.90","kind":"host","path":"host://host-a","source":"selection","range":{"start":0,"end":14},"payload":{"hostId":"host-a","address":"120.77.239.90","displayName":"pg-primary"}}]}`,
	})
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()

	runReq := waitForAssistantTransportRunTurn(t, runtime)
	if runReq.HostID != "" {
		t.Fatalf("RunTurn HostID = %q, want empty fail-closed host; metadata=%#v", runReq.HostID, runReq.Metadata)
	}
	if got := runReq.Metadata["aiops.tool.execCommandAllowed"]; got != "false" {
		t.Fatalf("exec allowed = %q, want false; metadata=%#v", got, runReq.Metadata)
	}
}

func TestAssistantTransportDoesNotAutoSearchOpsManualForDiagnosisWhenFallbackEnabled(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	services := appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo))))
	server := NewHTTPServer(services, WithOpsManualAutoRetrieval(true))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-ops-manual", "排查 Redis")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(firstAssistantTransportStreamFrame(text), `"type":"ops_manual_match"`) || strings.Contains(firstAssistantTransportStreamFrame(text), `"type":"ops_manual_search_result"`) {
		t.Fatalf("first response frame = %q, should not show ops manual artifact before final answer", firstAssistantTransportStreamFrame(text))
	}
	if strings.Contains(text, `"type":"ops_manual_match"`) {
		t.Fatalf("response = %q, should not use legacy ops_manual_match fallback", text)
	}
	if strings.Contains(text, `"type":"ops_manual_search_result"`) {
		t.Fatalf("response = %q, should not auto search ops manuals for diagnosis-only requests", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, should still run the chat turn", text)
	}
}

func TestAssistantTransportSkipsOpsManualArtifactWhenAutoRetrievalDisabled(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	services := appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo))))
	server := NewHTTPServer(services, WithOpsManualAutoRetrieval(false))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-ops-manual-disabled", "排查 Redis")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(text, `"type":"ops_manual_match"`) {
		t.Fatalf("response = %q, should not include ops_manual_match when auto retrieval is disabled", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, should still run the chat turn", text)
	}
}

func TestAssistantTransportDoesNotAutoSearchOpsManualByDefault(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	services := appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo))))
	server := NewHTTPServer(services)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-ops-manual-default", "排查 Redis")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(text, `"type":"ops_manual_search_result"`) || strings.Contains(text, `"type":"ops_manual_match"`) {
		t.Fatalf("response = %q, should not auto search ops manuals by default; the LLM must call search_ops_manuals", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, should still run the chat turn", text)
	}
}

func TestAssistantTransportAddsOpsManualNeedInfoArtifactForResolutionFallback(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	services := appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo))))
	server := NewHTTPServer(services, WithOpsManualAutoRetrieval(true))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-ops-manual", "生产 payment-api 的 Redis used_memory_rss 持续上涨，Coroot 显示 p95 升高，请通过 ssh 修复")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(firstAssistantTransportStreamFrame(text), `"type":"ops_manual_match"`) || strings.Contains(firstAssistantTransportStreamFrame(text), `"type":"ops_manual_search_result"`) {
		t.Fatalf("first response frame = %q, should not show ops manual artifact before final answer", firstAssistantTransportStreamFrame(text))
	}
	if strings.Contains(text, `"type":"ops_manual_match"`) {
		t.Fatalf("response = %q, should not use legacy ops_manual_match fallback", text)
	}
	if !strings.Contains(text, `"type":"ops_manual_search_result"`) || !strings.Contains(text, `"status":"need_info"`) || !strings.Contains(text, `"target_instance"`) {
		t.Fatalf("response = %q, should add terminal ops manual search fallback without guessing target_instance from text only", text)
	}
}

func TestAssistantTransportRendersOpsManualSearchToolArtifact(t *testing.T) {
	now := time.Now().UTC()
	result := opsmanual.SearchOpsManualsResult{
		Decision:      opsmanual.DecisionNeedInfo,
		Summary:       "缺少目标实例、环境、症状和指标。",
		NextQuestions: []string{"目标 Redis 实例是哪一个？"},
		Manuals: []opsmanual.SearchManualHit{
			{
				Manual: opsmanual.OpsManual{
					ID:    "manual-redis-memory",
					Title: "Redis 内存压力排障",
				},
				UsableMode:        opsmanual.DecisionNeedInfo,
				RecommendedAction: "collect_context",
			},
		},
	}
	raw, _ := json.Marshal(result)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-tool-ops-manual",
		SessionID:   "sess-tool-ops-manual",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search-ops-manuals",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "need_info",
					Data: json.RawMessage(`{
						"toolCallId":"call-search-ops-manuals",
						"toolName":"search_ops_manuals",
						"displayKind":"ops_manual_search_result",
						"outputPreview":` + string(raw) + `
					}`),
				},
				CreatedAt: now,
			},
		},
	}
	state, err := appui.NewTransportProjector().ProjectTurnSnapshot(appui.NewAiopsTransportState("sess-tool-ops-manual", "thread-tool-ops-manual"), turn)
	if err != nil {
		t.Fatal(err)
	}
	projected := state.Turns["turn-tool-ops-manual"]
	if len(projected.AgentUIArtifacts) != 1 || projected.AgentUIArtifacts[0].Type != "ops_manual_search_result" {
		t.Fatalf("artifacts = %#v, want one ops_manual_search_result", projected.AgentUIArtifacts)
	}
	if projected.AgentUIArtifacts[0].Status != "need_info" {
		t.Fatalf("artifact status = %q, want need_info", projected.AgentUIArtifacts[0].Status)
	}
}

func TestAssistantTransportRendersOpsManualPreflightToolArtifact(t *testing.T) {
	raw, _ := json.Marshal(opsmanual.PreflightResult{
		Status:     opsmanual.PreflightStatusPassed,
		Ready:      true,
		ManualID:   "manual-pg-backup",
		WorkflowID: "workflow-pg-backup",
		NextAction: "confirm_execution",
	})
	artifact, ok := assistantTransportOpsManualPreflightArtifactFromToolResult("turn-preflight", "tool-result-preflight", runtimekernel.ToolResult{
		ToolCallID: "call-preflight",
		Content:    string(raw),
		Display: &runtimekernel.ToolDisplayPayload{
			Type:  "ops_manual_preflight_result",
			Title: "run_ops_manual_preflight",
			Data:  raw,
		},
	})
	if !ok {
		t.Fatal("expected preflight artifact")
	}
	if artifact.Type != "ops_manual_preflight_result" || artifact.Status != "passed" || artifact.Severity != "success" {
		t.Fatalf("artifact = %#v, want passed preflight artifact", artifact)
	}
	if len(artifact.Actions) != 1 || artifact.Actions[0]["id"] != "confirm_execution" {
		t.Fatalf("actions = %#v, want confirm_execution", artifact.Actions)
	}
}

func TestAssistantTransportRendersOpsManualParamResolutionToolArtifact(t *testing.T) {
	raw, _ := json.Marshal(opsmanual.ParamResolutionResult{
		Status:     opsmanual.ParamResolutionResolved,
		ManualID:   "manual-redis-rca",
		WorkflowID: "workflow-redis-rca",
		NextAction: "run_preflight",
	})
	artifact, ok := assistantTransportOpsManualParamResolutionArtifactFromToolResult("turn-param", "tool-result-param", runtimekernel.ToolResult{
		ToolCallID: "call-param",
		Content:    string(raw),
		Display: &runtimekernel.ToolDisplayPayload{
			Type:  "ops_manual_param_resolution",
			Title: "resolve_ops_manual_params",
			Data:  raw,
		},
	})
	if !ok {
		t.Fatal("expected param resolution artifact")
	}
	if artifact.Type != "ops_manual_param_resolution" || artifact.Status != "resolved" || artifact.Severity != "success" {
		t.Fatalf("artifact = %#v, want resolved param artifact", artifact)
	}
	if len(artifact.Actions) != 1 || artifact.Actions[0]["id"] != "run_preflight" {
		t.Fatalf("actions = %#v, want run_preflight", artifact.Actions)
	}
}

func TestAssistantTransportAddsTerminalOpsManualSearchFallbackWhenModelSkipsTool(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-terminal-ops-manual-fallback", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	completedAt := now.Add(time.Second)
	session.CurrentTurn = nil
	session.TurnHistory = []runtimekernel.TurnSnapshot{
		{
			ID:          "turn-terminal-ops-manual-fallback",
			SessionID:   "sess-terminal-ops-manual-fallback",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeChat,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			StartedAt:   now,
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
			FinalOutput: "请补充目标实例和现象。",
			AgentItems: []agentstate.TurnItem{
				{ID: "user-redis", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "修复 Redis"}, CreatedAt: now},
				assistantMessageFinalItemForServerTest("final-redis", agentstate.ItemStatusCompleted, "请补充目标实例和现象。", completedAt),
			},
		},
	}
	sessions.Update(session)

	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo)))), WithOpsManualAutoRetrieval(true))
	initial := appui.NewAiopsTransportState("sess-terminal-ops-manual-fallback", "thread-terminal-ops-manual-fallback")
	initial.Status = appui.AiopsTransportStatusWorking
	writer := &assistantTransportCaptureWriter{}

	next, err := server.streamAssistantTransportState(context.Background(), newAssistantTransportStreamEncoder(writer), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err != nil {
		t.Fatalf("streamAssistantTransportState() error = %v", err)
	}

	turn := next.Turns["turn-terminal-ops-manual-fallback"]
	if len(turn.AgentUIArtifacts) != 1 || turn.AgentUIArtifacts[0].Type != "ops_manual_search_result" {
		t.Fatalf("agent UI artifacts = %#v, want one terminal ops manual search fallback", turn.AgentUIArtifacts)
	}
	if turn.AgentUIArtifacts[0].Status != "need_info" {
		t.Fatalf("artifact status = %q, want need_info", turn.AgentUIArtifacts[0].Status)
	}
	if !strings.Contains(writer.String(), "ops_manual_search_result") {
		t.Fatalf("stream = %q, want terminal fallback artifact frame", writer.String())
	}
}

func TestAssistantTransportDoesNotAddOpsManualFallbackWhileTurnRunning(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-running-ops-manual-fallback", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-running-ops-manual-fallback",
		SessionID:   "sess-running-ops-manual-fallback",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-redis", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "排查 Redis"}, CreatedAt: now},
		},
	}
	repo := opsmanual.NewMemoryStore()
	if err := repo.SaveManual(assistantTransportRedisManual()); err != nil {
		t.Fatalf("SaveManual() error = %v", err)
	}
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(opsmanual.NewService(repo)))))
	state := appui.NewAiopsTransportState("sess-running-ops-manual-fallback", "thread-running-ops-manual-fallback")
	projected, err := projectAssistantTransportSessionState(server, state, &runtimekernel.SessionState{ID: session.ID, CurrentTurn: turn}, appui.NewTransportProjector())
	if err != nil {
		t.Fatalf("projectAssistantTransportSessionState() error = %v", err)
	}
	if artifacts := projected.Turns["turn-running-ops-manual-fallback"].AgentUIArtifacts; len(artifacts) != 0 {
		t.Fatalf("agent UI artifacts = %#v, want no fallback before terminal turn", artifacts)
	}
}

func TestAssistantTransportDoesNotSynthesizeRunnerWorkflowArtifactAfterConfirmation(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayloadWithMetadata(t, "", "thread-generate-workflow", "确认生成工作流候选：Redis 运维手册", map[string]string{
		"opsManualAction": "generate_runner_workflow_candidate",
	})
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if strings.Contains(text, `"type":"runner_workflow_generation"`) {
		t.Fatalf("response = %q, should not synthesize runner_workflow_generation before a real workflow generation result", text)
	}
	if !strings.Contains(text, "final answer") {
		t.Fatalf("response = %q, want normal chat response to continue", text)
	}
}

func TestAssistantTransportRecordsOpsManualSuppressionAfterSkip(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	repo := opsmanual.NewMemoryStore()
	store := opsmanual.NewMemorySessionOpsContextStore()
	domain := opsmanual.NewService(repo, opsmanual.WithSessionOpsContextStore(store))
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(domain))))
	state := appui.NewAiopsTransportState("sess-skip-manual", "thread-skip-manual")
	state.CurrentTurnID = "turn-skip-manual"

	server.decorateAssistantTransportAgentUIArtifacts(state, appui.TransportCommand{
		Type: appui.TransportCommandTypeAddMessage,
		AddMessage: &appui.TransportAddMessageCommand{
			SessionID: "sess-skip-manual",
			ThreadID:  "thread-skip-manual",
			Message: appui.TransportUserMessage{
				Text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups。",
			},
			Metadata: map[string]string{
				"opsManualAction":          "skip_ops_manual",
				"opsManualSkipped":         "true",
				"opsManualManualId":        "manual-pg-backup-ubuntu",
				"opsManualObjectType":      "postgresql",
				"opsManualOperationAction": "backup",
				"opsManualTargetScope":     "host:pg-ubuntu-01",
				"opsManualFlowId":          "flow-test",
			},
		},
	})

	facts, err := store.ListFacts(context.Background(), "sess-skip-manual", opsmanual.SessionOpsFactFilter{
		Keys: []string{opsmanual.SessionOpsFactOpsManualSuppression},
		Now:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("ListFacts() error = %v", err)
	}
	if len(facts) != 1 {
		t.Fatalf("facts = %#v, want one suppression fact", facts)
	}
	suppression, ok := opsmanual.OpsManualSuppressionFromFact(facts[0])
	if !ok {
		t.Fatalf("fact = %#v, want suppression fact", facts[0])
	}
	if !suppression.Matches(opsmanual.OpsManualSuppression{
		ManualID:    "manual-pg-backup-ubuntu",
		ObjectType:  "postgresql",
		Action:      "backup",
		TargetScope: "host:pg-ubuntu-01",
	}) {
		t.Fatalf("suppression = %#v, want clicked manual scope", suppression)
	}
}

func TestAssistantTransportRecordsManualGuidedReferenceEvent(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	repo := opsmanual.NewMemoryStore()
	domain := opsmanual.NewService(repo)
	server := NewHTTPServer(appui.NewServices(runtime, sessions, appui.WithOpsManualService(appui.NewOpsManualService(domain))))
	state := appui.NewAiopsTransportState("sess-reference-manual", "thread-reference-manual")
	state.CurrentTurnID = "turn-reference-manual"

	server.decorateAssistantTransportAgentUIArtifacts(state, appui.TransportCommand{
		Type: appui.TransportCommandTypeAddMessage,
		AddMessage: &appui.TransportAddMessageCommand{
			SessionID: "sess-reference-manual",
			ThreadID:  "thread-reference-manual",
			Message: appui.TransportUserMessage{
				Text: "仅参考 Redis SSH 排障运维手册继续只读排查。",
			},
			Metadata: map[string]string{
				"opsManualAction":     "reference_ops_manual",
				"opsManualManualId":   "manual-redis-rca-ssh",
				"opsManualWorkflowId": "workflow-redis-rca-ssh",
				"opsManualFlowId":     "flow-reference-redis",
			},
		},
	})

	events, err := repo.ListManualGuidedChatEvents(opsmanual.ListManualGuidedChatEventsRequest{OpsManualFlowID: "flow-reference-redis"})
	if err != nil {
		t.Fatalf("ListManualGuidedChatEvents() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one manual-guided reference event", events)
	}
	event := events[0]
	if event.ReferenceMode != "manual_guided_chat" || event.WorkflowRunID != "" || event.ManualID != "manual-redis-rca-ssh" {
		t.Fatalf("event = %#v, want manual-guided chat without workflow run", event)
	}
	records, err := repo.ListRunRecords(opsmanual.ListRunRecordsRequest{OpsManualFlowID: "flow-reference-redis"})
	if err != nil || len(records) != 0 {
		t.Fatalf("run records = %#v, err=%v; reference must not create workflow run", records, err)
	}
}

func TestAssistantTransportDoesNotShowRunnerWorkflowArtifactWhenGenerationTurnFails(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, runErr: context.DeadlineExceeded}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayloadWithMetadata(t, "", "thread-generate-workflow-failed", "确认生成工作流候选：PostgreSQL 备份 Ubuntu 运维手册", map[string]string{
		"opsManualAction": "generate_runner_workflow_candidate",
	})
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	if !strings.Contains(text, "context deadline exceeded") {
		t.Fatalf("response = %q, want backend error", text)
	}
	if strings.Contains(text, `"type":"runner_workflow_generation"`) {
		t.Fatalf("response = %q, should not show runner_workflow_generation when backend generation fails", text)
	}
}

func TestAssistantTransportAPIApprovalDecisionAcksBeforeResumeCompletes(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-approval-ack", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-approval-ack",
		SessionID:   session.ID,
		SessionType: session.Type,
		Mode:        session.Mode,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-ack",
			SessionID: session.ID,
			TurnID:    "turn-approval-ack",
			Command:   "ifconfig en0 down",
			Reason:    "needs approval",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &assistantTransportBlockingResumeRuntime{
		sessions: sessions,
		started:  make(chan runtimekernel.ResumeRequest, 1),
		release:  make(chan struct{}),
	}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := map[string]any{
		"state": map[string]any{
			"schemaVersion": "aiops.transport.v2",
			"sessionId":     session.ID,
			"threadId":      session.ID,
			"status":        "blocked",
			"currentTurnId": "turn-approval-ack",
			"turns": map[string]any{
				"turn-approval-ack": map[string]any{
					"id":     "turn-approval-ack",
					"status": "blocked",
					"process": []map[string]any{
						{
							"id":         "cmd-approval-ack",
							"kind":       "command",
							"status":     "blocked",
							"command":    "ifconfig en0 down",
							"approvalId": "approval-ack",
						},
					},
				},
			},
			"turnOrder": []string{"turn-approval-ack"},
			"pendingApprovals": map[string]any{
				"approval-ack": map[string]any{
					"id":     "approval-ack",
					"turnId": "turn-approval-ack",
					"status": "blocked",
				},
			},
			"mcpSurfaces": map[string]any{},
			"artifacts":   map[string]any{},
			"runtimeLiveness": map[string]any{
				"activeTurns":          map[string]any{},
				"activeAgents":         map[string]any{},
				"pendingApprovals":     map[string]any{"approval-ack": true},
				"pendingUserInputs":    map[string]any{},
				"activeCommandStreams": map[string]any{},
			},
			"seq":       0,
			"updatedAt": now.Format(time.RFC3339Nano),
		},
		"threadId": session.ID,
		"commands": []map[string]any{
			{
				"type":       "aiops.approval-decision",
				"approvalId": "approval-ack",
				"decision":   "accept",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/assistant/transport", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("read first stream line: %v", err)
	}
	if !strings.Contains(line, `"path":["pendingApprovals"],"value":{}`) {
		t.Fatalf("first stream line = %q, want pendingApprovals cleared before resume completes", line)
	}
	if !strings.Contains(line, `"path":["status"],"value":"working"`) {
		t.Fatalf("first stream line = %q, want transport working ack before resume completes", line)
	}

	select {
	case req := <-runtime.started:
		if req.ApprovalID != "approval-ack" || req.Decision != "approved" {
			t.Fatalf("ResumeTurn request = %+v, want approved approval-ack", req)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ResumeTurn was not started asynchronously")
	}
	close(runtime.release)
}

func TestAssistantTransportAPIHostCommandApprovalDecisionAcksBeforeExecutionCompletes(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	approvalID := "approval-host-command-runtime"

	session := sessions.GetOrCreate("sess-host-command-approval", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-host-command-approval",
		SessionID:   "sess-host-command-approval",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        approvalID,
			SessionID: "sess-host-command-approval",
			TurnID:    "turn-host-command-approval",
			ToolName:  "host_command",
			Command:   "touch /tmp/aiops-check",
			Source:    "host_command_policy",
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}},
	}
	session.PendingApprovals = append([]runtimekernel.PendingApproval(nil), session.CurrentTurn.PendingApprovals...)
	sessions.Update(session)

	runtime := &assistantTransportBlockingResumeRuntime{
		sessions: sessions,
		started:  make(chan runtimekernel.ResumeRequest, 1),
		release:  make(chan struct{}),
	}
	baseServices := appui.NewServices(runtime, sessions)
	server := NewHTTPServer(baseServices)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	now := time.Now().UTC()
	payload := map[string]any{
		"state": map[string]any{
			"schemaVersion": "aiops.transport.v2",
			"sessionId":     "sess-host-command-approval",
			"threadId":      "sess-host-command-approval",
			"status":        "blocked",
			"currentTurnId": "turn-host-command-approval",
			"turns": map[string]any{
				"turn-host-command-approval": map[string]any{
					"id":     "turn-host-command-approval",
					"status": "blocked",
					"process": []map[string]any{
						{
							"id":         "cmd-host-command-approval",
							"kind":       "command",
							"status":     "blocked",
							"command":    "touch /tmp/aiops-check",
							"approvalId": approvalID,
						},
					},
				},
			},
			"turnOrder": []string{"turn-host-command-approval"},
			"pendingApprovals": map[string]any{
				approvalID: map[string]any{
					"id":     approvalID,
					"turnId": "turn-host-command-approval",
					"status": "blocked",
				},
			},
			"mcpSurfaces": map[string]any{},
			"artifacts":   map[string]any{},
			"runtimeLiveness": map[string]any{
				"activeTurns":          map[string]any{},
				"activeAgents":         map[string]any{},
				"pendingApprovals":     map[string]any{approvalID: true},
				"pendingUserInputs":    map[string]any{},
				"activeCommandStreams": map[string]any{},
			},
			"seq":       0,
			"updatedAt": now.Format(time.RFC3339Nano),
		},
		"threadId": "sess-host-command-approval",
		"commands": []map[string]any{
			{
				"type":       "aiops.approval-decision",
				"approvalId": approvalID,
				"decision":   "accept",
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/assistant/transport", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()

	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("read first stream line: %v", err)
	}
	if !strings.Contains(line, `"path":["pendingApprovals"],"value":{}`) {
		t.Fatalf("first stream line = %q, want pendingApprovals cleared before host command execution completes", line)
	}
	if !strings.Contains(line, `"path":["status"],"value":"working"`) {
		t.Fatalf("first stream line = %q, want transport working ack before host command execution completes", line)
	}

	select {
	case req := <-runtime.started:
		if req.ApprovalID != approvalID || req.Decision != "approved" {
			t.Fatalf("ResumeTurn request = %+v, want approved %s", req, approvalID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("ResumeTurn was not started asynchronously")
	}
	close(runtime.release)
}

func TestAssistantTransportDiffPreservesFinalTextWhenTurnMetadataChanges(t *testing.T) {
	start := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	prev := appui.NewAiopsTransportState("sess-1", "thread-1")
	prev.TurnOrder = []string{"turn-1"}
	prev.Turns["turn-1"] = appui.AiopsTransportTurn{
		ID:        "turn-1",
		Status:    appui.AiopsTransportTurnStatusWorking,
		StartedAt: start.Format(time.RFC3339Nano),
		UpdatedAt: start.Format(time.RFC3339Nano),
		Final: &appui.AiopsTransportFinal{
			ID:     "final-1",
			Text:   "第一段",
			Status: appui.AiopsTransportFinalStatusRunning,
		},
	}
	next := prev
	next.Turns = map[string]appui.AiopsTransportTurn{
		"turn-1": {
			ID:        "turn-1",
			Status:    appui.AiopsTransportTurnStatusWorking,
			StartedAt: start.Format(time.RFC3339Nano),
			UpdatedAt: start.Add(time.Second).Format(time.RFC3339Nano),
			Final: &appui.AiopsTransportFinal{
				ID:     "final-1",
				Text:   "第一段第二段",
				Status: appui.AiopsTransportFinalStatusRunning,
			},
		},
	}

	ops := assistantTransportDiffStateOps(prev, next)

	if len(ops) != 2 {
		t.Fatalf("ops length = %d, want metadata set + append-text: %+v", len(ops), ops)
	}
	if ops[0].Type != assistantTransportStreamOpSet {
		t.Fatalf("first op = %+v, want set", ops[0])
	}
	turn, ok := ops[0].Value.(appui.AiopsTransportTurn)
	if !ok {
		t.Fatalf("first op value = %T, want AiopsTransportTurn", ops[0].Value)
	}
	if turn.Final == nil || turn.Final.Text != "第一段" {
		t.Fatalf("set turn final text = %+v, want previous text preserved", turn.Final)
	}
	if ops[1].Type != assistantTransportStreamOpAppendText || ops[1].Value != "第二段" {
		t.Fatalf("second op = %+v, want append second chunk", ops[1])
	}
}

func TestAssistantTransportSessionTurnShouldCloseStreamForSuspendedTurns(t *testing.T) {
	session := &runtimekernel.SessionState{
		CurrentTurn: &runtimekernel.TurnSnapshot{
			ID:          "turn-blocked",
			Lifecycle:   runtimekernel.TurnLifecycleSuspended,
			ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
			UpdatedAt:   time.Now().UTC(),
		},
	}

	if !assistantTransportSessionTurnShouldCloseStream(session) {
		t.Fatal("suspended turn should close assistant transport stream so inline approval can take over")
	}
}

func TestAssistantTransportAPIStreamsFailedStateAndErrorRecordOnBackendError(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, runErr: context.DeadlineExceeded}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := map[string]any{
		"state": map[string]any{
			"schemaVersion":    "aiops.transport.v2",
			"sessionId":        "",
			"threadId":         "thread-1",
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
		"threadId": "thread-1",
		"commands": []map[string]any{
			{
				"type": "add-message",
				"message": map[string]any{
					"role": "user",
					"content": []map[string]any{
						{"type": "text", "text": "investigate payment-api"},
					},
				},
			},
		},
	}
	payload, _ := json.Marshal(body)

	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	text := string(raw)
	const visibleStreamFailure = "模型流中断，已保留已生成内容"
	if !strings.Contains(text, "3:\""+visibleStreamFailure+"\"") {
		t.Fatalf("response = %q, want user-visible error record", text)
	}
	if !strings.Contains(text, "\"path\":[\"lastError\"],\"value\":\""+visibleStreamFailure+"\"") {
		t.Fatalf("response = %q, want lastError update", text)
	}
	if !strings.Contains(text, context.DeadlineExceeded.Error()) {
		t.Fatalf("response = %q, want raw backend error retained in timeline", text)
	}
	if !strings.Contains(text, "\"path\":[\"status\"],\"value\":\"failed\"") {
		t.Fatalf("response = %q, want failed status update", text)
	}
}

func TestAssistantTransportAPIBackendErrorMarksCurrentTurnFailed(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &assistantTransportAPITestRuntime{sessions: sessions, runErr: context.DeadlineExceeded}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	payload := assistantTransportAddMessagePayload(t, "", "thread-1", "investigate payment-api")
	resp, err := http.Post(ts.URL+"/api/v1/assistant/transport", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/assistant/transport error = %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	session := sessions.GetLatest()
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("session current turn is nil after failed run")
	}
	state, err := projectAssistantTransportSessionState(nil, appui.NewAiopsTransportState(session.ID, "thread-1"), session, appui.NewTransportProjector())
	if err != nil {
		t.Fatalf("projectAssistantTransportSessionState() error = %v", err)
	}
	if state.Status != appui.AiopsTransportStatusFailed {
		t.Fatalf("state.Status = %q, want failed", state.Status)
	}
	if state.LastError != "模型流中断，已保留已生成内容" {
		t.Fatalf("state.LastError = %q, want user-visible stream failure text", state.LastError)
	}
	if state.Turns[state.CurrentTurnID].Status != appui.AiopsTransportTurnStatusFailed {
		t.Fatalf("turn status = %q, want failed", state.Turns[state.CurrentTurnID].Status)
	}
}

func TestAssistantTransportProjectionHydratesHostChildSessionCompletion(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	now := time.Now().UTC()
	main := sessions.GetOrCreate("sess-hostops-main", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	main.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-hostops-main",
		SessionID:   main.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]string{
			"aiops.hostops.routeKind":      "host_ops",
			"aiops.hostops.missionId":      "hostops:turn-hostops-main",
			"aiops.hostops.managerAgentId": "hostops-manager:turn-hostops-main",
			"aiops.hostops.mentions":       `[{"raw":"@host-a","hostId":"host-a","displayName":"@host-a","source":"inventory","resolved":true}]`,
		},
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "user-1",
				Type:      agentstate.TurnItemTypeUserMessage,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Summary: "@host-a 检查内存"},
				CreatedAt: now,
			},
			{
				ID:     "spawn-1",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "hostops.spawn_host_agent",
					Summary: "spawned child",
					Data: json.RawMessage(`{
						"toolName":"spawn_host_agent",
						"displayKind":"hostops.spawn_host_agent",
						"outputPreview":{
							"children":[{
								"id":"child-a",
								"missionId":"hostops:turn-hostops-main",
								"sessionId":"host-child:hostops:turn-hostops-main:host-a",
								"hostId":"host-a",
								"hostDisplayName":"@host-a",
								"task":"@host-a 检查内存",
								"status":"running",
								"updatedAt":"2026-06-18T01:00:00Z"
							}]
						}
					}`),
				},
				CreatedAt: now,
			},
		},
	}
	sessions.Update(main)
	child := sessions.GetOrCreate("host-child:hostops:turn-hostops-main:host-a", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	completedAt := now.Add(time.Minute)
	child.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-child-a",
		SessionID:   child.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
		FinalOutput: "内存检查完成：可用 1.0Gi。",
	}
	sessions.Update(child)

	projected, err := projectAssistantTransportSessionState(nil, appui.NewAiopsTransportState(main.ID, main.ID), main, appui.NewTransportProjector(), sessions)
	if err != nil {
		t.Fatalf("projectAssistantTransportSessionState() error = %v", err)
	}
	childState := projected.ChildAgents["child-a"]
	if childState.Status != "completed" {
		t.Fatalf("child status = %q, want completed: %+v", childState.Status, childState)
	}
	if childState.LastOutputPreview != "内存检查完成：可用 1.0Gi。" {
		t.Fatalf("LastOutputPreview = %q", childState.LastOutputPreview)
	}
	mission := projected.HostMissions["hostops:turn-hostops-main"]
	if mission.Status != "completed" {
		t.Fatalf("mission status = %q, want completed: %+v", mission.Status, mission)
	}
}

func TestAssistantTransportStreamWaitsForHostChildCompletionAfterMainTurnCompletes(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	now := time.Now().UTC()
	main := sessions.GetOrCreate("sess-hostops-stream", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	main.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-hostops-stream",
		SessionID:   main.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]string{
			"aiops.hostops.routeKind":      "host_ops",
			"aiops.hostops.missionId":      "hostops:turn-hostops-stream",
			"aiops.hostops.managerAgentId": "hostops-manager:turn-hostops-stream",
		},
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "user-1",
				Type:      agentstate.TurnItemTypeUserMessage,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Summary: "@host-a 检查磁盘"},
				CreatedAt: now,
			},
			{
				ID:     "spawn-1",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "hostops.spawn_host_agent",
					Summary: "spawned child",
					Data: json.RawMessage(`{
						"toolName":"spawn_host_agent",
						"displayKind":"hostops.spawn_host_agent",
						"outputPreview":{
							"children":[{
								"id":"child-stream-a",
								"missionId":"hostops:turn-hostops-stream",
								"sessionId":"host-child:hostops:turn-hostops-stream:host-a",
								"hostId":"host-a",
								"hostDisplayName":"@host-a",
								"task":"@host-a 检查磁盘",
								"status":"running",
								"updatedAt":"2026-06-18T01:00:00Z"
							}]
						}
					}`),
				},
				CreatedAt: now,
			},
		},
	}
	sessions.Update(main)

	child := sessions.GetOrCreate("host-child:hostops:turn-hostops-stream:host-a", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	child.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-child-stream-a",
		SessionID:   child.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
	}
	sessions.Update(child)

	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	writer := &assistantTransportCaptureWriter{}
	initial := appui.NewAiopsTransportState(main.ID, main.ID)
	initial.Status = appui.AiopsTransportStatusWorking
	initial.CurrentTurnID = main.CurrentTurn.ID

	go func() {
		time.Sleep(30 * time.Millisecond)
		nextChild := sessions.Get(child.ID)
		completedAt := now.Add(time.Minute)
		nextChild.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCompleted
		nextChild.CurrentTurn.UpdatedAt = completedAt
		nextChild.CurrentTurn.CompletedAt = &completedAt
		nextChild.CurrentTurn.FinalOutput = "磁盘检查完成：根分区使用率 79%。"
		sessions.Update(nextChild)
	}()

	next, err := server.streamAssistantTransportState(context.Background(), newAssistantTransportStreamEncoder(writer), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err != nil {
		t.Fatalf("streamAssistantTransportState() error = %v", err)
	}
	childState := next.ChildAgents["child-stream-a"]
	if childState.Status != "completed" {
		t.Fatalf("child status = %q, want completed: %+v", childState.Status, childState)
	}
	if childState.LastOutputPreview != "磁盘检查完成：根分区使用率 79%。" {
		t.Fatalf("child output = %q", childState.LastOutputPreview)
	}
	text := writer.String()
	if !strings.Contains(text, "\"status\":\"completed\"") || !strings.Contains(text, "磁盘检查完成") {
		t.Fatalf("stream text = %q, want completed child update", text)
	}
}

func TestTransportDisconnectCancelsActiveRunWhenClientContextCancels(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-disconnect", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-disconnect",
		SessionID:   "sess-disconnect",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "investigate payment-api"}, CreatedAt: now},
		},
	}
	sessions.Update(session)

	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	initial := appui.NewAiopsTransportState("sess-disconnect", "thread-disconnect")
	initial.Status = appui.AiopsTransportStatusWorking
	initial.CurrentTurnID = "turn-disconnect"
	_, err := server.streamAssistantTransportState(ctx, newAssistantTransportStreamEncoder(io.Discard), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err == nil {
		t.Fatal("streamAssistantTransportState() error = nil, want context cancellation")
	}

	updated := sessions.Get("sess-disconnect")
	if updated == nil || updated.CurrentTurn == nil {
		t.Fatal("updated current turn is nil")
	}
	if updated.CurrentTurn.Lifecycle != runtimekernel.TurnLifecycleCanceled {
		t.Fatalf("turn lifecycle = %q, want canceled", updated.CurrentTurn.Lifecycle)
	}
}

func TestAssistantTransportStreamProjectsTerminalTurnFromHistory(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-history-terminal", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	completedAt := now.Add(2 * time.Second)
	session.CurrentTurn = nil
	session.TurnHistory = []runtimekernel.TurnSnapshot{
		{
			ID:          "turn-history-terminal",
			SessionID:   "sess-history-terminal",
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeChat,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			StartedAt:   now,
			UpdatedAt:   completedAt,
			CompletedAt: &completedAt,
			FinalOutput: "历史 turn 的最终回答",
			AgentItems: []agentstate.TurnItem{
				{ID: "user-history", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "hello"}, CreatedAt: now},
				assistantMessageFinalItemForServerTest("final-history", agentstate.ItemStatusCompleted, "历史 turn 的最终回答", completedAt),
			},
		},
	}
	sessions.Update(session)

	runtime := &assistantTransportAPITestRuntime{sessions: sessions}
	server := NewHTTPServer(appui.NewServices(runtime, sessions))
	initial := appui.NewAiopsTransportState("sess-history-terminal", "thread-history-terminal")
	initial.Status = appui.AiopsTransportStatusWorking
	writer := &assistantTransportCaptureWriter{}

	next, err := server.streamAssistantTransportState(context.Background(), newAssistantTransportStreamEncoder(writer), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err != nil {
		t.Fatalf("streamAssistantTransportState() error = %v", err)
	}

	if next.Status != appui.AiopsTransportStatusIdle {
		t.Fatalf("next.Status = %q, want idle", next.Status)
	}
	if next.Turns["turn-history-terminal"].Final == nil || next.Turns["turn-history-terminal"].Final.Text != "历史 turn 的最终回答" {
		t.Fatalf("projected final = %+v, want history final output", next.Turns["turn-history-terminal"].Final)
	}
	text := writer.String()
	if !strings.Contains(text, "\"path\":[\"status\"],\"value\":\"idle\"") || !strings.Contains(text, "历史 turn 的最终回答") {
		t.Fatalf("stream text = %q, want idle status and final output", text)
	}
}

func TestAssistantTransportStreamWaitsForAcceptedTurnBeforeProjectingPreviousTerminalTurn(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-wait-accepted", runtimekernel.SessionTypeHost, runtimekernel.ModeChat)
	now := time.Now().UTC()
	oldCompletedAt := now.Add(-2 * time.Second)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-old",
		SessionID:   session.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now.Add(-5 * time.Second),
		UpdatedAt:   oldCompletedAt,
		CompletedAt: &oldCompletedAt,
		FinalOutput: "旧 turn 输出",
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "turn-old-user",
				Type:      agentstate.TurnItemTypeUserMessage,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Summary: "旧问题"},
				CreatedAt: now.Add(-5 * time.Second),
			},
		},
	}
	sessions.Update(session)

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	initial := appui.NewAiopsTransportState(session.ID, session.ID)
	initial.Status = appui.AiopsTransportStatusWorking
	initial.CurrentTurnID = "turn-new"
	initial.TurnOrder = []string{"turn-old", "turn-new"}
	initial.Turns["turn-old"] = appui.AiopsTransportTurn{
		ID:          "turn-old",
		Status:      appui.AiopsTransportTurnStatusCompleted,
		StartedAt:   now.Add(-5 * time.Second).Format(time.RFC3339Nano),
		CompletedAt: oldCompletedAt.Format(time.RFC3339Nano),
		Final: &appui.AiopsTransportFinal{
			ID:     "turn-old-final",
			Text:   "旧 turn 输出",
			Status: appui.AiopsTransportFinalStatusCompleted,
		},
	}
	initial.Turns["turn-new"] = appui.AiopsTransportTurn{
		ID:        "turn-new",
		Status:    appui.AiopsTransportTurnStatusSubmitted,
		StartedAt: now.Format(time.RFC3339Nano),
		User: &appui.AiopsTransportMessage{
			ID:        "turn-new-user",
			Text:      "第二次请求",
			CreatedAt: now.Format(time.RFC3339Nano),
		},
	}
	initial.RuntimeLiveness.ActiveTurns["turn-new"] = true

	go func() {
		time.Sleep(20 * time.Millisecond)
		updated := sessions.Get(session.ID)
		if updated == nil {
			return
		}
		startedAt := time.Now().UTC()
		updated.CurrentTurn = &runtimekernel.TurnSnapshot{
			ID:          "turn-new",
			SessionID:   updated.ID,
			SessionType: runtimekernel.SessionTypeHost,
			Mode:        runtimekernel.ModeChat,
			Lifecycle:   runtimekernel.TurnLifecycleRunning,
			StartedAt:   startedAt,
			UpdatedAt:   startedAt,
			AgentItems: []agentstate.TurnItem{
				{
					ID:        "turn-new-user",
					Type:      agentstate.TurnItemTypeUserMessage,
					Status:    agentstate.ItemStatusCompleted,
					Payload:   agentstate.PayloadEnvelope{Summary: "第二次请求"},
					CreatedAt: startedAt,
				},
			},
		}
		updated.ActiveTurn = runtimekernel.ActiveTurnState{
			TurnID: "turn-new",
			Kind:   "regular",
			Status: string(runtimekernel.TurnLifecycleRunning),
		}
		sessions.Update(updated)

		time.Sleep(20 * time.Millisecond)
		updated = sessions.Get(session.ID)
		if updated == nil || updated.CurrentTurn == nil {
			return
		}
		completedAt := time.Now().UTC()
		updated.CurrentTurn.Lifecycle = runtimekernel.TurnLifecycleCompleted
		updated.CurrentTurn.UpdatedAt = completedAt
		updated.CurrentTurn.CompletedAt = &completedAt
		updated.CurrentTurn.FinalOutput = "第二次请求已完成"
		updated.CurrentTurn.AgentItems = append(updated.CurrentTurn.AgentItems,
			assistantMessageFinalItemForServerTest("turn-new-final", agentstate.ItemStatusCompleted, "第二次请求已完成", completedAt),
		)
		updated.ActiveTurn = runtimekernel.ActiveTurnState{
			TurnID: "turn-new",
			Kind:   "regular",
			Status: string(runtimekernel.TurnLifecycleCompleted),
		}
		sessions.Update(updated)
	}()

	writer := &assistantTransportCaptureWriter{}
	next, err := server.streamAssistantTransportState(context.Background(), newAssistantTransportStreamEncoder(writer), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err != nil {
		t.Fatalf("streamAssistantTransportState() error = %v", err)
	}
	if next.CurrentTurnID != "turn-new" {
		t.Fatalf("next.CurrentTurnID = %q, want turn-new", next.CurrentTurnID)
	}
	if next.Turns["turn-new"].Final == nil || next.Turns["turn-new"].Final.Text != "第二次请求已完成" {
		t.Fatalf("projected new turn final = %+v, want second turn final output", next.Turns["turn-new"].Final)
	}
	text := writer.String()
	if strings.Contains(text, "\"path\":[\"currentTurnId\"],\"value\":\"turn-old\"") {
		t.Fatalf("stream text = %q, should not project previous terminal currentTurnId", text)
	}
	if !strings.Contains(text, "第二次请求已完成") {
		t.Fatalf("stream text = %q, want second turn final output", text)
	}
}

func TestAssistantTransportStreamWaitsForRuntimeAfterApprovalAcceptedLocally(t *testing.T) {
	now := time.Now().UTC()
	initial := appui.NewAiopsTransportState("sess-approved-local", "thread-approved-local")
	initial.Status = appui.AiopsTransportStatusWorking
	initial.CurrentTurnID = "turn-approved-local"
	initial.TurnOrder = []string{"turn-approved-local"}
	initial.Turns["turn-approved-local"] = appui.AiopsTransportTurn{
		ID:     "turn-approved-local",
		Status: appui.AiopsTransportTurnStatusWorking,
		Process: []appui.AiopsProcessBlock{{
			ID:         "cmd-approved-local",
			Kind:       appui.AiopsTransportProcessKindCommand,
			Status:     appui.AiopsTransportProcessStatusRunning,
			Command:    "ifconfig en0 down",
			Text:       "ifconfig en0 down",
			ApprovalID: "approval-stale",
		}},
	}
	initial.RuntimeLiveness.ActiveTurns["turn-approved-local"] = true

	latest := &runtimekernel.TurnSnapshot{
		ID:          "turn-approved-local",
		SessionID:   "sess-approved-local",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-stale",
			SessionID: "sess-approved-local",
			TurnID:    "turn-approved-local",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}

	if !assistantTransportShouldWaitForAcceptedApproval(initial, latest) {
		t.Fatal("assistantTransportShouldWaitForAcceptedApproval() = false, want true while runtime still exposes accepted pending approval")
	}
}

func TestAssistantTransportStreamDoesNotWaitWhenRuntimeFirstReportsPendingApproval(t *testing.T) {
	now := time.Now().UTC()
	initial := appui.NewAiopsTransportState("sess-new-approval", "thread-new-approval")
	initial.Status = appui.AiopsTransportStatusWorking
	initial.CurrentTurnID = "turn-new-approval"
	initial.TurnOrder = []string{"turn-new-approval"}
	initial.Turns["turn-new-approval"] = appui.AiopsTransportTurn{
		ID:     "turn-new-approval",
		Status: appui.AiopsTransportTurnStatusWorking,
		Process: []appui.AiopsProcessBlock{{
			ID:      "cmd-launchctl",
			Kind:    appui.AiopsTransportProcessKindCommand,
			Status:  appui.AiopsTransportProcessStatusRunning,
			Command: "launchctl print system/com.docker.helper",
			Text:    "launchctl print system/com.docker.helper",
		}},
	}
	initial.RuntimeLiveness.ActiveTurns["turn-new-approval"] = true

	latest := &runtimekernel.TurnSnapshot{
		ID:          "turn-new-approval",
		SessionID:   "sess-new-approval",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-new",
			SessionID:  "sess-new-approval",
			TurnID:     "turn-new-approval",
			ToolName:   "exec_command",
			ToolCallID: "call-launchctl",
			Reason:     "non-read-only terminal command requires a signed ActionToken",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
	}

	if assistantTransportShouldWaitForAcceptedApproval(initial, latest) {
		t.Fatal("assistantTransportShouldWaitForAcceptedApproval() = true, want false so the blocked approval state is projected")
	}
}

func TestAssistantTransportStreamWaitsForRuntimeAfterApprovalRejectedLocally(t *testing.T) {
	now := time.Now().UTC()
	initial := appui.NewAiopsTransportState("sess-rejected-local", "thread-rejected-local")
	initial.Status = appui.AiopsTransportStatusFailed
	initial.CurrentTurnID = "turn-rejected-local"
	initial.TurnOrder = []string{"turn-rejected-local"}
	initial.Turns["turn-rejected-local"] = appui.AiopsTransportTurn{
		ID:     "turn-rejected-local",
		Status: appui.AiopsTransportTurnStatusFailed,
		Process: []appui.AiopsProcessBlock{{
			ID:         "cmd-rejected-local",
			Kind:       appui.AiopsTransportProcessKindCommand,
			Status:     appui.AiopsTransportProcessStatusRejected,
			Command:    "ifconfig en0 down",
			Text:       "ifconfig en0 down",
			ApprovalID: "approval-stale",
		}},
	}

	latest := &runtimekernel.TurnSnapshot{
		ID:          "turn-rejected-local",
		SessionID:   "sess-rejected-local",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now,
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-stale",
			SessionID: "sess-rejected-local",
			TurnID:    "turn-rejected-local",
			CreatedAt: now,
			UpdatedAt: now,
		}},
	}

	if !assistantTransportShouldWaitForAcceptedApproval(initial, latest) {
		t.Fatal("assistantTransportShouldWaitForAcceptedApproval() = false, want true while runtime still exposes rejected pending approval")
	}
}

func TestAssistantTransportStreamClearsApprovalWithoutTransportErrorOnDeniedApproval(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("sess-denied-approval", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	now := time.Now().UTC()
	completedAt := now.Add(time.Second)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-denied-approval",
		SessionID:   session.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleFailed,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   completedAt,
		CompletedAt: &completedAt,
		Error:       "approval denied",
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "user-denied",
				Type:      agentstate.TurnItemTypeUserMessage,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Summary: "运行 ifconfig en0 down"},
				CreatedAt: now,
			},
		},
	}
	sessions.Update(session)

	server := NewHTTPServer(appui.NewServices(&assistantTransportAPITestRuntime{sessions: sessions}, sessions))
	initial := appui.NewAiopsTransportState(session.ID, session.ID)
	initial.Status = appui.AiopsTransportStatusBlocked
	initial.CurrentTurnID = "turn-denied-approval"
	initial.PendingApprovals["approval-stale"] = appui.AiopsTransportApproval{
		ID:     "approval-stale",
		TurnID: "turn-denied-approval",
		Status: string(appui.AiopsTransportProcessStatusBlocked),
	}
	initial.RuntimeLiveness.PendingApprovals["approval-stale"] = true

	writer := &assistantTransportCaptureWriter{}
	next, err := server.streamAssistantTransportState(context.Background(), newAssistantTransportStreamEncoder(writer), sessions, appui.NewTransportProjector(), server.ui.ChatService(), initial)
	if err != nil {
		t.Fatalf("streamAssistantTransportState() error = %v", err)
	}

	if len(next.PendingApprovals) != 0 {
		t.Fatalf("next.PendingApprovals = %#v, want cleared approvals", next.PendingApprovals)
	}
	text := writer.String()
	if strings.Contains(text, "3:\"approval denied\"") {
		t.Fatalf("stream text = %q, should not emit a transport error for user-denied approval", text)
	}
	if !strings.Contains(text, "\"path\":[\"pendingApprovals\"],\"value\":{}") {
		t.Fatalf("stream text = %q, want pendingApprovals cleared", text)
	}
}

func assistantTransportAddMessagePayload(t *testing.T, sessionID, threadID, message string) []byte {
	return assistantTransportAddMessagePayloadWithMetadata(t, sessionID, threadID, message, nil)
}

func assistantTransportAddMessagePayloadWithMetadata(t *testing.T, sessionID, threadID, message string, metadata map[string]string) []byte {
	t.Helper()
	body := map[string]any{
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
		"threadId": threadID,
		"commands": []map[string]any{
			{
				"type": "add-message",
				"message": map[string]any{
					"role":     "user",
					"metadata": metadata,
					"content": []map[string]any{
						{"type": "text", "text": message},
					},
				},
			},
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return payload
}

func assistantTransportRedisManual() opsmanual.OpsManual {
	return opsmanual.OpsManual{
		ID:          "manual-redis-memory",
		Title:       "Redis 内存压力排障",
		Status:      opsmanual.ManualStatusVerified,
		WorkflowRef: opsmanual.WorkflowRef{WorkflowID: "workflow-redis-memory"},
		Operation:   opsmanual.OperationProfile{TargetType: "redis", Action: "rca_or_repair", Stateful: true},
		Applicability: opsmanual.ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext: opsmanual.RequiredContext{
			RequiredInputs:   []string{"target_instance"},
			RequiredEvidence: []string{"used_memory_rss", "p95"},
		},
		Preconditions:    []string{"can connect"},
		Validation:       []string{"memory recovered"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Redis memory pressure manual.",
	}
}

func assistantMessageFinalItemForServerTest(id string, status agentstate.ItemStatus, summary string, at time.Time) agentstate.TurnItem {
	streamState := "complete"
	if status == agentstate.ItemStatusRunning {
		streamState = "streaming"
	}
	data, _ := json.Marshal(map[string]string{
		"displayKind": "assistant.message",
		"phase":       "final_answer",
		"streamState": streamState,
	})
	return agentstate.TurnItem{
		ID:     id,
		Type:   agentstate.TurnItemTypeAssistantMessage,
		Status: status,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "assistant_message",
			Summary: summary,
			Data:    data,
		},
		CreatedAt: at,
		UpdatedAt: at,
	}
}
