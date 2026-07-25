package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	edit "runner/workflow/edit"
)

const (
	workflowCandidateApplyApprovalAction = "request_apply_approval"
	workflowCandidateApplyToolName       = "workflow.apply_patch"
	workflowCandidateApplySource         = "workflow_history_candidate_apply"
)

// WorkflowCandidateApplyApprovalRuntime is the narrow RuntimeKernel boundary
// used by the HTTP adapter. BeginControlledToolTurn owns the PendingApproval
// and ActionToken lifecycle; this adapter never executes workflow.apply_patch.
type WorkflowCandidateApplyApprovalRuntime interface {
	BeginControlledToolTurn(context.Context, runtimekernel.ControlledToolTurnRequest) (runtimekernel.ControlledToolTurnResult, error)
}

// WorkflowCandidateApplyApprovalStore is the immutable Candidate boundary used
// to reconstruct an apply request without accepting mutation data from the
// browser.
type WorkflowCandidateApplyApprovalStore interface {
	GetCurrent(context.Context, edit.WorkflowKey, string) (edit.WorkflowCandidate, error)
	GetRevision(context.Context, edit.WorkflowKey, string, uint64) (service.WorkflowCandidateRevision, error)
	CheckApplyEligibility(context.Context, service.WorkflowCandidateApplyBinding, time.Time) (service.WorkflowCandidateRevision, error)
}

type workflowCandidateApplyApprovalApplication interface {
	GetContext(context.Context, edit.WorkflowKey, service.WorkflowContextQuery) (service.WorkflowContextResult, error)
}

type workflowCandidateApplyApprovalRequest struct {
	SchemaVersion     string `json:"schemaVersion"`
	WorkflowID        string `json:"workflowId"`
	CandidateID       string `json:"candidateId"`
	CandidateRevision uint64 `json:"candidateRevision"`
	IdempotencyKey    string `json:"idempotencyKey"`
	Action            string `json:"action"`
}

type workflowCandidateApplyApprovalBinding struct {
	CandidateID       string `json:"candidateId"`
	CandidateRevision uint64 `json:"candidateRevision"`
	ChangeSetID       string `json:"changeSetId"`
	PreviewHash       string `json:"previewHash"`
	ExpectedRevision  string `json:"expectedRevision"`
}

type workflowCandidateApplyApprovalResponse struct {
	Kind          string                                `json:"kind"`
	SchemaVersion string                                `json:"schemaVersion"`
	Status        string                                `json:"status"`
	SessionID     string                                `json:"sessionId"`
	TurnID        string                                `json:"turnId"`
	Candidate     workflowCandidateApplyApprovalBinding `json:"candidate"`
	Approval      runtimekernel.ControlledToolApproval  `json:"approval"`
}

