package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

func TestHostOpsTranscriptAPIRequiresChildAgentID(t *testing.T) {
	srv := NewHTTPServer(hostOpsAPITestServices{Services: appui.NewServices(hostOpsAPITestRuntime{}, runtimekernel.NewSessionManager())})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-ops/child-agents/transcript", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHostOpsTranscriptAPIReturnsTranscriptItems(t *testing.T) {
	service := &hostOpsAPITestHostOpsService{
		transcript: appui.HostChildTranscriptView{
			ChildAgentID: "agent-1",
			Items: []hostops.TranscriptItem{
				{ID: "item-1", Type: hostops.TranscriptItemManagerMessage, Content: "收集主机状态证据"},
			},
		},
	}
	srv := NewHTTPServer(hostOpsAPITestServices{
		Services: appui.NewServices(hostOpsAPITestRuntime{}, runtimekernel.NewSessionManager()),
		hostOps:  service,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-ops/child-agents/agent-1/transcript", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var payload appui.HostChildTranscriptView
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ChildAgentID != "agent-1" || len(payload.Items) != 1 || payload.Items[0].Content != "收集主机状态证据" {
		t.Fatalf("payload = %+v, want agent-1 transcript item", payload)
	}
}

func TestHostOpsTranscriptAPIAddsHostAgentRuntimeConversation(t *testing.T) {
	now := time.Date(2026, 6, 17, 19, 31, 0, 0, time.UTC)
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate("host-child:hostops:turn-1:host-a", runtimekernel.SessionTypeHost, runtimekernel.ModeInspect)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:          "turn-1",
		SessionID:   session.ID,
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "model-1",
				Type:   agentstate.TurnItemTypeModelCall,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "calling model",
					Data:    json.RawMessage(`{"iteration":1,"traceFile":".data/model-input-traces/host-a/iteration-001.md","visibleTools":["host_command"]}`),
				},
				CreatedAt: now,
			},
			{
				ID:     "tool-call-1",
				Type:   agentstate.TurnItemTypeToolCall,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "host_command",
					Data:    json.RawMessage(`{"toolName":"host_command","inputSummary":"docker stats --no-stream"}`),
				},
				CreatedAt: now.Add(time.Second),
			},
			{
				ID:     "tool-result-1",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "runner-web 0.00%",
					Data:    json.RawMessage(`{"toolName":"host_command","outputSummary":"runner-web 0.00%","evidenceRefs":["ev-runtime-1"]}`),
				},
				CreatedAt: now.Add(2 * time.Second),
			},
			assistantMessageFinalItemForServerTest("final-1", agentstate.ItemStatusCompleted, "容器资源正常", now.Add(3*time.Second)),
		},
	}
	sessions.Update(session)
	service := &hostOpsAPITestHostOpsService{
		transcript: appui.HostChildTranscriptView{
			ChildAgentID: "host-child-hostops-turn-1-host-a",
			Items: []hostops.TranscriptItem{
				{ID: "manager-1", Type: hostops.TranscriptItemManagerMessage, Content: "检查主机 Docker 资源", CreatedAt: now},
			},
		},
	}
	srv := NewHTTPServer(hostOpsAPITestServices{
		Services: appui.NewServices(hostOpsAPITestRuntime{}, sessions),
		hostOps:  service,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/host-ops/child-agents/host-child-hostops-turn-1-host-a/transcript", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var payload appui.HostChildTranscriptView
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertTranscriptContains(t, payload.Items, "manager_message", "检查主机 Docker 资源")
	assertTranscriptContains(t, payload.Items, "llm_request", ".data/model-input-traces/host-a/iteration-001.md")
	assertTranscriptContains(t, payload.Items, "tool_call", "docker stats --no-stream")
	assertTranscriptContains(t, payload.Items, "tool_result", "runner-web 0.00%")
	assertTranscriptContains(t, payload.Items, "tool_result", "ev-runtime-1")
	assertTranscriptContains(t, payload.Items, "llm_response", "容器资源正常")
}

func TestHostOpsMissionAPICreatesAndGetsMission(t *testing.T) {
	service := &hostOpsAPITestHostOpsService{
		mission: appui.HostOperationView{ID: "mission-1", Status: "waiting_plan_acceptance", PlanRequired: true},
	}
	srv := NewHTTPServer(hostOpsAPITestServices{
		Services: appui.NewServices(hostOpsAPITestRuntime{}, runtimekernel.NewSessionManager()),
		hostOps:  service,
	})
	body := []byte(`{"id":"mission-1","goal":"在多台主机上执行通用运维任务","hostIds":["host-a","host-b"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-ops/missions", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if service.create.Goal == "" || len(service.create.HostIDs) != 2 {
		t.Fatalf("CreateMission command = %+v", service.create)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/host-ops/missions/mission-1", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if service.getMissionID != "mission-1" {
		t.Fatalf("GetMission id = %q, want mission-1", service.getMissionID)
	}
}

func assertTranscriptContains(t *testing.T, items []hostops.TranscriptItem, itemType string, wantContent string) {
	t.Helper()
	for _, item := range items {
		if string(item.Type) == itemType && strings.Contains(item.Content, wantContent) {
			return
		}
	}
	t.Fatalf("transcript does not contain type %q with content %q: %+v", itemType, wantContent, items)
}

func TestHostOpsMissionAPIAcceptsAndRevisesPlan(t *testing.T) {
	service := &hostOpsAPITestHostOpsService{
		mission: appui.HostOperationView{ID: "mission-1", Status: "running", PlanAccepted: true},
	}
	srv := NewHTTPServer(hostOpsAPITestServices{
		Services: appui.NewServices(hostOpsAPITestRuntime{}, runtimekernel.NewSessionManager()),
		hostOps:  service,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-ops/missions/mission-1/plans/plan-1/accept", nil)
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if service.acceptMissionID != "mission-1" || service.acceptPlanID != "plan-1" {
		t.Fatalf("accept mission/plan = %q/%q", service.acceptMissionID, service.acceptPlanID)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/host-ops/missions/mission-1/plans/revise", bytes.NewReader([]byte(`{"instruction":"调整步骤顺序"}`)))
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if service.reviseMissionID != "mission-1" || service.reviseInstruction != "调整步骤顺序" {
		t.Fatalf("revise = %q/%q", service.reviseMissionID, service.reviseInstruction)
	}
}

func TestHostOpsChildAgentAPIMessageAndStop(t *testing.T) {
	service := &hostOpsAPITestHostOpsService{
		child: appui.HostChildAgentView{ID: "agent-1", Status: "running"},
	}
	srv := NewHTTPServer(hostOpsAPITestServices{
		Services: appui.NewServices(hostOpsAPITestRuntime{}, runtimekernel.NewSessionManager()),
		hostOps:  service,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-ops/child-agents/agent-1/messages", bytes.NewReader([]byte(`{"content":"继续执行下一步"}`)))
	rec := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if service.messageChildID != "agent-1" || service.messageContent != "继续执行下一步" {
		t.Fatalf("message = %q/%q", service.messageChildID, service.messageContent)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/host-ops/child-agents/agent-1/stop", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if service.stopChildID != "agent-1" {
		t.Fatalf("stop child id = %q, want agent-1", service.stopChildID)
	}
}

type hostOpsAPITestRuntime struct{}

func (hostOpsAPITestRuntime) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (hostOpsAPITestRuntime) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (hostOpsAPITestRuntime) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type hostOpsAPITestServices struct {
	*appui.Services
	hostOps appui.HostOpsService
}

func (s hostOpsAPITestServices) HostOpsService() appui.HostOpsService {
	return s.hostOps
}

type hostOpsAPITestHostOpsService struct {
	create            appui.HostMissionCreateCommand
	mission           appui.HostOperationView
	getMissionID      string
	acceptMissionID   string
	acceptPlanID      string
	reviseMissionID   string
	reviseInstruction string
	messageChildID    string
	messageContent    string
	stopChildID       string
	child             appui.HostChildAgentView
	transcript        appui.HostChildTranscriptView
}

func (s *hostOpsAPITestHostOpsService) CreateMission(_ context.Context, command appui.HostMissionCreateCommand) (appui.HostOperationView, error) {
	s.create = command
	return s.mission, nil
}

func (s *hostOpsAPITestHostOpsService) GetMission(_ context.Context, missionID string) (appui.HostOperationView, error) {
	s.getMissionID = missionID
	return s.mission, nil
}

func (s *hostOpsAPITestHostOpsService) AcceptPlan(_ context.Context, missionID, planID string) (appui.HostOperationView, error) {
	s.acceptMissionID = missionID
	s.acceptPlanID = planID
	return s.mission, nil
}

func (s *hostOpsAPITestHostOpsService) RevisePlan(_ context.Context, missionID, instruction string) (appui.HostOperationView, error) {
	s.reviseMissionID = missionID
	s.reviseInstruction = instruction
	return s.mission, nil
}

func (s *hostOpsAPITestHostOpsService) SendChildMessage(_ context.Context, childAgentID, content string) (appui.HostChildAgentView, error) {
	s.messageChildID = childAgentID
	s.messageContent = content
	return s.child, nil
}

func (s *hostOpsAPITestHostOpsService) StopChildAgent(_ context.Context, childAgentID string) (appui.HostChildAgentView, error) {
	s.stopChildID = childAgentID
	return s.child, nil
}

func (s *hostOpsAPITestHostOpsService) ChildTranscript(_ context.Context, childAgentID string) (appui.HostChildTranscriptView, error) {
	if s.transcript.ChildAgentID == "" {
		s.transcript.ChildAgentID = childAgentID
	}
	return s.transcript, nil
}
