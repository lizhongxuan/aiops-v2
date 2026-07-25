package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
)

type WorkflowManualCommandKind string

const (
	workflowManualCommandBatchLimit = 64

	WorkflowManualAddNode        WorkflowManualCommandKind = "add_node"
	WorkflowManualUpdateNode     WorkflowManualCommandKind = "update_node"
	WorkflowManualDeleteNode     WorkflowManualCommandKind = "delete_node"
	WorkflowManualConnectEdge    WorkflowManualCommandKind = "connect_edge"
	WorkflowManualUpdateEdge     WorkflowManualCommandKind = "update_edge"
	WorkflowManualDisconnectEdge WorkflowManualCommandKind = "disconnect_edge"
	WorkflowManualMove           WorkflowManualCommandKind = "move"
)

// WorkflowManualCommand is the server-owned translation boundary for Canvas
// actions. It deliberately excludes arbitrary maps and raw Graph replacement.
type WorkflowManualCommand struct {
	Kind         WorkflowManualCommandKind `json:"kind"`
	Node         *edit.EditableNode        `json:"node,omitempty"`
	NodeID       string                    `json:"nodeId,omitempty"`
	NodePatch    *edit.NodePatch           `json:"nodePatch,omitempty"`
	Edge         *edit.EditableEdge        `json:"edge,omitempty"`
	EdgeID       string                    `json:"edgeId,omitempty"`
	EdgePatch    *edit.EdgePatch           `json:"edgePatch,omitempty"`
	CascadeEdges bool                      `json:"cascadeEdges,omitempty"`
	Layout       *edit.LayoutPatch         `json:"layout,omitempty"`
}

type WorkflowManualCommandRequest struct {
	SchemaVersion    string                  `json:"schemaVersion"`
	WorkflowKey      edit.WorkflowKey        `json:"workflowKey"`
	ExpectedRevision string                  `json:"expectedRevision"`
	IdempotencyKey   string                  `json:"idempotencyKey"`
	ActorID          string                  `json:"actorId"`
	CanvasSessionID  string                  `json:"canvasSessionId"`
	SessionID        string                  `json:"sessionId"`
	Reason           string                  `json:"reason"`
	Confirmed        bool                    `json:"confirmed,omitempty"`
	Command          WorkflowManualCommand   `json:"command,omitempty"`
	Commands         []WorkflowManualCommand `json:"commands,omitempty"`
}

type WorkflowManualCommandResult struct {
	Preview WorkflowEditPreviewResult    `json:"preview"`
	Receipt edit.WorkflowMutationReceipt `json:"receipt"`
	Event   *edit.WorkflowDomainEvent    `json:"event,omitempty"`
}

type workflowEditBaselineEnsurer interface {
	EnsureWorkflowEditBaseline(context.Context, edit.WorkflowKey, string, string) (edit.Revision, bool, error)
}

// ExecuteManualCommand sends authenticated Canvas edits through the exact
// Preview -> persisted approval -> Apply pipeline used by agent mutations.
// Actor/session/key values must already have been projected from trusted HTTP
// context by the caller; browser request fields are not an authority source.
func (a *WorkflowEditApplication) ExecuteManualCommand(ctx context.Context, request WorkflowManualCommandRequest) (WorkflowManualCommandResult, error) {
	if err := validateWorkflowManualRequest(request); err != nil {
		return WorkflowManualCommandResult{}, err
	}
	if ensurer, ok := a.backend.(workflowEditBaselineEnsurer); ok {
		revision, compatibleMigration, err := ensurer.EnsureWorkflowEditBaseline(ctx, request.WorkflowKey, request.ExpectedRevision, request.IdempotencyKey)
		if err != nil {
			return WorkflowManualCommandResult{}, err
		}
		if compatibleMigration {
			request.ExpectedRevision = revision.ResourceVersion
		}
	}
	commands, err := workflowManualRequestCommands(request)
	if err != nil {
		return WorkflowManualCommandResult{}, err
	}
	operations := make([]edit.WorkflowOperation, 0, len(commands))
	for _, command := range commands {
		operation, operationErr := workflowManualOperation(command)
		if operationErr != nil {
			return WorkflowManualCommandResult{}, operationErr
		}
		operations = append(operations, operation)
	}
	return a.executeWorkflowCommandBatch(ctx, workflowCommandBatchRequest{
		SchemaVersion: request.SchemaVersion, WorkflowKey: request.WorkflowKey, ExpectedRevision: request.ExpectedRevision,
		IdempotencyKey: request.IdempotencyKey, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID,
		SessionID: request.SessionID, Reason: request.Reason, Confirmed: request.Confirmed,
		Intent: workflowManualRequestIntent(commands), Operations: operations,
	})
}

