package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"runner/server/service"
	edit "runner/workflow/edit"
)

type workflowCandidateReviseApplication interface {
	ReviseCandidate(context.Context, service.WorkflowCandidateReviseRequest) (service.WorkflowEditPreviewResult, error)
}

type workflowCandidateReviseRequest struct {
	SchemaVersion       string                 `json:"schemaVersion"`
	WorkflowID          string                 `json:"workflowId"`
	CandidateID         string                 `json:"candidateId"`
	BaseChangeSetID     string                 `json:"baseChangeSetId"`
	Expected            edit.Revision          `json:"expected"`
	IdempotencyKey      string                 `json:"idempotencyKey"`
	Intent              string                 `json:"intent"`
	AdditionalOperation edit.WorkflowOperation `json:"additionalOperation"`
}

// handleWorkflowCandidateRevise is intentionally registered by the root HTTP
// wiring. It accepts no identity fields: tenant, actor and sessions are
// projected exclusively from WorkflowCommandScopeResolver.
func (s *HTTPServer) handleWorkflowCandidateRevise(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	application, ok := s.workflowCommandApplication.(workflowCandidateReviseApplication)
	if !ok || s.workflowCommandScopeResolver == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow candidate revision is not configured", "retry after the Runner application is available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowCandidateReviseRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow candidate revision body", "send one strict candidate revision envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow candidate revision body must contain exactly one JSON object", "send one strict candidate revision envelope")
		return
	}
	workflowID := strings.TrimSpace(request.WorkflowID)
	if request.SchemaVersion != edit.SchemaVersion || workflowID == "" || request.WorkflowID != workflowID ||
		strings.TrimSpace(request.CandidateID) == "" || request.CandidateID != strings.TrimSpace(request.CandidateID) ||
		strings.TrimSpace(request.BaseChangeSetID) == "" || request.BaseChangeSetID != strings.TrimSpace(request.BaseChangeSetID) ||
		strings.TrimSpace(request.IdempotencyKey) == "" || request.IdempotencyKey != strings.TrimSpace(request.IdempotencyKey) ||
		strings.TrimSpace(request.Intent) == "" || request.Intent != strings.TrimSpace(request.Intent) ||
		!workflowCandidateRevisionValue(request.Expected.ResourceVersion) || !workflowCandidateRevisionValue(request.Expected.SemanticHash) ||
		!workflowCandidateRevisionValue(request.Expected.LayoutHash) || request.AdditionalOperation.Value == nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow candidate identity, full revision, idempotency and operation are required", "refresh the candidate and retry")
		return
	}
	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, workflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow candidate scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: workflowID}
	if key.Validate() != nil || strings.TrimSpace(scope.ActorID) == "" || strings.TrimSpace(scope.CanvasSessionID) == "" || strings.TrimSpace(scope.SessionID) == "" {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow candidate scope is incomplete", "reopen the workflow editor session")
		return
	}
	result, err := application.ReviseCandidate(r.Context(), service.WorkflowCandidateReviseRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: key, CandidateID: request.CandidateID,
		BaseChangeSetID: request.BaseChangeSetID, Expected: request.Expected, IdempotencyKey: request.IdempotencyKey,
		Intent: request.Intent, AdditionalOperation: request.AdditionalOperation,
		ActorID: strings.TrimSpace(scope.ActorID), CanvasSessionID: strings.TrimSpace(scope.CanvasSessionID), SessionID: strings.TrimSpace(scope.SessionID),
	})
	if err != nil {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	s.ensureWorkflowEventBinding(key)
	if s.workflowEventBindings != nil {
		_ = s.workflowEventBindings.Refresh(key)
	}
	writeJSON(w, http.StatusOK, result)
}

func workflowCandidateRevisionValue(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsRune(value, '\x00')
}
