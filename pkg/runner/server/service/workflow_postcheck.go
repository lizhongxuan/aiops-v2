package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"runner/workflow/edit"
)

// ValidateChange resolves its Graph exclusively from the server-owned
// committed snapshot or immutable Candidate; callers cannot supply a Graph.
func (a *WorkflowEditApplication) ValidateChange(ctx context.Context, request edit.ValidateRequest) (result edit.ValidationResult, resultErr error) {
	startedAt := time.Now().UTC()
	defer func() {
		a.observeWorkflowEdit(ctx, WorkflowEditObservation{Operation: WorkflowEditObserveValidate, Key: request.WorkflowKey, ChangeSetID: request.ChangeSetID, StartedAt: startedAt, Duration: time.Since(startedAt)}, resultErr)
	}()
	if a == nil || a.backend == nil || a.references == nil {
		return edit.ValidationResult{}, ErrWorkflowEditConfiguration
	}
	if request.SchemaVersion != edit.SchemaVersion || request.WorkflowKey.Validate() != nil || (request.Revision != "" && request.ChangeSetID != "") || strings.TrimSpace(request.Revision) != request.Revision || strings.TrimSpace(request.ChangeSetID) != request.ChangeSetID {
		return edit.ValidationResult{}, fmt.Errorf("%w: invalid workflow validation request", ErrInvalid)
	}
	snapshot, err := a.readSnapshot(ctx, request.WorkflowKey)
	if err != nil {
		return edit.ValidationResult{}, err
	}
	graph := snapshot.Graph
	if request.Revision != "" && request.Revision != snapshot.Revision.ResourceVersion {
		return edit.ValidationResult{}, workflowCommitDomainError(edit.ErrorStaleRevision, request.WorkflowKey, snapshot.Revision.ResourceVersion, request.Revision, "workflow revision changed before validation", "reload the current workflow")
	}
	if request.ChangeSetID != "" {
		if a.candidates == nil {
			return edit.ValidationResult{}, ErrWorkflowEditConfiguration
		}
		candidate, resolveErr := a.candidates.ResolveChangeSet(ctx, request.WorkflowKey, request.ChangeSetID)
		if resolveErr != nil {
			return edit.ValidationResult{}, resolveErr
		}
		if candidate.Base != snapshot.Revision {
			return edit.ValidationResult{}, workflowPreviewConflict(request.WorkflowKey, candidate.Base, snapshot.Revision, []string{"/resourceVersion"}, nil)
		}
		applied, applyErr := applyWorkflowPreviewOperations(snapshot.Graph, candidate.NormalizedOperations)
		if applyErr != nil {
			return edit.ValidationResult{}, applyErr
		}
		graph = applied.Graph
	}
	validated, err := a.validateSnapshot(ctx, snapshot, graph, snapshot.Revision.ResourceVersion)
	if err != nil {
		return edit.ValidationResult{}, err
	}
	basis, err := a.workflowPreviewValidationBasis(ctx, snapshot, graph)
	if err != nil {
		return edit.ValidationResult{}, err
	}
	return workflowPreviewValidation(validated, basis), nil
}

type WorkflowPostCheckOutcome string

const (
	WorkflowPostCheckPassed                WorkflowPostCheckOutcome = "passed"
	WorkflowPostCheckConfigurationRequired WorkflowPostCheckOutcome = "configuration_required"
	WorkflowPostCheckFailed                WorkflowPostCheckOutcome = "failed"
)

// WorkflowPostCheckFacts must be produced by a server-side validator/dry-run;
// it is not decoded from model tool arguments.
type WorkflowPostCheckFacts struct {
	Key                         edit.WorkflowKey
	TransactionID               string
	Basis                       edit.ValidationBasis
	Outcome                     WorkflowPostCheckOutcome
	Diagnostics                 []edit.WorkflowDiagnostic
	Configuration               WorkflowConfigurationRequirements
	Mode                        string
	ResignedFromResourceVersion string
}

