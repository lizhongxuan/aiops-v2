// Package store provides data persistence with in-memory state and async JSON file writes.
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/agentui"
	"aiops-v2/internal/opsmanual"
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

	// Web settings
	GetWebSettings() (*WebSettings, error)
	SaveWebSettings(settings *WebSettings) error

	// Hosts
	GetHost(id string) (*HostRecord, error)
	ListHosts() ([]HostRecord, error)
	SaveHost(host *HostRecord) error
	DeleteHost(id string) error

	// MCP servers runtime page
	GetMCPServers() ([]MCPServerRecord, error)
	SaveMCPServers(items []MCPServerRecord) error

	// Agent profile & catalogs
	GetSkillCatalog() ([]SkillCatalogEntry, error)
	SaveSkillCatalog(items []SkillCatalogEntry) error
	GetAgentMCPCatalog() ([]AgentMCPCatalogEntry, error)
	SaveAgentMCPCatalog(items []AgentMCPCatalogEntry) error
	GetAgentProfiles() ([]AgentProfileRecord, error)
	SaveAgentProfiles(items []AgentProfileRecord) error

	// Agent event log and projection
	AppendAgentEvent(sessionID string, event agentui.AgentEvent) error
	ListAgentEvents(sessionID string, afterSeq int64) ([]agentui.AgentEvent, error)
	SaveAgentEventProjection(sessionID string, projection agentui.AgentEventProjection) error
	LoadAgentEventProjection(sessionID string) (agentui.AgentEventProjection, bool, error)

	// Tool result spills
	GetToolResultSpill(id string) (*tooling.ResultSpill, error)
	ListToolResultSpills() ([]*tooling.ResultSpill, error)
	SaveToolResultSpill(spill *tooling.ResultSpill) error
	DeleteToolResultSpill(id string) error

	// Ops manuals
	GetOpsManual(id string) (opsmanual.OpsManual, bool, error)
	ListOpsManuals() ([]opsmanual.OpsManual, error)
	SaveOpsManual(manual opsmanual.OpsManual) error
	DeleteOpsManual(id string) error
	GetOpsManualCandidate(id string) (opsmanual.ManualCandidate, bool, error)
	ListOpsManualCandidates() ([]opsmanual.ManualCandidate, error)
	SaveOpsManualCandidate(candidate opsmanual.ManualCandidate) error
	DeleteOpsManualCandidate(id string) error
	ListOpsManualRunRecords(manualID string, workflowID string, limit int) ([]opsmanual.RunRecord, error)
	SaveOpsManualRunRecord(record opsmanual.RunRecord) error

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
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Kind              string           `json:"kind"`
	Renderer          string           `json:"renderer"`
	RendererVersion   string           `json:"rendererVersion,omitempty"`
	SchemaVersion     string           `json:"schemaVersion,omitempty"`
	PayloadSchema     map[string]any   `json:"payloadSchema,omitempty"`
	MetadataSchema    map[string]any   `json:"metadataSchema,omitempty"`
	ActionPolicy      map[string]any   `json:"actionPolicy,omitempty"`
	DisplayPolicy     map[string]any   `json:"displayPolicy,omitempty"`
	RedactionPolicy   map[string]any   `json:"redactionPolicy,omitempty"`
	SamplePayloads    []map[string]any `json:"samplePayloads,omitempty"`
	BundleSupport     []string         `json:"bundleSupport,omitempty"`
	PlacementDefaults []string         `json:"placementDefaults,omitempty"`
	Summary           string           `json:"summary,omitempty"`
	Capabilities      []string         `json:"capabilities,omitempty"`
	TriggerTypes      []string         `json:"triggerTypes,omitempty"`
	EditableFields    []string         `json:"editableFields,omitempty"`
	Status            string           `json:"status"`
	BuiltIn           bool             `json:"builtIn"`
	Version           int              `json:"version"`
	CreatedAt         time.Time        `json:"createdAt"`
	UpdatedAt         time.Time        `json:"updatedAt"`
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

// SettingModelOption represents one selectable model option for the web UI.
type SettingModelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WebSettings stores lightweight application settings used by the web shell.
type WebSettings struct {
	Quota           string               `json:"quota,omitempty"`
	Model           string               `json:"model,omitempty"`
	ReasoningEffort string               `json:"reasoningEffort,omitempty"`
	Models          []SettingModelOption `json:"models,omitempty"`
}

