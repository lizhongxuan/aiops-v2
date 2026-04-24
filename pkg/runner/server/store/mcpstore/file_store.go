package mcpstore

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

func (s *FileStore) List(_ context.Context) ([]ServerRecord, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}

	items := make([]ServerRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		item, err := s.Get(context.Background(), id)
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

func (s *FileStore) Get(_ context.Context, id string) (ServerRecord, error) {
	id, err := sanitizeID(id)
	if err != nil {
		return ServerRecord{}, err
	}

	path := filepath.Join(s.Dir, id+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ServerRecord{}, ErrNotFound
		}
		return ServerRecord{}, err
	}

	var record ServerRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return ServerRecord{}, err
	}
	record.ID = id
	record.EnvVars = copyEnvVars(record.EnvVars)
	record.Tools = copyTools(record.Tools)
	if record.Status == "" {
		record.Status = StatusStopped
	}
	return record, nil
}

func (s *FileStore) Create(ctx context.Context, record ServerRecord) (ServerRecord, error) {
	id, err := sanitizeID(record.ID)
	if err != nil {
		return ServerRecord{}, err
	}
	if _, err := s.Get(ctx, id); err == nil {
		return ServerRecord{}, ErrExists
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return ServerRecord{}, err
	}

	now := time.Now().UTC()
	record.ID = id
	record.EnvVars = copyEnvVars(record.EnvVars)
	record.Tools = copyTools(record.Tools)
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.Status == "" {
		record.Status = StatusStopped
	}
	if err := s.write(record); err != nil {
		return ServerRecord{}, err
	}
	return record, nil
}

func (s *FileStore) Update(ctx context.Context, id string, record ServerRecord) (ServerRecord, error) {
	id, err := sanitizeID(id)
	if err != nil {
		return ServerRecord{}, err
	}
	current, err := s.Get(ctx, id)
	if err != nil {
		return ServerRecord{}, err
	}

	record.ID = id
	record.CreatedAt = current.CreatedAt
	record.UpdatedAt = time.Now().UTC()
	record.EnvVars = copyEnvVars(record.EnvVars)
	record.Tools = copyTools(record.Tools)
	if record.Status == "" {
		record.Status = current.Status
	}
	if err := s.write(record); err != nil {
		return ServerRecord{}, err
	}
	return record, nil
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	id, err := sanitizeID(id)
	if err != nil {
		return err
	}
	path := filepath.Join(s.Dir, id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *FileStore) write(record ServerRecord) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, record.ID+".json"), payload, 0o644)
}

func sanitizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	switch {
	case id == "":
		return "", ErrNotFound
	case strings.Contains(id, "/"), strings.Contains(id, "\\"), strings.Contains(id, ".."):
		return "", ErrNotFound
	default:
		return id, nil
	}
}

func copyEnvVars(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		out[key] = value
	}
	return out
}

func copyTools(items []ToolRecord) []ToolRecord {
	if len(items) == 0 {
		return []ToolRecord{}
	}
	out := make([]ToolRecord, 0, len(items))
	for _, item := range items {
		out = append(out, ToolRecord{
			Name:             item.Name,
			Description:      item.Description,
			ParametersSchema: copyAnyMap(item.ParametersSchema),
		})
	}
	return out
}

func copyAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