// WorkflowConfigurationRequirements contains identifiers only. Values are
// deliberately absent from the contract so post-check and UI artifacts cannot
// become a credential transport.
type WorkflowConfigurationRequirements struct {
	SecretRefs   []string `json:"secretRefs,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

type workflowPostCheckArtifact struct {
	TransactionID               string                            `json:"transactionId"`
	Basis                       edit.ValidationBasis              `json:"basis"`
	Outcome                     WorkflowPostCheckOutcome          `json:"outcome"`
	Diagnostics                 []edit.WorkflowDiagnostic         `json:"diagnostics"`
	Configuration               WorkflowConfigurationRequirements `json:"configuration,omitempty"`
	Mode                        string                            `json:"mode,omitempty"`
	ResignedFromResourceVersion string                            `json:"resignedFromResourceVersion,omitempty"`
	CheckedAt                   time.Time                         `json:"checkedAt"`
}

func (a *WorkflowEditApplication) RecordPostCheck(ctx context.Context, facts WorkflowPostCheckFacts) (result edit.WorkflowMutationReceipt, resultErr error) {
	startedAt := time.Now().UTC()
	defer func() {
		preferred := WorkflowEditObserveOutcome("")
		if resultErr != nil && facts.Outcome == WorkflowPostCheckPassed {
			preferred = WorkflowEditOutcomeInvalidCompleted
		}
		a.observeWorkflowEdit(ctx, WorkflowEditObservation{Operation: WorkflowEditObservePostCheck, Outcome: preferred, Key: facts.Key, TransactionID: facts.TransactionID, StartedAt: startedAt, Duration: time.Since(startedAt)}, resultErr)
	}()
	if a == nil || a.commits == nil || a.now == nil {
		return edit.WorkflowMutationReceipt{}, ErrWorkflowEditConfiguration
	}
	if facts.Key.Validate() != nil || !validWorkflowApprovalValue(facts.TransactionID, true) || facts.Basis.Validate() != nil {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: invalid workflow post-check identity", ErrInvalid)
	}
	normalized, err := normalizeWorkflowPostCheckFacts(facts)
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	facts = normalized
	status, valid := workflowPostCheckStatus(facts.Outcome, facts.Diagnostics)
	switch facts.Outcome {
	case WorkflowPostCheckPassed:
		for _, diagnostic := range facts.Diagnostics {
			if strings.EqualFold(diagnostic.Severity, "error") {
				return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: passed post-check contains an error", ErrInvalid)
			}
		}
	case WorkflowPostCheckConfigurationRequired:
	case WorkflowPostCheckFailed:
	default:
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: invalid workflow post-check outcome", ErrInvalid)
	}
	if !valid && len(facts.Diagnostics) == 0 {
		return edit.WorkflowMutationReceipt{}, fmt.Errorf("%w: blocking workflow post-check requires diagnostics", ErrInvalid)
	}
	checkedAt := a.now().UTC()
	artifact, err := visualWriteEnvelope("workflow.postcheck", workflowPostCheckArtifact{
		TransactionID: facts.TransactionID, Basis: facts.Basis, Outcome: facts.Outcome,
		Diagnostics: facts.Diagnostics, Configuration: facts.Configuration, Mode: facts.Mode,
		ResignedFromResourceVersion: facts.ResignedFromResourceVersion, CheckedAt: checkedAt,
	})
	if err != nil {
		return edit.WorkflowMutationReceipt{}, err
	}
	validation := edit.ValidationResult{Kind: "validation", Basis: facts.Basis, Valid: valid, Diagnostics: append([]edit.WorkflowDiagnostic(nil), facts.Diagnostics...)}
	return a.commits.UpdateReceiptLifecycle(ctx, WorkflowReceiptLifecycleUpdate{Key: facts.Key, TransactionID: facts.TransactionID, ExpectedBasis: facts.Basis, Status: status, Validation: validation, Artifact: artifact})
}

func normalizeWorkflowPostCheckFacts(facts WorkflowPostCheckFacts) (WorkflowPostCheckFacts, error) {
	configuration, err := normalizeWorkflowConfigurationRequirements(facts.Configuration)
	if err != nil {
		return WorkflowPostCheckFacts{}, fmt.Errorf("%w: invalid workflow configuration requirement", ErrInvalid)
	}
	facts.Configuration = configuration
	if facts.Mode != "" && facts.Mode != WorkflowDryRunModeDraftNonProduction {
		return WorkflowPostCheckFacts{}, fmt.Errorf("%w: invalid workflow post-check mode", ErrInvalid)
	}
	if facts.ResignedFromResourceVersion != "" && !validWorkflowApprovalValue(facts.ResignedFromResourceVersion, true) {
		return WorkflowPostCheckFacts{}, fmt.Errorf("%w: invalid workflow post-check reissue basis", ErrInvalid)
	}
	if facts.Outcome != WorkflowPostCheckConfigurationRequired && (len(configuration.SecretRefs) > 0 || len(configuration.Environments) > 0) {
		return WorkflowPostCheckFacts{}, fmt.Errorf("%w: configuration requirements require configuration_required outcome", ErrInvalid)
	}
	if facts.Outcome == WorkflowPostCheckConfigurationRequired && (len(configuration.SecretRefs) > 0 || len(configuration.Environments) > 0) {
		// Structured names are authoritative. Arbitrary diagnostic text may contain
		// provider error bodies or credential values, so it is not copied.
		facts.Diagnostics = workflowConfigurationDiagnostics(configuration)
	} else {
		facts.Diagnostics = append([]edit.WorkflowDiagnostic(nil), facts.Diagnostics...)
	}
	return facts, nil
}

func normalizeWorkflowConfigurationRequirements(value WorkflowConfigurationRequirements) (WorkflowConfigurationRequirements, error) {
	normalize := func(values []string) ([]string, error) {
		seen := make(map[string]struct{}, len(values))
		out := make([]string, 0, len(values))
		for _, item := range values {
			if !validWorkflowConfigurationName(item) {
				return nil, ErrInvalid
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
		sort.Strings(out)
		return out, nil
	}
	secretRefs, err := normalize(value.SecretRefs)
	if err != nil {
		return WorkflowConfigurationRequirements{}, err
	}
	environments, err := normalize(value.Environments)
	if err != nil {
		return WorkflowConfigurationRequirements{}, err
	}
	return WorkflowConfigurationRequirements{SecretRefs: secretRefs, Environments: environments}, nil
}

func validWorkflowConfigurationName(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 256 {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("._:/@+-", character):
		default:
			return false
		}
	}
	return true
}

func workflowConfigurationDiagnostics(value WorkflowConfigurationRequirements) []edit.WorkflowDiagnostic {
	diagnostics := make([]edit.WorkflowDiagnostic, 0, len(value.SecretRefs)+len(value.Environments))
	for _, name := range value.SecretRefs {
		diagnostics = append(diagnostics, edit.WorkflowDiagnostic{Code: "MISSING_SECRET_REF", Severity: "error", Field: "secretRef", Message: "Required secretRef is not configured: " + name})
	}
	for _, name := range value.Environments {
		diagnostics = append(diagnostics, edit.WorkflowDiagnostic{Code: "MISSING_ENVIRONMENT", Severity: "error", Field: "environment", Message: "Required environment is not configured: " + name})
	}
	return diagnostics
}

func workflowPostCheckStatus(outcome WorkflowPostCheckOutcome, diagnostics []edit.WorkflowDiagnostic) (string, bool) {
	switch outcome {
	case WorkflowPostCheckPassed:
		for _, diagnostic := range diagnostics {
			if strings.EqualFold(diagnostic.Severity, "warning") {
				return "completed_with_warnings", true
			}
		}
		return "completed", true
	case WorkflowPostCheckConfigurationRequired:
		return "configuration_required", false
	case WorkflowPostCheckFailed:
		return "saved_with_issues", false
	default:
		return "", false
	}
}