// HostRecord stores one managed host entry for inventory-oriented pages.
type HostRecord struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Kind              string            `json:"kind,omitempty"`
	Address           string            `json:"address,omitempty"`
	Transport         string            `json:"transport,omitempty"`
	Status            string            `json:"status,omitempty"`
	Executable        bool              `json:"executable,omitempty"`
	TerminalCapable   bool              `json:"terminalCapable,omitempty"`
	OS                string            `json:"os,omitempty"`
	Arch              string            `json:"arch,omitempty"`
	AgentVersion      string            `json:"agentVersion,omitempty"`
	LastHeartbeat     string            `json:"lastHeartbeat,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	LastError         string            `json:"lastError,omitempty"`
	SSHUser           string            `json:"sshUser,omitempty"`
	SSHPort           int               `json:"sshPort,omitempty"`
	SSHCredentialRef  string            `json:"sshCredentialRef,omitempty"`
	AgentURL          string            `json:"agentUrl,omitempty"`
	AgentTokenRef     string            `json:"agentTokenRef,omitempty"`
	InstallState      string            `json:"installState,omitempty"`
	InstallRunID      string            `json:"installRunId,omitempty"`
	InstallWorkflowID string            `json:"installWorkflowId,omitempty"`
	InstallStep       string            `json:"installStep,omitempty"`
	ControlMode       string            `json:"controlMode,omitempty"`
	CreatedAt         time.Time         `json:"createdAt,omitempty"`
	UpdatedAt         time.Time         `json:"updatedAt,omitempty"`
}

// MCPServerRecord stores one MCP runtime server configuration and its latest
// UI-facing runtime status.
type MCPServerRecord struct {
	Name          string            `json:"name"`
	Transport     string            `json:"transport,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	URL           string            `json:"url,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Disabled      bool              `json:"disabled,omitempty"`
	Status        string            `json:"status,omitempty"`
	Error         string            `json:"error,omitempty"`
	ToolCount     int               `json:"toolCount,omitempty"`
	ResourceCount int               `json:"resourceCount,omitempty"`
	CreatedAt     time.Time         `json:"createdAt,omitempty"`
	UpdatedAt     time.Time         `json:"updatedAt,omitempty"`
}

// SkillCatalogEntry stores one skill catalog item exposed to the Agent Profile
// surface.
type SkillCatalogEntry struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	Description           string    `json:"description,omitempty"`
	Source                string    `json:"source,omitempty"`
	DefaultEnabled        bool      `json:"defaultEnabled,omitempty"`
	DefaultActivationMode string    `json:"defaultActivationMode,omitempty"`
	CreatedAt             time.Time `json:"createdAt,omitempty"`
	UpdatedAt             time.Time `json:"updatedAt,omitempty"`
}

// AgentMCPCatalogEntry stores one MCP catalog item used by agent profiles.
type AgentMCPCatalogEntry struct {
	ID                           string    `json:"id"`
	Name                         string    `json:"name"`
	Type                         string    `json:"type,omitempty"`
	Source                       string    `json:"source,omitempty"`
	DefaultEnabled               bool      `json:"defaultEnabled,omitempty"`
	Permission                   string    `json:"permission,omitempty"`
	RequiresExplicitUserApproval bool      `json:"requiresExplicitUserApproval,omitempty"`
	CreatedAt                    time.Time `json:"createdAt,omitempty"`
	UpdatedAt                    time.Time `json:"updatedAt,omitempty"`
}

