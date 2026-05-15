package opsmanual

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type CandidateRepository interface {
	GetCandidate(id string) (ManualCandidate, error)
	ListCandidates() ([]ManualCandidate, error)
	SaveCandidate(ManualCandidate) error
	DeleteCandidate(id string) error
}

type RunRecordRepository interface {
	SaveRunRecord(RunRecord) error
}

type MemoryStore struct {
	mu         sync.RWMutex
	manuals    map[string]OpsManual
	candidates map[string]ManualCandidate
	records    map[string]RunRecord
}

var _ ManualRepository = (*MemoryStore)(nil)
var _ CandidateRepository = (*MemoryStore)(nil)
var _ RunRecordRepository = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		manuals:    map[string]OpsManual{},
		candidates: map[string]ManualCandidate{},
		records:    map[string]RunRecord{},
	}
}

func (s *MemoryStore) ListManuals(req ListManualsRequest) ([]OpsManual, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]OpsManual, 0, len(s.manuals))
	for _, manual := range s.manuals {
		if !manualMatchesRequest(manual, req) {
			continue
		}
		out = append(out, cloneManual(manual))
	}
	sortManuals(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out, nil
}

func (s *MemoryStore) GetManual(id string) (OpsManual, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	manual, ok := s.manuals[id]
	if !ok {
		return OpsManual{}, fmt.Errorf("ops manual %q not found", id)
	}
	return cloneManual(manual), nil
}

func (s *MemoryStore) SaveManual(manual OpsManual) error {
	if strings.TrimSpace(manual.ID) == "" {
		return fmt.Errorf("ops manual id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.manuals[manual.ID] = cloneManual(manual)
	return nil
}

func (s *MemoryStore) DeleteManual(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.manuals[id]; !ok {
		return fmt.Errorf("ops manual %q not found", id)
	}
	delete(s.manuals, id)
	return nil
}

func (s *MemoryStore) GetCandidate(id string) (ManualCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidate, ok := s.candidates[id]
	if !ok {
		return ManualCandidate{}, fmt.Errorf("ops manual candidate %q not found", id)
	}
	return cloneCandidate(candidate), nil
}

func (s *MemoryStore) ListCandidates() ([]ManualCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ManualCandidate, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		out = append(out, cloneCandidate(candidate))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

func (s *MemoryStore) SaveCandidate(candidate ManualCandidate) error {
	if strings.TrimSpace(candidate.ID) == "" {
		return fmt.Errorf("ops manual candidate id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates[candidate.ID] = cloneCandidate(candidate)
	return nil
}

func (s *MemoryStore) DeleteCandidate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.candidates[id]; !ok {
		return fmt.Errorf("ops manual candidate %q not found", id)
	}
	delete(s.candidates, id)
	return nil
}

func (s *MemoryStore) ListRunRecords(req ListRunRecordsRequest) ([]RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RunRecord, 0, len(s.records))
	for _, record := range s.records {
		if req.ManualID != "" && record.ManualID != req.ManualID {
			continue
		}
		if req.WorkflowID != "" && record.WorkflowID != req.WorkflowID {
			continue
		}
		out = append(out, cloneRunRecord(record))
	}
	sortRunRecords(out)
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryStore) SaveRunRecord(record RunRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("ops manual run record id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ID] = cloneRunRecord(record)
	return nil
}

func manualMatchesRequest(manual OpsManual, req ListManualsRequest) bool {
	if req.Status != "" && manual.Status != req.Status {
		return false
	}
	if req.TargetType != "" && !equalFold(manual.Operation.TargetType, req.TargetType) {
		return false
	}
	if req.Action != "" && !equalFold(manual.Operation.Action, req.Action) {
		return false
	}
	if req.Middleware != "" && !equalFold(manual.Applicability.Middleware, req.Middleware) {
		return false
	}
	if req.ExecutionSurface != "" && !hasAnyFold(manual.Applicability.ExecutionSurface, req.ExecutionSurface) {
		return false
	}
	return true
}

func sortManuals(manuals []OpsManual) {
	sort.Slice(manuals, func(i, j int) bool {
		if manuals[i].UpdatedAt == manuals[j].UpdatedAt {
			return manuals[i].Title < manuals[j].Title
		}
		return manuals[i].UpdatedAt > manuals[j].UpdatedAt
	})
}

func sortRunRecords(records []RunRecord) {
	sort.Slice(records, func(i, j int) bool {
		left := records[i].CompletedAt
		if left == "" {
			left = records[i].StartedAt
		}
		right := records[j].CompletedAt
		if right == "" {
			right = records[j].StartedAt
		}
		if left == right {
			return records[i].ID < records[j].ID
		}
		return left > right
	})
}
