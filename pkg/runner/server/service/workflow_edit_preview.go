package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

const workflowPreviewTTL = 30 * time.Minute

type WorkflowEditPreviewRequest struct {
	edit.PreviewPatchRequest
	CandidateID          string                   `json:"candidateId"`
	BaseChangeSetID      string                   `json:"baseChangeSetId,omitempty"`
	AdditionalOperations []edit.WorkflowOperation `json:"additionalOperations,omitempty"`
	// RevisionIdempotencyKey is a trusted, server-only input used by direct
	// Candidate revisions. The raw value is never persisted or serialized.
	RevisionIdempotencyKey string         `json:"-"`
	Actor                  edit.ActorKind `json:"actor"`
	ActorID                string         `json:"actorId"`
	CanvasSessionID        string         `json:"canvasSessionId"`
	AgentSessionID         string         `json:"agentSessionId,omitempty"`
	TurnID                 string         `json:"turnId,omitempty"`
	ToolCallID             string         `json:"toolCallId,omitempty"`
}

type WorkflowEditPreviewRisk struct {
	Level            WorkflowRiskLevel `json:"level"`
	OperationCount   int               `json:"operationCount"`
	AddedNodeCount   int               `json:"addedNodeCount"`
	DeletedNodeCount int               `json:"deletedNodeCount"`
	DeletesExisting  bool              `json:"deletesExisting"`
	RequiresApproval bool              `json:"requiresApproval"`
}

type WorkflowEditPreviewDiff struct {
	Graph               edit.GraphDiff             `json:"graph"`
	Inverse             edit.InversePlan           `json:"inverse"`
	RebaseBase          map[string]json.RawMessage `json:"rebaseBase"`
	RebaseInvariantHash string                     `json:"rebaseInvariantHash"`
}

type WorkflowEditPreviewResult struct {
	Kind              string                    `json:"kind"`
	SchemaVersion     string                    `json:"schemaVersion"`
	ChangeSet         edit.WorkflowChangeSet    `json:"changeSet"`
	Candidate         edit.WorkflowCandidate    `json:"candidate"`
	CandidateRevision uint64                    `json:"candidateRevision"`
	PreviewHash       string                    `json:"previewHash"`
	ValidationBasis   edit.ValidationBasis      `json:"validationBasis"`
	Diagnostics       []edit.WorkflowDiagnostic `json:"diagnostics"`
	Risk              WorkflowEditPreviewRisk   `json:"risk"`
	Diff              WorkflowEditPreviewDiff   `json:"diff"`
	Graph             visual.Graph              `json:"-"`
	Rebased           bool                      `json:"rebased"`
}

