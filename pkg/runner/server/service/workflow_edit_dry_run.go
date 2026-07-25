package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

const (
	WorkflowDryRunModeDraftNonProduction = "draft_non_production"
	WorkflowDryRunStatusPassed           = "dry_run_passed"
)

// WorkflowEditDryRunExecutor is the narrow environment boundary. The
// application supplies the server-owned Graph; executor implementations return
// structured diagnostics and configuration identifiers, never raw output.
type WorkflowEditDryRunExecutor interface {
	DryRunWorkflow(context.Context, edit.WorkflowKey, visual.Graph, WorkflowEditDryRunExecutionRequest) (WorkflowEditDryRunExecutionResult, error)
}

type WorkflowEditDryRunExecutionRequest struct {
	Mode        string `json:"mode"`
	Environment string `json:"environment,omitempty"`
}

type WorkflowEditDryRunExecutionResult struct {
	Outcome       WorkflowPostCheckOutcome          `json:"outcome"`
	Diagnostics   []edit.WorkflowDiagnostic         `json:"diagnostics,omitempty"`
	Configuration WorkflowConfigurationRequirements `json:"configuration,omitempty"`
}

type WorkflowEditDryRunRequest struct {
	Key           edit.WorkflowKey     `json:"workflowKey"`
	TransactionID string               `json:"transactionId,omitempty"`
	ExpectedBasis edit.ValidationBasis `json:"expectedBasis"`
	Environment   string               `json:"environment,omitempty"`
}

type WorkflowEditDryRunResult struct {
	Kind          string                            `json:"kind"`
	SchemaVersion string                            `json:"schemaVersion"`
	Mode          string                            `json:"mode"`
	Basis         edit.ValidationBasis              `json:"basis"`
	Status        string                            `json:"status"`
	Valid         bool                              `json:"valid"`
	Diagnostics   []edit.WorkflowDiagnostic         `json:"diagnostics"`
	Configuration WorkflowConfigurationRequirements `json:"configuration,omitempty"`
	CheckedAt     time.Time                         `json:"checkedAt"`
	Receipt       *edit.WorkflowMutationReceipt     `json:"receipt,omitempty"`
}

// SetWorkflowDryRunExecutor is a composition-time hook. Runtime code must set
// it before serving requests; DryRun fails closed when no executor is wired.
func (a *WorkflowEditApplication) SetWorkflowDryRunExecutor(executor WorkflowEditDryRunExecutor) {
	if a != nil {
		a.dryRun = executor
	}
}

// DryRunCurrent persists a draft-only post-check against the transaction that
// currently owns the committed revision. The browser does not choose or forge
// a transaction id; the server resolves it from CommitStore.
func (a *WorkflowEditApplication) DryRunCurrent(ctx context.Context, key edit.WorkflowKey, expectedBasis edit.ValidationBasis, environment string) (WorkflowEditDryRunResult, error) {
	if a == nil || a.commits == nil {
		return WorkflowEditDryRunResult{}, ErrWorkflowEditConfiguration
	}
	current, err := a.commits.Snapshot(ctx, key)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	if current.ValidationBasis != expectedBasis || current.Receipt.TransactionID != current.TransactionID {
		return WorkflowEditDryRunResult{}, workflowDryRunStaleBasisError(key, current.ValidationBasis, expectedBasis)
	}
	return a.DryRun(ctx, WorkflowEditDryRunRequest{
		Key: key, TransactionID: current.TransactionID, ExpectedBasis: expectedBasis, Environment: environment,
	})
}