// AgentProfileRecord keeps one saved agent profile document in a JSON-friendly
// shape so the Web API can preserve its contract without a second model layer.
type AgentProfileRecord map[string]any

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
	webCfg   *WebSettings
	hosts    map[string]*HostRecord
	mcpSrv   []MCPServerRecord
	skillCat []SkillCatalogEntry
	agentMCP []AgentMCPCatalogEntry
	profiles []AgentProfileRecord
	spills   map[string]*tooling.ResultSpill

	opsManuals          map[string]opsmanual.OpsManual
	opsManualCandidates map[string]opsmanual.ManualCandidate
	opsManualRunRecords map[string]opsmanual.RunRecord
	opsManualGuidedChat map[string]opsmanual.ManualGuidedChatEvent

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
		dataDir:             dataDir,
		sessions:            make(map[string]*runtimekernel.SessionState),
		tasks:               make(map[string]*runtimekernel.WorkspaceTask),
		dirty:               make(map[string]bool),
		hosts:               make(map[string]*HostRecord),
		spills:              make(map[string]*tooling.ResultSpill),
		opsManuals:          make(map[string]opsmanual.OpsManual),
		opsManualCandidates: make(map[string]opsmanual.ManualCandidate),
		opsManualRunRecords: make(map[string]opsmanual.RunRecord),
		opsManualGuidedChat: make(map[string]opsmanual.ManualGuidedChatEvent),
		stopCh:              make(chan struct{}),
		doneCh:              make(chan struct{}),
		interval:            flushInterval,
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
// Web settings
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetWebSettings() (*WebSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.webCfg == nil {
		return nil, fmt.Errorf("web settings not found")
	}
	cp := cloneWebSettings(*s.webCfg)
	return &cp, nil
}

func (s *JSONFileStore) SaveWebSettings(settings *WebSettings) error {
	if settings == nil {
		return fmt.Errorf("settings is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneWebSettings(*settings)
	s.webCfg = &cp
	s.dirty["websettings"] = true
	return nil
}

// ---------------------------------------------------------------------------
// Hosts
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetHost(id string) (*HostRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	host, ok := s.hosts[id]
	if !ok {
		return nil, fmt.Errorf("host %q not found", id)
	}
	cp := cloneHostRecord(*host)
	return &cp, nil
}

func (s *JSONFileStore) ListHosts() ([]HostRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]HostRecord, 0, len(s.hosts))
	for _, host := range s.hosts {
		result = append(result, cloneHostRecord(*host))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (s *JSONFileStore) SaveHost(host *HostRecord) error {
	if host == nil {
		return fmt.Errorf("host is nil")
	}
	if host.ID == "" {
		return fmt.Errorf("host id is required")
	}
	cp := cloneHostRecord(*host)
	now := time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	s.hosts[cp.ID] = &cp
	s.dirty["host:"+cp.ID] = true
	return nil
}

func (s *JSONFileStore) DeleteHost(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hosts[id]; !ok {
		return fmt.Errorf("host %q not found", id)
	}
	delete(s.hosts, id)
	s.dirty["delete_host:"+id] = true
	return nil
}

// ---------------------------------------------------------------------------
// MCP servers runtime page
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetMCPServers() ([]MCPServerRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMCPServerRecords(s.mcpSrv), nil
}

func (s *JSONFileStore) SaveMCPServers(items []MCPServerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpSrv = cloneMCPServerRecords(items)
	s.dirty["mcpservers"] = true
	return nil
}

// ---------------------------------------------------------------------------
// Agent profile & catalogs
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetSkillCatalog() ([]SkillCatalogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSkillCatalogEntries(s.skillCat), nil
}

func (s *JSONFileStore) SaveSkillCatalog(items []SkillCatalogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skillCat = cloneSkillCatalogEntries(items)
	s.dirty["skillcatalog"] = true
	return nil
}

func (s *JSONFileStore) GetAgentMCPCatalog() ([]AgentMCPCatalogEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgentMCPCatalogEntries(s.agentMCP), nil
}

func (s *JSONFileStore) SaveAgentMCPCatalog(items []AgentMCPCatalogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentMCP = cloneAgentMCPCatalogEntries(items)
	s.dirty["agentmcpcatalog"] = true
	return nil
}

func (s *JSONFileStore) GetAgentProfiles() ([]AgentProfileRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgentProfileRecords(s.profiles)
}

func (s *JSONFileStore) SaveAgentProfiles(items []AgentProfileRecord) error {
	cp, err := cloneAgentProfileRecords(items)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles = cp
	s.dirty["agentprofiles"] = true
	return nil
}

// ---------------------------------------------------------------------------
// Agent event log and projection
// ---------------------------------------------------------------------------

func (s *JSONFileStore) AppendAgentEvent(sessionID string, event agentui.AgentEvent) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(event.SessionID)
	}
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return err
	}
	if event.SessionID == "" {
		event.SessionID = sessionID
	}
	if event.SessionID != sessionID {
		return fmt.Errorf("event session %q does not match target session %q", event.SessionID, sessionID)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.agentEventLogPath(sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *JSONFileStore) ListAgentEvents(sessionID string, afterSeq int64) ([]agentui.AgentEvent, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	file, err := os.Open(s.agentEventLogPath(sessionID))
	if os.IsNotExist(err) {
		return []agentui.AgentEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	events := []agentui.AgentEvent{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event agentui.AgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, err
		}
		if event.Seq > afterSeq {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Seq < events[j].Seq
	})
	return events, nil
}

func (s *JSONFileStore) SaveAgentEventProjection(sessionID string, projection agentui.AgentEventProjection) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(projection.SessionID)
	}
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return err
	}
	if projection.SessionID == "" {
		projection.SessionID = sessionID
	}
	if projection.SessionID != sessionID {
		return fmt.Errorf("projection session %q does not match target session %q", projection.SessionID, sessionID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONLocked(s.agentEventProjectionRelPath(sessionID), projection)
}

func (s *JSONFileStore) LoadAgentEventProjection(sessionID string) (agentui.AgentEventProjection, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return agentui.AgentEventProjection{}, false, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(s.agentEventProjectionPath(sessionID))
	if os.IsNotExist(err) {
		return agentui.AgentEventProjection{}, false, nil
	}
	if err != nil {
		return agentui.AgentEventProjection{}, false, err
	}
	var projection agentui.AgentEventProjection
	if err := json.Unmarshal(raw, &projection); err != nil {
		return agentui.AgentEventProjection{}, false, err
	}
	return projection, true, nil
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
// Ops manuals
// ---------------------------------------------------------------------------

func (s *JSONFileStore) GetOpsManual(id string) (opsmanual.OpsManual, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	manual, ok := s.opsManuals[id]
	if !ok {
		return opsmanual.OpsManual{}, false, nil
	}
	return cloneOpsManual(manual), true, nil
}

func (s *JSONFileStore) ListOpsManuals() ([]opsmanual.OpsManual, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]opsmanual.OpsManual, 0, len(s.opsManuals))
	for _, manual := range s.opsManuals {
		out = append(out, cloneOpsManual(manual))
	}
	sortOpsManuals(out)
	return out, nil
}

