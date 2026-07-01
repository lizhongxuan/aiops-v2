package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"aiops-v2/internal/agentui"
	"aiops-v2/internal/incidents"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

const (
	gormNamespaceSessions            = "sessions"
	gormNamespaceWorkspaceTasks      = "workspace_tasks"
	gormNamespaceApprovalAudits      = "approval_audits"
	gormNamespaceUICards             = "ui_cards"
	gormNamespaceLLMConfig           = "llm_config"
	gormNamespaceWebSettings         = "web_settings"
	gormNamespaceRuntimeSettings     = "runtime_settings"
	gormNamespaceCorootConfig        = "coroot_config"
	gormNamespaceHosts               = "hosts"
	gormNamespaceMCPServers          = "mcp_servers"
	gormNamespaceSkillCatalog        = "skill_catalog"
	gormNamespaceAgentMCPCatalog     = "agent_mcp_catalog"
	gormNamespaceAgentProfiles       = "agent_profiles"
	gormNamespaceToolSpills          = "tool_spills"
	gormNamespaceAgentProjections    = "agent_projections"
	gormNamespaceIncidents           = "incidents"
	gormNamespaceIncidentEvidence    = "incident_evidence"
	gormNamespaceOpsManuals          = "ops_manuals"
	gormNamespaceOpsManualCandidates = "ops_manual_candidates"
	gormNamespaceOpsManualRunRecords = "ops_manual_run_records"
	gormNamespaceOpsManualGuidedChat = "ops_manual_manual_guided_chat"
)

const gormSingletonKey = "current"

var _ Store = (*GormStore)(nil)
var _ incidents.Store = (*GormStore)(nil)
var _ opsmanual.ManualRepository = (*GormStore)(nil)
var _ opsmanual.ManualGuidedChatEventRepository = (*GormStore)(nil)

// GormStore implements Store using a GORM-backed database. Domain objects are
// stored as JSON payloads to preserve the existing application contract while
// allowing SQL durability and GORM-managed migrations.
type GormStore struct {
	db *gorm.DB
}

type gormKVRecord struct {
	Namespace string `gorm:"primaryKey;size:64"`
	RecordKey string `gorm:"primaryKey;column:record_key;size:255"`
	Payload   []byte
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (gormKVRecord) TableName() string { return "aiops_kv_records" }

type gormAgentEventRecord struct {
	ID        uint   `gorm:"primaryKey"`
	SessionID string `gorm:"index:idx_aiops_agent_events_session_seq,priority:1;size:128"`
	Seq       int64  `gorm:"index:idx_aiops_agent_events_session_seq,priority:2"`
	EventID   string `gorm:"size:128;index"`
	Payload   []byte
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

func (gormAgentEventRecord) TableName() string { return "aiops_agent_events" }

// NewGormStore creates a Store over an existing GORM connection and migrates
// the small persistence schema required by aiops-v2.
func NewGormStore(db *gorm.DB) (*GormStore, error) {
	if db == nil {
		return nil, fmt.Errorf("gorm db is nil")
	}
	if err := db.AutoMigrate(&gormKVRecord{}, &gormAgentEventRecord{}); err != nil {
		return nil, fmt.Errorf("migrate gorm store: %w", err)
	}
	return &GormStore{db: db}, nil
}

// NewPostgresStore opens a PostgreSQL-backed GORM store. The caller must supply
// a DSN; connection and migration errors are returned to startup.
func NewPostgresStore(dsn string) (*GormStore, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres gorm store: %w", err)
	}
	return NewGormStore(db)
}

func (s *GormStore) GetSession(id string) (*runtimekernel.SessionState, error) {
	var session runtimekernel.SessionState
	ok, err := s.loadKV(gormNamespaceSessions, id, &session)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("session %q not found", id)
	}
	return cloneSessionState(&session)
}

func (s *GormStore) SaveSession(session *runtimekernel.SessionState) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if session.ID == "" {
		return fmt.Errorf("session id is required")
	}
	cp, err := cloneSessionState(session)
	if err != nil {
		return err
	}
	return s.saveKV(gormNamespaceSessions, cp.ID, cp)
}

