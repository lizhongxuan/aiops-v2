package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"runner/workflow"
	"runner/workflowstore"
)

type workflowMeta struct {
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
}

type WorkflowService struct {
	store    *workflowstore.Store
	metaPath string
	mu       sync.Mutex
}

func NewWorkflowService(dir string) *WorkflowService {
	return &WorkflowService{
		store:    workflowstore.New(dir),
		metaPath: filepath.Join(dir, ".meta.json"),
	}
}

func (s *WorkflowService) List(_ context.Context, labels map[string]string) ([]*WorkflowRecord, error) {
	items, err := s.store.List()
	if err != nil {
		return nil, err
	}
	metas, err := s.loadMeta()
	if err != nil {
		return nil, err
	}

	out := make([]*WorkflowRecord, 0, len(items))
	for _, item := range items {
		record := &WorkflowRecord{
			Name:        item.Name,
			Description: item.Description,
			UpdatedAt:   item.UpdatedAt,
		}
		if meta, ok := metas[item.Name]; ok {
			record.Labels = copyStringMap(meta.Labels)
			record.CreatedAt = meta.CreatedAt
			if !meta.UpdatedAt.IsZero() {
				record.UpdatedAt = meta.UpdatedAt
			}
		}
		if !matchLabels(record.Labels, labels) {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *WorkflowService) Get(_ context.Context, name string) (*WorkflowRecord, error) {
	wf, raw, err := s.store.Get(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	metas, err := s.loadMeta()
	if err != nil {
		return nil, err
	}
	record := &WorkflowRecord{
		Name:        wf.Name,
		Description: wf.Description,
		Version:     wf.Version,
		RawYAML:     append([]byte{}, raw...),
	}
	if meta, ok := metas[wf.Name]; ok {
		record.Labels = copyStringMap(meta.Labels)
		record.CreatedAt = meta.CreatedAt
		record.UpdatedAt = meta.UpdatedAt
	}
	if record.UpdatedAt.IsZero() {
		if summaries, err := s.store.List(); err == nil {
			for _, item := range summaries {
				if item.Name == wf.Name {
					record.UpdatedAt = item.UpdatedAt
					break
				}
			}
		}
	}
	return record, nil
}

func (s *WorkflowService) Create(_ context.Context, record *WorkflowRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	raw := record.RawYAML
	if len(raw) == 0 {
		return fmt.Errorf("%w: yaml is required", ErrInvalid)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if strings.TrimSpace(wf.Name) != name {
		return fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	if _, err := s.Get(context.Background(), name); err == nil {
		return ErrAlreadyExists
	} else if err != nil && err != ErrNotFound {
		return err
	}
	if _, err := s.store.Put(name, raw); err != nil {
		return err
	}
	return s.upsertMeta(name, record.Labels, true)
}

func (s *WorkflowService) Update(_ context.Context, name string, record *WorkflowRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	raw := record.RawYAML
	if len(raw) == 0 {
		return fmt.Errorf("%w: yaml is required", ErrInvalid)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if strings.TrimSpace(wf.Name) != name {
		return fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	existing, err := s.Get(context.Background(), name)
	if err != nil {
		return err
	}
	if _, err := s.store.Put(name, raw); err != nil {
		return err
	}
	labels := record.Labels
	if labels == nil {
		labels = existing.Labels
	}
	return s.upsertMeta(name, labels, false)
}

func (s *WorkflowService) Delete(_ context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	path := filepath.Join(s.store.Dir, name+".yaml")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metas, err := s.loadMetaNoLock()
	if err != nil {
		return err
	}
	delete(metas, name)
	return s.saveMetaNoLock(metas)
}

func (s *WorkflowService) Validate(_ context.Context, yamlContent []byte) error {
	wf, err := workflow.Load(yamlContent)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func (s *WorkflowService) upsertMeta(name string, labels map[string]string, isCreate bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	metas, err := s.loadMetaNoLock()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	meta, ok := metas[name]
	if !ok {
		meta = workflowMeta{Name: name, CreatedAt: now}
	}
	if isCreate {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	meta.Labels = copyStringMap(labels)
	metas[name] = meta
	return s.saveMetaNoLock(metas)
}

func (s *WorkflowService) loadMeta() (map[string]workflowMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadMetaNoLock()
}

func (s *WorkflowService) loadMetaNoLock() (map[string]workflowMeta, error) {
	raw, err := os.ReadFile(s.metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]workflowMeta{}, nil
		}
		return nil, err
	}
	data := map[string]workflowMeta{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *WorkflowService) saveMetaNoLock(data map[string]workflowMeta) error {
	if err := os.MkdirAll(filepath.Dir(s.metaPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.metaPath), "wf-meta-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.metaPath)
}

func matchLabels(actual map[string]string, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}
	for k, v := range filter {
		if strings.TrimSpace(actual[k]) != strings.TrimSpace(v) {
			return false
		}
	}
	return true
}

func copyStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
