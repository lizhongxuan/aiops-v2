package appui

import (
	"context"
	"strings"

	"aiops-v2/internal/operatorruntime"
)

// OperatorRuntimeService exposes generic self-healing Operator runtime catalog
// and run state operations to HTTP handlers.
type OperatorRuntimeService struct {
	store operatorruntime.Store
}

func NewOperatorRuntimeService(store operatorruntime.Store) *OperatorRuntimeService {
	if store == nil {
		store = operatorruntime.NewMemoryStore()
	}
	return &OperatorRuntimeService{store: store}
}

func (s *OperatorRuntimeService) ListResources(ctx context.Context) ([]operatorruntime.ManagedResource, error) {
	return s.store.ListResources(ctx)
}

func (s *OperatorRuntimeService) CreateResource(ctx context.Context, item operatorruntime.ManagedResource) (operatorruntime.ManagedResource, error) {
	if err := s.store.SaveResource(ctx, item); err != nil {
		return operatorruntime.ManagedResource{}, err
	}
	return operatorruntime.NormalizeResource(item), nil
}

func (s *OperatorRuntimeService) ListPGClusters(ctx context.Context) ([]operatorruntime.PGCluster, error) {
	return s.store.ListResources(ctx)
}

func (s *OperatorRuntimeService) CreatePGCluster(ctx context.Context, item operatorruntime.PGCluster) (operatorruntime.PGCluster, error) {
	if err := s.store.SaveResource(ctx, item); err != nil {
		return operatorruntime.PGCluster{}, err
	}
	return operatorruntime.NormalizeResource(item), nil
}

func (s *OperatorRuntimeService) ListInspectionTemplates(ctx context.Context) ([]operatorruntime.InspectionTemplate, error) {
	return s.store.ListInspectionTemplates(ctx)
}

func (s *OperatorRuntimeService) CreateInspectionTemplate(ctx context.Context, item operatorruntime.InspectionTemplate) (operatorruntime.InspectionTemplate, error) {
	if err := s.store.SaveInspectionTemplate(ctx, item); err != nil {
		return operatorruntime.InspectionTemplate{}, err
	}
	return item, nil
}

func (s *OperatorRuntimeService) ListProblemTypes(ctx context.Context) ([]operatorruntime.ProblemType, error) {
	return s.store.ListProblemTypes(ctx)
}

func (s *OperatorRuntimeService) CreateProblemType(ctx context.Context, item operatorruntime.ProblemType) (operatorruntime.ProblemType, error) {
	if err := s.store.SaveProblemType(ctx, item); err != nil {
		return operatorruntime.ProblemType{}, err
	}
	return item, nil
}

func (s *OperatorRuntimeService) ListActions(ctx context.Context) ([]operatorruntime.ActionCatalogItem, error) {
	return s.store.ListActions(ctx)
}

func (s *OperatorRuntimeService) CreateAction(ctx context.Context, item operatorruntime.ActionCatalogItem) (operatorruntime.ActionCatalogItem, error) {
	if err := s.store.SaveAction(ctx, item); err != nil {
		return operatorruntime.ActionCatalogItem{}, err
	}
	return item, nil
}

func (s *OperatorRuntimeService) ListWorkflowBindings(ctx context.Context) ([]operatorruntime.WorkflowBinding, error) {
	return s.store.ListWorkflowBindings(ctx)
}

func (s *OperatorRuntimeService) CreateWorkflowBinding(ctx context.Context, item operatorruntime.WorkflowBinding) (operatorruntime.WorkflowBinding, error) {
	if err := s.store.SaveWorkflowBinding(ctx, item); err != nil {
		return operatorruntime.WorkflowBinding{}, err
	}
	return item, nil
}

func (s *OperatorRuntimeService) ListRules(ctx context.Context) ([]operatorruntime.GuardRule, error) {
	return s.store.ListGuardRules(ctx)
}

func (s *OperatorRuntimeService) CreateRule(ctx context.Context, item operatorruntime.GuardRule) (operatorruntime.GuardRule, error) {
	if err := s.store.SaveGuardRule(ctx, item); err != nil {
		return operatorruntime.GuardRule{}, err
	}
	return item, nil
}

func (s *OperatorRuntimeService) EnableRule(ctx context.Context, id string) (operatorruntime.GuardRule, error) {
	return s.store.SetGuardRuleEnabled(ctx, strings.TrimSpace(id), true)
}

func (s *OperatorRuntimeService) DisableRule(ctx context.Context, id string) (operatorruntime.GuardRule, error) {
	return s.store.SetGuardRuleEnabled(ctx, strings.TrimSpace(id), false)
}

func (s *OperatorRuntimeService) ListRuns(ctx context.Context) ([]operatorruntime.GuardRun, error) {
	return s.store.ListGuardRuns(ctx)
}

func (s *OperatorRuntimeService) GetRun(ctx context.Context, id string) (operatorruntime.GuardRun, bool, error) {
	return s.store.GetGuardRun(ctx, strings.TrimSpace(id))
}

func (s *OperatorRuntimeService) AppendRunEvent(ctx context.Context, runID string, event operatorruntime.GuardRunEvent) (operatorruntime.GuardRun, error) {
	if err := s.store.AppendGuardRunEvent(ctx, strings.TrimSpace(runID), event); err != nil {
		return operatorruntime.GuardRun{}, err
	}
	run, _, err := s.store.GetGuardRun(ctx, strings.TrimSpace(runID))
	return run, err
}