func (s *GormStore) ListSessions() ([]*runtimekernel.SessionState, error) {
	var sessions []runtimekernel.SessionState
	if err := s.listKV(gormNamespaceSessions, &sessions); err != nil {
		return nil, err
	}
	result := make([]*runtimekernel.SessionState, 0, len(sessions))
	for i := range sessions {
		cp, err := cloneSessionState(&sessions[i])
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

func (s *GormStore) DeleteSession(id string) error {
	return s.deleteRequiredKV(gormNamespaceSessions, id, "session")
}

func (s *GormStore) GetWorkspaceTask(id string) (*runtimekernel.WorkspaceTask, error) {
	var task runtimekernel.WorkspaceTask
	ok, err := s.loadKV(gormNamespaceWorkspaceTasks, id, &task)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("workspace task %q not found", id)
	}
	return cloneWorkspaceTask(&task)
}

func (s *GormStore) ListWorkspaceTasks() ([]*runtimekernel.WorkspaceTask, error) {
	var tasks []runtimekernel.WorkspaceTask
	if err := s.listKV(gormNamespaceWorkspaceTasks, &tasks); err != nil {
		return nil, err
	}
	result := make([]*runtimekernel.WorkspaceTask, 0, len(tasks))
	for i := range tasks {
		cp, err := cloneWorkspaceTask(&tasks[i])
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

func (s *GormStore) SaveWorkspaceTask(task *runtimekernel.WorkspaceTask) error {
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
	return s.saveKV(gormNamespaceWorkspaceTasks, cp.ID, cp)
}

func (s *GormStore) DeleteWorkspaceTask(id string) error {
	return s.deleteRequiredKV(gormNamespaceWorkspaceTasks, id, "workspace task")
}

func (s *GormStore) AppendApprovalAudit(record *runtimekernel.ApprovalRecord) error {
	if record == nil {
		return fmt.Errorf("record is nil")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	audits, err := s.ListApprovalAudits()
	if err != nil {
		return err
	}
	cp := *record
	audits = append(audits, &cp)
	return s.saveKV(gormNamespaceApprovalAudits, gormSingletonKey, audits)
}

func (s *GormStore) ListApprovalAudits() ([]*runtimekernel.ApprovalRecord, error) {
	var audits []*runtimekernel.ApprovalRecord
	ok, err := s.loadKV(gormNamespaceApprovalAudits, gormSingletonKey, &audits)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []*runtimekernel.ApprovalRecord{}, nil
	}
	result := make([]*runtimekernel.ApprovalRecord, len(audits))
	copy(result, audits)
	return result, nil
}

func (s *GormStore) GetUICards() ([]UICard, error) {
	var cards []UICard
	ok, err := s.loadKV(gormNamespaceUICards, gormSingletonKey, &cards)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []UICard{}, nil
	}
	result := make([]UICard, len(cards))
	copy(result, cards)
	return result, nil
}

func (s *GormStore) SaveUICards(cards []UICard) error {
	cp := make([]UICard, len(cards))
	copy(cp, cards)
	return s.saveKV(gormNamespaceUICards, gormSingletonKey, cp)
}

func (s *GormStore) GetLLMConfig() (*LLMConfig, error) {
	var config LLMConfig
	ok, err := s.loadKV(gormNamespaceLLMConfig, gormSingletonKey, &config)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("llm config not found")
	}
	return &config, nil
}

func (s *GormStore) SaveLLMConfig(config *LLMConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	cp := *config
	return s.saveKV(gormNamespaceLLMConfig, gormSingletonKey, cp)
}

func (s *GormStore) GetWebSettings() (*WebSettings, error) {
	var settings WebSettings
	ok, err := s.loadKV(gormNamespaceWebSettings, gormSingletonKey, &settings)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("web settings not found")
	}
	cp := cloneWebSettings(settings)
	return &cp, nil
}

func (s *GormStore) SaveWebSettings(settings *WebSettings) error {
	if settings == nil {
		return fmt.Errorf("settings is nil")
	}
	cp := cloneWebSettings(*settings)
	return s.saveKV(gormNamespaceWebSettings, gormSingletonKey, cp)
}

