// Package store provides data persistence with in-memory state and async JSON file writes.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"aiops-v2/internal/runtimekernel"
)

// ---------------------------------------------------------------------------
// Store interface
// ---------------------------------------------------------------------------

// Store defines the persistence interface for sessions, approvals, UI cards, and LLM config.
type Store interface {
	// Session CRUD
	GetSession(id string) (*runtimekernel.SessionState, error)
	SaveSession(session *runtimekernel.SessionState) error
	ListSessions() ([]*runtimekernel.SessionState, error)
	DeleteSession(id string) error

	// Approval audit log
	AppendApprovalAudit(record *runtimekernel.ApprovalRecord) error
	ListApprovalAudits() ([]*runtimekernel.ApprovalRecord, error)

	// UI cards
	GetUICards() ([]UICard, error)
	SaveUICards(cards []UICard) error

	// LLM config
	GetLLMConfig() (*LLMConfig, error)
	SaveLLMConfig(config *LLMConfig) error

	// Flush forces an immediate write of all dirty state to disk.
	Flush() error

	// Close stops the async writer and flushes remaining state.
	Close() error
}

// ---------------------------------------------------------------------------
// Supporting types
// ---------------------------------------------------------------------------

// UICard represents a UI card definition persisted in ui-cards.json.
type UICard struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Kind              string    `json:"kind"`
	Renderer          string    `json:"renderer"`
	BundleSupport     []string  `json:"bundleSupport,omitempty"`
	PlacementDefaults []string  `json:"placementDefaults,omitempty"`
	Summary           string    `json:"summary,omitempty"`
	Capabilities      []string  `json:"capabilities,omitempty"`
	TriggerTypes      []string  `json:"triggerTypes,omitempty"`
	EditableFields    []string  `json:"editableFields,omitempty"`
	Status            string    `json:"status"`
	BuiltIn           bool      `json:"builtIn"`
	Version           int       `json:"version"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// LLMConfig represents the LLM provider configuration persisted in llm-config.json.
type LLMConfig struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	APIKey           string `json:"apiKey"`
	BaseURL          string `json:"baseURL"`
	FallbackProvider string `json:"fallbackProvider"`
	FallbackModel    string `json:"fallbackModel"`
	FallbackAPIKey   string `json:"fallbackApiKey"`
	CompactModel     string `json:"compactModel"`
}

// ---------------------------------------------------------------------------
// JSONFileStore implementation
// ---------------------------------------------------------------------------

// JSONFileStore implements Store with in-memory state and async JSON file persistence.
type JSONFileStore struct {
	mu       sync.RWMutex
	dataDir  string
	sessions map[string]*runtimekernel.SessionState
	audits   []*runtimekernel.ApprovalRecord
	uiCards  []UICard
	llmCfg   *LLMConfig

	// Async write control
	dirty    map[string]bool // tracks which data sets need flushing
	stopCh   chan struct{}
	doneCh   chan struct{}
	interval time.Duration
}


// NewJSONFileStore creates a new JSONFileStore rooted at dataDir.
// It loads existing state from disk and starts the async writer goroutine.
func NewJSONFileStore(dataDir string, flushInterval time.Duration) (*JSONFileStore, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "sessions"), 0755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}

	s := &JSONFileStore{
		dataDir:  dataDir,
		sessions: make(map[string]*runtimekernel.SessionState),
		dirty:    make(map[string]bool),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		interval: flushInterval,
	}

	if err := s.loadFromDisk(); err != nil {
		return nil, fmt.Errorf("load from disk: %w", err)
	}

	go s.asyncWriter()
	return s, nil
}

// ---------------------------------------------------------------------------
// Session CRUD
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetSession(id string) (*runtimekernel.SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	cp := *sess
	return &cp, nil
}

func (s *JSONFileStore) SaveSession(session *runtimekernel.SessionState) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if session.ID == "" {
		return fmt.Errorf("session id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *session
	s.sessions[cp.ID] = &cp
	s.dirty["session:"+cp.ID] = true
	return nil
}

func (s *JSONFileStore) ListSessions() ([]*runtimekernel.SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*runtimekernel.SessionState, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp := *sess
		result = append(result, &cp)
	}
	return result, nil
}

func (s *JSONFileStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(s.sessions, id)
	s.dirty["delete_session:"+id] = true
	return nil
}

// ---------------------------------------------------------------------------
// Approval audit log
// ---------------------------------------------------------------------------

func (s *JSONFileStore) AppendApprovalAudit(record *runtimekernel.ApprovalRecord) error {
	if record == nil {
		return fmt.Errorf("record is nil")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, record)
	s.dirty["audits"] = true
	return nil
}

func (s *JSONFileStore) ListApprovalAudits() ([]*runtimekernel.ApprovalRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*runtimekernel.ApprovalRecord, len(s.audits))
	copy(result, s.audits)
	return result, nil
}

// ---------------------------------------------------------------------------
// UI cards
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetUICards() ([]UICard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]UICard, len(s.uiCards))
	copy(result, s.uiCards)
	return result, nil
}

func (s *JSONFileStore) SaveUICards(cards []UICard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uiCards = make([]UICard, len(cards))
	copy(s.uiCards, cards)
	s.dirty["uicards"] = true
	return nil
}

// ---------------------------------------------------------------------------
// LLM config
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetLLMConfig() (*LLMConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.llmCfg == nil {
		return nil, fmt.Errorf("llm config not found")
	}
	cp := *s.llmCfg
	return &cp, nil
}

func (s *JSONFileStore) SaveLLMConfig(config *LLMConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *config
	s.llmCfg = &cp
	s.dirty["llmconfig"] = true
	return nil
}

// ---------------------------------------------------------------------------
// Flush / Close
// ---------------------------------------------------------------------------

func (s *JSONFileStore) Flush() error {
	s.mu.Lock()
	dirtyKeys := make(map[string]bool, len(s.dirty))
	for k, v := range s.dirty {
		dirtyKeys[k] = v
	}
	s.dirty = make(map[string]bool)
	s.mu.Unlock()

	return s.writeDirty(dirtyKeys)
}

func (s *JSONFileStore) Close() error {
	close(s.stopCh)
	<-s.doneCh
	return s.Flush()
}

// ---------------------------------------------------------------------------
// Async writer
// ---------------------------------------------------------------------------

func (s *JSONFileStore) asyncWriter() {
	defer close(s.doneCh)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = s.Flush()
		case <-s.stopCh:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Disk I/O helpers
// ---------------------------------------------------------------------------

func (s *JSONFileStore) writeDirty(dirtyKeys map[string]bool) error {
	if len(dirtyKeys) == 0 {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for key := range dirtyKeys {
		switch {
		case len(key) > 8 && key[:8] == "session:":
			id := key[8:]
			sess, ok := s.sessions[id]
			if !ok {
				continue
			}
			if err := s.writeJSON(filepath.Join("sessions", id+".json"), sess); err != nil {
				return err
			}
		case len(key) > 15 && key[:15] == "delete_session:":
			id := key[15:]
			path := filepath.Join(s.dataDir, "sessions", id+".json")
			_ = os.Remove(path)
		case key == "audits":
			if err := s.writeJSON("approval-audits.json", s.audits); err != nil {
				return err
			}
		case key == "uicards":
			if err := s.writeJSON("ui-cards.json", s.uiCards); err != nil {
				return err
			}
		case key == "llmconfig":
			if err := s.writeJSON("llm-config.json", s.llmCfg); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *JSONFileStore) writeJSON(relPath string, data interface{}) error {
	path := filepath.Join(s.dataDir, relPath)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *JSONFileStore) loadFromDisk() error {
	// Load sessions
	sessDir := filepath.Join(s.dataDir, "sessions")
	entries, err := os.ReadDir(sessDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sessDir, entry.Name()))
		if err != nil {
			continue
		}
		var sess runtimekernel.SessionState
		if err := json.Unmarshal(raw, &sess); err != nil {
			continue
		}
		s.sessions[sess.ID] = &sess
	}

	// Load approval audits
	auditsPath := filepath.Join(s.dataDir, "approval-audits.json")
	if raw, err := os.ReadFile(auditsPath); err == nil {
		var audits []*runtimekernel.ApprovalRecord
		if err := json.Unmarshal(raw, &audits); err == nil {
			s.audits = audits
		}
	}

	// Load UI cards
	cardsPath := filepath.Join(s.dataDir, "ui-cards.json")
	if raw, err := os.ReadFile(cardsPath); err == nil {
		var cards []UICard
		if err := json.Unmarshal(raw, &cards); err == nil {
			s.uiCards = cards
		}
	}

	// Load LLM config
	cfgPath := filepath.Join(s.dataDir, "llm-config.json")
	if raw, err := os.ReadFile(cfgPath); err == nil {
		var cfg LLMConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			s.llmCfg = &cfg
		}
	}

	return nil
}
