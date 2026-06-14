package store

import (
	"testing"
	"time"
)

func TestJSONFileStore_PersistsWebSettingsAndHosts(t *testing.T) {
	dataDir := t.TempDir()

	s, err := NewJSONFileStore(dataDir, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}

	if err := s.SaveWebSettings(&WebSettings{
		Quota:           "Unlimited",
		Model:           "gpt-4o",
		ReasoningEffort: "high",
		Models: []SettingModelOption{
			{ID: "gpt-4o", Name: "GPT-4o"},
		},
	}); err != nil {
		t.Fatalf("SaveWebSettings() error = %v", err)
	}
	if err := s.SaveHost(&HostRecord{
		ID:              "host-1",
		Name:            "web-01",
		Address:         "10.0.0.11",
		Status:          "online",
		Transport:       "grpc_reverse",
		Executable:      true,
		TerminalCapable: true,
		Labels:          map[string]string{"env": "prod"},
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewJSONFileStore(dataDir, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("reopen NewJSONFileStore() error = %v", err)
	}
	defer reopened.Close()

	settings, err := reopened.GetWebSettings()
	if err != nil {
		t.Fatalf("GetWebSettings() error = %v", err)
	}
	if settings.Model != "gpt-4o" || settings.ReasoningEffort != "high" {
		t.Fatalf("settings = %+v, want saved model/reasoning effort", settings)
	}

	host, err := reopened.GetHost("host-1")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if host.Name != "web-01" || host.Labels["env"] != "prod" {
		t.Fatalf("host = %+v, want saved host values", host)
	}

	hosts, err := reopened.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].ID != "host-1" {
		t.Fatalf("hosts = %+v, want one persisted host", hosts)
	}
}

func TestJSONFileStore_PersistsMCPAndAgentProfileState(t *testing.T) {
	dataDir := t.TempDir()

	s, err := NewJSONFileStore(dataDir, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}

	if err := s.SaveMCPServers([]MCPServerRecord{{
		Name:      "docs",
		Transport: "http",
		URL:       "http://127.0.0.1:8088/mcp",
		Disabled:  true,
		Status:    "disconnected",
	}}); err != nil {
		t.Fatalf("SaveMCPServers() error = %v", err)
	}
	if err := s.SaveSkillCatalog([]SkillCatalogEntry{{
		ID:                    "ops-triage",
		Name:                  "Ops Triage",
		DefaultEnabled:        true,
		DefaultActivationMode: "default_enabled",
		ResourceTypes:         []string{"log"},
		TaskIntents:           []string{"diagnose"},
		Paths:                 []string{"services/*"},
		Modes:                 []string{"read_only"},
		UserInvocable:         true,
		ModelInvocable:        true,
	}}); err != nil {
		t.Fatalf("SaveSkillCatalog() error = %v", err)
	}
	if err := s.SaveAgentMCPCatalog([]AgentMCPCatalogEntry{{
		ID:             "filesystem",
		Name:           "Filesystem MCP",
		Type:           "stdio",
		DefaultEnabled: true,
		Permission:     "readonly",
	}}); err != nil {
		t.Fatalf("SaveAgentMCPCatalog() error = %v", err)
	}
	if err := s.SaveAgentProfiles([]AgentProfileRecord{{
		"id":   "main-agent",
		"name": "Primary Agent",
		"type": "main-agent",
		"runtime": map[string]any{
			"model": "gpt-5.4",
		},
	}}); err != nil {
		t.Fatalf("SaveAgentProfiles() error = %v", err)
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := NewJSONFileStore(dataDir, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("reopen NewJSONFileStore() error = %v", err)
	}
	defer reopened.Close()

	mcpServers, err := reopened.GetMCPServers()
	if err != nil {
		t.Fatalf("GetMCPServers() error = %v", err)
	}
	if len(mcpServers) != 1 || mcpServers[0].Name != "docs" || !mcpServers[0].Disabled {
		t.Fatalf("mcpServers = %+v, want persisted disabled docs server", mcpServers)
	}

	skillCatalog, err := reopened.GetSkillCatalog()
	if err != nil {
		t.Fatalf("GetSkillCatalog() error = %v", err)
	}
	if len(skillCatalog) != 1 || skillCatalog[0].ResourceTypes[0] != "log" || !skillCatalog[0].ModelInvocable {
		t.Fatalf("skill catalog discovery metadata = %+v", skillCatalog)
	}
	if len(skillCatalog) != 1 || skillCatalog[0].ID != "ops-triage" {
		t.Fatalf("skillCatalog = %+v, want persisted ops-triage item", skillCatalog)
	}

	agentMCPs, err := reopened.GetAgentMCPCatalog()
	if err != nil {
		t.Fatalf("GetAgentMCPCatalog() error = %v", err)
	}
	if len(agentMCPs) != 1 || agentMCPs[0].ID != "filesystem" {
		t.Fatalf("agentMCPs = %+v, want persisted filesystem item", agentMCPs)
	}

	profiles, err := reopened.GetAgentProfiles()
	if err != nil {
		t.Fatalf("GetAgentProfiles() error = %v", err)
	}
	if len(profiles) != 1 || profiles[0]["id"] != "main-agent" {
		t.Fatalf("profiles = %+v, want persisted main-agent profile", profiles)
	}
}