func workflowManualRequestCommands(request WorkflowManualCommandRequest) ([]WorkflowManualCommand, error) {
	single := workflowManualCommandProvided(request.Command)
	batch := len(request.Commands) > 0
	if single == batch || len(request.Commands) > workflowManualCommandBatchLimit {
		return nil, fmt.Errorf("%w: provide exactly one manual command or one bounded command batch", ErrInvalid)
	}
	if single {
		return []WorkflowManualCommand{request.Command}, nil
	}
	return append([]WorkflowManualCommand(nil), request.Commands...), nil
}

func workflowManualCommandProvided(command WorkflowManualCommand) bool {
	return command.Kind != "" || command.Node != nil || command.NodeID != "" || command.NodePatch != nil ||
		command.Edge != nil || command.EdgeID != "" || command.EdgePatch != nil || command.CascadeEdges || command.Layout != nil
}

func workflowManualRequestIntent(commands []WorkflowManualCommand) string {
	if len(commands) == 1 {
		return "manual canvas command: " + string(commands[0].Kind)
	}
	return "manual canvas command batch"
}

type workflowCommandBatchRequest struct {
	SchemaVersion    string
	WorkflowKey      edit.WorkflowKey
	ExpectedRevision string
	IdempotencyKey   string
	ActorID          string
	CanvasSessionID  string
	SessionID        string
	Reason           string
	Confirmed        bool
	Intent           string
	Operations       []edit.WorkflowOperation
}

