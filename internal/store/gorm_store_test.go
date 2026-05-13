package store

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"aiops-v2/internal/agentui"
	"aiops-v2/internal/incidents"
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
	if err := first.SaveLLMConfig(&LLMConfig{Provider: "openai", Model: "gpt-5.4", APIKey: "sk-test", BaseURL: "http://127.0.0.1:8317/v1"}); err != nil {
		t.Fatalf("SaveLLMConfig() error = %v", err)
	}
	if err := first.SaveWebSettings(&WebSettings{Model: "gpt-5.4", Quota: "dev"}); err != nil {
		t.Fatalf("SaveWebSettings() error = %v", err)
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
	if err != nil || llm.Model != "gpt-5.4" {
		t.Fatalf("GetLLMConfig() = %#v, %v", llm, err)
	}
	settings, err := second.GetWebSettings()
	if err != nil || settings.Model != "gpt-5.4" {
		t.Fatalf("GetWebSettings() = %#v, %v", settings, err)
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
