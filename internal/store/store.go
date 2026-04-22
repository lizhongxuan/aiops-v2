// Package store provides data persistence with in-memory state and async JSON file writes.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
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

	// Workspace tasks
	GetWorkspaceTask(id string) (*runtimekernel.WorkspaceTask, error)
	ListWorkspaceTasks() ([]*runtimekernel.WorkspaceTask, error)
	SaveWorkspaceTask(task *runtimekernel.WorkspaceTask) error
	DeleteWorkspaceTask(id string) error

	// Approval audit log
	AppendApprovalAudit(record *runtimekernel.ApprovalRecord) error
	ListApprovalAudits() ([]*runtimekernel.ApprovalRecord, error)

	// UI cards
	GetUICards() ([]UICard, error)
	SaveUICards(cards []UICard) error

	// LLM config
	GetLLMConfig() (*LLMConfig, error)
	SaveLLMConfig(config *LLMConfig) error

	// Tool result spills
	GetToolResultSpill(id string) (*tooling.ResultSpill, error)
	ListToolResultSpills() ([]*tooling.ResultSpill, error)
	SaveToolResultSpill(spill *tooling.ResultSpill) error
	DeleteToolResultSpill(id string) error

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
	tasks    map[string]*runtimekernel.WorkspaceTask
	audits   []*runtimekernel.ApprovalRecord
	uiCards  []UICard
	llmCfg   *LLMConfig
	spills   map[string]*tooling.ResultSpill

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
		tasks:    make(map[string]*runtimekernel.WorkspaceTask),
		dirty:    make(map[string]bool),
		spills:   make(map[string]*tooling.ResultSpill),
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
	return cloneSessionState(sess)
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
	cp, err := cloneSessionState(session)
	if err != nil {
		return err
	}
	s.sessions[cp.ID] = cp
	s.dirty["session:"+cp.ID] = true
	return nil
}

func (s *JSONFileStore) ListSessions() ([]*runtimekernel.SessionState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*runtimekernel.SessionState, 0, len(s.sessions))
	for _, sess := range s.sessions {
		cp, err := cloneSessionState(sess)
		if err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
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

func (s *JSONFileStore) GetWorkspaceTask(id string) (*runtimekernel.WorkspaceTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("workspace task %q not found", id)
	}
	return cloneWorkspaceTask(task)
}

func (s *JSONFileStore) SaveWorkspaceTask(task *runtimekernel.WorkspaceTask) error {
	if task == nil {
		return fmt.Errorf("task is nil")
	}
	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	cp, err := cloneWorkspaceTask(task)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	if cp.StartTime.IsZero() {
		cp.StartTime = cp.CreatedAt
	}
	cp.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[cp.ID] = cp
	s.dirty["task:"+cp.ID] = true
	return nil
}

func (s *JSONFileStore) ListWorkspaceTasks() ([]*runtimekernel.WorkspaceTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*runtimekernel.WorkspaceTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		cp, err := cloneWorkspaceTask(task)
		if err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (s *JSONFileStore) DeleteWorkspaceTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; !ok {
		return fmt.Errorf("workspace task %q not found", id)
	}
	delete(s.tasks, id)
	s.dirty["delete_task:"+id] = true
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
// Tool result spills
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetToolResultSpill(id string) (*tooling.ResultSpill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spill, ok := s.spills[id]
	if !ok {
		return nil, fmt.Errorf("tool result spill %q not found", id)
	}
	return cloneToolResultSpill(spill)
}

func (s *JSONFileStore) ListToolResultSpills() ([]*tooling.ResultSpill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*tooling.ResultSpill, 0, len(s.spills))
	for _, spill := range s.spills {
		cp, err := cloneToolResultSpill(spill)
		if err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	return result, nil
}

func (s *JSONFileStore) SaveToolResultSpill(spill *tooling.ResultSpill) error {
	if spill == nil {
		return fmt.Errorf("spill is nil")
	}
	if spill.ID == "" {
		return fmt.Errorf("spill id is required")
	}
	cp, err := cloneToolResultSpill(spill)
	if err != nil {
		return err
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now().UTC()
	}
	if cp.Bytes == 0 && len(cp.Content) > 0 {
		cp.Bytes = int64(len(cp.Content))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.spills[cp.ID] = cp
	s.dirty["spill:"+cp.ID] = true
	return nil
}

func (s *JSONFileStore) DeleteToolResultSpill(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.spills[id]; !ok {
		return fmt.Errorf("tool result spill %q not found", id)
	}
	delete(s.spills, id)
	s.dirty["delete_spill:"+id] = true
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
		case len(key) > 5 && key[:5] == "task:":
			id := key[5:]
			task, ok := s.tasks[id]
			if !ok {
				continue
			}
			if err := s.writeJSON(filepath.Join("workspace-tasks", id+".json"), task); err != nil {
				return err
			}
		case len(key) > 12 && key[:12] == "delete_task:":
			id := key[12:]
			path := filepath.Join(s.dataDir, "workspace-tasks", id+".json")
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
		case len(key) > 6 && key[:6] == "spill:":
			id := key[6:]
			spill, ok := s.spills[id]
			if !ok {
				continue
			}
			if err := s.writeJSON(filepath.Join("tool-spills", id+".json"), spill); err != nil {
				return err
			}
		case len(key) > 13 && key[:13] == "delete_spill:":
			id := key[13:]
			path := filepath.Join(s.dataDir, "tool-spills", id+".json")
			_ = os.Remove(path)
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

	taskDir := filepath.Join(s.dataDir, "workspace-tasks")
	taskEntries, err := os.ReadDir(taskDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range taskEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(taskDir, entry.Name()))
		if err != nil {
			continue
		}
		var task runtimekernel.WorkspaceTask
		if err := json.Unmarshal(raw, &task); err != nil {
			continue
		}
		s.tasks[task.ID] = &task
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

	// Load tool result spills
	spillDir := filepath.Join(s.dataDir, "tool-spills")
	spillEntries, err := os.ReadDir(spillDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range spillEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(spillDir, entry.Name()))
		if err != nil {
			continue
		}
		var spill tooling.ResultSpill
		if err := json.Unmarshal(raw, &spill); err != nil {
			continue
		}
		s.spills[spill.ID] = &spill
	}

	return nil
}

func cloneSessionState(src *runtimekernel.SessionState) (*runtimekernel.SessionState, error) {
	if src == nil {
		return nil, nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst runtimekernel.SessionState
	if err := json.Unmarshal(raw, &dst); err != nil {
		return nil, err
	}
	return &dst, nil
}

func cloneToolResultSpill(src *tooling.ResultSpill) (*tooling.ResultSpill, error) {
	if src == nil {
		return nil, nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst tooling.ResultSpill
	if err := json.Unmarshal(raw, &dst); err != nil {
		return nil, err
	}
	return &dst, nil
}

func cloneWorkspaceTask(src *runtimekernel.WorkspaceTask) (*runtimekernel.WorkspaceTask, error) {
	if src == nil {
		return nil, nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst runtimekernel.WorkspaceTask
	if err := json.Unmarshal(raw, &dst); err != nil {
		return nil, err
	}
	return &dst, nil
}
