package appui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/skills"
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
			"model":                   "gpt-5.4",
			"reasoningEffort":         "medium",
			"approvalPolicy":          "untrusted",
			"sandboxMode":             "workspace-write",
			"planningPolicy":          "structured_events",
			"evidencePolicy":          "tool_sourced",
			"answerStyle":             "aiops_rca",
			"toolBudget":              "bounded",
			"reasoningSummary":        "enabled",
			"reasoningSummaryDisplay": "summary_only",
			"showRawReasoning":        false,
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

func TestAgentRuntimePromptSettingsFromProfileAppliesCompileContext(t *testing.T) {
	profile := testAgentProfileRecord("main-agent", "Primary Agent", "main-agent")
	runtime := AgentRuntimePromptSettingsFromProfile(profile)

	ctx := runtime.ApplyToCompileContext(promptcompiler.CompileContext{
		SessionType: "host",
		Mode:        "execute",
	})

	if ctx.PlanningPolicy != "structured_events" {
		t.Fatalf("PlanningPolicy = %q, want structured_events", ctx.PlanningPolicy)
	}
	if ctx.EvidencePolicy != "tool_sourced" || ctx.AnswerStyle != "aiops_rca" || ctx.ToolBudget != "bounded" {
		t.Fatalf("compile context prompt policies = %+v, want profile runtime policies", ctx)
	}
	if ctx.ReasoningSummary != "enabled" || ctx.ReasoningSummaryDisplay != "summary_only" {
		t.Fatalf("compile context reasoning summary = %+v, want summary_only enabled", ctx)
	}
	if ctx.ShowRawReasoning {
		t.Fatalf("ShowRawReasoning = true, want false by default")
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
	if len(list.Items) != 2 || len(list.SkillCatalog) != 3 || len(list.McpCatalog) != 3 {
		t.Fatalf("ListAgentProfiles() = %+v, want 2 profiles and bootstrap-merged catalogs", list)
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

func TestAgentProfileServiceMergesPluginRecommendationsWithoutReopeningUserDisabledBindings(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{
			{ID: "plugin-triage", Name: "User Plugin Triage", DefaultEnabled: false, DefaultActivationMode: "disabled"},
		},
		profiles: []store.AgentProfileRecord{
			{
				"id":          "main-agent",
				"name":        "User Main Agent",
				"description": "user profile survives",
				"skills": []any{
					map[string]any{"id": "plugin-triage", "name": "User Plugin Triage", "enabled": false, "activationMode": "disabled"},
					map[string]any{"id": "incident-summary", "name": "Incident Summary", "enabled": true, "activationMode": "default_enabled"},
				},
				"mcps": []any{
					map[string]any{"id": "plugin-mcp", "name": "Plugin MCP", "enabled": false, "permission": "readonly"},
				},
			},
		},
	}
	svc := NewAgentProfileService(
		newAgentProfileRepositories(repo, repo, repo),
		WithAgentProfilePluginSpecs([]plugins.Spec{testAgentProfilePluginSpec()}),
		WithAgentProfilePolicySettings(AgentProfilePolicySettings{
			DisabledSkills: map[string]string{"incident-summary": "disabled by tenant policy"},
		}),
	)

	list, err := svc.ListAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListAgentProfiles() error = %v", err)
	}
	main := findAgentProfileTest(t, list.Items, "main-agent")
	if main["description"] != "user profile survives" {
		t.Fatalf("profile description = %v, want saved user profile value", main["description"])
	}

	pluginSkill := findBindingTest(t, main["skills"], "plugin-triage")
	if got := profileBool(pluginSkill["enabled"]); got {
		t.Fatalf("plugin skill enabled = true, want user-disabled binding to stay disabled")
	}
	if pluginSkill["name"] != "User Plugin Triage" {
		t.Fatalf("plugin skill name = %v, want user catalog override", pluginSkill["name"])
	}

	policySkill := findBindingTest(t, main["skills"], "incident-summary")
	if got := profileBool(policySkill["enabled"]); got {
		t.Fatalf("policy-disabled skill enabled = true, want policy to override user config")
	}
	if reason := profileString(policySkill["unavailableReason"]); !strings.Contains(reason, "tenant policy") {
		t.Fatalf("policy-disabled skill unavailableReason = %q, want policy reason", reason)
	}

	pluginMCP := findBindingTest(t, main["mcps"], "plugin-mcp")
	if got := profileBool(pluginMCP["enabled"]); got {
		t.Fatalf("plugin MCP enabled = true, want user-disabled binding to stay disabled")
	}

	missingMCP := findBindingTest(t, main["mcps"], "missing-mcp")
	if got := profileBool(missingMCP["enabled"]); got {
		t.Fatalf("missing MCP enabled = true, want unavailable binding disabled")
	}
	if reason := profileString(missingMCP["unavailableReason"]); !strings.Contains(reason, "missing-mcp") || !strings.Contains(reason, "not registered") {
		t.Fatalf("missing MCP unavailableReason = %q, want explicit missing-server reason", reason)
	}

	if got := findSkillCatalogItemTest(list.SkillCatalog, "plugin-triage"); got == nil || got.Name != "User Plugin Triage" || got.DefaultEnabled {
		t.Fatalf("plugin skill catalog = %+v, want user override to win", got)
	}
	if got := findMcpCatalogItemTest(list.McpCatalog, "plugin-mcp"); got == nil || got.Name != "Plugin MCP" || !got.DefaultEnabled {
		t.Fatalf("plugin MCP catalog = %+v, want plugin MCP recommendation in catalog", got)
	}
}

func TestAgentProfileServicePluginRecommendationsDisappearOrBecomeUnavailableWhenPluginDisabled(t *testing.T) {
	withPlugin := NewAgentProfileService(
		newAgentProfileRepositories(&agentProfileRepoStub{}, &agentProfileRepoStub{}, &agentProfileRepoStub{}),
		WithAgentProfilePluginSpecs([]plugins.Spec{testAgentProfilePluginSpec()}),
	)
	withPluginList, err := withPlugin.ListAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListAgentProfiles() with plugin error = %v", err)
	}
	withPluginMain := findAgentProfileTest(t, withPluginList.Items, "main-agent")
	if binding := findBindingTest(t, withPluginMain["skills"], "plugin-triage"); binding == nil {
		t.Fatalf("plugin skill recommendation missing while plugin is enabled")
	}
	if binding := findBindingTest(t, withPluginMain["mcps"], "plugin-mcp"); binding == nil {
		t.Fatalf("plugin MCP recommendation missing while plugin is enabled")
	}

	withoutPlugin := NewAgentProfileService(newAgentProfileRepositories(&agentProfileRepoStub{}, &agentProfileRepoStub{}, &agentProfileRepoStub{}))
	withoutPluginList, err := withoutPlugin.ListAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListAgentProfiles() without plugin error = %v", err)
	}
	withoutPluginMain := findAgentProfileTest(t, withoutPluginList.Items, "main-agent")
	if binding := findBindingTest(t, withoutPluginMain["skills"], "plugin-triage"); binding != nil {
		t.Fatalf("plugin skill recommendation = %+v, want absent when plugin is disabled", binding)
	}
	if binding := findBindingTest(t, withoutPluginMain["mcps"], "plugin-mcp"); binding != nil {
		t.Fatalf("plugin MCP recommendation = %+v, want absent when plugin is disabled", binding)
	}

	savedRepo := &agentProfileRepoStub{
		profiles: []store.AgentProfileRecord{
			{
				"id": "main-agent",
				"skills": []any{
					map[string]any{"id": "plugin-triage", "name": "Plugin Triage", "enabled": true},
				},
				"mcps": []any{
					map[string]any{"id": "plugin-mcp", "name": "Plugin MCP", "enabled": true, "permission": "readonly"},
				},
			},
		},
	}
	savedWithoutPlugin := NewAgentProfileService(newAgentProfileRepositories(savedRepo, savedRepo, savedRepo))
	savedList, err := savedWithoutPlugin.ListAgentProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListAgentProfiles() saved without plugin error = %v", err)
	}
	savedMain := findAgentProfileTest(t, savedList.Items, "main-agent")
	savedSkill := findBindingTest(t, savedMain["skills"], "plugin-triage")
	if reason := profileString(savedSkill["unavailableReason"]); !strings.Contains(reason, "not registered") {
		t.Fatalf("saved plugin skill unavailableReason = %q, want unavailable after plugin disabled", reason)
	}
	savedMCP := findBindingTest(t, savedMain["mcps"], "plugin-mcp")
	if reason := profileString(savedMCP["unavailableReason"]); !strings.Contains(reason, "not registered") {
		t.Fatalf("saved plugin MCP unavailableReason = %q, want unavailable after plugin disabled", reason)
	}
}