func (s *GormStore) GetRuntimeSettings() (*RuntimeSettings, error) {
	var settings RuntimeSettings
	ok, err := s.loadKV(gormNamespaceRuntimeSettings, gormSingletonKey, &settings)
	if err != nil {
		return nil, err
	}
	if !ok {
		defaults := DefaultRuntimeSettings()
		return &defaults, nil
	}
	cp := cloneRuntimeSettings(settings)
	return &cp, nil
}

func (s *GormStore) SaveRuntimeSettings(settings *RuntimeSettings) error {
	if settings == nil {
		return fmt.Errorf("settings is nil")
	}
	cp := cloneRuntimeSettings(*settings)
	cp.UpdatedAt = time.Now().UTC()
	return s.saveKV(gormNamespaceRuntimeSettings, gormSingletonKey, cp)
}

func (s *GormStore) GetCorootConfig() (*CorootConfig, error) {
	var config CorootConfig
	ok, err := s.loadKV(gormNamespaceCorootConfig, gormSingletonKey, &config)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("coroot config not found")
	}
	cp := cloneCorootConfig(config)
	return &cp, nil
}

func (s *GormStore) SaveCorootConfig(config *CorootConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	cp := cloneCorootConfig(*config)
	now := time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	cp.UpdatedAt = now
	return s.saveKV(gormNamespaceCorootConfig, gormSingletonKey, cp)
}

func (s *GormStore) GetHost(id string) (*HostRecord, error) {
	var host HostRecord
	ok, err := s.loadKV(gormNamespaceHosts, id, &host)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("host %q not found", id)
	}
	cp := cloneHostRecord(host)
	return &cp, nil
}

func (s *GormStore) ListHosts() ([]HostRecord, error) {
	var hosts []HostRecord
	if err := s.listKV(gormNamespaceHosts, &hosts); err != nil {
		return nil, err
	}
	result := make([]HostRecord, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, cloneHostRecord(host))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result, nil
}

func (s *GormStore) SaveHost(host *HostRecord) error {
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
	return s.saveKV(gormNamespaceHosts, cp.ID, cp)
}

func (s *GormStore) DeleteHost(id string) error {
	return s.deleteRequiredKV(gormNamespaceHosts, id, "host")
}

func (s *GormStore) GetMCPServers() ([]MCPServerRecord, error) {
	var items []MCPServerRecord
	ok, err := s.loadKV(gormNamespaceMCPServers, gormSingletonKey, &items)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []MCPServerRecord{}, nil
	}
	return cloneMCPServerRecords(items), nil
}

func (s *GormStore) SaveMCPServers(items []MCPServerRecord) error {
	return s.saveKV(gormNamespaceMCPServers, gormSingletonKey, cloneMCPServerRecords(items))
}

func (s *GormStore) GetSkillCatalog() ([]SkillCatalogEntry, error) {
	var items []SkillCatalogEntry
	ok, err := s.loadKV(gormNamespaceSkillCatalog, gormSingletonKey, &items)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []SkillCatalogEntry{}, nil
	}
	return cloneSkillCatalogEntries(items), nil
}

func (s *GormStore) SaveSkillCatalog(items []SkillCatalogEntry) error {
	return s.saveKV(gormNamespaceSkillCatalog, gormSingletonKey, cloneSkillCatalogEntries(items))
}

func (s *GormStore) GetAgentMCPCatalog() ([]AgentMCPCatalogEntry, error) {
	var items []AgentMCPCatalogEntry
	ok, err := s.loadKV(gormNamespaceAgentMCPCatalog, gormSingletonKey, &items)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []AgentMCPCatalogEntry{}, nil
	}
	return cloneAgentMCPCatalogEntries(items), nil
}

func (s *GormStore) SaveAgentMCPCatalog(items []AgentMCPCatalogEntry) error {
	return s.saveKV(gormNamespaceAgentMCPCatalog, gormSingletonKey, cloneAgentMCPCatalogEntries(items))
}

func (s *GormStore) GetAgentProfiles() ([]AgentProfileRecord, error) {
	var items []AgentProfileRecord
	ok, err := s.loadKV(gormNamespaceAgentProfiles, gormSingletonKey, &items)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []AgentProfileRecord{}, nil
	}
	return cloneAgentProfileRecords(items)
}

func (s *GormStore) SaveAgentProfiles(items []AgentProfileRecord) error {
	cp, err := cloneAgentProfileRecords(items)
	if err != nil {
		return err
	}
	return s.saveKV(gormNamespaceAgentProfiles, gormSingletonKey, cp)
}

