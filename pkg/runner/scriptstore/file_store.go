package scriptstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FileStore struct {
	Dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) List(_ context.Context, filter Filter) ([]Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}

	lang := normalizeLanguage(filter.Language)
	tag := strings.TrimSpace(filter.Tag)
	out := make([]Script, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var script Script
		if err := json.Unmarshal(raw, &script); err != nil {
			return nil, err
		}
		if lang != "" && normalizeLanguage(script.Language) != lang {
			continue
		}
		if tag != "" && !contains(script.Tags, tag) {
			continue
		}
		out = append(out, cloneScript(script))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (s *FileStore) Get(_ context.Context, name string) (Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(name)
	if err != nil {
		return Script{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Script{}, ErrNotFound
		}
		return Script{}, err
	}
	var script Script
	if err := json.Unmarshal(raw, &script); err != nil {
		return Script{}, err
	}
	return cloneScript(script), nil
}

func (s *FileStore) Create(_ context.Context, script Script) (Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDir(); err != nil {
		return Script{}, err
	}
	name, err := sanitizeName(script.Name)
	if err != nil {
		return Script{}, err
	}
	script.Name = name
	script.Language = normalizeLanguage(script.Language)
	if err := validateLanguage(script.Language); err != nil {
		return Script{}, err
	}
	if strings.TrimSpace(script.Content) == "" {
		return Script{}, fmt.Errorf("script content is required")
	}
	path, err := s.path(script.Name)
	if err != nil {
		return Script{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return Script{}, ErrExists
	}
	now := time.Now().UTC()
	script.Version = 1
	script.Checksum = calculateChecksum(script.Content)
	script.CreatedAt = now
	script.UpdatedAt = now

	if err := s.writeScript(path, script); err != nil {
		return Script{}, err
	}
	return cloneScript(script), nil
}

func (s *FileStore) Update(_ context.Context, name string, script Script) (Script, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(name)
	if err != nil {
		return Script{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Script{}, ErrNotFound
		}
		return Script{}, err
	}
	var existing Script
	if err := json.Unmarshal(raw, &existing); err != nil {
		return Script{}, err
	}

	existing.Description = script.Description
	if script.Tags != nil {
		existing.Tags = append([]string{}, script.Tags...)
	}
	if trimmed := strings.TrimSpace(script.Content); trimmed != "" {
		existing.Content = script.Content
	}
	if language := normalizeLanguage(script.Language); language != "" {
		if err := validateLanguage(language); err != nil {
			return Script{}, err
		}
		existing.Language = language
	}
	if strings.TrimSpace(existing.Content) == "" {
		return Script{}, fmt.Errorf("script content is required")
	}
	existing.Version++
	existing.UpdatedAt = time.Now().UTC()
	existing.Checksum = calculateChecksum(existing.Content)

	if err := s.writeScript(path, existing); err != nil {
		return Script{}, err
	}
	return cloneScript(existing), nil
}

func (s *FileStore) Delete(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *FileStore) ensureDir() error {
	if strings.TrimSpace(s.Dir) == "" {
		return fmt.Errorf("scripts dir is empty")
	}
	return os.MkdirAll(s.Dir, 0o755)
}

func (s *FileStore) path(name string) (string, error) {
	safe, err := sanitizeName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, safe+".json"), nil
}

func (s *FileStore) writeScript(path string, script Script) error {
	payload, err := json.MarshalIndent(script, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "script-*.json")
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
	return os.Rename(tmpPath, path)
}

func sanitizeName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("script name is empty")
	}
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return "", fmt.Errorf("invalid script name %q", name)
	}
	return trimmed, nil
}

func normalizeLanguage(language string) string {
	return strings.ToLower(strings.TrimSpace(language))
}

func validateLanguage(language string) error {
	switch normalizeLanguage(language) {
	case "shell", "python":
		return nil
	default:
		return fmt.Errorf("unsupported language %q", language)
	}
}

func calculateChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func contains(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func cloneScript(input Script) Script {
	out := input
	out.Tags = append([]string{}, input.Tags...)
	return out
}
