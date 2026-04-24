package appui

import (
	"context"
	"fmt"
	"testing"

	"aiops-v2/internal/store"
)

type agentProfileRepoStub struct {
	skillCatalog []store.SkillCatalogEntry
	mcpCatalog   []store.AgentMCPCatalogEntry
	profiles     []store.AgentProfileRecord
}

func cloneProfileRecordTest(record store.AgentProfileRecord) store.AgentProfileRecord {
	if record == nil {
		return nil
	}
	cp := make(store.AgentProfileRecord, len(record))
	for key, value := range record {
		cp[key] = value
	}
	return cp
}

func (r *agentProfileRepoStub) GetSkillCatalog() ([]store.SkillCatalogEntry, error) {
	out := make([]store.SkillCatalogEntry, len(r.skillCatalog))
	copy(out, r.skillCatalog)
	return out, nil
}

func (r *agentProfileRepoStub) SaveSkillCatalog(items []store.SkillCatalogEntry) error {
	r.skillCatalog = append([]store.SkillCatalogEntry(nil), items...)
	return nil
}

func (r *agentProfileRepoStub) GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error) {
	out := make([]store.AgentMCPCatalogEntry, len(r.mcpCatalog))
	copy(out, r.mcpCatalog)
	return out, nil
}

func (r *agentProfileRepoStub) SaveAgentMCPCatalog(items []store.AgentMCPCatalogEntry) error {
	r.mcpCatalog = append([]store.AgentMCPCatalogEntry(nil), items...)
	return nil
}

func (r *agentProfileRepoStub) GetAgentProfiles() ([]store.AgentProfileRecord, error) {
	out := make([]store.AgentProfileRecord, 0, len(r.profiles))
	for _, profile := range r.profiles {
		out = append(out, cloneProfileRecordTest(profile))
	}
	return out, nil
}

func (r *agentProfileRepoStub) SaveAgentProfiles(items []store.AgentProfileRecord) error {
	r.profiles = make([]store.AgentProfileRecord, 0, len(items))
	for _, profile := range items {
		r.profiles = append(r.profiles, cloneProfileRecordTest(profile))
	}
	return nil
}

func testAgentProfileRecord(id, name, profileType string) store.AgentProfileRecord {
	return store.AgentProfileRecord{
		"id":          id,
		"name":        name,
		"type":        profileType,
		"description": fmt.Sprintf("%s description", name),
		"runtime": map[string]any{
			"model":           "gpt-5.4",
			"reasoningEffort": "medium",
			"approvalPolicy":  "untrusted",
			"sandboxMode":     "workspace-write",
		},
		"systemPrompt": map[string]any{
			"content": "line one\nline two",
			"preview": "line one",
		},
		"commandPermissions": map[string]any{
			"enabled":     true,
			"defaultMode": "approval_required",
			"categoryPolicies": map[string]any{
				"system_inspection": "allow",
				"service_read":      "allow",
			},
		},
		"capabilityPermissions": map[string]any{
			"commandExecution": "enabled",
			"fileRead":         "enabled",
		},
		"skills": []any{
			map[string]any{"id": "ops-triage", "name": "Ops Triage", "enabled": true, "activationMode": "default_enabled"},
			map[string]any{"id": "safe-change-review", "name": "Safe Change Review", "enabled": false, "activationMode": "disabled"},
		},
		"mcps": []any{
			map[string]any{"id": "filesystem", "name": "Filesystem MCP", "enabled": true, "permission": "readonly"},
			map[string]any{"id": "docs", "name": "Docs MCP", "enabled": true, "permission": "readonly"},
		},
	}
}