func (a *WorkflowEditApplication) Preview(ctx context.Context, request WorkflowEditPreviewRequest) (result WorkflowEditPreviewResult, resultErr error) {
	startedAt := time.Now().UTC()
	defer func() {
		a.observeWorkflowEdit(ctx, WorkflowEditObservation{
			Operation: WorkflowEditObservePreview, Key: request.WorkflowKey, ActorID: request.ActorID,
			SessionID: request.AgentSessionID, TurnID: request.TurnID, ToolCallID: request.ToolCallID,
			CandidateID: request.CandidateID, CandidateRevision: result.CandidateRevision, ChangeSetID: result.ChangeSet.ID,
			StartedAt: startedAt, Duration: time.Since(startedAt),
		}, resultErr)
	}()
	if a == nil || a.backend == nil || a.candidates == nil {
		return WorkflowEditPreviewResult{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowPreviewRequest(request); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	revision, parent, originals, inputs, intent, rebased, err := a.workflowPreviewInputs(ctx, request, snapshot)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	normalized, err := normalizeWorkflowPreviewOperations(inputs)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	applied, err := applyWorkflowPreviewOperations(snapshot.Graph, normalized)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	validation, err := a.validateSnapshot(ctx, snapshot, applied.Graph, snapshot.Revision.ResourceVersion)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	basis, err := a.workflowPreviewValidationBasis(ctx, snapshot, applied.Graph)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	previewHash, err := workflowPreviewHash(request.WorkflowKey, revision, parent, normalized, basis)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	diff := workflowPreviewDiff(snapshot.Graph, applied)
	risk := workflowPreviewRisk(normalized, diff.Graph)
	rawDiff, err := json.Marshal(diff)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	rawRisk, err := json.Marshal(risk)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	now := time.Now().UTC()
	changeSetID := workflowPreviewChangeSetID(request.WorkflowKey, request.CandidateID, revision, parent, previewHash)
	if request.RevisionIdempotencyKey != "" {
		changeSetID, err = workflowCandidateReviseChangeSetID(request, previewHash)
		if err != nil {
			return WorkflowEditPreviewResult{}, err
		}
	}
	editValidation := workflowPreviewValidation(validation, basis)
	record := WorkflowCandidateRevision{WorkflowChangeSet: edit.WorkflowChangeSet{
		SchemaVersion: edit.SchemaVersion, ID: changeSetID, Key: request.WorkflowKey, CandidateID: request.CandidateID,
		CandidateRevision: revision, ParentChangeSetID: parent, Actor: request.Actor, Intent: intent, Base: snapshot.Revision,
		Operations: originals, PreviewHash: previewHash, Validation: editValidation, State: WorkflowCandidateRevisionReady,
		CreatedAt: now, ExpiresAt: now.Add(workflowPreviewTTL),
	}, ParentRevision: revision - 1, NormalizedOperations: normalized, Risk: rawRisk, Diff: rawDiff, ActorID: request.ActorID,
		CanvasSessionID: request.CanvasSessionID, AgentSessionID: request.AgentSessionID, TurnID: request.TurnID, ToolCallID: request.ToolCallID}
	var candidate edit.WorkflowCandidate
	if revision == 1 {
		candidate, err = a.candidates.Create(ctx, record)
	} else {
		candidate, err = a.candidates.Revise(ctx, request.WorkflowKey, request.CandidateID, revision-1, record)
	}
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	return WorkflowEditPreviewResult{Kind: "preview", SchemaVersion: edit.SchemaVersion, ChangeSet: record.WorkflowChangeSet,
		Candidate: candidate, CandidateRevision: revision, PreviewHash: previewHash, ValidationBasis: basis,
		Diagnostics: append([]edit.WorkflowDiagnostic(nil), editValidation.Diagnostics...), Risk: risk, Diff: diff, Graph: applied.Graph, Rebased: rebased}, nil
}

func (a *WorkflowEditApplication) workflowPreviewInputs(ctx context.Context, request WorkflowEditPreviewRequest, snapshot WorkflowEditSnapshot) (uint64, string, []edit.WorkflowOperation, []edit.WorkflowOperation, string, bool, error) {
	if request.BaseChangeSetID == "" {
		if request.Expected != snapshot.Revision {
			return 0, "", nil, nil, "", false, workflowPreviewConflict(request.WorkflowKey, request.Expected, snapshot.Revision, []string{"/resourceVersion"}, nil)
		}
		return 1, "", cloneWorkflowOperations(request.Operations), cloneWorkflowOperations(request.Operations), request.Intent, false, nil
	}
	current, err := a.candidates.GetCurrent(ctx, request.WorkflowKey, request.CandidateID)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	if current.CurrentChangeSetID != request.BaseChangeSetID {
		return 0, "", nil, nil, "", false, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, current.CurrentChangeSetID, request.BaseChangeSetID, "candidate revision changed", "reload the current candidate")
	}
	base, err := a.candidates.GetRevision(ctx, request.WorkflowKey, request.CandidateID, current.CurrentRevision)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	if base.ActorID != request.ActorID || base.CanvasSessionID != request.CanvasSessionID || !workflowPreviewActorCanRevise(base.Actor, request.Actor) {
		return 0, "", nil, nil, "", false, workflowPreviewDomainError(edit.ErrorApprovalRequired, request.WorkflowKey, "", "", "candidate revision belongs to another editing scope", "create a new candidate")
	}
	var artifact WorkflowEditPreviewDiff
	if edit.DecodeStrict(base.Diff, &artifact) != nil {
		return 0, "", nil, nil, "", false, fmt.Errorf("invalid persisted preview diff")
	}
	rebased, err := workflowPreviewCanRebase(request.WorkflowKey, base, artifact, snapshot)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	originals := append(cloneWorkflowOperations(base.Operations), cloneWorkflowOperations(request.AdditionalOperations)...)
	inputs := append(cloneWorkflowOperations(base.NormalizedOperations), cloneWorkflowOperations(request.AdditionalOperations)...)
	intent := request.Intent
	if strings.TrimSpace(intent) == "" {
		intent = base.Intent
	}
	return current.CurrentRevision + 1, base.ID, originals, inputs, intent, rebased, nil
}

// WorkflowCandidateReviseRequest is the trusted application boundary for a
// user's direct edit of a Ghost Candidate. Browser-supplied identity is not
// represented here: ActorID, CanvasSessionID, SessionID and TenantID must be
// projected from the authenticated server scope.
type WorkflowCandidateReviseRequest struct {
	SchemaVersion       string
	WorkflowKey         edit.WorkflowKey
	CandidateID         string
	BaseChangeSetID     string
	Expected            edit.Revision
	IdempotencyKey      string
	Intent              string
	AdditionalOperation edit.WorkflowOperation
	ActorID             string
	CanvasSessionID     string
	SessionID           string
}

// ReviseCandidate creates one immutable Candidate revision without applying
// it to the committed graph. Stable replay is encoded using irreversible
// digests in the generated changeSetId, so retries remain stable across
// process restarts without persisting the raw idempotency key.
func (a *WorkflowEditApplication) ReviseCandidate(ctx context.Context, request WorkflowCandidateReviseRequest) (WorkflowEditPreviewResult, error) {
	if a == nil || a.backend == nil || a.candidates == nil {
		return WorkflowEditPreviewResult{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowCandidateReviseRequest(request); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	base, current, revisions, err := a.workflowCandidateReviseBase(ctx, request)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	if err := workflowCandidateReviseScope(request, base); err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	if replay, found, replayErr := workflowCandidateReviseReplay(request, current, revisions, base); found || replayErr != nil {
		return replay, replayErr
	}
	if current.CurrentChangeSetID != request.BaseChangeSetID || current.CurrentRevision != base.CandidateRevision {
		return WorkflowEditPreviewResult{}, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, current.CurrentChangeSetID, request.BaseChangeSetID, "candidate revision changed", "reload the current candidate")
	}
	preview, err := a.Preview(ctx, WorkflowEditPreviewRequest{
		PreviewPatchRequest: edit.PreviewPatchRequest{
			SchemaVersion: request.SchemaVersion, WorkflowKey: request.WorkflowKey, Expected: request.Expected,
			Intent: request.Intent,
		},
		CandidateID: request.CandidateID, BaseChangeSetID: request.BaseChangeSetID,
		AdditionalOperations:   []edit.WorkflowOperation{request.AdditionalOperation},
		RevisionIdempotencyKey: request.IdempotencyKey,
		Actor:                  edit.ActorKind("user"), ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID,
		AgentSessionID: request.SessionID,
	})
	if err == nil {
		return preview, nil
	}
	// A concurrent identical request may have won the Candidate CAS. Reload and
	// resolve it as a replay instead of leaking a transient conflict.
	base, current, revisions, reloadErr := a.workflowCandidateReviseBase(ctx, request)
	if reloadErr == nil {
		if replay, found, replayErr := workflowCandidateReviseReplay(request, current, revisions, base); found || replayErr != nil {
			return replay, replayErr
		}
	}
	return WorkflowEditPreviewResult{}, err
}

func validateWorkflowCandidateReviseRequest(request WorkflowCandidateReviseRequest) error {
	validRevision := func(value string) bool {
		return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
	}
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || validateWorkflowCandidateID(request.CandidateID) != nil ||
		strings.TrimSpace(request.BaseChangeSetID) == "" || request.BaseChangeSetID != strings.TrimSpace(request.BaseChangeSetID) ||
		!validRevision(request.Expected.ResourceVersion) || !validRevision(request.Expected.SemanticHash) || !validRevision(request.Expected.LayoutHash) ||
		!validWorkflowApprovalValue(request.IdempotencyKey, true) || !validWorkflowApprovalValue(request.ActorID, true) ||
		!validWorkflowApprovalValue(request.CanvasSessionID, true) || !validWorkflowApprovalValue(request.SessionID, true) ||
		strings.TrimSpace(request.Intent) == "" || request.Intent != strings.TrimSpace(request.Intent) || request.AdditionalOperation.Value == nil {
		return fmt.Errorf("%w: invalid trusted candidate revise request", ErrInvalid)
	}
	if err := request.AdditionalOperation.Validate(); err != nil {
		return fmt.Errorf("%w: invalid candidate revise operation", ErrInvalid)
	}
	return nil
}

func (a *WorkflowEditApplication) workflowCandidateReviseBase(ctx context.Context, request WorkflowCandidateReviseRequest) (WorkflowCandidateRevision, edit.WorkflowCandidate, []WorkflowCandidateRevision, error) {
	current, err := a.candidates.GetCurrent(ctx, request.WorkflowKey, request.CandidateID)
	if err != nil {
		return WorkflowCandidateRevision{}, edit.WorkflowCandidate{}, nil, err
	}
	revisions, err := a.candidates.ListRevisions(ctx, request.WorkflowKey, request.CandidateID)
	if err != nil {
		return WorkflowCandidateRevision{}, edit.WorkflowCandidate{}, nil, err
	}
	for _, revision := range revisions {
		if revision.ID == request.BaseChangeSetID {
			return revision, current, revisions, nil
		}
	}
	return WorkflowCandidateRevision{}, edit.WorkflowCandidate{}, nil, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, current.CurrentChangeSetID, request.BaseChangeSetID, "candidate base does not exist", "reload the current candidate")
}

func workflowCandidateReviseScope(request WorkflowCandidateReviseRequest, base WorkflowCandidateRevision) error {
	if base.ActorID != request.ActorID || base.CanvasSessionID != request.CanvasSessionID || !workflowPreviewActorCanRevise(base.Actor, edit.ActorKind("user")) {
		return workflowPreviewDomainError(edit.ErrorApprovalRequired, request.WorkflowKey, "", "", "candidate revision belongs to another editing scope", "create a new candidate")
	}
	return nil
}

func workflowPreviewActorCanRevise(base, next edit.ActorKind) bool {
	if base == next {
		return true
	}
	return next == edit.ActorKind("user") && (base == edit.ActorKind("assistant") || base == edit.ActorKind("agent"))
}

func workflowCandidateReviseReplay(request WorkflowCandidateReviseRequest, current edit.WorkflowCandidate, revisions []WorkflowCandidateRevision, base WorkflowCandidateRevision) (WorkflowEditPreviewResult, bool, error) {
	if int(base.CandidateRevision) >= len(revisions) {
		return WorkflowEditPreviewResult{}, false, nil
	}
	child := revisions[base.CandidateRevision]
	if child.ParentChangeSetID != base.ID {
		return WorkflowEditPreviewResult{}, false, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, current.CurrentChangeSetID, request.BaseChangeSetID, "candidate revision chain is inconsistent", "reload the current candidate")
	}
	keyDigest, payloadDigest, err := workflowCandidateReviseDigests(request)
	if err != nil {
		return WorkflowEditPreviewResult{}, false, err
	}
	storedKey, storedPayload, ok := workflowCandidateReviseIDDigests(child.ID)
	if !ok || storedKey != keyDigest {
		return WorkflowEditPreviewResult{}, false, workflowPreviewDomainError(edit.ErrorStaleChangeSet, request.WorkflowKey, current.CurrentChangeSetID, request.BaseChangeSetID, "candidate revision changed", "reload the current candidate")
	}
	if storedPayload != payloadDigest {
		return WorkflowEditPreviewResult{}, true, workflowPreviewDomainError(edit.ErrorIdempotencyKeyReused, request.WorkflowKey, child.Base.ResourceVersion, request.Expected.ResourceVersion, "candidate revise idempotency key was already used for another request", "reuse the original request or choose a new idempotency key")
	}
	result, resultErr := workflowPreviewResultFromRevision(current, child)
	return result, true, resultErr
}

