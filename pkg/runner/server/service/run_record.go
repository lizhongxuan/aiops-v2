package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var errRunRecordNotFound = errors.New("run record not found")

type RunRecordStore interface {
	Upsert(ctx context.Context, meta RunMeta) error
	Get(ctx context.Context, runID string) (RunMeta, error)
	List(ctx context.Context, filter RunFilter) ([]RunMeta, error)
	Delete(ctx context.Context, runID string) error
}

type InMemoryRunRecordStore struct {
	mu    sync.RWMutex
	items map[string]RunMeta
}

func NewInMemoryRunRecordStore() *InMemoryRunRecordStore {
	return &InMemoryRunRecordStore{
		items: map[string]RunMeta{},
	}
}

func (s *InMemoryRunRecordStore) Upsert(_ context.Context, meta RunMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[meta.RunID] = cloneRunMeta(meta)
	return nil
}

func (s *InMemoryRunRecordStore) Get(_ context.Context, runID string) (RunMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[strings.TrimSpace(runID)]
	if !ok {
		return RunMeta{}, errRunRecordNotFound
	}
	return cloneRunMeta(item), nil
}

func (s *InMemoryRunRecordStore) List(_ context.Context, filter RunFilter) ([]RunMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filterAndSortRunMetas(mapValues(s.items), filter), nil
}

func (s *InMemoryRunRecordStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, strings.TrimSpace(runID))
	return nil
}

type FileRunRecordStore struct {
	Path string
	mu   sync.Mutex
}

type runRecordFile struct {
	UpdatedAt time.Time `json:"updated_at"`
	Items     []RunMeta `json:"items"`
}

func NewFileRunRecordStore(path string) *FileRunRecordStore {
	return &FileRunRecordStore{Path: path}
}

func (s *FileRunRecordStore) Upsert(_ context.Context, meta RunMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return err
	}
	index := -1
	for i := range data.Items {
		if data.Items[i].RunID == meta.RunID {
			index = i
			break
		}
	}
	if index >= 0 {
		data.Items[index] = cloneRunMeta(meta)
	} else {
		data.Items = append(data.Items, cloneRunMeta(meta))
	}
	sort.Slice(data.Items, func(i, j int) bool {
		return runMetaTime(data.Items[i]).After(runMetaTime(data.Items[j]))
	})
	return s.saveNoLock(data)
}

func (s *FileRunRecordStore) Get(_ context.Context, runID string) (RunMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return RunMeta{}, err
	}
	for _, item := range data.Items {
		if item.RunID == strings.TrimSpace(runID) {
			return cloneRunMeta(item), nil
		}
	}
	return RunMeta{}, errRunRecordNotFound
}

func (s *FileRunRecordStore) List(_ context.Context, filter RunFilter) ([]RunMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return nil, err
	}
	return filterAndSortRunMetas(data.Items, filter), nil
}

func (s *FileRunRecordStore) Delete(_ context.Context, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return err
	}
	filtered := make([]RunMeta, 0, len(data.Items))
	for _, item := range data.Items {
		if item.RunID == strings.TrimSpace(runID) {
			continue
		}
		filtered = append(filtered, item)
	}
	data.Items = filtered
	return s.saveNoLock(data)
}

func (s *FileRunRecordStore) loadNoLock() (runRecordFile, error) {
	if strings.TrimSpace(s.Path) == "" {
		return runRecordFile{}, nil
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return runRecordFile{Items: []RunMeta{}}, nil
		}
		return runRecordFile{}, err
	}
	var data runRecordFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return runRecordFile{}, err
	}
	if data.Items == nil {
		data.Items = []RunMeta{}
	}
	return data, nil
}

func (s *FileRunRecordStore) saveNoLock(data runRecordFile) error {
	if strings.TrimSpace(s.Path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data.UpdatedAt = time.Now().UTC()
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, payload, 0o644)
}

func DeriveRunRecordFile(runStateFile string) string {
	trimmed := strings.TrimSpace(runStateFile)
	ext := filepath.Ext(trimmed)
	if ext == "" {
		return trimmed + "-records.json"
	}
	return strings.TrimSuffix(trimmed, ext) + "-records" + ext
}

func filterAndSortRunMetas(items []RunMeta, filter RunFilter) []RunMeta {
	statusFilter := strings.TrimSpace(filter.Status)
	workflowFilter := strings.TrimSpace(filter.Workflow)
	out := make([]RunMeta, 0, len(items))
	for _, item := range items {
		if statusFilter != "" && item.Status != statusFilter {
			continue
		}
		if workflowFilter != "" && item.WorkflowName != workflowFilter {
			continue
		}
		out = append(out, cloneRunMeta(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return runMetaTime(out[i]).After(runMetaTime(out[j]))
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out
}

func mapValues(items map[string]RunMeta) []RunMeta {
	out := make([]RunMeta, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func cloneRunMeta(input RunMeta) RunMeta {
	out := input
	out.Vars = cloneAnyMap(input.Vars)
	if len(input.Labels) > 0 {
		out.Labels = make(map[string]string, len(input.Labels))
		for key, value := range input.Labels {
			out.Labels[key] = value
		}
	}
	return out
}

func runMetaTime(meta RunMeta) time.Time {
	switch {
	case !meta.CreatedAt.IsZero():
		return meta.CreatedAt
	case !meta.QueuedAt.IsZero():
		return meta.QueuedAt
	case !meta.StartedAt.IsZero():
		return meta.StartedAt
	case !meta.FinishedAt.IsZero():
		return meta.FinishedAt
	default:
		return time.Time{}
	}
}
