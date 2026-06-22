package operatorruntime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type MemoryStore struct {
	mu        sync.RWMutex
	resources map[string]ManagedResource
	templates map[string]InspectionTemplate
	problems  map[string]ProblemType
	actions   map[string]ActionCatalogItem
	bindings  map[string]WorkflowBinding
	rules     map[string]GuardRule
	runs      map[string]GuardRun
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		resources: map[string]ManagedResource{},
		templates: map[string]InspectionTemplate{},
		problems:  map[string]ProblemType{},
		actions:   map[string]ActionCatalogItem{},
		bindings:  map[string]WorkflowBinding{},
		rules:     map[string]GuardRule{},
		runs:      map[string]GuardRun{},
	}
}

func (s *MemoryStore) SaveResource(_ context.Context, item ManagedResource) error {
	item = NormalizeResource(item)
	if err := ValidateManagedResource(item); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resources[item.ID] = item
	return nil
}

func (s *MemoryStore) ListResources(context.Context) ([]ManagedResource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.resources), nil
}

func (s *MemoryStore) GetResource(_ context.Context, id string) (ManagedResource, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.resources[id]
	return item, ok, nil
}

func (s *MemoryStore) SavePGCluster(ctx context.Context, item PGCluster) error {
	return s.SaveResource(ctx, item)
}

func (s *MemoryStore) ListPGClusters(ctx context.Context) ([]PGCluster, error) {
	return s.ListResources(ctx)
}

func (s *MemoryStore) GetPGCluster(ctx context.Context, id string) (PGCluster, bool, error) {
	return s.GetResource(ctx, id)
}

func (s *MemoryStore) SaveInspectionTemplate(_ context.Context, item InspectionTemplate) error {
	if err := ValidateInspectionTemplate(item); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[item.ID] = item
	return nil
}

func (s *MemoryStore) ListInspectionTemplates(context.Context) ([]InspectionTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.templates), nil
}

func (s *MemoryStore) GetInspectionTemplate(_ context.Context, id string) (InspectionTemplate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.templates[id]
	return item, ok, nil
}

func (s *MemoryStore) SaveProblemType(ctx context.Context, item ProblemType) error {
	templates, _ := s.ListInspectionTemplates(ctx)
	if len(templates) > 0 {
		if err := ValidateProblemType(item, templates[0], 1); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.problems[item.ID] = item
	return nil
}

func (s *MemoryStore) ListProblemTypes(context.Context) ([]ProblemType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.problems), nil
}

func (s *MemoryStore) GetProblemType(_ context.Context, id string) (ProblemType, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.problems[id]
	return item, ok, nil
}

func (s *MemoryStore) SaveAction(_ context.Context, item ActionCatalogItem) error {
	if err := ValidateAction(item); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions[item.ID] = item
	return nil
}

func (s *MemoryStore) ListActions(context.Context) ([]ActionCatalogItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.actions), nil
}

func (s *MemoryStore) GetAction(_ context.Context, id string) (ActionCatalogItem, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.actions[id]
	return item, ok, nil
}

func (s *MemoryStore) SaveWorkflowBinding(_ context.Context, item WorkflowBinding) error {
	if err := ValidateWorkflowBinding(item); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[item.ID] = item
	return nil
}

func (s *MemoryStore) ListWorkflowBindings(context.Context) ([]WorkflowBinding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.bindings), nil
}

func (s *MemoryStore) GetWorkflowBinding(_ context.Context, id string) (WorkflowBinding, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.bindings[id]
	return item, ok, nil
}

func (s *MemoryStore) SaveGuardRule(ctx context.Context, item GuardRule) error {
	catalog, err := s.validationCatalog(ctx)
	if err != nil {
		return err
	}
	if err := ValidateGuardRule(item, catalog); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[item.ID] = item
	return nil
}

func (s *MemoryStore) ListGuardRules(context.Context) ([]GuardRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.rules), nil
}

func (s *MemoryStore) GetGuardRule(_ context.Context, id string) (GuardRule, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.rules[id]
	return item, ok, nil
}

func (s *MemoryStore) SetGuardRuleEnabled(ctx context.Context, id string, enabled bool) (GuardRule, error) {
	s.mu.RLock()
	item, ok := s.rules[id]
	s.mu.RUnlock()
	if !ok {
		return GuardRule{}, fmt.Errorf("guard rule not found")
	}
	item.Enabled = enabled
	if err := s.SaveGuardRule(ctx, item); err != nil {
		return GuardRule{}, err
	}
	return item, nil
}

func (s *MemoryStore) CreateGuardRun(_ context.Context, run GuardRun) error {
	if run.ID == "" {
		return fmt.Errorf("guard run id is required")
	}
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
	return nil
}

func (s *MemoryStore) AppendGuardRunEvent(_ context.Context, runID string, event GuardRunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return fmt.Errorf("guard run not found")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	run.Events = append(run.Events, event)
	run.UpdatedAt = event.CreatedAt
	s.runs[runID] = run
	return nil
}

func (s *MemoryStore) UpdateGuardRunState(_ context.Context, runID string, state GuardRunState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok {
		return fmt.Errorf("guard run not found")
	}
	run.State = state
	run.UpdatedAt = time.Now().UTC()
	s.runs[runID] = run
	return nil
}

func (s *MemoryStore) ListGuardRuns(context.Context) ([]GuardRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return values(s.runs), nil
}

func (s *MemoryStore) GetGuardRun(_ context.Context, id string) (GuardRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.runs[id]
	return item, ok, nil
}

func (s *MemoryStore) validationCatalog(context.Context) (ValidationCatalog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ValidationCatalog{
		Resources:        values(s.resources),
		Clusters:         values(s.resources),
		Templates:        values(s.templates),
		ProblemTypes:     values(s.problems),
		Actions:          values(s.actions),
		WorkflowBindings: values(s.bindings),
	}, nil
}

func values[T any](items map[string]T) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