func workflowPreviewResultFromRevision(candidate edit.WorkflowCandidate, revision WorkflowCandidateRevision) (WorkflowEditPreviewResult, error) {
	var risk WorkflowEditPreviewRisk
	var diff WorkflowEditPreviewDiff
	if json.Unmarshal(revision.Risk, &risk) != nil || json.Unmarshal(revision.Diff, &diff) != nil {
		return WorkflowEditPreviewResult{}, ErrWorkflowEditSnapshot
	}
	return WorkflowEditPreviewResult{
		Kind: "preview", SchemaVersion: edit.SchemaVersion, ChangeSet: revision.WorkflowChangeSet, Candidate: candidate,
		CandidateRevision: revision.CandidateRevision, PreviewHash: revision.PreviewHash, ValidationBasis: revision.Validation.Basis,
		Diagnostics: append([]edit.WorkflowDiagnostic(nil), revision.Validation.Diagnostics...), Risk: risk, Diff: diff,
	}, nil
}

func workflowCandidateReviseDigests(request WorkflowCandidateReviseRequest) (string, string, error) {
	keyDigest, err := workflowQueryDigest([]any{"candidate-revise-key", request.WorkflowKey, request.CandidateID, request.IdempotencyKey})
	if err != nil {
		return "", "", err
	}
	payloadDigest, err := workflowQueryDigest([]any{
		"candidate-revise-payload", request.SchemaVersion, request.WorkflowKey, request.CandidateID, request.BaseChangeSetID,
		request.Expected, request.Intent, request.AdditionalOperation, request.ActorID, request.CanvasSessionID, request.SessionID,
	})
	return strings.TrimPrefix(keyDigest, "sha256:"), strings.TrimPrefix(payloadDigest, "sha256:"), err
}

