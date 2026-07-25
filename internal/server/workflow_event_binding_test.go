package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"runner/server/service"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowGraphOpenLazilyBindsDurableEventsOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &workflowEventBindingSource{events: make(chan edit.WorkflowDomainEvent)}
	upstreamCalls := 0
	server := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalls++
			w.WriteHeader(http.StatusOK)
		})),
		withWorkflowCommandApplication(nil, WorkflowCommandScopeResolverFunc(func(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error) {
			return WorkflowCommandTrustedScope{TenantID: "tenant-a", ActorID: "user-a", CanvasSessionID: "canvas-a", SessionID: "session-a"}, nil
		})),
		WithWorkflowEventSource(ctx, source),
	)

	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/api/runner-studio/workflows/workflow-a/graph", nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("graph status = %d", response.Code)
		}
	}
	if upstreamCalls != 2 {
		t.Fatalf("upstream calls = %d, want 2", upstreamCalls)
	}
	recoverCalls, subscribeCalls, _ := source.calls()
	if recoverCalls != 1 || subscribeCalls != 1 {
		t.Fatalf("event binding calls = recover:%d subscribe:%d, want 1/1", recoverCalls, subscribeCalls)
	}

	server.CloseWorkflowEventBindings()
	deadline := time.Now().Add(time.Second)
	for {
		_, _, stopCalls := source.calls()
		if stopCalls == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("event stop calls = %d, want 1", stopCalls)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWorkflowEventBindingDoesNotActivateForNonGraphRunnerRoute(t *testing.T) {
	source := &workflowEventBindingSource{events: make(chan edit.WorkflowDomainEvent)}
	server := NewHTTPServer(
		appui.NewServices(websocketAPITestRuntime{}, nil),
		WithRunnerStudioHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })),
		withWorkflowCommandApplication(nil, WorkflowCommandScopeResolverFunc(func(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error) {
			return WorkflowCommandTrustedScope{TenantID: "tenant-a", ActorID: "user-a", CanvasSessionID: "canvas-a", SessionID: "session-a"}, nil
		})),
		WithWorkflowEventSource(context.Background(), source),
	)
	request := httptest.NewRequest(http.MethodGet, "/api/runner-studio/workflows", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	recoverCalls, subscribeCalls, _ := source.calls()
	if recoverCalls != 0 || subscribeCalls != 0 {
		t.Fatalf("non-graph route unexpectedly bound events: recover:%d subscribe:%d", recoverCalls, subscribeCalls)
	}
}

type workflowEventBindingSource struct {
	mu             sync.Mutex
	events         chan edit.WorkflowDomainEvent
	recoverCalls   int
	subscribeCalls int
	stopCalls      int
}

func (s *workflowEventBindingSource) Recover(context.Context, edit.WorkflowKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoverCalls++
	return nil
}

func (s *workflowEventBindingSource) Replay(context.Context, edit.WorkflowKey, uint64, int) (service.WorkflowEventReplay, error) {
	return service.WorkflowEventReplay{}, nil
}

func (s *workflowEventBindingSource) Snapshot(_ context.Context, key edit.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	return service.WorkflowEditSnapshot{Key: key, Revision: edit.Revision{ResourceVersion: "rv-1", SemanticHash: "semantic-1", LayoutHash: "layout-1"}, Graph: visual.Graph{}}, nil
}

func (s *workflowEventBindingSource) Subscribe(context.Context, edit.WorkflowKey, uint64) (<-chan edit.WorkflowDomainEvent, func(), error) {
	s.mu.Lock()
	s.subscribeCalls++
	s.mu.Unlock()
	return s.events, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.stopCalls++
	}, nil
}

func (s *workflowEventBindingSource) calls() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recoverCalls, s.subscribeCalls, s.stopCalls
}