func (s *GormStore) AppendAgentEvent(sessionID string, event agentui.AgentEvent) error {
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
	return s.db.Create(&gormAgentEventRecord{
		SessionID: sessionID,
		Seq:       event.Seq,
		EventID:   event.EventID,
		Payload:   raw,
		CreatedAt: time.Now().UTC(),
	}).Error
}

func (s *GormStore) ListAgentEvents(sessionID string, afterSeq int64) ([]agentui.AgentEvent, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return nil, err
	}
	var records []gormAgentEventRecord
	if err := s.db.Where("session_id = ? AND seq > ?", sessionID, afterSeq).
		Order("seq ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]agentui.AgentEvent, 0, len(records))
	for _, record := range records {
		var event agentui.AgentEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *GormStore) SaveAgentEventProjection(sessionID string, projection agentui.AgentEventProjection) error {
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
	return s.saveKV(gormNamespaceAgentProjections, sessionID, projection)
}

func (s *GormStore) LoadAgentEventProjection(sessionID string) (agentui.AgentEventProjection, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if err := validateAgentEventSessionID(sessionID); err != nil {
		return agentui.AgentEventProjection{}, false, err
	}
	var projection agentui.AgentEventProjection
	ok, err := s.loadKV(gormNamespaceAgentProjections, sessionID, &projection)
	return projection, ok, err
}

func (s *GormStore) GetToolResultSpill(id string) (*tooling.ResultSpill, error) {
	var spill tooling.ResultSpill
	ok, err := s.loadKV(gormNamespaceToolSpills, id, &spill)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("tool result spill %q not found", id)
	}
	return cloneToolResultSpill(&spill)
}

func (s *GormStore) ListToolResultSpills() ([]*tooling.ResultSpill, error) {
	var spills []tooling.ResultSpill
	if err := s.listKV(gormNamespaceToolSpills, &spills); err != nil {
		return nil, err
	}
	result := make([]*tooling.ResultSpill, 0, len(spills))
	for i := range spills {
		cp, err := cloneToolResultSpill(&spills[i])
		if err != nil {
			return nil, err
		}
		result = append(result, cp)
	}
	return result, nil
}

func (s *GormStore) SaveToolResultSpill(spill *tooling.ResultSpill) error {
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
	return s.saveKV(gormNamespaceToolSpills, cp.ID, cp)
}

func (s *GormStore) DeleteToolResultSpill(id string) error {
	return s.deleteRequiredKV(gormNamespaceToolSpills, id, "tool result spill")
}

func (s *GormStore) PutIncident(incident incidents.IncidentCase) {
	if strings.TrimSpace(incident.ID) == "" {
		return
	}
	_ = s.saveKV(gormNamespaceIncidents, incident.ID, cloneGormIncident(incident))
}

func (s *GormStore) GetIncident(id string) (incidents.IncidentCase, bool) {
	var incident incidents.IncidentCase
	ok, err := s.loadKV(gormNamespaceIncidents, strings.TrimSpace(id), &incident)
	if err != nil || !ok {
		return incidents.IncidentCase{}, false
	}
	return cloneGormIncident(incident), true
}

func (s *GormStore) ListIncidents() []incidents.IncidentCase {
	var items []incidents.IncidentCase
	if err := s.listKV(gormNamespaceIncidents, &items); err != nil {
		return []incidents.IncidentCase{}
	}
	out := make([]incidents.IncidentCase, 0, len(items))
	for _, item := range items {
		out = append(out, cloneGormIncident(item))
	}
	return out
}

func (s *GormStore) PutEvidence(incidentID string, evidence incidents.EvidenceRef) {
	incidentID = strings.TrimSpace(incidentID)
	if incidentID == "" || strings.TrimSpace(evidence.ID) == "" {
		return
	}
	items := s.ListEvidence(incidentID)
	items = append(items, evidence)
	_ = s.saveKV(gormNamespaceIncidentEvidence, incidentID, items)
}

