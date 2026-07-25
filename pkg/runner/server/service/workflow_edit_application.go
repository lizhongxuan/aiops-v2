package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

var (
	ErrWorkflowEditConfiguration = errors.New("workflow edit application is not configured")
	ErrWorkflowEditSnapshot      = errors.New("invalid workflow edit snapshot")
	ErrWorkflowEditCursor        = errors.New("invalid or stale workflow edit cursor")
	ErrWorkflowEditBudget        = errors.New("workflow context item exceeds response budget")
	ErrWorkflowEditSecret        = errors.New("workflow context contains an inline credential")
)

type WorkflowEditSnapshot struct {
	Key                 edit.WorkflowKey                       `json:"workflowKey"`
	Revision            edit.Revision                          `json:"revision"`
	Graph               visual.Graph                           `json:"graph"`
	RunGateFacts        *WorkflowRunGateProjection             `json:"runGateFacts,omitempty"`
	Receipts            []WorkflowReceiptProjection            `json:"receipts,omitempty"`
	ValidationArtifacts []WorkflowValidationArtifactProjection `json:"validationArtifacts,omitempty"`
	LifecycleArtifacts  []WorkflowLifecycleArtifactProjection  `json:"lifecycleArtifacts,omitempty"`
	DurableHistory      []WorkflowDurableHistoryProjection     `json:"durableHistory,omitempty"`
}
type WorkflowEditReadBackend interface {
	ReadWorkflow(context.Context, edit.WorkflowKey) (WorkflowEditSnapshot, error)
	ListWorkflowActions(context.Context, edit.WorkflowKey, ActionCatalogFilter) ([]ActionSpec, error)
	ValidateWorkflow(context.Context, edit.WorkflowKey, visual.Graph) (*VisualWorkflowValidationResult, error)
	CompileWorkflow(context.Context, edit.WorkflowKey, visual.Graph) (*CompiledVisualWorkflow, error)
}
type WorkflowActionAccess struct {
	Allowed             bool     `json:"allowed"`
	RequiredPermissions []string `json:"requiredPermissions,omitempty"`
	Reason              string   `json:"reason,omitempty"`
}
type WorkflowEditActionAccessResolver interface {
	ResolveWorkflowActionAccess(context.Context, edit.WorkflowKey, ActionSpec) (WorkflowActionAccess, error)
}
type WorkflowEditReferenceValidator interface {
	ValidateWorkflowReferences(context.Context, edit.WorkflowKey, visual.Graph) ([]VisualWorkflowIssue, error)
}
type WorkflowEditApplication struct {
	backend      WorkflowEditReadBackend
	actionAccess WorkflowEditActionAccessResolver
	references   WorkflowEditReferenceValidator
	candidates   WorkflowCandidateStore
	approvals    *WorkflowApprovalFileStore
	commits      *WorkflowCommitStore
	dryRun       WorkflowEditDryRunExecutor
	observer     WorkflowEditObserver
	now          func() time.Time
}

func NewWorkflowEditApplication(backend WorkflowEditReadBackend, access WorkflowEditActionAccessResolver, references WorkflowEditReferenceValidator, candidateStores ...WorkflowCandidateStore) (*WorkflowEditApplication, error) {
	if backend == nil || access == nil || references == nil {
		return nil, ErrWorkflowEditConfiguration
	}
	application := &WorkflowEditApplication{backend: backend, actionAccess: access, references: references, now: time.Now}
	if executor, ok := backend.(WorkflowEditDryRunExecutor); ok {
		application.dryRun = executor
	}
	if len(candidateStores) > 0 {
		application.candidates = candidateStores[0]
	}
	return application, nil
}

type WorkflowEditValidateRequest struct {
	Key              edit.WorkflowKey `json:"workflowKey"`
	Graph            visual.Graph     `json:"graph"`
	ExpectedRevision string           `json:"expectedRevision,omitempty"`
}
type WorkflowEditValidationResult struct {
	Revision   edit.Revision                  `json:"revision"`
	Stale      bool                           `json:"stale"`
	Compiled   bool                           `json:"compiled"`
	Validation VisualWorkflowValidationResult `json:"validation"`
}

