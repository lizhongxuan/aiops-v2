package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"runner/server/service"
	edit "runner/workflow/edit"
	"runner/workflow/visual"
)

const workflowResourceVersionField = "resource_version"

type WorkflowGraphSnapshot struct {
	Key      edit.WorkflowKey `json:"workflowKey"`
	Revision edit.Revision    `json:"revision"`
	Graph    visual.Graph     `json:"graph"`
}

// WorkflowCommitter reserves the narrow write boundary used by later
// transaction work. Implementations must perform CAS and durable receipt
// persistence; the facade deliberately exposes no raw store.
type WorkflowCommitter interface {
	CommitWorkflow(context.Context, edit.WorkflowKey, edit.Revision, visual.Graph) (edit.WorkflowMutationReceipt, error)
}

type WorkflowEditFacade struct {
	visual      *service.VisualWorkflowService
	application *service.WorkflowEditApplication
	references  *workflowEditReferenceAdapter
	storageName func(edit.WorkflowKey) (string, error)
}

func NewWorkflowEditFacade(visualService *service.VisualWorkflowService) *WorkflowEditFacade {
	return newWorkflowEditFacade(visualService, nil)
}

func newWorkflowEditFacade(visualService *service.VisualWorkflowService, allowedActions []string) *WorkflowEditFacade {
	references := &workflowEditReferenceAdapter{storageName: workflowStorageName}
	facade := &WorkflowEditFacade{visual: visualService, references: references, storageName: workflowStorageName}
	application, err := service.NewWorkflowEditApplication(facade, newWorkflowEditActionAccess(allowedActions), references)
	if err != nil {
		panic("construct workflow edit application: " + err.Error())
	}
	facade.application = application
	return facade
}

func newWorkflowEditMutationFacade(visualService *service.VisualWorkflowService, allowedActions []string, tenantID, dataRoot string, commits *service.WorkflowCommitStore) (*WorkflowEditFacade, error) {
	storageName := workflowTenantStorageName(tenantID)
	references := &workflowEditReferenceAdapter{storageName: storageName}
	facade := &WorkflowEditFacade{visual: visualService, references: references, storageName: storageName}
	candidates := service.NewWorkflowCandidateFileStore(dataRoot)
	approvals := service.NewWorkflowApprovalFileStore(dataRoot, commits)
	application, err := service.NewWorkflowEditMutationApplication(facade, newWorkflowEditActionAccess(allowedActions), references, candidates, approvals, commits)
	if err != nil {
		return nil, err
	}
	facade.application = application
	return facade, nil
}

func (facade *WorkflowEditFacade) GetGraph(ctx context.Context, key edit.WorkflowKey) (WorkflowGraphSnapshot, error) {
	storageName, err := facade.resolveStorageName(key)
	if err != nil {
		return WorkflowGraphSnapshot{}, err
	}
	if facade == nil || facade.visual == nil {
		return WorkflowGraphSnapshot{}, fmt.Errorf("workflow edit facade is not configured")
	}
	graph, err := facade.visual.GetGraph(ctx, storageName)
	if err != nil {
		return WorkflowGraphSnapshot{}, err
	}
	revision, err := workflowGraphRevision(graph)
	if err != nil {
		return WorkflowGraphSnapshot{}, err
	}
	return WorkflowGraphSnapshot{Key: key, Revision: revision, Graph: graph}, nil
}

func (facade *WorkflowEditFacade) GetHashes(ctx context.Context, key edit.WorkflowKey) (edit.Revision, error) {
	snapshot, err := facade.GetGraph(ctx, key)
	if err != nil {
		return edit.Revision{}, err
	}
	return snapshot.Revision, nil
}

