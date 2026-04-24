package state

import (
	"context"
	"strings"
	"sync"
	"time"
)

type InMemoryRunStore struct {
	mu   sync.RWMutex
	runs map[string]RunState
}

func NewInMemoryRunStore() *InMemoryRunStore {
	return &InMemoryRunStore{
		runs: map[string]RunState{},
	}
}

var _ RunStateStore = (*InMemoryRunStore)(nil)

func (s *InMemoryRunStore) CreateRun(ctx context.Context, run RunState) error {
	_ = ctx
	if err := ValidateRunID(run.RunID); err != nil {
		return err
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = RunStatusQueued
	}
	if err := ValidateRunStatus(run.Status); err != nil {
		return err
	}
	now := time.Now().UTC()
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = now
	}
	if run.Version < 1 {
		run.Version = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[run.RunID]; ok {
		return ErrRunExists
	}
	s.runs[run.RunID] = CloneRunState(run)
	return nil
}

func (s *InMemoryRunStore) UpdateRun(ctx context.Context, run RunState) error {
	_ = ctx
	if err := ValidateRunID(run.RunID); err != nil {
		return err
	}
	if err := ValidateRunStatus(run.Status); err != nil {
		return err
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.runs[run.RunID]
	if !ok {
		return ErrRunNotFound
	}
	if err := ValidateRunTransition(prev.Status, run.Status); err != nil {
		return err
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = prev.StartedAt
	}
	if IsTerminalRunStatus(run.Status) && run.FinishedAt.IsZero() {
		run.FinishedAt = now
	}
	run.UpdatedAt = now
	if run.Version <= prev.Version {
		run.Version = prev.Version + 1
	}
	s.runs[run.RunID] = CloneRunState(run)
	return nil
}

func (s *InMemoryRunStore) GetRun(ctx context.Context, runID string) (RunState, error) {
	_ = ctx
	if err := ValidateRunID(runID); err != nil {
		return RunState{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	if !ok {
		return RunState{}, ErrRunNotFound
	}
	return CloneRunState(run), nil
}

func (s *InMemoryRunStore) ListRuns(ctx context.Context, filter ListFilter) ([]RunState, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]RunState, 0, len(s.runs))
	statusFilter := strings.TrimSpace(strings.ToLower(filter.Status))
	for _, run := range s.runs {
		if statusFilter != "" && strings.ToLower(strings.TrimSpace(run.Status)) != statusFilter {
			continue
		}
		out = append(out, CloneRunState(run))
	}
	sortRuns(out)
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (s *InMemoryRunStore) MarkInterruptedRunning(ctx context.Context, reason string) (int, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	updated := 0
	for runID, run := range s.runs {
		if !strings.EqualFold(run.Status, RunStatusRunning) {
			continue
		}
		if err := ValidateRunTransition(run.Status, RunStatusInterrupted); err != nil {
			return updated, err
		}
		run.Status = RunStatusInterrupted
		run.InterruptedReason = strings.TrimSpace(reason)
		if run.InterruptedReason != "" {
			run.Message = run.InterruptedReason
		}
		run.FinishedAt = now
		run.UpdatedAt = now
		run.Version++
		s.runs[runID] = CloneRunState(run)
		updated++
	}
	return updated, nil
}
