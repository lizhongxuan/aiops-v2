package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"runner/server/service"
	edit "runner/workflow/edit"
)

type workflowHistoryCommandApplication interface {
	Undo(context.Context, service.WorkflowEditHistoryActionRequest) (service.WorkflowEditPreviewResult, error)
	Redo(context.Context, service.WorkflowEditHistoryActionRequest) (service.WorkflowEditPreviewResult, error)
}

type workflowHistoryCommandRequest struct {
	SchemaVersion       string        `json:"schemaVersion"`
	WorkflowID          string        `json:"workflowId"`
	Expected            edit.Revision `json:"expected"`
	TargetTransactionID string        `json:"targetTransactionId"`
	IdempotencyKey      string        `json:"idempotencyKey"`
	Action              string        `json:"action"`
}

// handleWorkflowHistoryCommand creates an authoritative Undo/Redo candidate.
// It deliberately does not apply the preview: the normal review, approval and
// apply path remains the sole mutation authority.
func (s *HTTPServer) handleWorkflowHistoryCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	application, ok := s.workflowCommandApplication.(workflowHistoryCommandApplication)
	if !ok || s.workflowCommandScopeResolver == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow history command is not configured", "retry after the Runner application is available")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowHistoryCommandRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow history command body", "send one strict workflow history command envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow history command body must contain exactly one JSON object", "send one strict workflow history command envelope")
		return
	}

	workflowID := strings.TrimSpace(request.WorkflowID)
	targetTransactionID := strings.TrimSpace(request.TargetTransactionID)
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	action := strings.TrimSpace(request.Action)
	if request.SchemaVersion != edit.SchemaVersion || workflowID == "" || request.WorkflowID != workflowID ||
		targetTransactionID == "" || request.TargetTransactionID != targetTransactionID ||
		idempotencyKey == "" || request.IdempotencyKey != idempotencyKey ||
		(action != "undo" && action != "redo") || request.Action != action || !workflowHistoryRevisionComplete(request.Expected) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow history identity, full revision, target transaction, idempotency, and action are required", "refresh the workflow history and retry")
		return
	}

	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, workflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow history command scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: workflowID}
	actorID := strings.TrimSpace(scope.ActorID)
	canvasSessionID := strings.TrimSpace(scope.CanvasSessionID)
	sessionID := strings.TrimSpace(scope.SessionID)
	if key.Validate() != nil || actorID == "" || canvasSessionID == "" || sessionID == "" ||
		scope.ActorID != actorID || scope.CanvasSessionID != canvasSessionID || scope.SessionID != sessionID {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow history command scope is incomplete", "reopen the workflow editor session")
		return
	}

	candidateID, err := workflowHistoryCandidateID(key, request.Expected, targetTransactionID, idempotencyKey, action, actorID, canvasSessionID, sessionID)
	if err != nil {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	command := service.WorkflowEditHistoryActionRequest{
		SchemaVersion:       edit.SchemaVersion,
		WorkflowKey:         key,
		TargetTransactionID: targetTransactionID,
		CandidateID:         candidateID,
		Expected:            request.Expected,
		Actor:               edit.ActorKind("user"),
		ActorID:             actorID,
		CanvasSessionID:     canvasSessionID,
		AgentSessionID:      sessionID,
	}

	var result service.WorkflowEditPreviewResult
	if action == "undo" {
		result, err = application.Undo(r.Context(), command)
	} else {
		result, err = application.Redo(r.Context(), command)
	}
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

func workflowHistoryRevisionComplete(revision edit.Revision) bool {
	for _, value := range []string{revision.ResourceVersion, revision.SemanticHash, revision.LayoutHash} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return false
		}
	}
	return true
}

func workflowHistoryCandidateID(key edit.WorkflowKey, expected edit.Revision, targetTransactionID, idempotencyKey, action, actorID, canvasSessionID, sessionID string) (string, error) {
	payload, err := json.Marshal([]any{
		edit.SchemaVersion, "workflow-history-command", key, expected, targetTransactionID,
		idempotencyKey, action, actorID, canvasSessionID, sessionID,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "history-" + hex.EncodeToString(digest[:]), nil
}
