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

func TestWorkflowNoMemoryFallback(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	if server.workflowEditor != nil {
		t.Fatal("HTTP server constructed an implicit workflow editor store")
	}
}

func TestWorkflowCommandUsesSingleInjectedApplicationAndTrustedScope(t *testing.T) {
	application := &recordingWorkflowCommandApplication{result: workflowCommandTestResult()}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))

	response := postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": "r17",
		"idempotencyKey": "manual-add-1", "reason": "add health check",
		"command": map[string]any{"kind": "add_node", "node": map[string]any{
			"id": "check", "type": "action", "position": map[string]any{"x": 20, "y": 40},
		}},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", response.Code, response.Body.String())
	}
	if len(application.requests) != 1 {
		t.Fatalf("application calls = %d, want one authoritative command", len(application.requests))
	}
	request := application.requests[0]
	if request.WorkflowKey != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) || request.ActorID != "actor-a" || request.CanvasSessionID != "canvas-a" || request.SessionID != "session-a" {
		t.Fatalf("trusted request = %+v", request)
	}
	if request.ExpectedRevision != "r17" || request.IdempotencyKey != "manual-add-1" || request.Command.Kind != service.WorkflowManualAddNode {
		t.Fatalf("command request = %+v", request)
	}
	if len(request.Commands) != 0 {
		t.Fatalf("legacy command unexpectedly forwarded as batch: %+v", request.Commands)
	}
	var result service.WorkflowManualCommandResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || result.Receipt.TransactionID != "txn-manual-1" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestWorkflowCommandBatchUsesOneAuthoritativeApplicationCallInOrder(t *testing.T) {
	application := &recordingWorkflowCommandApplication{result: workflowCommandTestResult()}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))

	response := postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": "r17",
		"idempotencyKey": "manual-batch-1", "reason": "replace one canvas node atomically", "confirmed": true,
		"commands": []map[string]any{
			{"kind": "delete_node", "nodeId": "obsolete", "cascadeEdges": true},
			{"kind": "add_node", "node": map[string]any{
				"id": "replacement", "type": "action", "position": map[string]any{"x": 20, "y": 40},
			}},
		},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", response.Code, response.Body.String())
	}
	if len(application.requests) != 1 {
		t.Fatalf("application calls = %d, want one authoritative batch", len(application.requests))
	}
	request := application.requests[0]
	if request.WorkflowKey != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) || request.ActorID != "actor-a" || request.CanvasSessionID != "canvas-a" || request.SessionID != "session-a" {
		t.Fatalf("trusted request = %+v", request)
	}
	if request.Command.Kind != "" {
		t.Fatalf("batch unexpectedly forwarded as legacy command: %+v", request.Command)
	}
	if len(request.Commands) != 2 || request.Commands[0].Kind != service.WorkflowManualDeleteNode || request.Commands[0].NodeID != "obsolete" || request.Commands[1].Kind != service.WorkflowManualAddNode || request.Commands[1].Node == nil || request.Commands[1].Node.ID != "replacement" {
		t.Fatalf("ordered commands = %+v", request.Commands)
	}
}