func (a *WorkflowEditApplication) executeWorkflowCommandBatch(ctx context.Context, request workflowCommandBatchRequest) (WorkflowManualCommandResult, error) {
	if a == nil || a.candidates == nil || a.approvals == nil || a.commits == nil || a.now == nil {
		return WorkflowManualCommandResult{}, ErrWorkflowEditConfiguration
	}
	if err := validateWorkflowCommandBatch(request); err != nil {
		return WorkflowManualCommandResult{}, err
	}
	for _, operation := range request.Operations {
		if workflowManualRequiresConfirmation(operation) && !request.Confirmed {
			return WorkflowManualCommandResult{}, workflowApprovalError(edit.ErrorApprovalRequired, request.WorkflowKey, "this manual workflow change requires explicit confirmation", "confirm the current delete or high-risk change")
		}
	}

	candidateID, err := workflowCommandBatchCandidateID(request)
	if err != nil {
		return WorkflowManualCommandResult{}, err
	}
	preview, err := a.resolveOrCreateManualPreview(ctx, request, candidateID)
	if err != nil {
		return WorkflowManualCommandResult{}, err
	}
	for renewalAttempt := 0; renewalAttempt < 16; renewalAttempt++ {
		if preview.Risk.RequiresApproval && !request.Confirmed {
			return WorkflowManualCommandResult{}, workflowApprovalError(edit.ErrorApprovalRequired, request.WorkflowKey, "this manual workflow change requires explicit confirmation", "confirm the current high-risk change")
		}
		if !preview.ChangeSet.Validation.Valid {
			return WorkflowManualCommandResult{}, workflowPreviewDomainError(edit.ErrorPublishGateFailed, request.WorkflowKey, request.ExpectedRevision, request.ExpectedRevision, "manual workflow change failed validation", "repair the command and retry")
		}

		planHash, hashErr := workflowQueryDigest([]any{edit.SchemaVersion, "manual-command", request.WorkflowKey, request.ActorID, request.CanvasSessionID, request.SessionID, request.ExpectedRevision, request.Operations})
		if hashErr != nil {
			return WorkflowManualCommandResult{}, hashErr
		}
		candidate, resolveCandidateErr := a.candidates.ResolveChangeSet(ctx, request.WorkflowKey, preview.ChangeSet.ID)
		if resolveCandidateErr != nil {
			return WorkflowManualCommandResult{}, resolveCandidateErr
		}
		token, tokenErr := workflowManualCandidateToken(preview.Candidate.ID)
		if tokenErr != nil {
			return WorkflowManualCommandResult{}, tokenErr
		}
		now := a.now().UTC()
		grantID, approvalID := "manual-grant-"+token, "manual-approval-"+token
		selector := WorkflowTrustedApplySelector{Key: request.WorkflowKey, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID, ChangeSetID: preview.ChangeSet.ID, ApprovalID: approvalID}
		if trusted, resolveErr := a.ResolveTrustedApplyContext(ctx, selector); resolveErr == nil {
			result, applyErr := a.applyWorkflowCommandPreview(ctx, request, preview, trusted)
			if !workflowManualCanRenewExpiredPreview(preview, applyErr, a.now().UTC()) {
				return result, applyErr
			}
		} else if workflowManualCanRenewExpiredPreview(preview, resolveErr, now) {
			// Continue below with a fresh immutable Candidate identity. The old
			// preview and any partial approval facts remain auditable and cannot
			// authorize the replacement Candidate.
		} else if !errors.Is(resolveErr, ErrNotFound) {
			return WorkflowManualCommandResult{}, resolveErr
		} else {
			allowed := workflowManualAllowedOperations(candidate.NormalizedOperations)
			grant := WorkflowEditGrantRecord{Grant: edit.WorkflowEditGrant{
				ID: grantID, Key: request.WorkflowKey, ActorID: request.ActorID, PlanHash: planHash,
				BaseSemanticHash: preview.ChangeSet.Base.SemanticHash, AllowedOps: allowed,
				MaxOperations: len(candidate.NormalizedOperations), MaxAddedNodes: preview.Risk.AddedNodeCount,
				AllowDeleteExisting: preview.Risk.DeletesExisting, ExpiresAt: now.Add(30 * time.Minute),
			}, CanvasSessionID: request.CanvasSessionID, RiskScope: WorkflowGrantRiskScope{MaxRisk: preview.Risk.Level}, State: WorkflowApprovalActive, IssuedAt: now}
			if issueErr := a.approvals.IssueGrant(ctx, grant); issueErr != nil && !errors.Is(issueErr, ErrAlreadyExists) {
				return WorkflowManualCommandResult{}, issueErr
			}
			approval := WorkflowApplyApprovalRecord{Approval: edit.WorkflowApplyApproval{
				ID: approvalID, ActorID: request.ActorID, Key: request.WorkflowKey, CandidateID: preview.Candidate.ID,
				ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash,
				ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, ExpiresAt: now.Add(30 * time.Minute),
			}, GrantID: grantID, CanvasSessionID: request.CanvasSessionID, PlanHash: planHash, State: WorkflowApprovalActive, IssuedAt: now}
			if issueErr := a.approvals.IssueApplyApproval(ctx, approval); issueErr != nil && !errors.Is(issueErr, ErrAlreadyExists) {
				return WorkflowManualCommandResult{}, issueErr
			}

			trusted, resolveErr := a.ResolveTrustedApplyContext(ctx, selector)
			if resolveErr == nil {
				result, applyErr := a.applyWorkflowCommandPreview(ctx, request, preview, trusted)
				if !workflowManualCanRenewExpiredPreview(preview, applyErr, a.now().UTC()) {
					return result, applyErr
				}
			} else if !workflowManualCanRenewExpiredPreview(preview, resolveErr, a.now().UTC()) {
				return WorkflowManualCommandResult{}, resolveErr
			}
		}

		candidateID, err = workflowManualRenewedCandidateID(preview)
		if err != nil {
			return WorkflowManualCommandResult{}, err
		}
		preview, err = a.resolveOrCreateManualPreview(ctx, request, candidateID)
		if err != nil {
			return WorkflowManualCommandResult{}, err
		}
	}
	return WorkflowManualCommandResult{}, fmt.Errorf("%w: too many expired manual workflow candidates", ErrConflict)
}

func workflowManualCandidateToken(candidateID string) (string, error) {
	token := strings.TrimPrefix(candidateID, "manual-")
	if token == candidateID || len(token) < 32 {
		return "", fmt.Errorf("%w: invalid manual workflow candidate identity", ErrInvalid)
	}
	return token[:32], nil
}