func (s *GormStore) ListEvidence(incidentID string) []incidents.EvidenceRef {
	var items []incidents.EvidenceRef
	ok, err := s.loadKV(gormNamespaceIncidentEvidence, strings.TrimSpace(incidentID), &items)
	if err != nil || !ok {
		return []incidents.EvidenceRef{}
	}
	return append([]incidents.EvidenceRef(nil), items...)
}

func (s *GormStore) GetOpsManual(id string) (opsmanual.OpsManual, bool, error) {
	var manual opsmanual.OpsManual
	ok, err := s.loadKV(gormNamespaceOpsManuals, id, &manual)
	if err != nil || !ok {
		return opsmanual.OpsManual{}, ok, err
	}
	return cloneOpsManual(manual), true, nil
}

func (s *GormStore) ListOpsManuals() ([]opsmanual.OpsManual, error) {
	var manuals []opsmanual.OpsManual
	if err := s.listKV(gormNamespaceOpsManuals, &manuals); err != nil {
		return nil, err
	}
	out := make([]opsmanual.OpsManual, 0, len(manuals))
	for _, manual := range manuals {
		out = append(out, cloneOpsManual(manual))
	}
	sortOpsManuals(out)
	return out, nil
}

func (s *GormStore) SaveOpsManual(manual opsmanual.OpsManual) error {
	if strings.TrimSpace(manual.ID) == "" {
		return fmt.Errorf("ops manual id is required")
	}
	return s.saveKV(gormNamespaceOpsManuals, manual.ID, cloneOpsManual(manual))
}

func (s *GormStore) DeleteOpsManual(id string) error {
	return s.deleteRequiredKV(gormNamespaceOpsManuals, id, "ops manual")
}

func (s *GormStore) GetOpsManualCandidate(id string) (opsmanual.ManualCandidate, bool, error) {
	var candidate opsmanual.ManualCandidate
	ok, err := s.loadKV(gormNamespaceOpsManualCandidates, id, &candidate)
	if err != nil || !ok {
		return opsmanual.ManualCandidate{}, ok, err
	}
	return cloneOpsManualCandidate(candidate), true, nil
}

func (s *GormStore) ListOpsManualCandidates() ([]opsmanual.ManualCandidate, error) {
	var candidates []opsmanual.ManualCandidate
	if err := s.listKV(gormNamespaceOpsManualCandidates, &candidates); err != nil {
		return nil, err
	}
	out := make([]opsmanual.ManualCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, cloneOpsManualCandidate(candidate))
	}
	sortOpsManualCandidates(out)
	return out, nil
}

func (s *GormStore) SaveOpsManualCandidate(candidate opsmanual.ManualCandidate) error {
	if strings.TrimSpace(candidate.ID) == "" {
		return fmt.Errorf("ops manual candidate id is required")
	}
	return s.saveKV(gormNamespaceOpsManualCandidates, candidate.ID, cloneOpsManualCandidate(candidate))
}

func (s *GormStore) DeleteOpsManualCandidate(id string) error {
	return s.deleteRequiredKV(gormNamespaceOpsManualCandidates, id, "ops manual candidate")
}

func (s *GormStore) ListOpsManualRunRecords(manualID string, workflowID string, limit int) ([]opsmanual.RunRecord, error) {
	var records []opsmanual.RunRecord
	if err := s.listKV(gormNamespaceOpsManualRunRecords, &records); err != nil {
		return nil, err
	}
	recordMap := make(map[string]opsmanual.RunRecord, len(records))
	for _, record := range records {
		recordMap[record.ID] = record
	}
	return filterOpsManualRunRecords(recordMap, manualID, workflowID, limit), nil
}

func (s *GormStore) SaveOpsManualRunRecord(record opsmanual.RunRecord) error {
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("ops manual run record id is required")
	}
	return s.saveKV(gormNamespaceOpsManualRunRecords, record.ID, cloneOpsManualRunRecord(record))
}

func (s *GormStore) SaveManualGuidedChatEvent(event opsmanual.ManualGuidedChatEvent) error {
	if strings.TrimSpace(event.ID) == "" {
		return fmt.Errorf("ops manual manual-guided chat event id is required")
	}
	return s.saveKV(gormNamespaceOpsManualGuidedChat, event.ID, cloneManualGuidedChatEvent(event))
}