// handleWorkflowCandidateApplyApproval starts one exact, server-authored
// workflow.apply_patch turn for a durable reviewed Candidate. The
// browser supplies only an optimistic Candidate selector. Mutation bindings,
// business authorization identifiers, turn identity, and tool arguments are
// reconstructed from the trusted request scope and immutable Candidate store.
func (s *HTTPServer) handleWorkflowCandidateApplyApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeWorkflowCommandError(w, http.StatusMethodNotAllowed, edit.ErrorSchemaViolation, "method not allowed", "send a POST request")
		return
	}
	application, applicationOK := s.workflowCommandApplication.(workflowCandidateApplyApprovalApplication)
	if !applicationOK || s.workflowCommandScopeResolver == nil || s.workflowCandidateApplyApprovalStore == nil || s.workflowCandidateApplyApprovalRuntime == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow candidate Runtime approval is not configured", "retry after the Workflow Runtime is available")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWorkflowCommandBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request workflowCandidateApplyApprovalRequest
	if err := decoder.Decode(&request); err != nil {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "invalid workflow candidate approval body", "send one strict Candidate approval envelope")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow candidate approval body must contain exactly one JSON object", "send one strict Candidate approval envelope")
		return
	}
	if !validWorkflowCandidateApplyApprovalRequest(request) {
		writeWorkflowCommandError(w, http.StatusBadRequest, edit.ErrorSchemaViolation, "workflow Candidate identity, revision, idempotency, and approval action are required", "reload the current Candidate and retry")
		return
	}

	scope, err := s.workflowCommandScopeResolver.ResolveWorkflowCommandScope(r.Context(), r, request.WorkflowID)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "workflow Candidate approval scope is not authorized", "sign in again or request access to this workflow")
		return
	}
	key := edit.WorkflowKey{TenantID: strings.TrimSpace(scope.TenantID), WorkflowID: request.WorkflowID}
	actorID, canvasSessionID, sessionID := strings.TrimSpace(scope.ActorID), strings.TrimSpace(scope.CanvasSessionID), strings.TrimSpace(scope.SessionID)
	if key.Validate() != nil || !exactWorkflowCandidateApplyIdentifier(scope.TenantID, 256) || !exactWorkflowCandidateApplyIdentifier(scope.ActorID, 256) ||
		!exactWorkflowCandidateApplyIdentifier(scope.CanvasSessionID, 256) || !exactWorkflowCandidateApplyIdentifier(scope.SessionID, 256) {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "trusted workflow Candidate approval scope is incomplete", "reopen the workflow editor session")
		return
	}

	sessionSource := s.assistantTransportSessionSource()
	if sessionSource == nil {
		writeWorkflowCommandError(w, http.StatusServiceUnavailable, edit.ErrorMissingConfiguration, "workflow Runtime session source is not configured", "retry after the Workflow Runtime is available")
		return
	}
	runtimeSession := sessionSource.Get(sessionID)
	if !validWorkflowCandidateRuntimeSession(runtimeSession, sessionID) {
		writeWorkflowCommandError(w, http.StatusConflict, edit.ErrorApprovalRequired, "the trusted workflow Runtime session is unavailable", "reopen Workflow AI Chat and review the Candidate again")
		return
	}

	candidate, err := s.resolveWorkflowCandidateApplyApproval(r.Context(), key, request)
	if err != nil {
		writeWorkflowCandidateApplyApprovalResolutionError(w, err)
		return
	}
	if candidate.Actor != edit.ActorKind("user") || candidate.ActorID != actorID || candidate.CanvasSessionID != canvasSessionID || candidate.AgentSessionID != sessionID || candidate.TurnID != "" || candidate.ToolCallID != "" {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "the Candidate does not belong to the trusted workflow session", "reload the Candidate from the current authenticated session")
		return
	}

	currentContext, err := application.GetContext(r.Context(), key, service.WorkflowContextQuery{View: service.WorkflowContextSummaryView})
	if err != nil {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	if currentContext.Key != key || currentContext.Revision != candidate.Base {
		writeWorkflowCommandError(w, http.StatusConflict, edit.ErrorStaleRevision, "the Workflow changed after this Candidate was created", "reload the workflow and create a new Candidate")
		return
	}

	controlled, argumentsHash, err := buildWorkflowCandidateControlledApplyRequest(scope, candidate, request.IdempotencyKey)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusInternalServerError, edit.ErrorMissingConfiguration, "the exact workflow approval binding could not be constructed", "reload the Candidate before retrying")
		return
	}
	result, err := s.workflowCandidateApplyApprovalRuntime.BeginControlledToolTurn(r.Context(), controlled)
	if err != nil {
		writeWorkflowCommandError(w, http.StatusConflict, edit.ErrorApprovalRequired, "the exact workflow approval turn could not be started", "finish the active turn or retry this same request")
		return
	}
	if !validWorkflowCandidateControlledApproval(result, controlled, argumentsHash) {
		writeWorkflowCommandError(w, http.StatusInternalServerError, edit.ErrorMissingConfiguration, "Workflow Runtime did not return an exact pending approval", "reload the Candidate before retrying")
		return
	}
	writeJSON(w, http.StatusAccepted, workflowCandidateApplyApprovalResponse{
		Kind: "workflow_apply_approval", SchemaVersion: edit.SchemaVersion, Status: result.Status,
		SessionID: result.SessionID, TurnID: result.TurnID,
		Candidate: workflowCandidateApplyApprovalBinding{
			CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision,
			ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion,
		},
		Approval: *result.Approval,
	})
}

