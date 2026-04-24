package envstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileStore struct {
	Dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) List(_ context.Context) ([]EnvironmentRecord, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}

	items := make([]EnvironmentRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		item, err := s.Get(context.Background(), name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *FileStore) Get(_ context.Context, name string) (EnvironmentRecord, error) {
	name, err := sanitizeEnvironmentName(name)
	if err != nil {
		return EnvironmentRecord{}, err
	}

	path := filepath.Join(s.Dir, name+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EnvironmentRecord{}, ErrNotFound
		}
		return EnvironmentRecord{}, err
	}

	var record EnvironmentRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return EnvironmentRecord{}, err
	}
	record.Name = name
	if record.Vars == nil {
		record.Vars = []EnvVar{}
	}
	return record, nil
}

func (s *FileStore) Create(ctx context.Context, record EnvironmentRecord) (EnvironmentRecord, error) {
	name, err := sanitizeEnvironmentName(record.Name)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	if _, err := s.Get(ctx, name); err == nil {
		return EnvironmentRecord{}, ErrExists
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return EnvironmentRecord{}, err
	}

	now := time.Now().UTC()
	record.Name = name
	record.CreatedAt = now
	record.UpdatedAt = now
	record.Vars = append([]EnvVar{}, record.Vars...)
	if err := s.write(record); err != nil {
		return EnvironmentRecord{}, err
	}
	return record, nil
}

func (s *FileStore) AddVar(_ context.Context, name string, envVar EnvVar) (EnvironmentRecord, error) {
	record, err := s.Get(context.Background(), name)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	for _, item := range record.Vars {
		if item.Key == envVar.Key {
			return EnvironmentRecord{}, ErrVarExist
		}
	}
	record.Vars = append(record.Vars, envVar)
	record.UpdatedAt = time.Now().UTC()
	if err := s.write(record); err != nil {
		return EnvironmentRecord{}, err
	}
	return record, nil
}

func (s *FileStore) UpdateVar(_ context.Context, name, key string, envVar EnvVar) (EnvironmentRecord, error) {
	record, err := s.Get(context.Background(), name)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	index := -1
	for i, item := range record.Vars {
		if item.Key == key {
			index = i
			break
		}
	}
	if index < 0 {
		return EnvironmentRecord{}, ErrVarMiss
	}
	record.Vars[index] = envVar
	record.UpdatedAt = time.Now().UTC()
	if err := s.write(record); err != nil {
		return EnvironmentRecord{}, err
	}
	return record, nil
}

func (s *FileStore) DeleteVar(_ context.Context, name, key string) (EnvironmentRecord, error) {
	record, err := s.Get(context.Background(), name)
	if err != nil {
		return EnvironmentRecord{}, err
	}
	filtered := make([]EnvVar, 0, len(record.Vars))
	found := false
	for _, item := range record.Vars {
		if item.Key == key {
			found = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !found {
		return EnvironmentRecord{}, ErrVarMiss
	}
	record.Vars = filtered
	record.UpdatedAt = time.Now().UTC()
	if err := s.write(record); err != nil {
		return EnvironmentRecord{}, err
	}
	return record, nil
}

func (s *FileStore) write(record EnvironmentRecord) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, record.Name+".json")
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func sanitizeEnvironmentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", ErrNotFound
	case strings.Contains(name, "/"), strings.Contains(name, "\\"), strings.Contains(name, ".."):
		return "", ErrNotFound
	default:
		return name, nil
	}
}