func (facade *WorkflowEditFacade) EnsureWorkflowEditBaseline(ctx context.Context, key edit.WorkflowKey, expected, idempotencyKey string) (edit.Revision, bool, error) {
	storageName, err := facade.resolveStorageName(key)
	if err != nil {
		return edit.Revision{}, false, err
	}
	if facade == nil || facade.visual == nil {
		return edit.Revision{}, false, service.ErrWorkflowEditConfiguration
	}
	return facade.visual.EnsureWorkflowEditBaseline(ctx, storageName, expected, idempotencyKey)
}

func (facade *WorkflowEditFacade) ListActions(ctx context.Context, key edit.WorkflowKey, filter service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if facade == nil || facade.visual == nil {
		return nil, fmt.Errorf("workflow edit facade is not configured")
	}
	return facade.visual.ListActions(ctx, filter), nil
}

func (facade *WorkflowEditFacade) Validate(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	if err := facade.validateWorkflowGraphBinding(key, graph); err != nil {
		return nil, err
	}
	if facade == nil || facade.visual == nil {
		return nil, fmt.Errorf("workflow edit facade is not configured")
	}
	return facade.visual.Validate(ctx, graph)
}

func (facade *WorkflowEditFacade) Compile(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.CompiledVisualWorkflow, error) {
	if err := facade.validateWorkflowGraphBinding(key, graph); err != nil {
		return nil, err
	}
	if facade == nil || facade.visual == nil {
		return nil, fmt.Errorf("workflow edit facade is not configured")
	}
	return facade.visual.Compile(ctx, graph)
}

// DryRunWorkflow adapts the Workflow Edit application's server-owned snapshot
// to the existing Visual Workflow draft simulator. The edit application owns
// lifecycle persistence, so this boundary deliberately does not ask the legacy
// Visual service to mutate workflow status.
func (facade *WorkflowEditFacade) DryRunWorkflow(ctx context.Context, key edit.WorkflowKey, graph visual.Graph, request service.WorkflowEditDryRunExecutionRequest) (service.WorkflowEditDryRunExecutionResult, error) {
	if facade == nil || facade.visual == nil {
		return service.WorkflowEditDryRunExecutionResult{}, service.ErrWorkflowEditConfiguration
	}
	if err := facade.validateWorkflowGraphBinding(key, graph); err != nil {
		return service.WorkflowEditDryRunExecutionResult{}, err
	}
	boundEnvironment := strings.TrimSpace(graph.Workflow.ValidationEnv)
	if request.Mode != service.WorkflowDryRunModeDraftNonProduction || request.Environment != strings.TrimSpace(request.Environment) || request.Environment != boundEnvironment {
		return service.WorkflowEditDryRunExecutionResult{}, fmt.Errorf("%w: workflow dry-run request is not bound to the current draft environment", service.ErrInvalid)
	}

	result, err := facade.visual.DryRunWithOptions(ctx, graph, nil, service.VisualWorkflowDryRunOptions{TriggeredBy: "workflow-edit"})
	if err != nil {
		return service.WorkflowEditDryRunExecutionResult{}, err
	}
	if result == nil {
		return service.WorkflowEditDryRunExecutionResult{}, service.ErrWorkflowEditConfiguration
	}
	diagnostics := workflowEditDryRunDiagnostics(result.Errors, result.Warnings)
	if !result.Valid || len(result.Errors) > 0 {
		if len(result.Errors) == 0 {
			return service.WorkflowEditDryRunExecutionResult{}, fmt.Errorf("%w: visual workflow dry-run returned an invalid result without errors", service.ErrWorkflowEditConfiguration)
		}
		return service.WorkflowEditDryRunExecutionResult{Outcome: service.WorkflowPostCheckFailed, Diagnostics: diagnostics}, nil
	}
	return service.WorkflowEditDryRunExecutionResult{Outcome: service.WorkflowPostCheckPassed, Diagnostics: diagnostics}, nil
}