func (s *JSONFileStore) SaveOpsManual(manual opsmanual.OpsManual) error {
	if strings.TrimSpace(manual.ID) == "" {
		return fmt.Errorf("ops manual id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opsManuals[manual.ID] = cloneOpsManual(manual)
	s.dirty["opsmanual:"+manual.ID] = true
	return nil
}

func (s *JSONFileStore) DeleteOpsManual(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.opsManuals[id]; !ok {
		return fmt.Errorf("ops manual %q not found", id)
	}
	delete(s.opsManuals, id)
	s.dirty["delete_opsmanual:"+id] = true
	return nil
}

func (s *JSONFileStore) GetOpsManualCandidate(id string) (opsmanual.ManualCandidate, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidate, ok := s.opsManualCandidates[id]
	if !ok {
		return opsmanual.ManualCandidate{}, false, nil
	}
	return cloneOpsManualCandidate(candidate), true, nil
}

func (s *JSONFileStore) ListOpsManualCandidates() ([]opsmanual.ManualCandidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]opsmanual.ManualCandidate, 0, len(s.opsManualCandidates))
	for _, candidate := range s.opsManualCandidates {
		out = append(out, cloneOpsManualCandidate(candidate))
	}
	sortOpsManualCandidates(out)
	return out, nil
}