func workflowManualCanRenewExpiredPreview(preview WorkflowEditPreviewResult, err error, at time.Time) bool {
	if err == nil || at.Before(preview.ChangeSet.ExpiresAt.UTC()) {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var domain *edit.DomainError
	return errors.As(err, &domain) && domain.Code == edit.ErrorStaleChangeSet && domain.Message == "candidate has expired"
}

func workflowManualRenewedCandidateID(expired WorkflowEditPreviewResult) (string, error) {
	digest, err := workflowQueryDigest([]any{
		edit.SchemaVersion, "manual-candidate-renewal", expired.Candidate.ID,
		expired.CandidateRevision, expired.ChangeSet.ID, expired.ChangeSet.ExpiresAt.UTC(),
	})
	if err != nil {
		return "", err
	}
	token := strings.TrimPrefix(digest, "sha256:")
	if len(token) < 32 {
		return "", fmt.Errorf("%w: invalid manual workflow renewal digest", ErrInvalid)
	}
	return "manual-" + token[:32], nil
}

func (a *WorkflowEditApplication) applyWorkflowCommandPreview(ctx context.Context, request workflowCommandBatchRequest, preview WorkflowEditPreviewResult, trusted WorkflowEditTrustedApplyContext) (WorkflowManualCommandResult, error) {
	receipt, err := a.Apply(ctx, trusted, WorkflowEditApplyRequest{ApplyPatchRequest: edit.ApplyPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: request.WorkflowKey, ChangeSetID: preview.ChangeSet.ID,
		PreviewHash: preview.PreviewHash, ExpectedRevision: request.ExpectedRevision, IdempotencyKey: request.IdempotencyKey,
		CandidateGrantID: trusted.GrantID, ApplyApprovalID: trusted.ApprovalID, Reason: "manual canvas command: " + request.Reason,
	}, CandidateID: preview.Candidate.ID, CandidateRevision: preview.CandidateRevision, ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID})
	if err != nil {
		return WorkflowManualCommandResult{}, err
	}
	return WorkflowManualCommandResult{Preview: preview, Receipt: receipt, Event: a.manualCommandEvent(ctx, receipt.TransactionID, request.WorkflowKey)}, nil
}

func (a *WorkflowEditApplication) resolveOrCreateManualPreview(ctx context.Context, request workflowCommandBatchRequest, candidateID string) (WorkflowEditPreviewResult, error) {
	if current, err := a.candidates.GetCurrent(ctx, request.WorkflowKey, candidateID); err == nil {
		revision, readErr := a.candidates.GetRevision(ctx, request.WorkflowKey, candidateID, current.CurrentRevision)
		if readErr != nil {
			return WorkflowEditPreviewResult{}, readErr
		}
		want, _ := workflowQueryDigest(request.Operations)
		got, _ := workflowQueryDigest(revision.Operations)
		if want != got || revision.Actor != edit.ActorKind("user") || revision.ActorID != request.ActorID || revision.CanvasSessionID != request.CanvasSessionID || revision.AgentSessionID != request.SessionID || revision.Base.ResourceVersion != request.ExpectedRevision {
			return WorkflowEditPreviewResult{}, workflowCommitDomainError(edit.ErrorIdempotencyKeyReused, request.WorkflowKey, revision.Base.ResourceVersion, request.ExpectedRevision, "manual command idempotency key was already used for another request", "reuse the original command or choose a new idempotency key")
		}
		var risk WorkflowEditPreviewRisk
		var diff WorkflowEditPreviewDiff
		if json.Unmarshal(revision.Risk, &risk) != nil || json.Unmarshal(revision.Diff, &diff) != nil {
			return WorkflowEditPreviewResult{}, ErrWorkflowEditSnapshot
		}
		return WorkflowEditPreviewResult{Kind: "preview", SchemaVersion: edit.SchemaVersion, ChangeSet: revision.WorkflowChangeSet, Candidate: current, CandidateRevision: revision.CandidateRevision, PreviewHash: revision.PreviewHash, ValidationBasis: revision.Validation.Basis, Diagnostics: append([]edit.WorkflowDiagnostic(nil), revision.Validation.Diagnostics...), Risk: risk, Diff: diff}, nil
	} else if !errors.Is(err, ErrNotFound) {
		return WorkflowEditPreviewResult{}, err
	}
	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return WorkflowEditPreviewResult{}, err
	}
	if snapshot.Revision.ResourceVersion != request.ExpectedRevision {
		return WorkflowEditPreviewResult{}, workflowPreviewConflict(request.WorkflowKey, edit.Revision{ResourceVersion: request.ExpectedRevision}, snapshot.Revision, []string{"/resourceVersion"}, nil)
	}
	preview, err := a.Preview(ctx, WorkflowEditPreviewRequest{PreviewPatchRequest: edit.PreviewPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: request.WorkflowKey, Expected: snapshot.Revision,
		Intent: request.Intent, Operations: request.Operations,
	}, CandidateID: candidateID, Actor: edit.ActorKind("user"), ActorID: request.ActorID, CanvasSessionID: request.CanvasSessionID, AgentSessionID: request.SessionID})
	if errors.Is(err, ErrAlreadyExists) {
		return a.resolveOrCreateManualPreview(ctx, request, candidateID)
	}
	return preview, err
}

