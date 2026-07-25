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

type workflowCandidateAbandonApplication interface {
	AbandonCandidate(context.Context, service.WorkflowCandidateAbandonRequest) (service.WorkflowCandidateAbandonResult, error)
}

type workflowCandidateAbandonRequest struct {
	SchemaVersion     string        `json:"schemaVersion"`
	WorkflowID        string        `json:"workflowId"`
	CandidateID       string        `json:"candidateId"`
	CandidateRevision uint64        `json:"candidateRevision"`
	ChangeSetID       string        `json:"changeSetId"`
	PreviewHash       string        `json:"previewHash"`
	Expected          edit.Revision `json:"expected"`
	IdempotencyKey    string        `json:"idempotencyKey"`
	Action            string        `json:"action"`
}

func (s *HTTPServer) handleWorkflowCandidateAbandon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	application, ok := s.workflowCommandApplication.(workflowCandidateAbandonApplication)
	if !ok || s.workflowCommandScopeResolver == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow candidate discard is not configured", "retry after the Runner application is available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowCandidateAbandonRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow candidate discard body", "send one strict candidate discard envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow candidate discard body must contain exactly one JSON object", "send one strict candidate discard envelope")
		return
	}
	workflowID := strings.TrimSpace(request.WorkflowID)
	if request.SchemaVersion != edit.SchemaVersion || request.Action != "discard" || workflowID == "" || request.WorkflowID != workflowID || request.CandidateRevision == 0 ||
		!workflowCandidateRevisionValue(request.CandidateID) || !workflowCandidateRevisionValue(request.ChangeSetID) || !workflowCandidateRevisionValue(request.PreviewHash) ||
		!workflowCandidateRevisionValue(request.IdempotencyKey) || !workflowHistoryRevisionComplete(request.Expected) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow candidate binding, full revision, idempotency, and discard action are required", "reload the current candidate and retry")
		return
	}
	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, workflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow candidate scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: workflowID}
	actorID, canvasSessionID, sessionID := strings.TrimSpace(scope.ActorID), strings.TrimSpace(scope.CanvasSessionID), strings.TrimSpace(scope.SessionID)
	if key.Validate() != nil || actorID == "" || canvasSessionID == "" || sessionID == "" || scope.ActorID != actorID || scope.CanvasSessionID != canvasSessionID || scope.SessionID != sessionID {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow candidate scope is incomplete", "reopen the workflow editor session")
		return
	}
	result, err := application.AbandonCandidate(r.Context(), service.WorkflowCandidateAbandonRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: key, CandidateID: request.CandidateID,
		CandidateRevision: request.CandidateRevision, ChangeSetID: request.ChangeSetID, PreviewHash: request.PreviewHash,
		Expected: request.Expected, IdempotencyKey: request.IdempotencyKey, ActorID: actorID,
		CanvasSessionID: canvasSessionID, SessionID: sessionID,
	})
	if err != nil {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
