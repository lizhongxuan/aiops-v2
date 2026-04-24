package skillstore

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

func (s *FileStore) List(_ context.Context) ([]SkillRecord, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}

	items := make([]SkillRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
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

func (s *FileStore) Get(_ context.Context, name string) (SkillRecord, error) {
	name, err := sanitizeSkillName(name)
	if err != nil {
		return SkillRecord{}, err
	}

	contentPath := filepath.Join(s.Dir, name+".md")
	metaPath := filepath.Join(s.Dir, name+".meta.json")
	content, err := os.ReadFile(contentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return SkillRecord{}, ErrNotFound
		}
		return SkillRecord{}, err
	}

	record := SkillRecord{
		Name:    name,
		Content: string(content),
	}
	metaRaw, err := os.ReadFile(metaPath)
	if err == nil {
		if err := json.Unmarshal(metaRaw, &record); err != nil {
			return SkillRecord{}, err
		}
		record.Name = name
	} else if !os.IsNotExist(err) {
		return SkillRecord{}, err
	}

	return record, nil
}

func (s *FileStore) Create(ctx context.Context, record SkillRecord) (SkillRecord, error) {
	name, err := sanitizeSkillName(record.Name)
	if err != nil {
		return SkillRecord{}, err
	}
	if _, err := s.Get(ctx, name); err == nil {
		return SkillRecord{}, ErrExists
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return SkillRecord{}, err
	}

	now := time.Now().UTC()
	record.Name = name
	record.CreatedAt = now
	record.UpdatedAt = now
	if err := s.write(record); err != nil {
		return SkillRecord{}, err
	}
	return record, nil
}

func (s *FileStore) Update(_ context.Context, name string, record SkillRecord) (SkillRecord, error) {
	name, err := sanitizeSkillName(name)
	if err != nil {
		return SkillRecord{}, err
	}
	existing, err := s.Get(context.Background(), name)
	if err != nil {
		return SkillRecord{}, err
	}
	existing.Description = record.Description
	existing.Content = record.Content
	existing.Triggers = append([]string{}, record.Triggers...)
	existing.UpdatedAt = time.Now().UTC()
	if err := s.write(existing); err != nil {
		return SkillRecord{}, err
	}
	return existing, nil
}

func (s *FileStore) Delete(_ context.Context, name string) error {
	name, err := sanitizeSkillName(name)
	if err != nil {
		return err
	}

	contentPath := filepath.Join(s.Dir, name+".md")
	metaPath := filepath.Join(s.Dir, name+".meta.json")
	if err := os.Remove(contentPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *FileStore) write(record SkillRecord) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}

	contentPath := filepath.Join(s.Dir, record.Name+".md")
	metaPath := filepath.Join(s.Dir, record.Name+".meta.json")
	if err := os.WriteFile(contentPath, []byte(record.Content), 0o644); err != nil {
		return err
	}

	meta := SkillRecord{
		Name:        record.Name,
		Description: record.Description,
		Triggers:    append([]string{}, record.Triggers...),
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, payload, 0o644)
}

func sanitizeSkillName(name string) (string, error) {
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