func (s *GormStore) ListManualGuidedChatEvents(req opsmanual.ListManualGuidedChatEventsRequest) ([]opsmanual.ManualGuidedChatEvent, error) {
	var events []opsmanual.ManualGuidedChatEvent
	if err := s.listKV(gormNamespaceOpsManualGuidedChat, &events); err != nil {
		return nil, err
	}
	eventMap := make(map[string]opsmanual.ManualGuidedChatEvent, len(events))
	for _, event := range events {
		eventMap[event.ID] = event
	}
	return filterManualGuidedChatEvents(eventMap, req), nil
}

func (s *GormStore) ListManuals(req opsmanual.ListManualsRequest) ([]opsmanual.OpsManual, error) {
	manuals, err := s.ListOpsManuals()
	if err != nil {
		return nil, err
	}
	return filterOpsManuals(manuals, req), nil
}

func (s *GormStore) GetManual(id string) (opsmanual.OpsManual, error) {
	manual, ok, err := s.GetOpsManual(id)
	if err != nil {
		return opsmanual.OpsManual{}, err
	}
	if !ok {
		return opsmanual.OpsManual{}, fmt.Errorf("ops manual %q not found", id)
	}
	return manual, nil
}

func (s *GormStore) SaveManual(manual opsmanual.OpsManual) error {
	return s.SaveOpsManual(manual)
}

func (s *GormStore) ListRunRecords(req opsmanual.ListRunRecordsRequest) ([]opsmanual.RunRecord, error) {
	var records []opsmanual.RunRecord
	if err := s.listKV(gormNamespaceOpsManualRunRecords, &records); err != nil {
		return nil, err
	}
	recordMap := make(map[string]opsmanual.RunRecord, len(records))
	for _, record := range records {
		recordMap[record.ID] = record
	}
	return filterOpsManualRunRecordsByRequest(recordMap, req), nil
}

func (s *GormStore) GetCandidate(id string) (opsmanual.ManualCandidate, error) {
	candidate, ok, err := s.GetOpsManualCandidate(id)
	if err != nil {
		return opsmanual.ManualCandidate{}, err
	}
	if !ok {
		return opsmanual.ManualCandidate{}, fmt.Errorf("ops manual candidate %q not found", id)
	}
	return candidate, nil
}

func (s *GormStore) ListCandidates() ([]opsmanual.ManualCandidate, error) {
	return s.ListOpsManualCandidates()
}

func (s *GormStore) SaveCandidate(candidate opsmanual.ManualCandidate) error {
	return s.SaveOpsManualCandidate(candidate)
}

func (s *GormStore) DeleteCandidate(id string) error {
	return s.DeleteOpsManualCandidate(id)
}

func (s *GormStore) SaveRunRecord(record opsmanual.RunRecord) error {
	return s.SaveOpsManualRunRecord(record)
}

func (s *GormStore) Flush() error {
	return nil
}

func (s *GormStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *GormStore) saveKV(namespace, key string, value any) error {
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if key == "" {
		return fmt.Errorf("record key is required")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	record := gormKVRecord{
		Namespace: namespace,
		RecordKey: key,
		Payload:   raw,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "namespace"}, {Name: "record_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"payload":    raw,
			"updated_at": now,
		}),
	}).Create(&record).Error
}

func (s *GormStore) loadKV(namespace, key string, out any) (bool, error) {
	var record gormKVRecord
	err := s.db.Where("namespace = ? AND record_key = ?", namespace, key).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(record.Payload) == 0 {
		return true, nil
	}
	if err := json.Unmarshal(record.Payload, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *GormStore) listKV(namespace string, out any) error {
	var records []gormKVRecord
	if err := s.db.Where("namespace = ?", namespace).Order("record_key ASC").Find(&records).Error; err != nil {
		return err
	}
	raw := make([]json.RawMessage, 0, len(records))
	for _, record := range records {
		raw = append(raw, json.RawMessage(record.Payload))
	}
	joined, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(joined, out)
}

func (s *GormStore) deleteRequiredKV(namespace, key, label string) error {
	result := s.db.Where("namespace = ? AND record_key = ?", namespace, key).Delete(&gormKVRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%s %q not found", label, key)
	}
	return nil
}

func cloneGormIncident(in incidents.IncidentCase) incidents.IncidentCase {
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var out incidents.IncidentCase
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}
