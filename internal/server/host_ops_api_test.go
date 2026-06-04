package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
				{ID: "item-1", Type: hostops.TranscriptItemManagerMessage, Content: "检查PG版本"},
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
	if payload.ChildAgentID != "agent-1" || len(payload.Items) != 1 || payload.Items[0].Content != "检查PG版本" {
		t.Fatalf("payload = %+v, want agent-1 transcript item", payload)
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
	transcript appui.HostChildTranscriptView
}

func (s *hostOpsAPITestHostOpsService) AcceptPlan(context.Context, string, string) (appui.HostOperationView, error) {
	return appui.HostOperationView{}, nil
}

func (s *hostOpsAPITestHostOpsService) RevisePlan(context.Context, string, string) (appui.HostOperationView, error) {
	return appui.HostOperationView{}, nil
}

func (s *hostOpsAPITestHostOpsService) SendChildMessage(context.Context, string, string) (appui.HostChildAgentView, error) {
	return appui.HostChildAgentView{}, nil
}

func (s *hostOpsAPITestHostOpsService) StopChildAgent(context.Context, string) (appui.HostChildAgentView, error) {
	return appui.HostChildAgentView{}, nil
}

func (s *hostOpsAPITestHostOpsService) ChildTranscript(_ context.Context, childAgentID string) (appui.HostChildTranscriptView, error) {
	if s.transcript.ChildAgentID == "" {
		s.transcript.ChildAgentID = childAgentID
	}
	return s.transcript, nil
}
