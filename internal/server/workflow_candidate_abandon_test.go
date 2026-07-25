package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"runner/server/service"
	edit "runner/workflow/edit"
)

func TestWorkflowCandidateAbandonUsesStrictBindingAndTrustedScope(t *testing.T) {
	application := &recordingWorkflowCandidateAbandonApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	payload := workflowCandidateAbandonTestPayload()
	response := postWorkflowCandidateAbandon(t, server, payload)
	if response.Code != http.StatusOK || len(application.requests) != 1 {
		t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(application.requests))
	}
	request := application.requests[0]
	if request.WorkflowKey != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) || request.ActorID != "actor-a" || request.CanvasSessionID != "canvas-a" || request.SessionID != "session-a" {
		t.Fatalf("trusted scope = %#v", request)
	}
	if request.CandidateRevision != 2 || request.ChangeSetID != "changeset-2" || request.PreviewHash != "preview-2" {
		t.Fatalf("binding = %#v", request)
	}
	for _, field := range []string{"tenantId", "actorId", "graph", "unexpected"} {
		forged := workflowCandidateAbandonTestPayload()
		forged[field] = "forged"
		response = postWorkflowCandidateAbandon(t, server, forged)
		if response.Code != http.StatusBadRequest || len(application.requests) != 1 {
			t.Fatalf("field %s status=%d calls=%d", field, response.Code, len(application.requests))
		}
	}
}

func TestWorkflowCandidateAbandonRejectsPartialRevisionCrossScopeAndGet(t *testing.T) {
	application := &recordingWorkflowCandidateAbandonApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	payload := workflowCandidateAbandonTestPayload()
	payload["expected"] = map[string]any{"resourceVersion": "rv-1"}
	if response := postWorkflowCandidateAbandon(t, server, payload); response.Code != http.StatusBadRequest {
		t.Fatalf("partial revision status=%d", response.Code)
	}
	payload = workflowCandidateAbandonTestPayload()
	payload["workflowId"] = "workflow-forbidden"
	if response := postWorkflowCandidateAbandon(t, server, payload); response.Code != http.StatusForbidden {
		t.Fatalf("cross-scope status=%d", response.Code)
	}
	raw, _ := json.Marshal(workflowCandidateAbandonTestPayload())
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/runner-studio/workflow-candidate-actions", bytes.NewReader(raw)))
	if recorder.Code != http.StatusMethodNotAllowed || len(application.requests) != 0 {
		t.Fatalf("GET status=%d calls=%d", recorder.Code, len(application.requests))
	}
}

type recordingWorkflowCandidateAbandonApplication struct {
	requests []service.WorkflowCandidateAbandonRequest
}

func (application *recordingWorkflowCandidateAbandonApplication) ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	return service.WorkflowManualCommandResult{}, nil
}

func (application *recordingWorkflowCandidateAbandonApplication) AbandonCandidate(_ context.Context, request service.WorkflowCandidateAbandonRequest) (service.WorkflowCandidateAbandonResult, error) {
	application.requests = append(application.requests, request)
	return service.WorkflowCandidateAbandonResult{Kind: "candidate_discarded", SchemaVersion: edit.SchemaVersion}, nil
}

func workflowCandidateAbandonTestPayload() map[string]any {
	return map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "candidateId": "candidate-a", "candidateRevision": 2,
		"changeSetId": "changeset-2", "previewHash": "preview-2", "idempotencyKey": "discard-once", "action": "discard",
		"expected": map[string]any{"resourceVersion": "rv-1", "semanticHash": "semantic-1", "layoutHash": "layout-1"},
	}
}

func postWorkflowCandidateAbandon(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-candidate-actions", bytes.NewReader(raw)))
	return recorder
}