func TestWorkflowCommandRejectsInvalidManualCommandEnvelopeBeforeApplication(t *testing.T) {
	validCommand := map[string]any{"kind": "disconnect_edge", "edgeId": "edge-1"}
	base := func() map[string]any {
		return map[string]any{
			"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": "r17",
			"idempotencyKey": "manual-invalid-1", "reason": "strict envelope validation",
		}
	}
	tooMany := make([]map[string]any, maxWorkflowManualCommandBatchSize+1)
	for index := range tooMany {
		tooMany[index] = validCommand
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing command and commands", mutate: func(map[string]any) {}},
		{name: "command and commands are mutually exclusive", mutate: func(payload map[string]any) {
			payload["command"] = validCommand
			payload["commands"] = []map[string]any{validCommand}
		}},
		{name: "empty commands", mutate: func(payload map[string]any) { payload["commands"] = []map[string]any{} }},
		{name: "null commands", mutate: func(payload map[string]any) { payload["commands"] = nil }},
		{name: "batch exceeds safe bound", mutate: func(payload map[string]any) { payload["commands"] = tooMany }},
		{name: "unsupported legacy command kind", mutate: func(payload map[string]any) {
			payload["command"] = map[string]any{"kind": "replace_graph"}
		}},
		{name: "unsupported batch command kind", mutate: func(payload map[string]any) {
			payload["commands"] = []map[string]any{{"kind": "disconnect_edge", "edgeId": "edge-1"}, {"kind": "replace_graph"}}
		}},
		{name: "unknown nested command field", mutate: func(payload map[string]any) {
			payload["commands"] = []map[string]any{{"kind": "disconnect_edge", "edgeId": "edge-1", "tenantId": "forged"}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			application := &recordingWorkflowCommandApplication{result: workflowCommandTestResult()}
			server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
			payload := base()
			test.mutate(payload)
			response := postWorkflowCommand(t, server, payload)
			if response.Code != http.StatusBadRequest || len(application.requests) != 0 {
				t.Fatalf("status=%d body=%s calls=%d, want strict 400 before application", response.Code, response.Body.String(), len(application.requests))
			}
		})
	}
}

func TestWorkflowCommandMoveIsOneLayoutOnlyDragStop(t *testing.T) {
	application := &recordingWorkflowCommandApplication{result: workflowCommandTestResult()}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	response := postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": "r17",
		"idempotencyKey": "drag-stop-1", "reason": "move selected nodes",
		"command": map[string]any{"kind": "move", "layout": map[string]any{"nodes": []map[string]any{
			{"nodeId": "n1", "position": map[string]any{"x": 10, "y": 20}},
			{"nodeId": "n2", "position": map[string]any{"x": 30, "y": 40}},
		}}},
	})
	if response.Code != http.StatusOK || len(application.requests) != 1 {
		t.Fatalf("status = %d calls = %d, want one drag-stop transaction", response.Code, len(application.requests))
	}
	command := application.requests[0].Command
	if command.Kind != service.WorkflowManualMove || command.Layout == nil || len(command.Layout.Nodes) != 2 {
		t.Fatalf("command = %+v, want one typed layout-only command", command)
	}
}

func TestWorkflowDryRunUsesTrustedScopeAndServerResolvedCurrentTransaction(t *testing.T) {
	basis := edit.ValidationBasis{
		ResourceVersion: "r17", SemanticHash: "semantic", CatalogVersion: "catalog",
		ActionSchemaDigest: "actions", ReferenceSnapshotHash: "references", EnvironmentBindingVersion: "environment",
	}
	application := &recordingWorkflowCommandApplication{dryRunResult: service.WorkflowEditDryRunResult{
		Kind: "dry_run", SchemaVersion: edit.SchemaVersion, Mode: service.WorkflowDryRunModeDraftNonProduction,
		Basis: basis, Status: service.WorkflowDryRunStatusPassed, Valid: true,
	}}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	response := postWorkflowDryRun(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedBasis": basis, "environment": "staging",
	})
	if response.Code != http.StatusOK || len(application.dryRunRequests) != 1 {
		t.Fatalf("status = %d body=%s calls=%d", response.Code, response.Body.String(), len(application.dryRunRequests))
	}
	request := application.dryRunRequests[0]
	if request.key != (edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}) || request.basis != basis || request.environment != "staging" {
		t.Fatalf("trusted dry-run = %#v", request)
	}

	response = postWorkflowDryRun(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedBasis": basis,
		"environment": "staging", "transactionId": "forged-transaction",
	})
	if response.Code != http.StatusBadRequest || len(application.dryRunRequests) != 1 {
		t.Fatalf("forged transaction status=%d calls=%d", response.Code, len(application.dryRunRequests))
	}
}