func workflowEditDryRunDiagnostics(groups ...[]service.VisualWorkflowIssue) []edit.WorkflowDiagnostic {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	diagnostics := make([]edit.WorkflowDiagnostic, 0, count)
	for _, group := range groups {
		for _, issue := range group {
			diagnostics = append(diagnostics, edit.WorkflowDiagnostic{
				Code: issue.Code, Severity: issue.Severity, Message: issue.Message,
				NodeID: issue.NodeID, EdgeID: issue.EdgeID, Field: issue.Field, Suggestion: issue.Suggestion,
			})
		}
	}
	return diagnostics
}

func (facade *WorkflowEditFacade) ReadWorkflow(ctx context.Context, key edit.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	snapshot, err := facade.GetGraph(ctx, key)
	if err != nil {
		return service.WorkflowEditSnapshot{}, err
	}
	return service.WorkflowEditSnapshot{Key: snapshot.Key, Revision: snapshot.Revision, Graph: snapshot.Graph}, nil
}

func (facade *WorkflowEditFacade) ListWorkflowActions(ctx context.Context, key edit.WorkflowKey, filter service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	return facade.ListActions(ctx, key, filter)
}

func (facade *WorkflowEditFacade) ValidateWorkflow(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	return facade.Validate(ctx, key, graph)
}

func (facade *WorkflowEditFacade) CompileWorkflow(ctx context.Context, key edit.WorkflowKey, graph visual.Graph) (*service.CompiledVisualWorkflow, error) {
	return facade.Compile(ctx, key, graph)
}

func (facade *WorkflowEditFacade) Application() *service.WorkflowEditApplication {
	if facade == nil {
		return nil
	}
	return facade.application
}

func (facade *WorkflowEditFacade) ExecuteManualCommand(ctx context.Context, request service.WorkflowManualCommandRequest) (service.WorkflowManualCommandResult, error) {
	if facade == nil || facade.application == nil {
		return service.WorkflowManualCommandResult{}, service.ErrWorkflowEditConfiguration
	}
	return facade.application.ExecuteManualCommand(ctx, request)
}

func (facade *WorkflowEditFacade) SetWorkflowReferenceChecker(checker service.WorkflowReferenceChecker) {
	if facade != nil && facade.references != nil {
		facade.references.setChecker(checker)
	}
}

type workflowEditActionAccess struct{ allowed map[string]struct{} }

func newWorkflowEditActionAccess(actions []string) workflowEditActionAccess {
	access := workflowEditActionAccess{allowed: make(map[string]struct{}, len(actions))}
	for _, action := range actions {
		if action = strings.TrimSpace(action); action != "" {
			access.allowed[action] = struct{}{}
		}
	}
	return access
}

func (access workflowEditActionAccess) ResolveWorkflowActionAccess(_ context.Context, key edit.WorkflowKey, spec service.ActionSpec) (service.WorkflowActionAccess, error) {
	if err := key.Validate(); err != nil {
		return service.WorkflowActionAccess{}, err
	}
	if spec.Experimental || spec.Deprecated {
		return service.WorkflowActionAccess{RequiredPermissions: []string{"workflow.action.preview"}, Reason: "experimental or deprecated action is disabled by default"}, nil
	}
	risk := strings.ToLower(strings.TrimSpace(spec.Risk))
	if risk == "high" || risk == "critical" {
		return service.WorkflowActionAccess{RequiredPermissions: []string{"workflow.action.high_risk"}, Reason: "high-risk action requires an explicit approval scope"}, nil
	}
	if risk != "low" && risk != "medium" {
		return service.WorkflowActionAccess{RequiredPermissions: []string{"workflow.action.classified"}, Reason: "action risk is not classified by Runner policy"}, nil
	}
	if _, ok := access.allowed[spec.Action]; !ok {
		return service.WorkflowActionAccess{RequiredPermissions: []string{"workflow.action.use"}, Reason: "action is not enabled by Runner security policy"}, nil
	}
	return service.WorkflowActionAccess{Allowed: true}, nil
}

type workflowEditReferenceAdapter struct {
	mu          sync.RWMutex
	checker     service.WorkflowReferenceChecker
	storageName func(edit.WorkflowKey) (string, error)
}