func workflowCandidateReviseChangeSetID(request WorkflowEditPreviewRequest, _ string) (string, error) {
	revise := WorkflowCandidateReviseRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: request.WorkflowKey, CandidateID: request.CandidateID,
		BaseChangeSetID: request.BaseChangeSetID, Expected: request.Expected, IdempotencyKey: request.RevisionIdempotencyKey,
		Intent: request.Intent, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID, SessionID: request.AgentSessionID,
	}
	if len(request.AdditionalOperations) == 1 {
		revise.AdditionalOperation = request.AdditionalOperations[0]
	}
	keyDigest, payloadDigest, err := workflowCandidateReviseDigests(revise)
	if err != nil {
		return "", err
	}
	return "changeset-revise-" + keyDigest + "-" + payloadDigest, nil
}

func workflowCandidateReviseIDDigests(changeSetID string) (string, string, bool) {
	const prefix = "changeset-revise-"
	value := strings.TrimPrefix(changeSetID, prefix)
	if value == changeSetID || len(value) != 64+1+64 || value[64] != '-' {
		return "", "", false
	}
	return value[:64], value[65:], true
}

func validateWorkflowPreviewRequest(request WorkflowEditPreviewRequest) error {
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || strings.TrimSpace(request.CandidateID) == "" || strings.TrimSpace(string(request.Actor)) == "" || strings.TrimSpace(request.ActorID) == "" || strings.TrimSpace(request.CanvasSessionID) == "" {
		return fmt.Errorf("%w: invalid workflow preview request", ErrInvalid)
	}
	if request.BaseChangeSetID == "" {
		if len(request.Operations) == 0 || len(request.AdditionalOperations) != 0 || strings.TrimSpace(request.Intent) == "" {
			return fmt.Errorf("%w: initial preview requires intent and operations", ErrInvalid)
		}
	} else if len(request.Operations) != 0 || len(request.AdditionalOperations) == 0 {
		return fmt.Errorf("%w: candidate revise requires baseChangeSetId and additionalOperations", ErrInvalid)
	}
	return nil
}

