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

func TestWorkflowCandidateReviseUsesStrictBodyAndTrustedScope(t *testing.T) {
	application := &recordingWorkflowCandidateReviseApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	payload := workflowCandidateReviseTestPayload()
	response := postWorkflowCandidateRevise(t, server, payload)
	if response.Code != http.StatusOK || len(application.requests) != 1 {
		t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(application.requests))
	}
	request := application.requests[0]
	if request.WorkflowKey != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) || request.ActorID != "actor-a" || request.CanvasSessionID != "canvas-a" || request.SessionID != "session-a" {
		t.Fatalf("trusted scope not projected: %#v", request)
	}
	if request.CandidateID != "candidate-a" || request.BaseChangeSetID != "changeset-a" || request.IdempotencyKey != "revise-once" || request.AdditionalOperation.Value == nil {
		t.Fatalf("candidate revision request = %#v", request)
	}

	for _, field := range []string{"tenantId", "actorId", "canvasSessionId", "sessionId"} {
		t.Run("reject "+field, func(t *testing.T) {
			forged := workflowCandidateReviseTestPayload()
			forged[field] = "forged"
			response := postWorkflowCandidateRevise(t, server, forged)
			if response.Code != http.StatusBadRequest || len(application.requests) != 1 {
				t.Fatalf("status=%d calls=%d, want strict rejection", response.Code, len(application.requests))
			}
		})
	}
}

func TestWorkflowCandidateReviseRejectsUnknownNestedFieldsAndCrossScope(t *testing.T) {
	application := &recordingWorkflowCandidateReviseApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	payload := workflowCandidateReviseTestPayload()
	operation := payload["additionalOperation"].(map[string]any)
	operation["unexpected"] = true
	response := postWorkflowCandidateRevise(t, server, payload)
	if response.Code != http.StatusBadRequest || len(application.requests) != 0 {
		t.Fatalf("nested unknown field status=%d calls=%d", response.Code, len(application.requests))
	}

	payload = workflowCandidateReviseTestPayload()
	payload["workflowId"] = "workflow-forbidden"
	response = postWorkflowCandidateRevise(t, server, payload)
	if response.Code != http.StatusForbidden || len(application.requests) != 0 {
		t.Fatalf("cross-scope status=%d calls=%d", response.Code, len(application.requests))
	}
}

func TestWorkflowCandidateReviseRequiresFullRevisionAndPost(t *testing.T) {
	application := &recordingWorkflowCandidateReviseApplication{}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	payload := workflowCandidateReviseTestPayload()
	payload["expected"] = map[string]any{"resourceVersion": "rv-1", "semanticHash": "semantic-1"}
	response := postWorkflowCandidateRevise(t, server, payload)
	if response.Code != http.StatusBadRequest || len(application.requests) != 0 {
		t.Fatalf("partial revision status=%d calls=%d", response.Code, len(application.requests))
	}

	raw, _ := json.Marshal(workflowCandidateReviseTestPayload())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/runner-studio/workflow-candidate-revise", bytes.NewReader(raw))
	server.handleWorkflowCandidateRevise(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || len(application.requests) != 0 {
		t.Fatalf("GET status=%d calls=%d", recorder.Code, len(application.requests))
	}
}

type recordingWorkflowCandidateReviseApplication struct {
	requests []service.WorkflowCandidateReviseRequest
	result   service.WorkflowEditPreviewResult
	err      error
}

func (application *recordingWorkflowCandidateReviseApplication) ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	return service.WorkflowManualCommandResult{}, nil
}

func (application *recordingWorkflowCandidateReviseApplication) ReviseCandidate(_ context.Context, request service.WorkflowCandidateReviseRequest) (service.WorkflowEditPreviewResult, error) {
	application.requests = append(application.requests, request)
	return application.result, application.err
}

func workflowCandidateReviseTestPayload() map[string]any {
	return map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "candidateId": "candidate-a",
		"baseChangeSetId": "changeset-a", "idempotencyKey": "revise-once", "intent": "move the candidate node",
		"expected": map[string]any{"resourceVersion": "rv-1", "semanticHash": "semantic-1", "layoutHash": "layout-1"},
		"additionalOperation": map[string]any{
			"op": "update_node", "nodeId": "check", "set": map[string]any{"position": map[string]any{"x": 120, "y": 80}},
		},
	}
}

func postWorkflowCandidateRevise(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-candidate-revise", bytes.NewReader(raw))
	server.handleWorkflowCandidateRevise(recorder, request)
	return recorder
}