func (a *WorkflowEditApplication) Validate(ctx context.Context, request WorkflowEditValidateRequest) (WorkflowEditValidationResult, error) {
	if a == nil || a.backend == nil {
		return WorkflowEditValidationResult{}, ErrWorkflowEditConfiguration
	}
	snapshot, err := a.readSnapshot(ctx, request.Key)
	if err != nil {
		return WorkflowEditValidationResult{}, err
	}
	return a.validateSnapshot(ctx, snapshot, request.Graph, request.ExpectedRevision)
}

func (a *WorkflowEditApplication) validateSnapshot(ctx context.Context, snapshot WorkflowEditSnapshot, graph visual.Graph, expected string) (WorkflowEditValidationResult, error) {
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		return WorkflowEditValidationResult{}, fmt.Errorf("%w: hash graph", ErrWorkflowEditSnapshot)
	}
	result := WorkflowEditValidationResult{
		Revision: edit.Revision{ResourceVersion: snapshot.Revision.ResourceVersion, SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash},
		Stale:    strings.TrimSpace(expected) != "" && expected != snapshot.Revision.ResourceVersion,
	}
	validation, err := a.backend.ValidateWorkflow(ctx, snapshot.Key, graph)
	if err != nil {
		return WorkflowEditValidationResult{}, err
	}
	if validation == nil {
		return WorkflowEditValidationResult{}, fmt.Errorf("%w: validator returned no result", ErrWorkflowEditConfiguration)
	}
	result.Validation = VisualWorkflowValidationResult{
		Valid: validation.Valid, Errors: append([]VisualWorkflowIssue(nil), validation.Errors...),
		Warnings: append([]VisualWorkflowIssue(nil), validation.Warnings...), Summary: validation.Summary,
	}
	if validation.Valid {
		compiled, compileErr := a.backend.CompileWorkflow(ctx, snapshot.Key, graph)
		if compileErr != nil || compiled == nil {
			result.Validation.Errors = append(result.Validation.Errors, workflowEditIssue("compile", "COMPILE_FAILED", "workflow compile failed"))
		} else {
			result.Compiled = true
			result.Validation.Warnings = append(result.Validation.Warnings, compiled.Warnings...)
		}
	}
	referenceIssues, err := a.references.ValidateWorkflowReferences(ctx, snapshot.Key, graph)
	if err != nil {
		return WorkflowEditValidationResult{}, err
	}
	for _, issue := range referenceIssues {
		if strings.EqualFold(issue.Severity, "warning") {
			result.Validation.Warnings = append(result.Validation.Warnings, issue)
		} else {
			result.Validation.Errors = append(result.Validation.Errors, issue)
		}
	}
	if result.Stale {
		result.Validation.Errors = append(result.Validation.Errors, workflowEditIssue("revision", "STALE_REVISION", "workflow revision changed; refresh context"))
	}
	result.Validation.Errors = dedupeIssues(result.Validation.Errors)
	result.Validation.Warnings = dedupeIssues(result.Validation.Warnings)
	result.Validation.Valid = result.Compiled && len(result.Validation.Errors) == 0
	result.Validation.Summary = buildVisualValidationSummary(result.Validation)
	return result, nil
}

func (a *WorkflowEditApplication) readSnapshot(ctx context.Context, key edit.WorkflowKey) (WorkflowEditSnapshot, error) {
	if err := key.Validate(); err != nil {
		return WorkflowEditSnapshot{}, fmt.Errorf("%w: invalid workflow key", ErrWorkflowEditSnapshot)
	}
	snapshot, err := a.backend.ReadWorkflow(ctx, key)
	if err != nil {
		return WorkflowEditSnapshot{}, err
	}
	if snapshot.Key != key || strings.TrimSpace(snapshot.Revision.ResourceVersion) == "" {
		return WorkflowEditSnapshot{}, ErrWorkflowEditSnapshot
	}
	hashes, err := VisualWorkflowGraphHashes(snapshot.Graph)
	if err != nil || hashes.SemanticHash != snapshot.Revision.SemanticHash || hashes.LayoutHash != snapshot.Revision.LayoutHash {
		return WorkflowEditSnapshot{}, ErrWorkflowEditSnapshot
	}
	return snapshot, nil
}

func workflowEditIssue(issueType, code, message string) VisualWorkflowIssue {
	return VisualWorkflowIssue{Severity: "error", Type: issueType, Code: code, Message: message}
}