func normalizeWorkflowPreviewOperations(input []edit.WorkflowOperation) ([]edit.WorkflowOperation, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var cloned []edit.WorkflowOperation
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, workflowPreviewDomainError(edit.ErrorInvalidPatch, edit.WorkflowKey{}, "", "", "workflow operations are invalid", "repair the operation list")
	}
	result := make([]edit.WorkflowOperation, 0, len(cloned))
	for _, operation := range cloned {
		if err := operation.Validate(); err != nil {
			return nil, workflowPreviewDomainError(edit.ErrorInvalidPatch, edit.WorkflowKey{}, "", "", "workflow operation is invalid", "repair the operation")
		}
		if value, ok := operation.Value.(*edit.UpdateLayoutOperation); ok {
			sort.Slice(value.Layout.Nodes, func(i, j int) bool { return value.Layout.Nodes[i].NodeID < value.Layout.Nodes[j].NodeID })
			for _, patch := range value.Layout.Nodes {
				position := patch.Position
				result = append(result, edit.NewOperation(&edit.UpdateNodeOperation{NodeID: patch.NodeID, Set: edit.NodePatch{Position: &position}}))
			}
			if value.Layout.Viewport != nil {
				result = append(result, edit.NewOperation(&edit.UpdateLayoutOperation{Layout: edit.LayoutPatch{Viewport: value.Layout.Viewport}}))
			}
			continue
		}
		result = append(result, operation)
	}
	return result, nil
}