func TestAgentProfileServiceLifecycle(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{
			{ID: "ops-triage", Name: "Ops Triage", DefaultEnabled: true},
			{ID: "incident-summary", Name: "Incident Summary", DefaultEnabled: true},
		},
		mcpCatalog: []store.AgentMCPCatalogEntry{
			{ID: "filesystem", Name: "Filesystem MCP", Type: "stdio", DefaultEnabled: true, Permission: "readonly"},
			{ID: "docs", Name: "Docs MCP", Type: "http", DefaultEnabled: true, Permission: "readonly"},
		},
		profiles: []store.AgentProfileRecord{
			testAgentProfileRecord("main-agent", "Primary Agent", "main-agent"),
			testAgentProfileRecord("host-agent-default", "Host Agent Default", "host-agent-default"),
		},
	}

	svc := NewAgentProfileService(newAgentProfileRepositories(repo, repo, repo))

	list, err := svc.ListAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListAgentProfiles() error = %v", err)
	}
	if len(list.Items) != 2 || len(list.SkillCatalog) != 2 || len(list.McpCatalog) != 2 {
		t.Fatalf("ListAgentProfiles() = %+v, want 2 items and both catalogs", list)
	}

	profile, err := svc.GetAgentProfile(context.Background())
	if err != nil {
		t.Fatalf("GetAgentProfile() error = %v", err)
	}
	if profile["id"] != "main-agent" {
		t.Fatalf("GetAgentProfile() id = %v, want main-agent", profile["id"])
	}

	updated, err := svc.SaveAgentProfile(context.Background(), store.AgentProfileRecord{
		"id":           "main-agent",
		"name":         "Primary Agent",
		"type":         "main-agent",
		"description":  "updated",
		"systemPrompt": map[string]any{"content": "updated prompt", "preview": "updated prompt"},
	})
	if err != nil {
		t.Fatalf("SaveAgentProfile() error = %v", err)
	}
	if updated["description"] != "updated" {
		t.Fatalf("SaveAgentProfile() description = %v, want updated", updated["description"])
	}

	resetProfile, err := svc.ResetAgentProfile(context.Background(), "host-agent-default")
	if err != nil {
		t.Fatalf("ResetAgentProfile() error = %v", err)
	}
	if resetProfile["id"] != "host-agent-default" {
		t.Fatalf("ResetAgentProfile() id = %v, want host-agent-default", resetProfile["id"])
	}

	preview, err := svc.PreviewAgentProfile(context.Background(), "main-agent")
	if err != nil {
		t.Fatalf("PreviewAgentProfile() error = %v", err)
	}
	if preview.ProfileID != "main-agent" || preview.SystemPrompt == "" {
		t.Fatalf("PreviewAgentProfile() = %+v, want profileId main-agent and a prompt", preview)
	}
	if len(preview.EnabledSkills) != 1 || len(preview.EnabledMcps) != 2 {
		t.Fatalf("PreviewAgentProfile() enabled items = %+v, want filtered collections", preview)
	}

	exported, err := svc.ExportAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ExportAgentProfiles() error = %v", err)
	}
	if exported.Count != 2 || len(exported.Profiles) != 2 {
		t.Fatalf("ExportAgentProfiles() = %+v, want 2 profiles", exported)
	}

	imported, err := svc.ImportAgentProfiles(context.Background(), AgentProfilesImportPayload{
		Profiles: []store.AgentProfileRecord{
			testAgentProfileRecord("main-agent", "Primary Agent", "main-agent"),
		},
	})
	if err != nil {
		t.Fatalf("ImportAgentProfiles() error = %v", err)
	}
	if imported.Count != 1 || len(repo.profiles) != 1 {
		t.Fatalf("ImportAgentProfiles() = %+v, repo = %+v", imported, repo.profiles)
	}
}

func TestAgentProfileServiceCatalogMutations(t *testing.T) {
	repo := &agentProfileRepoStub{}
	svc := NewAgentProfileService(newAgentProfileRepositories(repo, repo, repo))

	skillResult, err := svc.SaveSkillCatalogItem(context.Background(), SkillCatalogItem{
		ID:          "ops-triage",
		Name:        "Ops Triage",
		Description: "triage",
		Source:      "built-in",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveSkillCatalogItem() error = %v", err)
	}
	if len(skillResult.Items) != 1 || skillResult.Items[0].ID != "ops-triage" {
		t.Fatalf("SaveSkillCatalogItem() = %+v, want ops-triage", skillResult)
	}

	skillAfterDelete, err := svc.DeleteSkillCatalogItem(context.Background(), "ops-triage")
	if err != nil {
		t.Fatalf("DeleteSkillCatalogItem() error = %v", err)
	}
	if len(skillAfterDelete.Items) != 0 {
		t.Fatalf("DeleteSkillCatalogItem() = %+v, want empty list", skillAfterDelete)
	}

	mcpResult, err := svc.SaveMcpCatalogItem(context.Background(), McpCatalogItem{
		ID:         "filesystem",
		Name:       "Filesystem MCP",
		Type:       "stdio",
		Source:     "built-in",
		Enabled:    true,
		Permission: "readonly",
	})
	if err != nil {
		t.Fatalf("SaveMcpCatalogItem() error = %v", err)
	}
	if len(mcpResult.Items) != 1 || mcpResult.Items[0].ID != "filesystem" {
		t.Fatalf("SaveMcpCatalogItem() = %+v, want filesystem", mcpResult)
	}

	mcpAfterDelete, err := svc.DeleteMcpCatalogItem(context.Background(), "filesystem")
	if err != nil {
		t.Fatalf("DeleteMcpCatalogItem() error = %v", err)
	}
	if len(mcpAfterDelete.Items) != 0 {
		t.Fatalf("DeleteMcpCatalogItem() = %+v, want empty list", mcpAfterDelete)
	}

	if len(repo.skillCatalog) != 0 {
		t.Fatalf("repo.skillCatalog = %+v, want empty slice", repo.skillCatalog)
	}
	if len(repo.mcpCatalog) != 0 {
		t.Fatalf("repo.mcpCatalog = %+v, want empty slice", repo.mcpCatalog)
	}
}
