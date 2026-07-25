package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"runner/server/service"
	edit "runner/workflow/edit"
)

func TestWorkflowHistoryCommandUsesTrustedScopeAndCreatesPreviewOnly(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{}
	server := newWorkflowHistoryTestServer(application, workflowCommandTestScopeResolver())
	expected := workflowHistoryTestRevision()

	response := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-idem-1", expected))
	if response.Code != http.StatusOK || len(application.undoRequests) != 1 || len(application.redoRequests) != 0 {
		t.Fatalf("status=%d body=%s undo=%d redo=%d", response.Code, response.Body.String(), len(application.undoRequests), len(application.redoRequests))
	}
	request := application.undoRequests[0]
	if request.WorkflowKey != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) ||
		request.Expected != expected || request.TargetTransactionID != "txn-17" {
		t.Fatalf("authoritative request = %+v", request)
	}
	if request.Actor != edit.ActorKind("user") || request.ActorID != "actor-a" || request.CanvasSessionID != "canvas-a" || request.AgentSessionID != "session-a" {
		t.Fatalf("trusted identity = %+v", request)
	}
	if request.CandidateID == "" {
		t.Fatal("server-derived candidate identity is empty")
	}
	var result service.WorkflowEditPreviewResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || result.Kind != "preview" || result.Candidate.ID != request.CandidateID {
		t.Fatalf("preview = %+v, error = %v", result, err)
	}
}

func TestWorkflowHistoryCommandDispatchesRedo(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{}
	server := newWorkflowHistoryTestServer(application, workflowCommandTestScopeResolver())
	response := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("redo", "history-idem-redo", workflowHistoryTestRevision()))
	if response.Code != http.StatusOK || len(application.redoRequests) != 1 || len(application.undoRequests) != 0 {
		t.Fatalf("status=%d body=%s undo=%d redo=%d", response.Code, response.Body.String(), len(application.undoRequests), len(application.redoRequests))
	}
}

func TestWorkflowHistoryCommandCandidateIdentityIsCompleteAndIdempotent(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{}
	server := newWorkflowHistoryTestServer(application, workflowCommandTestScopeResolver())
	expected := workflowHistoryTestRevision()
	payload := workflowHistoryTestPayload("undo", "history-idem-stable", expected)

	first := postWorkflowHistoryCommand(t, server, payload)
	second := postWorkflowHistoryCommand(t, server, payload)
	changed := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-idem-other", expected))
	if first.Code != http.StatusOK || second.Code != http.StatusOK || changed.Code != http.StatusOK || len(application.undoRequests) != 3 {
		t.Fatalf("statuses=%d/%d/%d calls=%d", first.Code, second.Code, changed.Code, len(application.undoRequests))
	}
	if application.undoRequests[0].CandidateID != application.undoRequests[1].CandidateID {
		t.Fatal("same trusted request and idempotency key produced different candidate identities")
	}
	if application.undoRequests[0].CandidateID == application.undoRequests[2].CandidateID {
		t.Fatal("different idempotency keys produced the same candidate identity")
	}

	for name, invalid := range map[string]edit.Revision{
		"resource version": {SemanticHash: expected.SemanticHash, LayoutHash: expected.LayoutHash},
		"semantic hash":    {ResourceVersion: expected.ResourceVersion, LayoutHash: expected.LayoutHash},
		"layout hash":      {ResourceVersion: expected.ResourceVersion, SemanticHash: expected.SemanticHash},
		"trimmed values":   {ResourceVersion: expected.ResourceVersion + " ", SemanticHash: expected.SemanticHash, LayoutHash: expected.LayoutHash},
	} {
		t.Run(name, func(t *testing.T) {
			before := len(application.undoRequests)
			response := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-invalid", invalid))
			if response.Code != http.StatusBadRequest || len(application.undoRequests) != before {
				t.Fatalf("status=%d calls=%d, want strict rejection", response.Code, len(application.undoRequests))
			}
		})
	}
}