func applyWorkflowPreviewOperations(base visual.Graph, operations []edit.WorkflowOperation) (edit.ApplyResult, error) {
	graphOperations := make([]edit.WorkflowOperation, 0, len(operations))
	viewports := make([]*visual.Viewport, 0)
	for _, operation := range operations {
		if layout, ok := operation.Value.(*edit.UpdateLayoutOperation); ok {
			viewports = append(viewports, layout.Layout.Viewport)
			continue
		}
		graphOperations = append(graphOperations, operation)
	}
	result, err := edit.ApplyOperations(base, graphOperations)
	if err != nil {
		return result, err
	}
	for _, viewport := range viewports {
		before := result.Graph.Layout.Viewport
		result.Graph.Layout.Viewport = *viewport
		copyBefore := before
		result.Inverse.Operations = append([]edit.WorkflowOperation{edit.NewOperation(&edit.UpdateLayoutOperation{Layout: edit.LayoutPatch{Viewport: &copyBefore}})}, result.Inverse.Operations...)
	}
	result.Diff = edit.DiffGraphs(base, result.Graph)
	if len(viewports) > 0 && base.Layout.Viewport != result.Graph.Layout.Viewport {
		result.Diff.TouchedPaths = append(result.Diff.TouchedPaths, "/layout/viewport")
		sort.Strings(result.Diff.TouchedPaths)
	}
	return result, nil
}

func (a *WorkflowEditApplication) workflowPreviewValidationBasis(ctx context.Context, snapshot WorkflowEditSnapshot, graph visual.Graph) (edit.ValidationBasis, error) {
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	actions, err := a.backend.ListWorkflowActions(ctx, snapshot.Key, ActionCatalogFilter{})
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i].Action < actions[j].Action })
	schemas := make([]any, 0, len(actions))
	for _, action := range actions {
		schemas = append(schemas, []any{action.Action, action.ArgsSchema, action.InputSchema, action.OutputSchema, action.InputsSchema, action.OutputsSchema, action.DefaultPorts, action.Defaults, action.RequiredArgs, action.ArgConflicts, action.Outputs})
	}
	catalogVersion, err := workflowQueryDigest(actions)
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	actionDigest, err := workflowQueryDigest(schemas)
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	references, err := workflowQueryDigest([]any{hashes.SemanticHash, graph.Workflow.Tests})
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	environment, err := workflowQueryDigest([]any{graph.Workflow.ValidationEnv, graph.Workflow.EnvPackages, graph.Workflow.Inventory})
	if err != nil {
		return edit.ValidationBasis{}, err
	}
	return edit.ValidationBasis{ResourceVersion: snapshot.Revision.ResourceVersion, SemanticHash: hashes.SemanticHash, CatalogVersion: catalogVersion, ActionSchemaDigest: actionDigest, ReferenceSnapshotHash: references, EnvironmentBindingVersion: environment}, nil
}

func workflowPreviewValidation(result WorkflowEditValidationResult, basis edit.ValidationBasis) edit.ValidationResult {
	diagnostics := make([]edit.WorkflowDiagnostic, 0, len(result.Validation.Errors)+len(result.Validation.Warnings))
	issues := append(append([]VisualWorkflowIssue(nil), result.Validation.Errors...), result.Validation.Warnings...)
	for _, issue := range issues {
		diagnostics = append(diagnostics, edit.WorkflowDiagnostic{Code: issue.Code, Severity: issue.Severity, Message: issue.Message, NodeID: issue.NodeID, EdgeID: issue.EdgeID, Field: issue.Field, Suggestion: issue.Suggestion})
	}
	return edit.ValidationResult{Kind: "validation", Basis: basis, Valid: result.Validation.Valid, Diagnostics: diagnostics}
}

