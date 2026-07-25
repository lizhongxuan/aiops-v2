package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"runner/server/service"
	edit "runner/workflow/edit"
)

const (
	maxWorkflowCommandBodyBytes       = 1 << 20
	maxWorkflowManualCommandBatchSize = 64
)

type workflowManualCommandApplication interface {
	ExecuteManualCommand(context.Context, service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error)
}

type workflowCurrentDryRunApplication interface {
	DryRunCurrent(context.Context, edit.WorkflowKey, edit.ValidationBasis, string) (service.WorkflowEditDryRunResult, error)
}

type WorkflowCommandTrustedScope struct {
	TenantID        string
	ActorID         string
	CanvasSessionID string
	SessionID       string
}

type WorkflowCommandScopeResolver interface {
	ResolveWorkflowCommandScope(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error)
}

type WorkflowCommandScopeResolverFunc func(context.Context, *http.Request, string) (WorkflowCommandTrustedScope, error)

func (resolve WorkflowCommandScopeResolverFunc) ResolveWorkflowCommandScope(ctx context.Context, request *http.Request, workflowID string) (WorkflowCommandTrustedScope, error) {
	return resolve(ctx, request, workflowID)
}

type workflowEditCommandRequest struct {
	SchemaVersion    string          `json:"schemaVersion"`
	WorkflowID       string          `json:"workflowId"`
	ExpectedRevision string          `json:"expectedRevision"`
	IdempotencyKey   string          `json:"idempotencyKey"`
	Reason           string          `json:"reason"`
	Confirmed        bool            `json:"confirmed,omitempty"`
	Command          json.RawMessage `json:"command,omitempty"`
	Commands         json.RawMessage `json:"commands,omitempty"`
}

type workflowDryRunRequest struct {
	SchemaVersion string               `json:"schemaVersion"`
	WorkflowID    string               `json:"workflowId"`
	ExpectedBasis edit.ValidationBasis `json:"expectedBasis"`
	Environment   string               `json:"environment"`
}

func (s *HTTPServer) handleWorkflowEditCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	if s.workflowCommandApplication == nil || s.workflowCommandScopeResolver == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow command application is not configured", "retry after the Runner application is available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowEditCommandRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow command body", "send one strict workflow command envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow command body must contain exactly one JSON object", "send one strict workflow command envelope")
		return
	}
	if request.SchemaVersion != edit.SchemaVersion || strings.TrimSpace(request.WorkflowID) == "" || strings.TrimSpace(request.ExpectedRevision) == "" || strings.TrimSpace(request.IdempotencyKey) == "" || strings.TrimSpace(request.Reason) == "" {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow command identity, revision, idempotency, and reason are required", "refresh the workflow and retry the command")
		return
	}
	command, commands, err := decodeWorkflowManualCommandEnvelope(request)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow command must contain exactly one valid command or bounded commands batch", "send command or 1 to 64 ordered commands, but not both")
		return
	}
	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, strings.TrimSpace(request.WorkflowID))
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow command scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: strings.TrimSpace(request.WorkflowID)}
	if key.Validate() != nil || strings.TrimSpace(scope.ActorID) == "" || strings.TrimSpace(scope.CanvasSessionID) == "" || strings.TrimSpace(scope.SessionID) == "" {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow command scope is incomplete", "reopen the workflow editor session")
		return
	}
	applicationRequest := service.WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: key, ExpectedRevision: strings.TrimSpace(request.ExpectedRevision),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), ActorID: strings.TrimSpace(scope.ActorID),
		CanvasSessionID: strings.TrimSpace(scope.CanvasSessionID), SessionID: strings.TrimSpace(scope.SessionID),
		Reason: strings.TrimSpace(request.Reason), Confirmed: request.Confirmed, Command: command, Commands: commands,
	}
	result, err := s.workflowCommandApplication.ExecuteManualCommand(r.Context(), applicationRequest)
	if err != nil {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	s.ensureWorkflowEventBinding(key)
	writeJSON(w, http.StatusOK, result)
}

