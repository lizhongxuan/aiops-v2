package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type JSONStore struct {
	mu      sync.Mutex
	path    string
	enabled bool
	items   []Item
}

func NewJSONStore(cfg Config) *JSONStore {
	return &JSONStore{path: strings.TrimSpace(cfg.Path), enabled: cfg.Enabled}
}

func (s *JSONStore) Put(ctx context.Context, item Item) (Item, error) {
	_ = ctx
	if !s.enabled {
		return Item{}, ErrDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return Item{}, err
	}
	now := time.Now().UTC()
	if item.ID == "" {
		item.ID = memoryID(item)
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.Text = redactSecrets(item.Text)
	s.items = append(s.items, item)
	if err := s.saveLocked(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *JSONStore) Search(ctx context.Context, query Query) ([]Item, error) {
	_ = ctx
	if !s.enabled {
		return nil, ErrDisabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	tokens := queryTokens(query.Text)
	now := time.Now().UTC()
	var matches []Item
	for i := range s.items {
		item := &s.items[i]
		if item.Stale || !scopeMatches(*item, query) || !matchesTokens(item.Text, tokens) {
			continue
		}
		item.UsageCount++
		item.LastUsedAt = now
		matches = append(matches, *item)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].UsageCount != matches[j].UsageCount {
			return matches[i].UsageCount > matches[j].UsageCount
		}
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})
	if query.Limit > 0 && len(matches) > query.Limit {
		matches = matches[:query.Limit]
	}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return matches, nil
}

func (s *JSONStore) loadLocked() error {
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read memory store: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("parse memory store: %w", err)
	}
	s.items = items
	return nil
}

func (s *JSONStore) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create memory store dir: %w", err)
	}
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory store: %w", err)
	}
	return os.WriteFile(s.path, append(data, '\n'), 0o644)
}

func memoryID(item Item) string {
	sum := sha256.Sum256([]byte(string(item.Scope) + "\x00" + item.SessionID + "\x00" + item.ProjectID + "\x00" + item.Text))
	return "mem-" + hex.EncodeToString(sum[:])[:16]
}

func scopeMatches(item Item, query Query) bool {
	if query.Scope != "" && item.Scope != query.Scope {
		return false
	}
	if query.SessionID != "" && item.SessionID != query.SessionID {
		return false
	}
	if query.ProjectID != "" && item.ProjectID != query.ProjectID {
		return false
	}
	return true
}

func queryTokens(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= '\u4e00' && r <= '\u9fff')
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) >= 2 {
			out = append(out, field)
		}
	}
	return out
}

func matchesTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	lower := strings.ToLower(text)
	for _, token := range tokens {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

var secretPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)

func redactSecrets(text string) string {
	return secretPattern.ReplaceAllString(text, "$1=[REDACTED]")
}