func TestWorkflowCommandReturnsTypedConflictWithoutLocalSuccess(t *testing.T) {
	application := &recordingWorkflowCommandApplication{err: &edit.DomainError{Kind: "error", Code: edit.ErrorStaleRevision, Message: "workflow changed", CurrentRevision: "r18", BaseRevision: "r17"}}
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	response := postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "expectedRevision": "r17",
		"idempotencyKey": "manual-delete-1", "reason": "remove obsolete edge", "confirmed": true,
		"command": map[string]any{"kind": "disconnect_edge", "edgeId": "edge-1"},
	})
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.Code)
	}
	var domain edit.DomainError
	if err := json.NewDecoder(response.Body).Decode(&domain); err != nil || domain.Code != edit.ErrorStaleRevision {
		t.Fatalf("domain = %+v, error = %v", domain, err)
	}
}

func TestWorkflowCommandRejectsMissingApplicationAndUntrustedIdentityFields(t *testing.T) {
	server := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	response := postWorkflowCommand(t, server, map[string]any{})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured status = %d, want 503", response.Code)
	}

	application := &recordingWorkflowCommandApplication{}
	server = NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil), withWorkflowCommandApplication(application, workflowCommandTestScopeResolver()))
	response = postWorkflowCommand(t, server, map[string]any{
		"schemaVersion": edit.SchemaVersion, "workflowId": "workflow-a", "tenantId": "tenant-b", "actorId": "forged",
		"expectedRevision": "r17", "idempotencyKey": "manual-1", "reason": "forged",
		"command": map[string]any{"kind": "disconnect_edge", "edgeId": "edge-1"},
	})
	if response.Code != http.StatusBadRequest || len(application.requests) != 0 {
		t.Fatalf("forged status = %d calls = %d, want strict rejection", response.Code, len(application.requests))
	}
}

type recordingWorkflowCommandApplication struct {
	requests       []service.WorkflowManualCommandRequest
	result         service.WorkflowManualCommandResult
	err            error
	dryRunRequests []struct {
		key         edit.WorkflowKey
		basis       edit.ValidationBasis
		environment string
	}
	dryRunResult service.WorkflowEditDryRunResult
	dryRunErr    error
}

func (application *recordingWorkflowCommandApplication) ExecuteManualCommand(_ context.Context, request service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	application.requests = append(application.requests, request)
	return application.result, application.err
}

func (application *recordingWorkflowCommandApplication) DryRunCurrent(_ context.Context, key edit.WorkflowKey, basis edit.ValidationBasis, environment string) (service.WorkflowEditDryRunResult, error) {
	application.dryRunRequests = append(application.dryRunRequests, struct {
		key         edit.WorkflowKey
		basis       edit.ValidationBasis
		environment string
	}{key: key, basis: basis, environment: environment})
	return application.dryRunResult, application.dryRunErr
}

func workflowCommandTestScopeResolver() WorkflowCommandScopeResolver {
	return WorkflowCommandScopeResolverFunc(func(_ context.Context, _ *http.Request, workflowID string) (WorkflowCommandTrustedScope, error) {
		if workflowID != "workflow-a" {
			return WorkflowCommandTrustedScope{}, errors.New("not authorized")
		}
		return WorkflowCommandTrustedScope{TenantID: "tenant-a", ActorID: "actor-a", CanvasSessionID: "canvas-a", SessionID: "session-a"}, nil
	})
}

func workflowCommandTestResult() service.WorkflowManualCommandResult {
	revision := edit.Revision{ResourceVersion: "r18", SemanticHash: "semantic", LayoutHash: "layout"}
	return service.WorkflowManualCommandResult{Receipt: edit.WorkflowMutationReceipt{
		SchemaVersion: edit.SchemaVersion, Kind: "receipt", TransactionID: "txn-manual-1",
		Key: edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}, Status: "verifying",
		RevisionBefore: edit.Revision{ResourceVersion: "r17"}, RevisionAfter: &revision,
	}}
}

func postWorkflowCommand(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-commands", bytes.NewReader(raw))
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func postWorkflowDryRun(t *testing.T, server *HTTPServer, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runner-studio/workflow-dry-run", bytes.NewReader(raw))
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}