func validateWorkflowCommandBatch(request workflowCommandBatchRequest) error {
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || !validWorkflowApprovalValue(request.ExpectedRevision, false) || !validWorkflowApprovalValue(request.IdempotencyKey, true) || !validWorkflowApprovalValue(request.ActorID, true) || !validWorkflowApprovalValue(request.CanvasSessionID, true) || !validWorkflowApprovalValue(request.SessionID, true) || !validWorkflowApprovalValue(request.Reason, false) || strings.TrimSpace(request.Intent) == "" || len(request.Operations) == 0 {
		return fmt.Errorf("%w: invalid trusted workflow command batch", ErrInvalid)
	}
	for _, operation := range request.Operations {
		if err := operation.Validate(); err != nil {
			return fmt.Errorf("%w: invalid workflow command operation: %v", ErrInvalid, err)
		}
	}
	redacted, err := redactWorkflowAuditDetails(map[string]any{"reason": request.Reason})
	if err != nil || redacted["reason"] != request.Reason {
		return ErrWorkflowEditSecret
	}
	return nil
}

func validateWorkflowManualRequest(request WorkflowManualCommandRequest) error {
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || !validWorkflowApprovalValue(request.ExpectedRevision, false) || !validWorkflowApprovalValue(request.IdempotencyKey, true) || !validWorkflowApprovalValue(request.ActorID, true) || !validWorkflowApprovalValue(request.CanvasSessionID, true) || !validWorkflowApprovalValue(request.SessionID, true) || !validWorkflowApprovalValue(request.Reason, false) {
		return fmt.Errorf("%w: invalid trusted manual workflow command identity", ErrInvalid)
	}
	if _, err := workflowManualRequestCommands(request); err != nil {
		return err
	}
	redacted, err := redactWorkflowAuditDetails(map[string]any{"reason": request.Reason})
	if err != nil || redacted["reason"] != request.Reason {
		return ErrWorkflowEditSecret
	}
	return nil
}