func TestWorkflowHistoryCommandRejectsBrowserIdentityAndUnknownFields(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{}
	server := newWorkflowHistoryTestServer(application, workflowCommandTestScopeResolver())
	for name, field := range map[string]any{
		"tenant":        "tenant-forged",
		"actor":         "actor-forged",
		"canvas":        "canvas-forged",
		"session":       "session-forged",
		"candidate":     "candidate-forged",
		"agent session": "agent-forged",
	} {
		t.Run(name, func(t *testing.T) {
			payload := workflowHistoryTestPayload("undo", "history-forged", workflowHistoryTestRevision())
			fieldName := map[string]string{
				"tenant": "tenantId", "actor": "actorId", "canvas": "canvasSessionId",
				"session": "sessionId", "candidate": "candidateId", "agent session": "agentSessionId",
			}[name]
			payload[fieldName] = field
			response := postWorkflowHistoryCommand(t, server, payload)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if len(application.undoRequests) != 0 || len(application.redoRequests) != 0 {
		t.Fatalf("forged request reached application: undo=%d redo=%d", len(application.undoRequests), len(application.redoRequests))
	}
}

func TestWorkflowHistoryCommandRejectsUnauthorizedOrIncompleteScope(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{}
	unauthorized := WorkflowCommandScopeResolverFunc(func(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error) {
		return WorkflowCommandTrustedScope{}, errors.New("denied")
	})
	server := newWorkflowHistoryTestServer(application, unauthorized)
	response := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-denied", workflowHistoryTestRevision()))
	if response.Code != http.StatusForbidden {
		t.Fatalf("unauthorized status=%d body=%s", response.Code, response.Body.String())
	}

	incomplete := WorkflowCommandScopeResolverFunc(func(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error) {
		return WorkflowCommandTrustedScope{TenantID: "tenant-a", ActorID: "actor-a", CanvasSessionID: "canvas-a"}, nil
	})
	server = newWorkflowHistoryTestServer(application, incomplete)
	response = postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-incomplete", workflowHistoryTestRevision()))
	if response.Code != http.StatusForbidden || len(application.undoRequests) != 0 {
		t.Fatalf("incomplete status=%d calls=%d", response.Code, len(application.undoRequests))
	}
}

func TestWorkflowHistoryCommandReturnsDomainConflictWithoutApplying(t *testing.T) {
	application := &recordingWorkflowHistoryApplication{err: &edit.DomainError{
		Kind: "error", Code: edit.ErrorStaleRevision, Message: "workflow changed",
		CurrentRevision: "r18", BaseRevision: "r17",
	}}
	server := newWorkflowHistoryTestServer(application, workflowCommandTestScopeResolver())
	response := postWorkflowHistoryCommand(t, server, workflowHistoryTestPayload("undo", "history-conflict", workflowHistoryTestRevision()))
	if response.Code != http.StatusConflict || len(application.undoRequests) != 1 {
		t.Fatalf("status=%d body=%s calls=%d", response.Code, response.Body.String(), len(application.undoRequests))
	}
	var domain edit.DomainError
	if err := json.NewDecoder(response.Body).Decode(&domain); err != nil || domain.Code != edit.ErrorStaleRevision {
		t.Fatalf("domain=%+v error=%v", domain, err)
	}
}

type recordingWorkflowHistoryApplication struct {
	undoRequests []service.WorkflowEditHistoryActionRequest
	redoRequests []service.WorkflowEditHistoryActionRequest
	err          error
}

func (application *recordingWorkflowHistoryApplication) ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	return service.WorkflowManualCommandResult{}, nil
}

func (application *recordingWorkflowHistoryApplication) Undo(_ context.Context, request service.WorkflowEditHistoryActionRequest) (service.WorkflowEditPreviewResult, error) {
	application.undoRequests = append(application.undoRequests, request)
	return workflowHistoryTestPreview(request), application.err
}

func (application *recordingWorkflowHistoryApplication) Redo(_ context.Context, request service.WorkflowEditHistoryActionRequest) (service.WorkflowEditPreviewResult, error) {
	application.redoRequests = append(application.redoRequests, request)
	return workflowHistoryTestPreview(request), application.err
}

func workflowHistoryTestPreview(request service.WorkflowEditHistoryActionRequest) service.WorkflowEditPreviewResult {
	return service.WorkflowEditPreviewResult{
		Kind: "preview", SchemaVersion: edit.SchemaVersion,
		Candidate: edit.WorkflowCandidate{ID: request.CandidateID, Key: request.WorkflowKey, CurrentRevision: 1, State: "ready"},
	}
}

func newWorkflowHistoryTestServer(application *recordingWorkflowHistoryApplication, resolver WorkflowCommandScopeResolver) *HTTPServer {
	return NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, resolver))
}

func workflowHistoryTestRevision() edit.Revision {
	return edit.Revision{ResourceVersion: "r17", SemanticHash: "semantic-17", LayoutHash: "layout-17"}
}

func workflowHistoryTestPayload(action, idempotencyKey string, expected edit.Revision) map[string]any {
	return map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expected": expected,
		"targetTransactionId": "txn-17", "idempotencyKey": idempotencyKey, "action": action,
	}
}

func postWorkflowHistoryCommand(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-history-actions", bytes.NewReader(raw))
	server.handleWorkflowHistoryCommand(response, request)
	return response
}