func (adapter *workflowEditReferenceAdapter) setChecker(checker service.WorkflowReferenceChecker) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.checker = checker
}

func (adapter *workflowEditReferenceAdapter) ValidateWorkflowReferences(ctx context.Context, key edit.WorkflowKey, _ visual.Graph) ([]service.VisualWorkflowIssue, error) {
	adapter.mu.RLock()
	checker := adapter.checker
	adapter.mu.RUnlock()
	if checker == nil {
		return []service.VisualWorkflowIssue{{Severity: "error", Type: "reference", Code: "REFERENCE_GUARD_UNAVAILABLE", Message: "workflow reference guard is unavailable"}}, nil
	}
	storageName, err := adapter.storageName(key)
	if err != nil {
		return nil, err
	}
	references, err := checker.ReferencesForWorkflow(ctx, storageName)
	if err != nil {
		return nil, err
	}
	decision := service.EvaluateWorkflowDeleteReferences(references)
	if decision.Allowed {
		return nil, nil
	}
	issue := service.VisualWorkflowIssue{Severity: "warning", Type: "reference", Code: "REFERENCE_PRESENT", Message: "workflow has active references"}
	if decision.ErrorCode == service.WorkflowErrorCodeInUse {
		issue.Severity, issue.Code, issue.Message = "error", "REFERENCE_LOCKED", "workflow is locked by a verified reference"
	}
	return []service.VisualWorkflowIssue{issue}, nil
}

var _ service.WorkflowEditReadBackend = (*WorkflowEditFacade)(nil)
var _ service.WorkflowEditDryRunExecutor = (*WorkflowEditFacade)(nil)

func workflowStorageName(key edit.WorkflowKey) (string, error) {
	if err := key.Validate(); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(key.TenantID + "\x00" + key.WorkflowID))
	return "wf2-" + hex.EncodeToString(digest[:]), nil
}

func workflowTenantStorageName(tenantID string) func(edit.WorkflowKey) (string, error) {
	trustedTenant := strings.TrimSpace(tenantID)
	return func(key edit.WorkflowKey) (string, error) {
		if err := key.Validate(); err != nil || trustedTenant == "" || key.TenantID != trustedTenant {
			return "", fmt.Errorf("invalid or unauthorized workflow key")
		}
		return key.WorkflowID, nil
	}
}

func (facade *WorkflowEditFacade) resolveStorageName(key edit.WorkflowKey) (string, error) {
	if facade == nil || facade.storageName == nil {
		return "", fmt.Errorf("workflow edit facade is not configured")
	}
	return facade.storageName(key)
}

func (facade *WorkflowEditFacade) validateWorkflowGraphBinding(key edit.WorkflowKey, graph visual.Graph) error {
	storageName, err := facade.resolveStorageName(key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(graph.Workflow.Name) != storageName {
		return fmt.Errorf("workflow graph is not bound to the canonical workflow key")
	}
	return nil
}

func validateWorkflowGraphBinding(key edit.WorkflowKey, graph visual.Graph) error {
	storageName, err := workflowStorageName(key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(graph.Workflow.Name) != storageName {
		return fmt.Errorf("workflow graph is not bound to the canonical workflow key")
	}
	return nil
}

func workflowGraphRevision(graph visual.Graph) (edit.Revision, error) {
	hashes, err := service.VisualWorkflowGraphHashes(graph)
	if err != nil {
		return edit.Revision{}, err
	}
	resourceVersion := ""
	if graph.UI != nil {
		resourceVersion = strings.TrimSpace(fmt.Sprint(graph.UI[workflowResourceVersionField]))
	}
	if resourceVersion == "" || resourceVersion == "<nil>" {
		return edit.Revision{}, fmt.Errorf("workflow graph is missing resource version")
	}
	return edit.Revision{
		ResourceVersion: resourceVersion,
		SemanticHash:    hashes.SemanticHash,
		LayoutHash:      hashes.LayoutHash,
	}, nil
}