func workflowPreviewDiff(base visual.Graph, applied edit.ApplyResult) WorkflowEditPreviewDiff {
	paths := append([]string(nil), applied.Diff.TouchedPaths...)
	for _, node := range base.Nodes {
		paths = append(paths, "/nodes/"+workflowPreviewEscape(node.ID)+"/position", "/nodes/"+workflowPreviewEscape(node.ID)+"/ports")
	}
	for _, edge := range base.Edges {
		paths = append(paths, "/edges/"+workflowPreviewEscape(edge.ID)+"/source_port", "/edges/"+workflowPreviewEscape(edge.ID)+"/target_port")
	}
	paths = append(paths, "/layout/viewport")
	sort.Strings(paths)
	values := map[string]json.RawMessage{}
	for _, path := range paths {
		if _, exists := values[path]; exists {
			continue
		}
		value, found := workflowPreviewPathValue(base, path)
		if !found {
			value = json.RawMessage("null")
		}
		values[path] = value
	}
	invariant, _ := workflowPreviewRebaseInvariantHash(base)
	return WorkflowEditPreviewDiff{Graph: applied.Diff, Inverse: applied.Inverse, RebaseBase: values, RebaseInvariantHash: invariant}
}

func workflowPreviewRisk(operations []edit.WorkflowOperation, diff edit.GraphDiff) WorkflowEditPreviewRisk {
	risk := WorkflowEditPreviewRisk{Level: WorkflowRiskLow, OperationCount: len(operations), AddedNodeCount: len(diff.Delta.AddedNodeIDs), DeletedNodeCount: len(diff.Delta.DeletedNodeIDs)}
	for _, operation := range operations {
		switch value := operation.Value.(type) {
		case *edit.DeleteNodeOperation, *edit.DeleteEdgeOperation:
			risk.Level, risk.DeletesExisting = WorkflowRiskHigh, true
		case *edit.UpdateNodeOperation:
			if value.Set.Position == nil || value.Set != (edit.NodePatch{Position: value.Set.Position}) {
				if value.Set.Ports != nil {
					risk.Level = WorkflowRiskHigh
				} else if risk.Level == WorkflowRiskLow {
					risk.Level = WorkflowRiskMedium
				}
			}
		case *edit.UpdateLayoutOperation:
		default:
			if risk.Level == WorkflowRiskLow {
				risk.Level = WorkflowRiskMedium
			}
		}
	}
	risk.RequiresApproval = risk.Level == WorkflowRiskHigh
	return risk
}

func workflowPreviewCanRebase(key edit.WorkflowKey, base WorkflowCandidateRevision, artifact WorkflowEditPreviewDiff, current WorkflowEditSnapshot) (bool, error) {
	if current.Revision == base.Base {
		return false, nil
	}
	changedTouched := workflowPreviewChangedPaths(current.Graph, artifact.RebaseBase, func(path string) bool {
		for _, touched := range artifact.Graph.TouchedPaths {
			if path == touched || strings.HasPrefix(path, touched+"/") || strings.HasPrefix(touched, path+"/") {
				return true
			}
		}
		return false
	})
	if current.Revision.SemanticHash != base.Base.SemanticHash {
		ports := workflowPreviewChangedPaths(current.Graph, artifact.RebaseBase, func(path string) bool {
			return strings.HasSuffix(path, "/ports") || strings.HasSuffix(path, "/source_port") || strings.HasSuffix(path, "/target_port")
		})
		paths := append(changedTouched, ports...)
		if len(paths) == 0 {
			paths = []string{"/semanticHash"}
		}
		return false, workflowPreviewConflict(key, base.Base, current.Revision, paths, nil)
	}
	invariant, err := workflowPreviewRebaseInvariantHash(current.Graph)
	if err != nil {
		return false, err
	}
	if invariant != artifact.RebaseInvariantHash {
		return false, workflowPreviewConflict(key, base.Base, current.Revision, []string{"/graph"}, nil)
	}
	if current.Revision.LayoutHash != base.Base.LayoutHash {
		if len(changedTouched) > 0 {
			return false, workflowPreviewConflict(key, base.Base, current.Revision, changedTouched, nil)
		}
		layoutDrift := workflowPreviewChangedPaths(current.Graph, artifact.RebaseBase, workflowPreviewLayoutAllowlisted)
		if len(layoutDrift) == 0 {
			return false, workflowPreviewConflict(key, base.Base, current.Revision, []string{"/layoutHash"}, nil)
		}
		return true, nil
	}
	return true, nil
}