func validWorkflowCandidateApplyApprovalRequest(request workflowCandidateApplyApprovalRequest) bool {
	validSelector := (validWorkflowHistoryCandidateSelector(request.CandidateID) && request.CandidateRevision == 1) ||
		(validWorkflowAgentCandidateSelector(request.CandidateID) && request.CandidateRevision >= 2)
	return request.SchemaVersion == edit.SchemaVersion &&
		exactWorkflowCandidateApplyIdentifier(request.WorkflowID, 256) &&
		validSelector &&
		exactWorkflowCandidateApplyIdentifier(request.IdempotencyKey, 256) &&
		request.Action == workflowCandidateApplyApprovalAction
}

func exactWorkflowCandidateApplyIdentifier(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func validWorkflowHistoryCandidateSelector(value string) bool {
	if !strings.HasPrefix(value, "history-") || len(value) != len("history-")+sha256.Size*2 {
		return false
	}
	encoded := strings.TrimPrefix(value, "history-")
	if encoded != strings.ToLower(encoded) {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == sha256.Size
}

func validWorkflowAgentCandidateSelector(value string) bool {
	const prefix = "candidate-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	encoded := strings.TrimPrefix(value, prefix)
	if encoded != strings.ToLower(encoded) {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == 12
}

func validWorkflowCandidateRuntimeSession(session *runtimekernel.SessionState, expectedID string) bool {
	return session != nil && session.ID == expectedID && exactWorkflowCandidateApplyIdentifier(session.ID, 256) &&
		session.Type == runtimekernel.SessionTypeWorkspace && session.Mode == runtimekernel.ModeExecute
}

func (s *HTTPServer) resolveWorkflowCandidateApplyApproval(ctx context.Context, key edit.WorkflowKey, request workflowCandidateApplyApprovalRequest) (service.WorkflowCandidateRevision, error) {
	current, err := s.workflowCandidateApplyApprovalStore.GetCurrent(ctx, key, request.CandidateID)
	if err != nil {
		return service.WorkflowCandidateRevision{}, err
	}
	if current.Key != key || current.ID != request.CandidateID || current.CurrentRevision != request.CandidateRevision || current.State != service.WorkflowCandidateStateActive ||
		!exactWorkflowCandidateApplyIdentifier(current.CurrentChangeSetID, 512) || !canonicalWorkflowCandidateApplyHash(current.CurrentPreviewHash) {
		return service.WorkflowCandidateRevision{}, &edit.DomainError{Kind: "error", Code: edit.ErrorStaleChangeSet, Message: "the selected Candidate is not current", RetryHint: "reload the current Candidate"}
	}
	revision, err := s.workflowCandidateApplyApprovalStore.GetRevision(ctx, key, current.ID, current.CurrentRevision)
	if err != nil {
		return service.WorkflowCandidateRevision{}, err
	}
	binding := service.WorkflowCandidateApplyBinding{
		Key: key, CandidateID: revision.CandidateID, CandidateRevision: revision.CandidateRevision,
		ChangeSetID: revision.ID, PreviewHash: revision.PreviewHash,
	}
	eligible, err := s.workflowCandidateApplyApprovalStore.CheckApplyEligibility(ctx, binding, time.Now().UTC())
	if err != nil {
		return service.WorkflowCandidateRevision{}, err
	}
	if !validPersistedWorkflowCandidate(key, current, revision, eligible) {
		return service.WorkflowCandidateRevision{}, errors.New("persisted workflow Candidate binding is invalid")
	}
	if validWorkflowHistoryCandidateSelector(request.CandidateID) {
		if !validPersistedWorkflowHistoryCandidate(revision) {
			return service.WorkflowCandidateRevision{}, workflowCandidateApplyApprovalScopeError(key, "the History Candidate approval lineage is invalid")
		}
		return eligible, nil
	}
	lineage := []service.WorkflowCandidateRevision{revision}
	for number := revision.CandidateRevision - 1; number >= 1; number-- {
		ancestor, ancestorErr := s.workflowCandidateApplyApprovalStore.GetRevision(ctx, key, current.ID, number)
		if ancestorErr != nil {
			return service.WorkflowCandidateRevision{}, ancestorErr
		}
		lineage = append(lineage, ancestor)
		if number == 1 {
			break
		}
	}
	if !validPersistedWorkflowAgentRevisionLineage(lineage) {
		return service.WorkflowCandidateRevision{}, workflowCandidateApplyApprovalScopeError(key, "the revised Agent Candidate approval lineage is invalid")
	}
	return eligible, nil
}

func validPersistedWorkflowCandidate(key edit.WorkflowKey, current edit.WorkflowCandidate, revision, eligible service.WorkflowCandidateRevision) bool {
	if revision.SchemaVersion != edit.SchemaVersion || revision.Key != key || revision.CandidateID != current.ID || revision.CandidateRevision != current.CurrentRevision ||
		revision.ID != current.CurrentChangeSetID || revision.PreviewHash != current.CurrentPreviewHash || revision.ExpiresAt != current.ExpiresAt ||
		revision.State != service.WorkflowCandidateRevisionReady || revision.Base.ResourceVersion == "" || revision.Base.SemanticHash == "" || revision.Base.LayoutHash == "" ||
		revision.Validation.Kind != "validation" || !revision.Validation.Valid || revision.Validation.Basis.Validate() != nil ||
		revision.Validation.Basis.ResourceVersion != revision.Base.ResourceVersion || !canonicalWorkflowCandidateApplyHash(revision.Validation.Basis.SemanticHash) ||
		!canonicalWorkflowCandidateApplyHash(revision.PreviewHash) || len(revision.Operations) == 0 || len(revision.NormalizedOperations) == 0 ||
		!json.Valid(revision.Risk) || !json.Valid(revision.Diff) || revision.CreatedAt.IsZero() || !revision.ExpiresAt.After(revision.CreatedAt) {
		return false
	}
	left, leftErr := json.Marshal(revision)
	right, rightErr := json.Marshal(eligible)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func validPersistedWorkflowHistoryCandidate(revision service.WorkflowCandidateRevision) bool {
	return validWorkflowHistoryCandidateSelector(revision.CandidateID) && revision.CandidateRevision == 1 && revision.ParentRevision == 0 &&
		revision.ParentChangeSetID == "" && revision.Actor == edit.ActorKind("user") && revision.TurnID == "" && revision.ToolCallID == "" &&
		validWorkflowCandidatePreviewBinding(revision)
}

// validPersistedWorkflowAgentRevisionLineage accepts the current revision
// first, followed by every persisted ancestor in descending revision order.
// The browser cannot omit or provide this lineage: the resolver reconstructs
// it revision-by-revision from the immutable Candidate store.
func validPersistedWorkflowAgentRevisionLineage(lineage []service.WorkflowCandidateRevision) bool {
	if len(lineage) < 2 {
		return false
	}
	current := lineage[0]
	if current.CandidateRevision < 2 || uint64(len(lineage)) != current.CandidateRevision ||
		!validWorkflowAgentCandidateSelector(current.CandidateID) || current.Actor != edit.ActorKind("user") ||
		current.State != service.WorkflowCandidateRevisionReady || current.TurnID != "" || current.ToolCallID != "" ||
		!validWorkflowCandidateReviseChangeSetID(current.ID) || !validWorkflowCandidateLineageRevision(current) {
		return false
	}
	for index := 1; index < len(lineage); index++ {
		child, parent := lineage[index-1], lineage[index]
		if parent.CandidateRevision+1 != child.CandidateRevision || child.ParentRevision != parent.CandidateRevision || child.ParentChangeSetID != parent.ID ||
			parent.Key != current.Key || parent.CandidateID != current.CandidateID || parent.Base != current.Base ||
			parent.ActorID != current.ActorID || parent.CanvasSessionID != current.CanvasSessionID || parent.AgentSessionID != current.AgentSessionID ||
			!validWorkflowCandidateLineageRevision(parent) || !workflowCandidateNormalizedOperationsPrefix(parent.NormalizedOperations, child.NormalizedOperations) {
			return false
		}
		if parent.CandidateRevision == 1 {
			if index != len(lineage)-1 || parent.ParentRevision != 0 || parent.ParentChangeSetID != "" ||
				parent.State != service.WorkflowCandidateRevisionSuperseded || parent.Actor != edit.ActorKind("agent") ||
				!validWorkflowAgentOriginalChangeSetID(parent.ID) || !exactWorkflowCandidateApplyIdentifier(parent.TurnID, 512) ||
				!exactWorkflowCandidateApplyIdentifier(parent.ToolCallID, 512) {
				return false
			}
			continue
		}
		if parent.State != service.WorkflowCandidateRevisionSuperseded || parent.Actor != edit.ActorKind("user") ||
			parent.TurnID != "" || parent.ToolCallID != "" || !validWorkflowCandidateReviseChangeSetID(parent.ID) {
			return false
		}
	}
	return true
}

func validWorkflowCandidateLineageRevision(revision service.WorkflowCandidateRevision) bool {
	return revision.SchemaVersion == edit.SchemaVersion && revision.Key.Validate() == nil &&
		validWorkflowAgentCandidateSelector(revision.CandidateID) && exactWorkflowCandidateApplyIdentifier(revision.ID, 512) &&
		strings.TrimSpace(revision.Intent) != "" && exactWorkflowCandidateApplyIdentifier(revision.ActorID, 256) &&
		exactWorkflowCandidateApplyIdentifier(revision.CanvasSessionID, 256) && exactWorkflowCandidateApplyIdentifier(revision.AgentSessionID, 256) &&
		revision.Base.ResourceVersion != "" && revision.Base.SemanticHash != "" && revision.Base.LayoutHash != "" &&
		revision.Validation.Kind == "validation" && revision.Validation.Valid && revision.Validation.Basis.Validate() == nil &&
		revision.Validation.Basis.ResourceVersion == revision.Base.ResourceVersion && canonicalWorkflowCandidateApplyHash(revision.Validation.Basis.SemanticHash) &&
		canonicalWorkflowCandidateApplyHash(revision.PreviewHash) && len(revision.Operations) > 0 && len(revision.NormalizedOperations) > 0 &&
		validWorkflowCandidatePreviewBinding(revision) && json.Valid(revision.Risk) && json.Valid(revision.Diff) && !revision.CreatedAt.IsZero() && revision.ExpiresAt.After(revision.CreatedAt)
}

func validWorkflowCandidatePreviewBinding(revision service.WorkflowCandidateRevision) bool {
	digest, err := workflowCandidateApplyDigest([]any{
		edit.SchemaVersion, revision.Key, revision.CandidateRevision, revision.ParentChangeSetID,
		revision.NormalizedOperations, revision.Validation.Basis,
	})
	return err == nil && revision.PreviewHash == "sha256:"+digest
}

func workflowCandidateNormalizedOperationsPrefix(parent, child []edit.WorkflowOperation) bool {
	if len(parent) == 0 || len(parent) > len(child) {
		return false
	}
	parentJSON, parentErr := json.Marshal(parent)
	childPrefixJSON, childErr := json.Marshal(child[:len(parent)])
	return parentErr == nil && childErr == nil && string(parentJSON) == string(childPrefixJSON)
}

func validWorkflowAgentOriginalChangeSetID(value string) bool {
	const prefix = "changeset-"
	encoded := strings.TrimPrefix(value, prefix)
	if encoded == value || len(encoded) != sha256.Size*2 || encoded != strings.ToLower(encoded) {
		return false
	}
	decoded, err := hex.DecodeString(encoded)
	return err == nil && len(decoded) == sha256.Size
}

func validWorkflowCandidateReviseChangeSetID(value string) bool {
	const prefix = "changeset-revise-"
	encoded := strings.TrimPrefix(value, prefix)
	if encoded == value || len(encoded) != sha256.Size*2+1+sha256.Size*2 || encoded[sha256.Size*2] != '-' || encoded != strings.ToLower(encoded) {
		return false
	}
	left, leftErr := hex.DecodeString(encoded[:sha256.Size*2])
	right, rightErr := hex.DecodeString(encoded[sha256.Size*2+1:])
	return leftErr == nil && rightErr == nil && len(left) == sha256.Size && len(right) == sha256.Size
}

func workflowCandidateApplyApprovalScopeError(key edit.WorkflowKey, message string) error {
	keyCopy := key
	return &edit.DomainError{Kind: "error", Code: edit.ErrorApprovalRequired, Key: &keyCopy, Message: message, RetryHint: "reload the current Candidate from the authenticated workflow session"}
}

func canonicalWorkflowCandidateApplyHash(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(decoded) == sha256.Size
}

func writeWorkflowCandidateApplyApprovalResolutionError(w http.ResponseWriter, err error) {
	var domain *edit.DomainError
	if errors.As(err, &domain) {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	if errors.Is(err, service.ErrNotFound) {
		writeWorkflowCommandError(w, http.StatusForbidden, edit.ErrorApprovalRequired, "the Candidate is unavailable in this workflow scope", "reload the Candidate from the current authenticated session")
		return
	}
	if errors.Is(err, service.ErrWorkflowEditConfiguration) {
		writeWorkflowCommandApplicationError(w, err)
		return
	}
	writeWorkflowCommandError(w, http.StatusInternalServerError, edit.ErrorMissingConfiguration, "the persisted Candidate binding is invalid", "reload the Candidate before retrying")
}

func buildWorkflowCandidateControlledApplyRequest(scope WorkflowCommandTrustedScope, candidate service.WorkflowCandidateRevision, idempotencyKey string) (runtimekernel.ControlledToolTurnRequest, string, error) {
	key := candidate.Key
	identity := runtimecontract.TurnIdentity{TenantID: strings.TrimSpace(scope.TenantID), UserID: strings.TrimSpace(scope.ActorID), SessionID: strings.TrimSpace(scope.SessionID)}
	artifact, err := appui.FreezeWorkflowTurnContext(appui.WorkflowTurnTrustedScope{
		TenantID: identity.TenantID, WorkflowID: key.WorkflowID, ActorID: identity.UserID, SessionID: identity.SessionID,
		CanvasSessionID: candidate.CanvasSessionID,
	}, appui.WorkflowTurnContext{
		WorkflowKey: key, Revision: candidate.Base,
		Environment: appui.WorkflowTurnEnvironment{BindingVersion: candidate.Validation.Basis.EnvironmentBindingVersion},
		Scope:       appui.WorkflowTurnContextScopeBinding{ActorID: identity.UserID, SessionID: identity.SessionID, CanvasSessionID: candidate.CanvasSessionID},
	})
	if err != nil {
		return runtimekernel.ControlledToolTurnRequest{}, "", err
	}

	turnToken, err := workflowCandidateApplyDigest([]any{
		edit.SchemaVersion, workflowCandidateApplySource, key, identity, candidate.CanvasSessionID,
		candidate.CandidateID, candidate.CandidateRevision, candidate.ID, candidate.PreviewHash, candidate.Base, idempotencyKey,
	})
	if err != nil {
		return runtimekernel.ControlledToolTurnRequest{}, "", err
	}
	bindingToken, err := workflowCandidateApplyDigest([]any{
		edit.SchemaVersion, workflowCandidateApplySource, key, identity, candidate.CanvasSessionID, candidate.CandidateID, candidate.CandidateRevision,
		candidate.ID, candidate.PreviewHash, candidate.Base, idempotencyKey,
	})
	if err != nil {
		return runtimekernel.ControlledToolTurnRequest{}, "", err
	}
	applyIdempotencyPrefix, reason, turnInput := "history-apply-", "apply the exact reviewed Workflow History Candidate", "Request approval for the exact reviewed Workflow History Candidate."
	if validWorkflowAgentCandidateSelector(candidate.CandidateID) {
		applyIdempotencyPrefix, reason, turnInput = "candidate-apply-", "apply the exact reviewed user-revised Agent Workflow Candidate", "Request approval for the exact reviewed user-revised Agent Workflow Candidate."
	}
	apply := edit.ApplyPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: key,
		ChangeSetID: candidate.ID, PreviewHash: candidate.PreviewHash, ExpectedRevision: candidate.Base.ResourceVersion,
		IdempotencyKey: applyIdempotencyPrefix + bindingToken[:32], CandidateGrantID: "workflow-grant-" + bindingToken[:32],
		ApplyApprovalID: "workflow-approval-" + bindingToken[:32], Reason: reason,
	}
	arguments, err := json.Marshal(apply)
	if err != nil {
		return runtimekernel.ControlledToolTurnRequest{}, "", err
	}
	argumentsHash, err := actionproposal.NormalizedInputHash(arguments)
	if err != nil {
		return runtimekernel.ControlledToolTurnRequest{}, "", err
	}
	turnID, toolCallID := "workflow-apply-turn-"+turnToken[:32], "workflow-apply-call-"+turnToken[:32]
	metadata := map[string]string{
		"source": "workflow_ai_chat", "workflowId": key.WorkflowID,
		"workflowCanvasSessionId":     candidate.CanvasSessionID,
		"aiops.workflowAgent.enabled": "true", "aiops.workflowAgent.routeApplied": "true",
		"profile": runtimekernel.RuntimePromptProfileWorkflowAgent, "toolProfile": runtimekernel.RuntimePromptProfileWorkflowAgent,
		"agentProfile": runtimekernel.RuntimePromptProfileWorkflowAgent, "runtimeProfile": runtimekernel.RuntimePromptProfileWorkflowAgent,
		"runtimeRoute": runtimekernel.RuntimePromptProfileWorkflowAgent, "enableToolPack": "workflow_editor",
	}
	artifacts := []runtimecontract.TurnArtifactEnvelope{artifact}
	bindings := []resourcebinding.ResourceBindingSnapshot{resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
		Type: resourcebinding.ResourceTypeWorkflow, ID: key.WorkflowID, Namespace: key.TenantID, Provider: "runner",
	}, resourcebinding.BindingOptions{
		Source: resourcebinding.BindingSourceWorkflow, VerifiedBy: appui.WorkflowTurnContextArtifactKind, TrustLevel: resourcebinding.TrustLevelVerified,
	})}
	turn := runtimekernel.TurnRequest{
		SessionType: runtimekernel.SessionTypeWorkspace, Mode: runtimekernel.ModeExecute,
		SessionID: identity.SessionID, TurnID: turnID, Input: turnInput,
		PermissionProfile: runtimekernel.RuntimePermissionProfileApprovalRequired,
		RollbackPolicy:    runtimekernel.RuntimeRollbackPolicyActionContractRequired,
		Metadata:          metadata, TrustedIdentity: &identity, ResourceBindings: bindings, TurnArtifacts: artifacts,
	}
	toolCall := runtimekernel.ToolCall{ID: toolCallID, Name: workflowCandidateApplyToolName, Arguments: arguments}
	execution := tooling.ToolExecutionContext{
		SessionID: identity.SessionID, TurnID: turnID, TenantID: identity.TenantID, UserID: identity.UserID,
		ToolCallID: toolCallID, ToolName: workflowCandidateApplyToolName,
		OriginalInput: arguments, SanitizedInput: arguments, Metadata: metadata, TurnArtifacts: artifacts,
	}
	return runtimekernel.ControlledToolTurnRequest{
		Turn: turn, ToolCall: toolCall, ExecutionContext: execution,
		Source: workflowCandidateApplySource, IdempotencyKey: idempotencyKey,
	}, argumentsHash, nil
}

func workflowCandidateApplyDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

func validWorkflowCandidateControlledApproval(result runtimekernel.ControlledToolTurnResult, request runtimekernel.ControlledToolTurnRequest, argumentsHash string) bool {
	approval := result.Approval
	return result.Status == "blocked" && result.SessionType == runtimekernel.SessionTypeWorkspace && result.Mode == runtimekernel.ModeExecute &&
		result.SessionID == request.Turn.SessionID && result.TurnID == request.Turn.TurnID && result.ToolResult == nil && approval != nil &&
		exactWorkflowCandidateApplyIdentifier(approval.ApprovalID, 512) && approval.SessionID == result.SessionID && approval.TurnID == result.TurnID &&
		approval.ToolCallID == request.ToolCall.ID && approval.ToolName == workflowCandidateApplyToolName && approval.ArgumentsHash == argumentsHash
}