func workflowManualOperation(command WorkflowManualCommand) (edit.WorkflowOperation, error) {
	empty := func() bool {
		return command.Node == nil && command.NodeID == "" && command.NodePatch == nil && command.Edge == nil && command.EdgeID == "" && command.EdgePatch == nil && !command.CascadeEdges && command.Layout == nil
	}
	var operation edit.WorkflowOperation
	switch command.Kind {
	case WorkflowManualAddNode:
		if command.Node == nil || command.NodeID != "" || command.NodePatch != nil || command.Edge != nil || command.EdgeID != "" || command.EdgePatch != nil || command.CascadeEdges || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid add-node command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.AddNodeOperation{Node: normalizeWorkflowManualAddedNode(*command.Node)})
	case WorkflowManualUpdateNode:
		if command.NodeID == "" || command.NodePatch == nil || command.Node != nil || command.Edge != nil || command.EdgeID != "" || command.EdgePatch != nil || command.CascadeEdges || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid update-node command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.UpdateNodeOperation{NodeID: command.NodeID, Set: *command.NodePatch})
	case WorkflowManualDeleteNode:
		if command.NodeID == "" || command.Node != nil || command.NodePatch != nil || command.Edge != nil || command.EdgeID != "" || command.EdgePatch != nil || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid delete-node command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.DeleteNodeOperation{NodeID: command.NodeID, CascadeEdges: command.CascadeEdges})
	case WorkflowManualConnectEdge:
		if command.Edge == nil || command.Node != nil || command.NodeID != "" || command.NodePatch != nil || command.EdgeID != "" || command.EdgePatch != nil || command.CascadeEdges || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid connect-edge command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.AddEdgeOperation{Edge: *command.Edge})
	case WorkflowManualUpdateEdge:
		if command.EdgeID == "" || command.EdgePatch == nil || command.Node != nil || command.NodeID != "" || command.NodePatch != nil || command.Edge != nil || command.CascadeEdges || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid update-edge command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.UpdateEdgeOperation{EdgeID: command.EdgeID, Set: *command.EdgePatch})
	case WorkflowManualDisconnectEdge:
		if command.EdgeID == "" || command.Node != nil || command.NodeID != "" || command.NodePatch != nil || command.Edge != nil || command.EdgePatch != nil || command.CascadeEdges || command.Layout != nil {
			return operation, fmt.Errorf("%w: invalid disconnect-edge command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.DeleteEdgeOperation{EdgeID: command.EdgeID})
	case WorkflowManualMove:
		if command.Layout == nil || command.Node != nil || command.NodeID != "" || command.NodePatch != nil || command.Edge != nil || command.EdgeID != "" || command.EdgePatch != nil || command.CascadeEdges {
			return operation, fmt.Errorf("%w: invalid move command", ErrInvalid)
		}
		operation = edit.NewOperation(&edit.UpdateLayoutOperation{Layout: *command.Layout})
	default:
		if empty() {
			return operation, fmt.Errorf("%w: unsupported manual workflow command", ErrInvalid)
		}
		return operation, fmt.Errorf("%w: unsupported manual workflow command", ErrInvalid)
	}
	if err := operation.Validate(); err != nil {
		return edit.WorkflowOperation{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return operation, nil
}

// normalizeWorkflowManualAddedNode makes browser-shaped JSON stable across
// the compile -> YAML -> parse boundary. workflow.Step serializes slice/map
// fields even when empty, so an omitted JSON field (nil) otherwise becomes an
// empty collection after compile and falsely trips the compiled hash gate.
func normalizeWorkflowManualAddedNode(node edit.EditableNode) edit.EditableNode {
	if node.Step != nil {
		step := *node.Step
		if step.Targets == nil {
			step.Targets = []string{}
		}
		if step.Args == nil {
			step.Args = map[string]any{}
		}
		if step.MustVars == nil {
			step.MustVars = []string{}
		}
		if step.Loop == nil {
			step.Loop = []any{}
		}
		if step.ExpectVars == nil {
			step.ExpectVars = []string{}
		}
		if step.Notify == nil {
			step.Notify = []string{}
		}
		stepName := strings.TrimSpace(step.Name)
		if node.StepName == nil || strings.TrimSpace(*node.StepName) == "" {
			node.StepName = &stepName
		}
		stepID := strings.TrimSpace(step.ID)
		if stepID == "" && node.StepID != nil {
			stepID = strings.TrimSpace(*node.StepID)
		}
		if stepID == "" {
			stepID = strings.TrimSpace(node.ID)
		}
		step.ID = stepID
		if node.StepID == nil || strings.TrimSpace(*node.StepID) == "" {
			node.StepID = &stepID
		}
		node.Step = &step
	}
	if node.Handler != nil && node.Handler.Args == nil {
		handler := *node.Handler
		handler.Args = map[string]any{}
		node.Handler = &handler
	}
	if node.Handler != nil && (node.HandlerName == nil || strings.TrimSpace(*node.HandlerName) == "") {
		handlerName := strings.TrimSpace(node.Handler.Name)
		node.HandlerName = &handlerName
	}
	if node.Ports != nil && len(*node.Ports) == 0 {
		node.Ports = nil
	}
	if node.Inputs != nil && len(*node.Inputs) == 0 {
		node.Inputs = nil
	}
	if node.Outputs != nil && len(*node.Outputs) == 0 {
		node.Outputs = nil
	}
	return node
}

func workflowManualRequiresConfirmation(operation edit.WorkflowOperation) bool {
	switch value := operation.Value.(type) {
	case *edit.DeleteNodeOperation, *edit.DeleteEdgeOperation:
		return true
	case *edit.UpdateNodeOperation:
		return value.Set.Ports != nil
	default:
		return false
	}
}

func workflowManualAllowedOperations(operations []edit.WorkflowOperation) []edit.OperationKind {
	seen := make(map[edit.OperationKind]struct{}, len(operations))
	result := make([]edit.OperationKind, 0, len(operations))
	for _, operation := range operations {
		kind := operation.Value.Kind()
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		result = append(result, kind)
	}
	return result
}

func (a *WorkflowEditApplication) manualCommandEvent(ctx context.Context, transactionID string, key edit.WorkflowKey) *edit.WorkflowDomainEvent {
	log, err := NewFileWorkflowEventLog(a.approvals.Root)
	if err != nil {
		return nil
	}
	replay, err := log.Replay(ctx, key, 0, 1000)
	if err != nil {
		return nil
	}
	for index := range replay.Events {
		if replay.Events[index].TransactionID == transactionID {
			return &replay.Events[index]
		}
	}
	return nil
}