func (s *JSONFileStore) SaveOpsManualCandidate(candidate opsmanual.ManualCandidate) error {
	if strings.TrimSpace(candidate.ID) == "" {
		return fmt.Errorf("ops manual candidate id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opsManualCandidates[candidate.ID] = cloneOpsManualCandidate(candidate)
	s.dirty["opsmanualcandidate:"+candidate.ID] = true
	return nil
}

func (s *JSONFileStore) DeleteOpsManualCandidate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.opsManualCandidates[id]; !ok {
		return fmt.Errorf("ops manual candidate %q not found", id)
	}
	delete(s.opsManualCandidates, id)
	s.dirty["delete_opsmanualcandidate:"+id] = true
	return nil
}

func (s *JSONFileStore) ListOpsManualRunRecords(manualID string, workflowID string, limit int) ([]opsmanual.RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filterOpsManualRunRecords(s.opsManualRunRecords, manualID, workflowID, limit), nil
}

func (s *JSONFileStore) SaveOpsManualRunRecord(record opsmanual.RunRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("ops manual run record id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opsManualRunRecords[record.ID] = cloneOpsManualRunRecord(record)
	s.dirty["opsmanualrunrecord:"+record.ID] = true
	return nil
}

func (s *JSONFileStore) SaveManualGuidedChatEvent(event opsmanual.ManualGuidedChatEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return fmt.Errorf("ops manual manual-guided chat event id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opsManualGuidedChat[event.ID] = cloneManualGuidedChatEvent(event)
	s.dirty["opsmanualmanualguidedevent:"+event.ID] = true
	return nil
}

func (s *JSONFileStore) ListManualGuidedChatEvents(req opsmanual.ListManualGuidedChatEventsRequest) ([]opsmanual.ManualGuidedChatEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filterManualGuidedChatEvents(s.opsManualGuidedChat, req), nil
}

func (s *JSONFileStore) ListManuals(req opsmanual.ListManualsRequest) ([]opsmanual.OpsManual, error) {
	manuals, err := s.ListOpsManuals()
	if err != nil {
		return nil, err
	}
	return filterOpsManuals(manuals, req), nil
}

func (s *JSONFileStore) GetManual(id string) (opsmanual.OpsManual, error) {
	manual, ok, err := s.GetOpsManual(id)
	if err != nil {
		return opsmanual.OpsManual{}, err
	}
	if !ok {
		return opsmanual.OpsManual{}, fmt.Errorf("ops manual %q not found", id)
	}
	return manual, nil
}

func (s *JSONFileStore) SaveManual(manual opsmanual.OpsManual) error {
	return s.SaveOpsManual(manual)
}

func (s *JSONFileStore) ListRunRecords(req opsmanual.ListRunRecordsRequest) ([]opsmanual.RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filterOpsManualRunRecordsByRequest(s.opsManualRunRecords, req), nil
}

func (s *JSONFileStore) GetCandidate(id string) (opsmanual.ManualCandidate, error) {
	candidate, ok, err := s.GetOpsManualCandidate(id)
	if err != nil {
		return opsmanual.ManualCandidate{}, err
	}
	if !ok {
		return opsmanual.ManualCandidate{}, fmt.Errorf("ops manual candidate %q not found", id)
	}
	return candidate, nil
}

func (s *JSONFileStore) ListCandidates() ([]opsmanual.ManualCandidate, error) {
	return s.ListOpsManualCandidates()
}

func (s *JSONFileStore) SaveCandidate(candidate opsmanual.ManualCandidate) error {
	return s.SaveOpsManualCandidate(candidate)
}

func (s *JSONFileStore) DeleteCandidate(id string) error {
	return s.DeleteOpsManualCandidate(id)
}

func (s *JSONFileStore) SaveRunRecord(record opsmanual.RunRecord) error {
	return s.SaveOpsManualRunRecord(record)
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
		case key == "websettings":
			if err := s.writeJSON("web-settings.json", s.webCfg); err != nil {
				return err
			}
		case len(key) > 5 && key[:5] == "host:":
			id := key[5:]
			host, ok := s.hosts[id]
			if !ok {
				continue
			}
			if err := s.writeJSON(filepath.Join("hosts", id+".json"), host); err != nil {
				return err
			}
		case len(key) > 12 && key[:12] == "delete_host:":
			id := key[12:]
			path := filepath.Join(s.dataDir, "hosts", id+".json")
			_ = os.Remove(path)
		case key == "mcpservers":
			if err := s.writeJSON("mcp-servers.json", s.mcpSrv); err != nil {
				return err
			}
		case key == "skillcatalog":
			if err := s.writeJSON("agent-skills.json", s.skillCat); err != nil {
				return err
			}
		case key == "agentmcpcatalog":
			if err := s.writeJSON("agent-mcps.json", s.agentMCP); err != nil {
				return err
			}
		case key == "agentprofiles":
			if err := s.writeJSON("agent-profiles.json", s.profiles); err != nil {
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
		case len(key) > 10 && key[:10] == "opsmanual:":
			if err := s.writeJSON("ops-manuals.json", mapValues(s.opsManuals, cloneOpsManual)); err != nil {
				return err
			}
		case len(key) > 17 && key[:17] == "delete_opsmanual:":
			if err := s.writeJSON("ops-manuals.json", mapValues(s.opsManuals, cloneOpsManual)); err != nil {
				return err
			}
		case len(key) > 19 && key[:19] == "opsmanualcandidate:":
			if err := s.writeJSON("ops-manual-candidates.json", mapValues(s.opsManualCandidates, cloneOpsManualCandidate)); err != nil {
				return err
			}
		case len(key) > 26 && key[:26] == "delete_opsmanualcandidate:":
			if err := s.writeJSON("ops-manual-candidates.json", mapValues(s.opsManualCandidates, cloneOpsManualCandidate)); err != nil {
				return err
			}
		case len(key) > 19 && key[:19] == "opsmanualrunrecord:":
			if err := s.writeJSON("ops-manual-run-records.json", mapValues(s.opsManualRunRecords, cloneOpsManualRunRecord)); err != nil {
				return err
			}
		case len(key) > 27 && key[:27] == "opsmanualmanualguidedevent:":
			if err := s.writeJSON("ops-manual-manual-guided-events.json", mapValues(s.opsManualGuidedChat, cloneManualGuidedChatEvent)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *JSONFileStore) writeJSON(relPath string, data interface{}) error {
	return s.writeJSONLocked(relPath, data)
}

func (s *JSONFileStore) writeJSONLocked(relPath string, data interface{}) error {
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

func validateAgentEventSessionID(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.Contains(sessionID, "..") || strings.ContainsAny(sessionID, `/\`) {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	return nil
}

func (s *JSONFileStore) agentEventLogPath(sessionID string) string {
	return filepath.Join(s.dataDir, s.agentEventLogRelPath(sessionID))
}

func (s *JSONFileStore) agentEventLogRelPath(sessionID string) string {
	return filepath.Join("sessions", sessionID, "agent-events.jsonl")
}

func (s *JSONFileStore) agentEventProjectionPath(sessionID string) string {
	return filepath.Join(s.dataDir, s.agentEventProjectionRelPath(sessionID))
}

func (s *JSONFileStore) agentEventProjectionRelPath(sessionID string) string {
	return filepath.Join("sessions", sessionID, "agent-projection.json")
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

	// Load web settings
	webCfgPath := filepath.Join(s.dataDir, "web-settings.json")
	if raw, err := os.ReadFile(webCfgPath); err == nil {
		var cfg WebSettings
		if err := json.Unmarshal(raw, &cfg); err == nil {
			s.webCfg = &cfg
		}
	}

	// Load hosts
	hostDir := filepath.Join(s.dataDir, "hosts")
	hostEntries, err := os.ReadDir(hostDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range hostEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(hostDir, entry.Name()))
		if err != nil {
			continue
		}
		var host HostRecord
		if err := json.Unmarshal(raw, &host); err != nil {
			continue
		}
		s.hosts[host.ID] = &host
	}

	// Load MCP runtime servers
	mcpServersPath := filepath.Join(s.dataDir, "mcp-servers.json")
	if raw, err := os.ReadFile(mcpServersPath); err == nil {
		var items []MCPServerRecord
		if err := json.Unmarshal(raw, &items); err == nil {
			s.mcpSrv = cloneMCPServerRecords(items)
		}
	}

	// Load agent skill catalog
	skillCatalogPath := filepath.Join(s.dataDir, "agent-skills.json")
	if raw, err := os.ReadFile(skillCatalogPath); err == nil {
		var items []SkillCatalogEntry
		if err := json.Unmarshal(raw, &items); err == nil {
			s.skillCat = cloneSkillCatalogEntries(items)
		}
	}

	// Load agent MCP catalog
	agentMCPCatalogPath := filepath.Join(s.dataDir, "agent-mcps.json")
	if raw, err := os.ReadFile(agentMCPCatalogPath); err == nil {
		var items []AgentMCPCatalogEntry
		if err := json.Unmarshal(raw, &items); err == nil {
			s.agentMCP = cloneAgentMCPCatalogEntries(items)
		}
	}

	// Load agent profiles
	agentProfilesPath := filepath.Join(s.dataDir, "agent-profiles.json")
	if raw, err := os.ReadFile(agentProfilesPath); err == nil {
		var items []AgentProfileRecord
		if err := json.Unmarshal(raw, &items); err == nil {
			cloned, cloneErr := cloneAgentProfileRecords(items)
			if cloneErr == nil {
				s.profiles = cloned
			}
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

	if raw, err := os.ReadFile(filepath.Join(s.dataDir, "ops-manuals.json")); err == nil {
		var items []opsmanual.OpsManual
		if err := json.Unmarshal(raw, &items); err == nil {
			for _, item := range items {
				s.opsManuals[item.ID] = cloneOpsManual(item)
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(s.dataDir, "ops-manual-candidates.json")); err == nil {
		var items []opsmanual.ManualCandidate
		if err := json.Unmarshal(raw, &items); err == nil {
			for _, item := range items {
				s.opsManualCandidates[item.ID] = cloneOpsManualCandidate(item)
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(s.dataDir, "ops-manual-run-records.json")); err == nil {
		var items []opsmanual.RunRecord
		if err := json.Unmarshal(raw, &items); err == nil {
			for _, item := range items {
				s.opsManualRunRecords[item.ID] = cloneOpsManualRunRecord(item)
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(s.dataDir, "ops-manual-manual-guided-events.json")); err == nil {
		var items []opsmanual.ManualGuidedChatEvent
		if err := json.Unmarshal(raw, &items); err == nil {
			for _, item := range items {
				s.opsManualGuidedChat[item.ID] = cloneManualGuidedChatEvent(item)
			}
		}
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

func cloneWebSettings(src WebSettings) WebSettings {
	src.Models = append([]SettingModelOption(nil), src.Models...)
	return src
}

func cloneHostRecord(src HostRecord) HostRecord {
	if len(src.Labels) > 0 {
		labels := make(map[string]string, len(src.Labels))
		for key, value := range src.Labels {
			labels[key] = value
		}
		src.Labels = labels
	}
	return src
}

func cloneMCPServerRecord(src MCPServerRecord) MCPServerRecord {
	src.Args = append([]string(nil), src.Args...)
	if len(src.Env) > 0 {
		env := make(map[string]string, len(src.Env))
		for key, value := range src.Env {
			env[key] = value
		}
		src.Env = env
	}
	return src
}

func cloneMCPServerRecords(src []MCPServerRecord) []MCPServerRecord {
	out := make([]MCPServerRecord, 0, len(src))
	for _, item := range src {
		out = append(out, cloneMCPServerRecord(item))
	}
	return out
}

func cloneSkillCatalogEntries(src []SkillCatalogEntry) []SkillCatalogEntry {
	out := make([]SkillCatalogEntry, len(src))
	copy(out, src)
	return out
}

func cloneAgentMCPCatalogEntries(src []AgentMCPCatalogEntry) []AgentMCPCatalogEntry {
	out := make([]AgentMCPCatalogEntry, len(src))
	copy(out, src)
	return out
}

func cloneAgentProfileRecords(src []AgentProfileRecord) ([]AgentProfileRecord, error) {
	if len(src) == 0 {
		return []AgentProfileRecord{}, nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	var dst []AgentProfileRecord
	if err := json.Unmarshal(raw, &dst); err != nil {
		return nil, err
	}
	return dst, nil
}

func cloneOpsManual(src opsmanual.OpsManual) opsmanual.OpsManual {
	raw, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst opsmanual.OpsManual
	if err := json.Unmarshal(raw, &dst); err != nil {
		return src
	}
	return dst
}

func cloneOpsManualCandidate(src opsmanual.ManualCandidate) opsmanual.ManualCandidate {
	raw, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst opsmanual.ManualCandidate
	if err := json.Unmarshal(raw, &dst); err != nil {
		return src
	}
	return dst
}

func cloneOpsManualRunRecord(src opsmanual.RunRecord) opsmanual.RunRecord {
	raw, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst opsmanual.RunRecord
	if err := json.Unmarshal(raw, &dst); err != nil {
		return src
	}
	return dst
}

func cloneManualGuidedChatEvent(src opsmanual.ManualGuidedChatEvent) opsmanual.ManualGuidedChatEvent {
	raw, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var dst opsmanual.ManualGuidedChatEvent
	if err := json.Unmarshal(raw, &dst); err != nil {
		return src
	}
	return dst
}

func mapValues[T any](items map[string]T, clone func(T) T) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		out = append(out, clone(item))
	}
	return out
}

func sortOpsManuals(items []opsmanual.OpsManual) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].Title < items[j].Title
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
}

func sortOpsManualCandidates(items []opsmanual.ManualCandidate) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
}

func filterOpsManualRunRecords(records map[string]opsmanual.RunRecord, manualID string, workflowID string, limit int) []opsmanual.RunRecord {
	return filterOpsManualRunRecordsByRequest(records, opsmanual.ListRunRecordsRequest{
		ManualID:   manualID,
		WorkflowID: workflowID,
		Limit:      limit,
	})
}

func filterOpsManualRunRecordsByRequest(records map[string]opsmanual.RunRecord, req opsmanual.ListRunRecordsRequest) []opsmanual.RunRecord {
	out := make([]opsmanual.RunRecord, 0, len(records))
	for _, record := range records {
		if req.OpsManualFlowID != "" && record.OpsManualFlowID != req.OpsManualFlowID {
			continue
		}
		if req.ManualID != "" && record.ManualID != req.ManualID {
			continue
		}
		if req.WorkflowID != "" && record.WorkflowID != req.WorkflowID {
			continue
		}
		out = append(out, cloneOpsManualRunRecord(record))
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].CompletedAt
		if left == "" {
			left = out[i].StartedAt
		}
		right := out[j].CompletedAt
		if right == "" {
			right = out[j].StartedAt
		}
		if left == right {
			return out[i].ID < out[j].ID
		}
		return left > right
	})
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func filterManualGuidedChatEvents(events map[string]opsmanual.ManualGuidedChatEvent, req opsmanual.ListManualGuidedChatEventsRequest) []opsmanual.ManualGuidedChatEvent {
	out := make([]opsmanual.ManualGuidedChatEvent, 0, len(events))
	for _, event := range events {
		if req.OpsManualFlowID != "" && event.OpsManualFlowID != req.OpsManualFlowID {
			continue
		}
		if req.SessionID != "" && event.SessionID != req.SessionID {
			continue
		}
		if req.ManualID != "" && event.ManualID != req.ManualID {
			continue
		}
		if req.WorkflowID != "" && event.WorkflowID != req.WorkflowID {
			continue
		}
		out = append(out, cloneManualGuidedChatEvent(event))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt == out[j].CreatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt > out[j].CreatedAt
	})
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func filterOpsManuals(manuals []opsmanual.OpsManual, req opsmanual.ListManualsRequest) []opsmanual.OpsManual {
	out := make([]opsmanual.OpsManual, 0, len(manuals))
	for _, manual := range manuals {
		if req.Status != "" && manual.Status != req.Status {
			continue
		}
		if req.TargetType != "" && !strings.EqualFold(manual.Operation.TargetType, req.TargetType) {
			continue
		}
		if req.Action != "" && !strings.EqualFold(manual.Operation.Action, req.Action) {
			continue
		}
		if req.Middleware != "" && !strings.EqualFold(manual.Applicability.Middleware, req.Middleware) {
			continue
		}
		if req.ExecutionSurface != "" && !containsFold(manual.Applicability.ExecutionSurface, req.ExecutionSurface) {
			continue
		}
		out = append(out, cloneOpsManual(manual))
	}
	sortOpsManuals(out)
	if req.Limit > 0 && len(out) > req.Limit {
		out = out[:req.Limit]
	}
	return out
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