func testAgentProfilePluginSpec() plugins.Spec {
	return plugins.Spec{
		Name: "observability",
		Manifest: plugins.Manifest{
			Name: "observability",
			AIOps: plugins.AIOpsManifest{
				AgentProfiles: []plugins.AgentProfileManifest{
					{
						ID:                    "main-agent",
						RecommendedSkills:     []string{"plugin-triage"},
						RecommendedMCPServers: []string{"plugin-mcp", "missing-mcp"},
						Mode:                  "suggested",
					},
				},
			},
		},
		Skills: []skills.Definition{
			{Name: "plugin-triage", Description: "Plugin triage skill"},
		},
		MCPServers: []plugins.MCPServerSpec{
			{Config: mcp.ServerConfig{ID: "plugin-mcp", Name: "Plugin MCP", Transport: "http"}},
		},
	}
}

func findAgentProfileTest(t *testing.T, profiles []store.AgentProfileRecord, id string) store.AgentProfileRecord {
	t.Helper()
	for _, profile := range profiles {
		if profileString(profile["id"]) == id {
			return profile
		}
	}
	t.Fatalf("profile %q not found in %+v", id, profiles)
	return nil
}

func findBindingTest(t *testing.T, raw any, id string) map[string]any {
	t.Helper()
	for _, item := range asAnySlice(raw) {
		binding := asAnyMap(item)
		if profileString(binding["id"]) == id {
			return binding
		}
	}
	return nil
}

func findSkillCatalogItemTest(items []SkillCatalogItem, id string) *SkillCatalogItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func findMcpCatalogItemTest(items []McpCatalogItem, id string) *McpCatalogItem {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