func workflowPreviewChangedPaths(graph visual.Graph, base map[string]json.RawMessage, include func(string) bool) []string {
	paths := make([]string, 0)
	for path, expected := range base {
		if !include(path) {
			continue
		}
		actual, found := workflowPreviewPathValue(graph, path)
		if !found {
			actual = json.RawMessage("null")
		}
		if !workflowPreviewJSONEqual(actual, expected) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func workflowPreviewJSONEqual(left, right json.RawMessage) bool {
	var compactLeft, compactRight bytes.Buffer
	if json.Compact(&compactLeft, left) != nil || json.Compact(&compactRight, right) != nil {
		return bytes.Equal(left, right)
	}
	return bytes.Equal(compactLeft.Bytes(), compactRight.Bytes())
}

func workflowPreviewLayoutAllowlisted(path string) bool {
	return path == "/layout/viewport" || strings.HasSuffix(path, "/position")
}

func workflowPreviewRebaseInvariantHash(graph visual.Graph) (string, error) {
	graph, err := cloneVisualGraphForHash(graph)
	if err != nil {
		return "", err
	}
	graph.Layout.Viewport, graph.UI = visual.Viewport{}, withoutGraphResourceVersion(graph.UI)
	// These fields are compiled mirrors of the authoritative visual nodes and
	// edges. A persisted layout edit recompiles them, so retaining either the
	// old mirror or its normalized step metadata makes position-only drift look
	// like an unrelated /graph conflict even though the semantic hash is stable.
	graph.Workflow.XRunnerUI = nil
	graph.Workflow.XRunnerGraph = nil
	graph.Workflow.Steps = nil
	graph.Workflow.Handlers = nil
	for index := range graph.Nodes {
		graph.Nodes[index].Position, graph.Nodes[index].State = visual.Position{}, nil
	}
	for index := range graph.Edges {
		graph.Edges[index].State = nil
	}
	return workflowQueryDigest(graph)
}

func workflowPreviewPathValue(graph visual.Graph, path string) (json.RawMessage, bool) {
	if path == "/layout/viewport" {
		raw, _ := json.Marshal(graph.Layout.Viewport)
		return raw, true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return nil, false
	}
	id := workflowPreviewUnescape(parts[1])
	var value any
	found := false
	if parts[0] == "nodes" {
		for _, node := range graph.Nodes {
			if node.ID == id {
				value, found = node, true
				break
			}
		}
	} else if parts[0] == "edges" {
		for _, edge := range graph.Edges {
			if edge.ID == id {
				value, found = edge, true
				break
			}
		}
	}
	if !found {
		return nil, false
	}
	raw, _ := json.Marshal(value)
	if len(parts) == 2 {
		return raw, true
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	field, found := fields[workflowPreviewUnescape(parts[2])]
	return field, found
}

func workflowPreviewHash(key edit.WorkflowKey, revision uint64, parent string, operations []edit.WorkflowOperation, basis edit.ValidationBasis) (string, error) {
	return workflowQueryDigest([]any{edit.SchemaVersion, key, revision, parent, operations, basis})
}

func workflowPreviewChangeSetID(key edit.WorkflowKey, candidateID string, revision uint64, parent, previewHash string) string {
	hash, _ := workflowQueryDigest([]any{key, candidateID, revision, parent, previewHash})
	return "changeset-" + strings.TrimPrefix(hash, "sha256:")
}

func workflowPreviewConflict(key edit.WorkflowKey, base, current edit.Revision, paths, safe []string) error {
	sort.Strings(paths)
	paths = slices.Compact(paths)
	conflict := edit.WorkflowConflict{Code: edit.ErrorStaleRevision, Base: base, Current: current, ConflictingPaths: paths, SafeRebasePaths: safe}
	details, _ := json.Marshal(conflict)
	keyCopy := key
	return &edit.DomainError{Kind: "error", Code: edit.ErrorStaleRevision, Key: &keyCopy, CurrentRevision: current.ResourceVersion, BaseRevision: base.ResourceVersion, ConflictingPaths: paths, Message: "workflow changed after the candidate base", RetryHint: "review the three-way conflict and retry", Details: details}
}

func workflowPreviewDomainError(code edit.ErrorCode, key edit.WorkflowKey, current, base, message, retry string) error {
	keyCopy := key
	return &edit.DomainError{Kind: "error", Code: code, Key: &keyCopy, CurrentRevision: current, BaseRevision: base, Message: message, RetryHint: retry}
}

func cloneWorkflowOperations(input []edit.WorkflowOperation) []edit.WorkflowOperation {
	return append([]edit.WorkflowOperation(nil), input...)
}

func workflowPreviewEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func workflowPreviewUnescape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}