// DryRun performs a non-production draft execution against a server-owned
// snapshot and binds the result to every ValidationBasis dimension. It checks
// the basis again after execution to close the read/execute race.
func (a *WorkflowEditApplication) DryRun(ctx context.Context, request WorkflowEditDryRunRequest) (WorkflowEditDryRunResult, error) {
	if a == nil || a.backend == nil || a.references == nil || a.dryRun == nil || a.now == nil {
		return WorkflowEditDryRunResult{}, ErrWorkflowEditConfiguration
	}
	if request.Key.Validate() != nil || request.ExpectedBasis.Validate() != nil || (request.TransactionID != "" && !validWorkflowApprovalValue(request.TransactionID, true)) || (request.Environment != "" && !validWorkflowConfigurationName(request.Environment)) {
		return WorkflowEditDryRunResult{}, fmt.Errorf("%w: invalid workflow dry-run request", ErrInvalid)
	}
	snapshot, err := a.workflowDryRunSnapshot(ctx, request.Key)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	boundEnvironment := strings.TrimSpace(snapshot.Graph.Workflow.ValidationEnv)
	if request.Environment != "" && request.Environment != boundEnvironment {
		return WorkflowEditDryRunResult{}, workflowPreviewDomainError(edit.ErrorValidationBasisStale, request.Key, snapshot.Revision.ResourceVersion, request.ExpectedBasis.ResourceVersion, "dry-run environment is not bound to the current workflow basis", "reload the current environment binding")
	}
	validated, err := a.validateSnapshot(ctx, snapshot, snapshot.Graph, snapshot.Revision.ResourceVersion)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	if !validated.Validation.Valid || !validated.Compiled {
		return WorkflowEditDryRunResult{}, workflowPreviewDomainError(edit.ErrorPublishGateFailed, request.Key, snapshot.Revision.ResourceVersion, request.ExpectedBasis.ResourceVersion, "workflow failed validation before dry-run", "repair and validate the current draft")
	}
	basis, err := a.workflowPreviewValidationBasis(ctx, snapshot, snapshot.Graph)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	if basis != request.ExpectedBasis {
		return WorkflowEditDryRunResult{}, workflowDryRunStaleBasisError(request.Key, basis, request.ExpectedBasis)
	}

	execution, err := a.dryRun.DryRunWorkflow(ctx, request.Key, snapshot.Graph, WorkflowEditDryRunExecutionRequest{Mode: WorkflowDryRunModeDraftNonProduction, Environment: boundEnvironment})
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	facts, err := normalizeWorkflowPostCheckFacts(WorkflowPostCheckFacts{
		Key: request.Key, TransactionID: request.TransactionID, Basis: basis, Outcome: execution.Outcome,
		Diagnostics: execution.Diagnostics, Configuration: execution.Configuration, Mode: WorkflowDryRunModeDraftNonProduction,
	})
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	if err := validateWorkflowDryRunOutcome(facts); err != nil {
		return WorkflowEditDryRunResult{}, err
	}

	latest, err := a.workflowDryRunSnapshot(ctx, request.Key)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	latestBasis, err := a.workflowPreviewValidationBasis(ctx, latest, latest.Graph)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	if latestBasis != basis {
		return WorkflowEditDryRunResult{}, workflowDryRunStaleBasisError(request.Key, latestBasis, basis)
	}

	status, valid := workflowPostCheckStatus(facts.Outcome, facts.Diagnostics)
	if status == "completed" || status == "completed_with_warnings" {
		status = WorkflowDryRunStatusPassed
	}
	result := WorkflowEditDryRunResult{
		Kind: "dry_run", SchemaVersion: edit.SchemaVersion, Mode: WorkflowDryRunModeDraftNonProduction,
		Basis: basis, Status: status, Valid: valid, Diagnostics: append([]edit.WorkflowDiagnostic(nil), facts.Diagnostics...),
		Configuration: facts.Configuration, CheckedAt: a.now().UTC(),
	}
	if request.TransactionID == "" {
		return result, nil
	}
	if a.commits == nil {
		return WorkflowEditDryRunResult{}, ErrWorkflowEditConfiguration
	}
	receipt, err := a.RecordPostCheck(ctx, facts)
	if err != nil {
		return WorkflowEditDryRunResult{}, err
	}
	result.Status, result.Valid, result.Receipt = receipt.Status, receipt.Validation.Valid, &receipt
	return result, nil
}

// ReissueLayoutOnlyLifecycleGate derives a fresh server artifact for the new
// resource version. It never accepts an artifact from the caller and refuses
// reissue unless history proves an allowlisted layout-only transaction and all
// other basis fields still match.
func (a *WorkflowEditApplication) ReissueLayoutOnlyLifecycleGate(ctx context.Context, key edit.WorkflowKey, transactionID string) (edit.WorkflowMutationReceipt, error) {
	if a == nil || a.commits == nil || a.backend == nil || a.references == nil || a.now == nil {
		return edit.WorkflowMutationReceipt{}, ErrWorkflowEditConfiguration
	}
	if key.Validate() != nil || !validWorkflowApprovalValue(transactionID, true) {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: invalid layout-only lifecycle request", ErrInvalid)
	}
	current, err := a.commits.Snapshot(ctx, key)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if current.TransactionID != transactionID || current.Receipt.TransactionID != transactionID || current.Receipt.Status != "verifying" || current.Receipt.RevisionAfter == nil || current.Receipt.RevisionAfter.ResourceVersion != current.ResourceVersion || current.Receipt.RevisionBefore.ResourceVersion == "" || current.Receipt.RevisionBefore.SemanticHash != current.Receipt.RevisionAfter.SemanticHash {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorValidationBasisStale, key, current.ResourceVersion, current.Receipt.RevisionBefore.ResourceVersion, "workflow transaction is not a provable layout-only gate reissue", "repeat validation and dry-run for the current draft")
	}
	var history struct {
		TouchedPaths []string `json:"touchedPaths"`
	}
	if current.History.Kind != "history" || current.History.SchemaVersion != edit.SchemaVersion || json.Unmarshal(current.History.Payload, &history) != nil {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: invalid workflow transaction history", ErrInvalid)
	}
	graph, err := parseCommittedVisualGraph(current.GraphPayload, current.ResourceVersion)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	snapshot := WorkflowEditSnapshot{Key: key, Revision: *current.Receipt.RevisionAfter, Graph: graph}
	basis, err := a.workflowPreviewValidationBasis(ctx, snapshot, graph)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	if basis != current.ValidationBasis || current.Receipt.Validation.Basis != basis {
		return edit.WorkflowMutationReceipt{}, workflowDryRunStaleBasisError(key, basis, current.ValidationBasis)
	}

	var previous workflowPostCheckArtifact
	found := false
	for index := len(current.LifecycleArtifacts) - 1; index >= 0; index-- {
		envelope := current.LifecycleArtifacts[index]
		if envelope.Kind != "workflow.postcheck" || envelope.SchemaVersion != edit.SchemaVersion {
			continue
		}
		var candidate workflowPostCheckArtifact
		if json.Unmarshal(envelope.Payload, &candidate) != nil || candidate.Basis.Validate() != nil || candidate.CheckedAt.IsZero() || candidate.Basis.ResourceVersion != current.Receipt.RevisionBefore.ResourceVersion || candidate.TransactionID == transactionID {
			continue
		}
		previous, found = candidate, true
		break
	}
	if !found {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorValidationBasisStale, key, basis.ResourceVersion, current.Receipt.RevisionBefore.ResourceVersion, "no server lifecycle artifact exists for the previous revision", "repeat validation and dry-run for the current draft")
	}
	if err := validateWorkflowLayoutOnlyGateReissue(previous.Basis, basis, history.TouchedPaths); err != nil {
		return edit.WorkflowMutationReceipt{}, workflowPreviewDomainError(edit.ErrorValidationBasisStale, key, basis.ResourceVersion, previous.Basis.ResourceVersion, "layout-only lifecycle artifact cannot be reissued", "repeat validation and dry-run for the current draft")
	}
	return a.RecordPostCheck(ctx, WorkflowPostCheckFacts{
		Key: key, TransactionID: transactionID, Basis: basis, Outcome: previous.Outcome,
		Diagnostics: previous.Diagnostics, Configuration: previous.Configuration, Mode: previous.Mode,
		ResignedFromResourceVersion: previous.Basis.ResourceVersion,
	})
}

