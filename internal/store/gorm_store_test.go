package store

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"aiops-v2/internal/agentui"
	"aiops-v2/internal/incidents"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

func newSQLiteGormStore(t *testing.T, path string) *GormStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite gorm db: %v", err)
	}
	store, err := NewGormStore(db)
	if err != nil {
		t.Fatalf("NewGormStore() error = %v", err)
	}
	return store
}

func TestGormStoreCoreRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)
	first := newSQLiteGormStore(t, dbPath)

	session := &runtimekernel.SessionState{
		ID:        "sess-gorm-1",
		Type:      runtimekernel.SessionTypeHost,
		Mode:      runtimekernel.ModeChat,
		HostID:    "server-local",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := first.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := first.SaveWorkspaceTask(&runtimekernel.WorkspaceTask{
		ID:          "task-1",
		Type:        "plan",
		Status:      "completed",
		Description: "verify persistence",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("SaveWorkspaceTask() error = %v", err)
	}
	if err := first.AppendApprovalAudit(&runtimekernel.ApprovalRecord{
		ID:        "approval-1",
		SessionID: "sess-gorm-1",
		TurnID:    "turn-1",
		ToolName:  "shell",
		Status:    "approved",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("AppendApprovalAudit() error = %v", err)
	}
	if err := first.SaveLLMConfig(&LLMConfig{Provider: "openai", Model: "gpt-5.4", APIKey: "sk-test", BaseURL: "http://127.0.0.1:8317/v1", ReasoningEffort: "high"}); err != nil {
		t.Fatalf("SaveLLMConfig() error = %v", err)
	}
	if err := first.SaveWebSettings(&WebSettings{Model: "gpt-5.4", Quota: "dev"}); err != nil {
		t.Fatalf("SaveWebSettings() error = %v", err)
	}
	if err := first.SaveCorootConfig(&CorootConfig{BaseURL: "http://coroot.example", Token: "token", Project: "5hxbfx6p"}); err != nil {
		t.Fatalf("SaveCorootConfig() error = %v", err)
	}
	if err := first.SaveHost(&HostRecord{ID: "host-1", Name: "web-1", Status: "online", Labels: map[string]string{"env": "prod"}}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}
	if err := first.SaveMCPServers([]MCPServerRecord{{Name: "coroot", Transport: "http", URL: "http://coroot", Status: "connected"}}); err != nil {
		t.Fatalf("SaveMCPServers() error = %v", err)
	}
	if err := first.SaveSkillCatalog([]SkillCatalogEntry{{ID: "skill-1", Name: "Coroot RCA", DefaultEnabled: true}}); err != nil {
		t.Fatalf("SaveSkillCatalog() error = %v", err)
	}
	if err := first.SaveAgentMCPCatalog([]AgentMCPCatalogEntry{{ID: "mcp-1", Name: "coroot", DefaultEnabled: true}}); err != nil {
		t.Fatalf("SaveAgentMCPCatalog() error = %v", err)
	}
	if err := first.SaveAgentProfiles([]AgentProfileRecord{{"id": "profile-1", "name": "Ops Agent"}}); err != nil {
		t.Fatalf("SaveAgentProfiles() error = %v", err)
	}
	if err := first.SaveToolResultSpill(&tooling.ResultSpill{ID: "spill-1", ToolName: "shell", ContentType: "text/plain", Content: []byte("hello"), CreatedAt: now}); err != nil {
		t.Fatalf("SaveToolResultSpill() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := newSQLiteGormStore(t, dbPath)
	defer second.Close()

	restoredSession, err := second.GetSession("sess-gorm-1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if restoredSession.HostID != "server-local" {
		t.Fatalf("restored HostID = %q", restoredSession.HostID)
	}
	tasks, err := second.ListWorkspaceTasks()
	if err != nil || len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Fatalf("ListWorkspaceTasks() = %#v, %v", tasks, err)
	}
	audits, err := second.ListApprovalAudits()
	if err != nil || len(audits) != 1 || audits[0].ID != "approval-1" {
		t.Fatalf("ListApprovalAudits() = %#v, %v", audits, err)
	}
	llm, err := second.GetLLMConfig()
	if err != nil || llm.Model != "gpt-5.4" || llm.ReasoningEffort != "high" {
		t.Fatalf("GetLLMConfig() = %#v, %v", llm, err)
	}
	settings, err := second.GetWebSettings()
	if err != nil || settings.Model != "gpt-5.4" {
		t.Fatalf("GetWebSettings() = %#v, %v", settings, err)
	}
	corootCfg, err := second.GetCorootConfig()
	if err != nil || corootCfg.BaseURL != "http://coroot.example" || corootCfg.Project != "5hxbfx6p" {
		t.Fatalf("GetCorootConfig() = %#v, %v", corootCfg, err)
	}
	host, err := second.GetHost("host-1")
	if err != nil || host.Labels["env"] != "prod" {
		t.Fatalf("GetHost() = %#v, %v", host, err)
	}
	mcps, err := second.GetMCPServers()
	if err != nil || len(mcps) != 1 || mcps[0].Name != "coroot" {
		t.Fatalf("GetMCPServers() = %#v, %v", mcps, err)
	}
	skills, err := second.GetSkillCatalog()
	if err != nil || len(skills) != 1 || skills[0].ID != "skill-1" {
		t.Fatalf("GetSkillCatalog() = %#v, %v", skills, err)
	}
	agentMCPs, err := second.GetAgentMCPCatalog()
	if err != nil || len(agentMCPs) != 1 || agentMCPs[0].ID != "mcp-1" {
		t.Fatalf("GetAgentMCPCatalog() = %#v, %v", agentMCPs, err)
	}
	profiles, err := second.GetAgentProfiles()
	if err != nil || len(profiles) != 1 || profiles[0]["id"] != "profile-1" {
		t.Fatalf("GetAgentProfiles() = %#v, %v", profiles, err)
	}
	spill, err := second.GetToolResultSpill("spill-1")
	if err != nil || string(spill.Content) != "hello" {
		t.Fatalf("GetToolResultSpill() = %#v, %v", spill, err)
	}
}

func TestGormStoreUICardsNestedPolicyRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ui-cards.db")
	first := newSQLiteGormStore(t, dbPath)
	now := time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)
	card := UICard{
		ID:                "custom-diagnosis",
		Name:              "Diagnosis Card",
		Kind:              "diagnosis",
		Renderer:          "agent-ui/diagnosis",
		RendererVersion:   "1.2.0",
		SchemaVersion:     "2026-05-16",
		PayloadSchema:     map[string]any{"type": "object", "required": []any{"summary"}},
		MetadataSchema:    map[string]any{"type": "object"},
		ActionPolicy:      map[string]any{"allowed": []any{"open_case"}},
		DisplayPolicy:     map[string]any{"density": "compact"},
		RedactionPolicy:   map[string]any{"mode": "strict"},
		SamplePayloads:    []map[string]any{{"summary": "redis memory pressure"}},
		BundleSupport:     []string{"web"},
		PlacementDefaults: []string{"assistant_turn"},
		Status:            "active",
		Version:           3,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := first.SaveUICards([]UICard{card}); err != nil {
		t.Fatalf("SaveUICards() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := newSQLiteGormStore(t, dbPath)
	defer second.Close()
	cards, err := second.GetUICards()
	if err != nil || len(cards) != 1 {
		t.Fatalf("GetUICards() = %#v, %v", cards, err)
	}
	restored := cards[0]
	if restored.RendererVersion != "1.2.0" || restored.SchemaVersion != "2026-05-16" {
		t.Fatalf("version fields lost after gorm round trip: %#v", restored)
	}
	if restored.PayloadSchema["type"] != "object" || restored.ActionPolicy["allowed"] == nil || len(restored.SamplePayloads) != 1 {
		t.Fatalf("nested policy fields lost after gorm round trip: %#v", restored)
	}
}

func TestJSONFileStoreUICardsNestedPolicyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	first, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	card := UICard{
		ID:              "custom-evidence",
		Name:            "Evidence Card",
		Kind:            "evidence",
		Renderer:        "agent-ui/evidence",
		RendererVersion: "0.4.0",
		SchemaVersion:   "2026-05-16",
		PayloadSchema:   map[string]any{"properties": map[string]any{"title": map[string]any{"type": "string"}}},
		MetadataSchema:  map[string]any{"additionalProperties": true},
		ActionPolicy:    map[string]any{"dangerous": false},
		DisplayPolicy:   map[string]any{"maxInlineRows": float64(5)},
		RedactionPolicy: map[string]any{"fields": []any{"token"}},
		SamplePayloads:  []map[string]any{{"title": "cpu saturation"}},
		Status:          "active",
		Version:         1,
	}
	if err := first.SaveUICards([]UICard{card}); err != nil {
		t.Fatalf("SaveUICards() error = %v", err)
	}
	if err := first.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("reopen NewJSONFileStore() error = %v", err)
	}
	defer second.Close()
	cards, err := second.GetUICards()
	if err != nil || len(cards) != 1 {
		t.Fatalf("GetUICards() = %#v, %v", cards, err)
	}
	restored := cards[0]
	if restored.RendererVersion != "0.4.0" || restored.PayloadSchema["properties"] == nil || len(restored.SamplePayloads) != 1 {
		t.Fatalf("nested policy fields lost after json round trip: %#v", restored)
	}
}

func TestGormStoreAgentEventRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "events.db")
	first := newSQLiteGormStore(t, dbPath)
	event := agentui.AgentEvent{
		EventID:    "event-1",
		Seq:        1,
		SessionID:  "sess-events",
		Kind:       agentui.AgentEventTurn,
		Phase:      agentui.AgentEventPhaseCompleted,
		Status:     agentui.AgentEventStatusCompleted,
		Visibility: agentui.AgentEventVisibilityPrimary,
		Source:     agentui.AgentEventSourceRuntime,
		CreatedAt:  "2026-05-12T10:00:00Z",
	}
	if err := first.AppendAgentEvent("sess-events", event); err != nil {
		t.Fatalf("AppendAgentEvent() error = %v", err)
	}
	projection := agentui.AgentEventProjection{
		SessionID: "sess-events",
		Status:    "completed",
		LastSeq:   1,
		RuntimeLiveness: agentui.RuntimeLiveness{
			ActiveTurns: map[string]bool{},
		},
	}
	if err := first.SaveAgentEventProjection("sess-events", projection); err != nil {
		t.Fatalf("SaveAgentEventProjection() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := newSQLiteGormStore(t, dbPath)
	defer second.Close()
	events, err := second.ListAgentEvents("sess-events", 0)
	if err != nil || len(events) != 1 || events[0].EventID != "event-1" {
		t.Fatalf("ListAgentEvents() = %#v, %v", events, err)
	}
	restored, ok, err := second.LoadAgentEventProjection("sess-events")
	if err != nil || !ok || restored.LastSeq != 1 {
		t.Fatalf("LoadAgentEventProjection() = %#v, %v, %v", restored, ok, err)
	}
}

func TestGormStoreIncidentRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "incidents.db")
	first := newSQLiteGormStore(t, dbPath)
	var incidentStore incidents.Store = first
	now := time.Date(2026, 5, 12, 11, 0, 0, 0, time.UTC)
	incidentStore.PutIncident(incidents.IncidentCase{
		ID:        "case-1",
		Title:     "PG repair",
		Status:    incidents.IncidentStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	})
	incidentStore.PutEvidence("case-1", incidents.EvidenceRef{
		ID:        "evidence-1",
		Source:    "coroot",
		Summary:   "connection pool saturated",
		CreatedAt: now,
	})
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := newSQLiteGormStore(t, dbPath)
	defer second.Close()
	var restoredStore incidents.Store = second
	incident, ok := restoredStore.GetIncident("case-1")
	if !ok || incident.Title != "PG repair" {
		t.Fatalf("GetIncident() = %#v, %v", incident, ok)
	}
	incidentsList := restoredStore.ListIncidents()
	if len(incidentsList) != 1 || incidentsList[0].ID != "case-1" {
		t.Fatalf("ListIncidents() = %#v", incidentsList)
	}
	evidence := restoredStore.ListEvidence("case-1")
	if len(evidence) != 1 || evidence[0].Summary != "connection pool saturated" {
		t.Fatalf("ListEvidence() = %#v", evidence)
	}
}