func decodeWorkflowManualCommandEnvelope(request workflowEditCommandRequest) (service.WorkflowManualCommand, []service.WorkflowManualCommand, error) {
	hasCommand := len(request.Command) != 0
	hasCommands := len(request.Commands) != 0
	if hasCommand == hasCommands {
		return service.WorkflowManualCommand{}, nil, errors.New("exactly one of command or commands is required")
	}
	if hasCommand {
		var command service.WorkflowManualCommand
		if err := decodeStrictWorkflowCommandJSON(request.Command, &command); err != nil || !isSupportedWorkflowManualCommandKind(command.Kind) {
			return service.WorkflowManualCommand{}, nil, errors.New("command is invalid")
		}
		return command, nil, nil
	}

	var commands []service.WorkflowManualCommand
	if err := decodeStrictWorkflowCommandJSON(request.Commands, &commands); err != nil {
		return service.WorkflowManualCommand{}, nil, errors.New("commands is invalid")
	}
	if len(commands) == 0 || len(commands) > maxWorkflowManualCommandBatchSize {
		return service.WorkflowManualCommand{}, nil, errors.New("commands batch is outside the safe bound")
	}
	for _, command := range commands {
		if !isSupportedWorkflowManualCommandKind(command.Kind) {
			return service.WorkflowManualCommand{}, nil, errors.New("commands contains an unsupported command kind")
		}
	}
	return service.WorkflowManualCommand{}, commands, nil
}

func decodeStrictWorkflowCommandJSON(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("workflow command value must contain exactly one JSON value")
	}
	return nil
}

func isSupportedWorkflowManualCommandKind(kind service.WorkflowManualCommandKind) bool {
	switch kind {
	case service.WorkflowManualAddNode,
		service.WorkflowManualUpdateNode,
		service.WorkflowManualDeleteNode,
		service.WorkflowManualConnectEdge,
		service.WorkflowManualUpdateEdge,
		service.WorkflowManualDisconnectEdge,
		service.WorkflowManualMove:
		return true
	default:
		return false
	}
}

func (s *HTTPServer) handleWorkflowDryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	application, ok := s.workflowCommandApplication.(workflowCurrentDryRunApplication)
	if !ok || s.workflowCommandScopeResolver == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow dry-run is not configured", "retry after the Runner application is available")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowDryRunRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow dry-run body", "send one strict workflow dry-run envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow dry-run body must contain exactly one JSON object", "send one strict workflow dry-run envelope")
		return
	}
	workflowID := strings.TrimSpace(request.WorkflowID)
	if request.SchemaVersion != edit.SchemaVersion || workflowID == "" || request.ExpectedBasis.Validate() != nil || request.Environment != strings.TrimSpace(request.Environment) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow dry-run identity and full validation basis are required", "refresh the current workflow gate facts")
		return
	}
	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, workflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow dry-run scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: workflowID}
	if key.Validate() != nil || strings.TrimSpace(scope.ActorID) == "" || strings.TrimSpace(scope.CanvasSessionID) == "" || strings.TrimSpace(scope.SessionID) == "" {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow dry-run scope is incomplete", "reopen the workflow editor session")
		return
	}
	result, err := application.DryRunCurrent(r.Context(), key, request.ExpectedBasis, request.Environment)
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

func writeWorkflowCommandApplicationError(w http.ResponseWriter, err error) {
	var domain *edit.DomainError
	if errors.As(err, &domain) {
		status := http.StatusUnprocessableEntity
		switch domain.Code {
		case edit.ErrorStaleRevision, edit.ErrorStaleChangeSet, edit.ErrorPreviewMismatch, edit.ErrorIdempotencyKeyReused, edit.ErrorReferenceLocked, edit.ErrorValidationBasisStale:
			status = http.StatusConflict
		case edit.ErrorApprovalRequired, edit.ErrorApprovalScopeExceeded:
			status = http.StatusForbidden
		case edit.ErrorInvalidWorkflowKey, edit.ErrorInvalidPatch, edit.ErrorSchemaViolation, edit.ErrorInvalidEdge, edit.ErrorWouldCreateCycle:
			status = http.StatusBadRequest
		}
		writeJSON(w, status, domain)
		return
	}
	if errors.Is(err, service.ErrWorkflowEditConfiguration) {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow command application is not configured", "retry after the Runner application is available")
		return
	}
	if errors.Is(err, service.ErrInvalid) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow command is invalid", "repair the typed command and retry")
		return
	}
	writeWorkflowCommandError(w, http.StatusInternalServerError, edit.ErrorMissingConfiguration, "workflow command could not be completed", "reload the committed workflow before retrying")
}

func writeWorkflowCommandError(w http.ResponseWriter, status int, code edit.ErrorCode, message, retry string) {
	writeJSON(w, status, edit.DomainError{Kind: "error", Code: code, Message: message, RetryHint: retry})
}