func (a *WorkflowEditApplication) workflowDryRunSnapshot(ctx context.Context, key edit.WorkflowKey) (WorkflowEditSnapshot, error) {
	if a.commits != nil {
		committed, err := a.commits.Snapshot(ctx, key)
		switch {
		case err == nil:
			if committed.Receipt.RevisionAfter == nil || committed.ResourceVersion != committed.Receipt.RevisionAfter.ResourceVersion || committed.ValidationBasis != committed.Receipt.Validation.Basis {
				return WorkflowEditSnapshot{}, ErrWorkflowEditSnapshot
			}
			graph, parseErr := parseCommittedVisualGraph(committed.GraphPayload, committed.ResourceVersion)
			if parseErr != nil {
				return WorkflowEditSnapshot{}, parseErr
			}
			return WorkflowEditSnapshot{Key: key, Revision: *committed.Receipt.RevisionAfter, Graph: graph}, nil
		case !errors.Is(err, ErrNotFound):
			return WorkflowEditSnapshot{}, err
		}
	}
	return a.readSnapshot(ctx, key)
}

func workflowDryRunStaleBasisError(key edit.WorkflowKey, current, expected edit.ValidationBasis) error {
	return workflowPreviewDomainError(edit.ErrorValidationBasisStale, key, current.ResourceVersion, expected.ResourceVersion, "workflow validation basis changed", "reload and repeat validation and dry-run")
}

func validateWorkflowDryRunOutcome(facts WorkflowPostCheckFacts) error {
	switch facts.Outcome {
	case WorkflowPostCheckPassed:
		for _, diagnostic := range facts.Diagnostics {
			if strings.EqualFold(diagnostic.Severity, "error") {
				return fmt.Errorf("%w: successful dry-run contains an error diagnostic", ErrInvalid)
			}
		}
	case WorkflowPostCheckConfigurationRequired:
		if len(facts.Diagnostics) == 0 {
			return fmt.Errorf("%w: configuration-required dry-run has no requirement", ErrInvalid)
		}
	case WorkflowPostCheckFailed:
		if len(facts.Diagnostics) == 0 {
			return fmt.Errorf("%w: failed dry-run has no diagnostic", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: invalid workflow dry-run outcome", ErrInvalid)
	}
	return nil
}

func validateWorkflowLayoutOnlyGateReissue(previous, current edit.ValidationBasis, touchedPaths []string) error {
	if previous.Validate() != nil || current.Validate() != nil || previous.ResourceVersion == current.ResourceVersion || len(touchedPaths) == 0 {
		return ErrInvalid
	}
	if previous.SemanticHash != current.SemanticHash || previous.CatalogVersion != current.CatalogVersion || previous.ActionSchemaDigest != current.ActionSchemaDigest || previous.ReferenceSnapshotHash != current.ReferenceSnapshotHash || previous.EnvironmentBindingVersion != current.EnvironmentBindingVersion {
		return ErrInvalid
	}
	if !workflowLayoutOnlyTouchedPaths(touchedPaths) {
		return ErrInvalid
	}
	return nil
}

func workflowLayoutOnlyTouchedPaths(touchedPaths []string) bool {
	if len(touchedPaths) == 0 {
		return false
	}
	for _, path := range touchedPaths {
		if path == "/layout/viewport" {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) != 4 || parts[0] != "" || parts[1] != "nodes" || parts[2] == "" || parts[3] != "position" {
			return false
		}
	}
	return true
}