func TestGormStoreOpsManualRoundTrip(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ops-manuals.db")
	first := newSQLiteGormStore(t, dbPath)
	manual := opsmanual.OpsManual{
		ID:          "manual-redis-memory",
		Title:       "Redis memory pressure",
		Status:      opsmanual.ManualStatusVerified,
		Tags:        []string{"redis", "memory"},
		WorkflowRef: opsmanual.WorkflowRef{WorkflowID: "workflow-redis-memory"},
		Operation:   opsmanual.OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		RetrievalProfile: opsmanual.RetrievalProfile{
			Keywords: []string{"used_memory_rss"},
			MinScore: opsmanual.ScoreThresholds{
				Candidate:     0.55,
				DirectExecute: 0.82,
			},
		},
		PreflightProbe: opsmanual.PreflightProbe{ID: "redis_readonly_probe", RequiredOutputs: []string{"redis_ping"}},
		UpdatedAt:      "2026-05-14T08:00:00Z",
	}
	if err := first.SaveOpsManual(manual); err != nil {
		t.Fatalf("SaveOpsManual() error = %v", err)
	}
	if err := first.SaveOpsManualCandidate(opsmanual.ManualCandidate{
		ID:             "candidate-redis-memory",
		ProposedManual: manual,
		ReviewStatus:   "pending",
		UpdatedAt:      "2026-05-14T08:01:00Z",
	}); err != nil {
		t.Fatalf("SaveOpsManualCandidate() error = %v", err)
	}
	if err := first.SaveOpsManualRunRecord(opsmanual.RunRecord{
		ID:               "record-1",
		OpsManualFlowID:  "flow-redis-1",
		ManualID:         "manual-redis-memory",
		WorkflowID:       "workflow-redis-memory",
		ValidationStatus: "passed",
		CompletedAt:      "2026-05-14T08:02:00Z",
	}); err != nil {
		t.Fatalf("SaveOpsManualRunRecord() error = %v", err)
	}
	if err := first.SaveManualGuidedChatEvent(opsmanual.ManualGuidedChatEvent{
		ID:              "manual-guided-1",
		SessionID:       "sess-redis",
		OpsManualFlowID: "flow-redis-1",
		ManualID:        "manual-redis-memory",
		WorkflowID:      "workflow-redis-memory",
		ReferenceMode:   "manual_guided_chat",
		StageSummary:    "只参考 Redis 手册排查",
		RedactionStatus: "redacted",
		CreatedAt:       "2026-05-14T08:03:00Z",
	}); err != nil {
		t.Fatalf("SaveManualGuidedChatEvent() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second := newSQLiteGormStore(t, dbPath)
	defer second.Close()
	restored, ok, err := second.GetOpsManual("manual-redis-memory")
	if err != nil || !ok || restored.WorkflowRef.WorkflowID != "workflow-redis-memory" {
		t.Fatalf("GetOpsManual() = %#v, %v, %v", restored, ok, err)
	}
	if restored.RetrievalProfile.Keywords[0] != "used_memory_rss" || restored.PreflightProbe.ID != "redis_readonly_probe" {
		t.Fatalf("enhanced fields lost after gorm round trip: %#v", restored)
	}
	manuals, err := second.ListOpsManuals()
	if err != nil || len(manuals) != 1 {
		t.Fatalf("ListOpsManuals() = %#v, %v", manuals, err)
	}
	candidates, err := second.ListOpsManualCandidates()
	if err != nil || len(candidates) != 1 || candidates[0].ID != "candidate-redis-memory" {
		t.Fatalf("ListOpsManualCandidates() = %#v, %v", candidates, err)
	}
	records, err := second.ListOpsManualRunRecords("manual-redis-memory", "", 10)
	if err != nil || len(records) != 1 || records[0].ValidationStatus != "passed" {
		t.Fatalf("ListOpsManualRunRecords() = %#v, %v", records, err)
	}
	flowRecords, err := second.ListRunRecords(opsmanual.ListRunRecordsRequest{OpsManualFlowID: "flow-redis-1"})
	if err != nil || len(flowRecords) != 1 || flowRecords[0].ID != "record-1" {
		t.Fatalf("ListRunRecords(flow) = %#v, %v", flowRecords, err)
	}
	references, err := second.ListManualGuidedChatEvents(opsmanual.ListManualGuidedChatEventsRequest{OpsManualFlowID: "flow-redis-1"})
	if err != nil || len(references) != 1 || references[0].ReferenceMode != "manual_guided_chat" {
		t.Fatalf("ListManualGuidedChatEvents() = %#v, %v", references, err)
	}
}

func TestJSONFileStoreOpsManualRoundTrip(t *testing.T) {
	dir := t.TempDir()
	first, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	manual := opsmanual.OpsManual{
		ID:               "manual-pg-backup",
		Title:            "PostgreSQL backup",
		Status:           opsmanual.ManualStatusVerified,
		Tags:             []string{"postgresql", "backup"},
		WorkflowRef:      opsmanual.WorkflowRef{WorkflowID: "workflow-pg-backup"},
		Operation:        opsmanual.OperationProfile{TargetType: "postgresql", Action: "backup"},
		RetrievalProfile: opsmanual.RetrievalProfile{Keywords: []string{"pg_isready", "backup_path"}},
		PreflightProbe:   opsmanual.PreflightProbe{ID: "pg_backup_probe", RequiredOutputs: []string{"pg_isready"}},
		UpdatedAt:        "2026-05-14T08:00:00Z",
	}
	if err := first.SaveOpsManual(manual); err != nil {
		t.Fatalf("SaveOpsManual() error = %v", err)
	}
	if err := first.SaveOpsManualCandidate(opsmanual.ManualCandidate{ID: "candidate-pg-backup", ProposedManual: manual, ReviewStatus: "pending"}); err != nil {
		t.Fatalf("SaveOpsManualCandidate() error = %v", err)
	}
	if err := first.SaveOpsManualRunRecord(opsmanual.RunRecord{ID: "record-pg-1", OpsManualFlowID: "flow-pg-1", ManualID: "manual-pg-backup", WorkflowID: "workflow-pg-backup", ExecutionStatus: "failed"}); err != nil {
		t.Fatalf("SaveOpsManualRunRecord() error = %v", err)
	}
	if err := first.SaveManualGuidedChatEvent(opsmanual.ManualGuidedChatEvent{
		ID:              "manual-guided-pg-1",
		SessionID:       "sess-pg",
		OpsManualFlowID: "flow-pg-1",
		ManualID:        "manual-pg-backup",
		WorkflowID:      "workflow-pg-backup",
		ReferenceMode:   "manual_guided_chat",
		StageSummary:    "只参考 PG 备份手册",
		RedactionStatus: "redacted",
		CreatedAt:       "2026-05-14T08:03:00Z",
	}); err != nil {
		t.Fatalf("SaveManualGuidedChatEvent() error = %v", err)
	}
	if err := first.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	second, err := NewJSONFileStore(dir, time.Hour)
	if err != nil {
		t.Fatalf("reopen NewJSONFileStore() error = %v", err)
	}
	defer second.Close()
	manuals, err := second.ListOpsManuals()
	if err != nil || len(manuals) != 1 || manuals[0].ID != "manual-pg-backup" {
		t.Fatalf("ListOpsManuals() = %#v, %v", manuals, err)
	}
	if manuals[0].RetrievalProfile.Keywords[0] != "pg_isready" || manuals[0].PreflightProbe.ID != "pg_backup_probe" {
		t.Fatalf("enhanced fields lost after json store round trip: %#v", manuals[0])
	}
	candidates, err := second.ListOpsManualCandidates()
	if err != nil || len(candidates) != 1 {
		t.Fatalf("ListOpsManualCandidates() = %#v, %v", candidates, err)
	}
	records, err := second.ListOpsManualRunRecords("manual-pg-backup", "", 0)
	if err != nil || len(records) != 1 || records[0].ExecutionStatus != "failed" {
		t.Fatalf("ListOpsManualRunRecords() = %#v, %v", records, err)
	}
	flowRecords, err := second.ListRunRecords(opsmanual.ListRunRecordsRequest{OpsManualFlowID: "flow-pg-1"})
	if err != nil || len(flowRecords) != 1 || flowRecords[0].ID != "record-pg-1" {
		t.Fatalf("ListRunRecords(flow) = %#v, %v", flowRecords, err)
	}
	references, err := second.ListManualGuidedChatEvents(opsmanual.ListManualGuidedChatEventsRequest{OpsManualFlowID: "flow-pg-1"})
	if err != nil || len(references) != 1 || references[0].StageSummary != "只参考 PG 备份手册" {
		t.Fatalf("ListManualGuidedChatEvents() = %#v, %v", references, err)
	}
}
