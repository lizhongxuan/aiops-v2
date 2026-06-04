package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/store"
)

func TestCapabilitySnapshotBuildsEffectiveViewWithReasons(t *testing.T) {
	profile := store.AgentProfileRecord{
		"id": "main-agent",
		"skills": []any{
			map[string]any{"id": "ops-triage", "name": "Ops Triage", "enabled": true, "source": "profile", "activationMode": "default_enabled"},
			map[string]any{"id": "safe-change-review", "name": "Safe Change Review", "enabled": true, "source": "profile", "activationMode": "default_enabled"},
			map[string]any{"id": "incident-summary", "name": "Incident Summary", "enabled": true, "source": "profile", "activationMode": "manual"},
		},
		"mcps": []any{
			map[string]any{"id": "filesystem", "name": "Filesystem MCP", "enabled": true, "source": "profile", "permission": "readonly", "runtimeStatus": "connected"},
			map[string]any{"id": "plugin-shell", "name": "Plugin Shell", "enabled": true, "source": "plugin:ops", "permission": "readwrite", "approvalStatus": "pending_approval"},
			map[string]any{"id": "docs", "name": "Docs MCP", "enabled": true, "source": "profile", "permission": "readonly", "runtimeStatus": "disconnected"},
		},
	}
	skills := []store.SkillCatalogEntry{
		{ID: "ops-triage", Name: "Ops Triage", Source: "builtin", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
		{ID: "safe-change-review", Name: "Safe Change Review", Source: "builtin", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
		{ID: "incident-summary", Name: "Incident Summary", Source: "builtin", DefaultEnabled: false, DefaultActivationMode: "manual"},
	}
	mcps := []store.AgentMCPCatalogEntry{
		{ID: "filesystem", Name: "Filesystem MCP", Source: "builtin", DefaultEnabled: true, Permission: "readonly"},
		{ID: "plugin-shell", Name: "Plugin Shell", Source: "plugin:ops", DefaultEnabled: true, Permission: "readwrite", RequiresExplicitUserApproval: true},
		{ID: "docs", Name: "Docs MCP", Source: "builtin", DefaultEnabled: true, Permission: "readonly"},
	}

	first := BuildCapabilitySnapshot(CapabilitySnapshotInput{
		Profile:      profile,
		SkillCatalog: skills,
		McpCatalog:   mcps,
		Policy: AgentProfilePolicySettings{
			DisabledSkills: map[string]string{"safe-change-review": "disabled by admin deny"},
		},
	})
	second := BuildCapabilitySnapshot(CapabilitySnapshotInput{
		Profile:      profile,
		SkillCatalog: skills,
		McpCatalog:   mcps,
		Policy: AgentProfilePolicySettings{
			DisabledSkills: map[string]string{"safe-change-review": "disabled by admin deny"},
		},
	})

	if first.Fingerprint == "" {
		t.Fatalf("Fingerprint is empty")
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("Fingerprint = %q then %q, want stable value", first.Fingerprint, second.Fingerprint)
	}

	triage := capabilitySnapshotItem(first, "skill", "ops-triage")
	if triage == nil || !triage.Enabled || triage.Source != "profile" || triage.Reason == "" {
		t.Fatalf("ops-triage item = %+v, want enabled profile item with reason", triage)
	}
	denied := capabilitySnapshotItem(first, "skill", "safe-change-review")
	if denied == nil || denied.Enabled || denied.Policy != "admin_deny" || denied.Reason != "disabled by admin deny" {
		t.Fatalf("safe-change-review item = %+v, want admin denied disabled item", denied)
	}
	userEnabled := capabilitySnapshotItem(first, "skill", "incident-summary")
	if userEnabled == nil || !userEnabled.Enabled || userEnabled.InvocationMode != "manual" {
		t.Fatalf("incident-summary item = %+v, want explicit user/profile enable over default disabled", userEnabled)
	}
	pending := capabilitySnapshotItem(first, "mcp_server", "plugin-shell")
	if pending == nil || pending.Enabled || pending.Policy != "pending_approval" || pending.RuntimeStatus != "pending_approval" {
		t.Fatalf("plugin-shell item = %+v, want pending approval disabled item", pending)
	}
	disconnected := capabilitySnapshotItem(first, "mcp_server", "docs")
	if disconnected == nil || disconnected.Enabled || disconnected.RuntimeStatus != "unavailable" || disconnected.Reason == "" {
		t.Fatalf("docs item = %+v, want unavailable disconnected MCP item", disconnected)
	}
}

func TestAgentProfilePreviewIncludesCapabilitySnapshotAndFiltersDenied(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{
			{ID: "ops-triage", Name: "Ops Triage", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
			{ID: "safe-change-review", Name: "Safe Change Review", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
		},
		mcpCatalog: []store.AgentMCPCatalogEntry{
			{ID: "filesystem", Name: "Filesystem MCP", Type: "stdio", DefaultEnabled: true, Permission: "readonly"},
			{ID: "plugin-shell", Name: "Plugin Shell", Type: "stdio", Source: "plugin:ops", DefaultEnabled: true, Permission: "readwrite", RequiresExplicitUserApproval: true},
		},
		profiles: []store.AgentProfileRecord{
			{
				"id":           "main-agent",
				"type":         "main-agent",
				"systemPrompt": map[string]any{"content": "line one"},
				"skills": []any{
					map[string]any{"id": "ops-triage", "enabled": true},
					map[string]any{"id": "safe-change-review", "enabled": true},
				},
				"mcps": []any{
					map[string]any{"id": "filesystem", "enabled": true},
					map[string]any{"id": "plugin-shell", "enabled": true, "approvalStatus": "pending_approval"},
				},
			},
		},
	}
	svc := NewAgentProfileService(
		newAgentProfileRepositories(repo, repo, repo),
		WithAgentProfilePolicySettings(AgentProfilePolicySettings{
			DisabledSkills: map[string]string{"safe-change-review": "disabled by admin deny"},
		}),
	)

	preview, err := svc.PreviewAgentProfile(context.Background(), "main-agent")
	if err != nil {
		t.Fatalf("PreviewAgentProfile() error = %v", err)
	}

	if preview.CapabilitySnapshot.Fingerprint == "" {
		t.Fatalf("CapabilitySnapshot.Fingerprint is empty")
	}
	if len(preview.CapabilitySnapshot.Items) < 4 {
		t.Fatalf("CapabilitySnapshot.Items = %+v, want skill and MCP items", preview.CapabilitySnapshot.Items)
	}
	for _, item := range preview.CapabilitySnapshot.Items {
		if item.Reason == "" {
			t.Fatalf("CapabilitySnapshot item missing reason: %+v", item)
		}
	}
	for _, skill := range preview.EnabledSkills {
		if profileString(skill["id"]) == "safe-change-review" {
			t.Fatalf("EnabledSkills contains admin denied skill: %+v", preview.EnabledSkills)
		}
	}
	for _, mcp := range preview.EnabledMcps {
		if profileString(mcp["id"]) == "plugin-shell" {
			t.Fatalf("EnabledMcps contains pending approval MCP: %+v", preview.EnabledMcps)
		}
	}
}

func TestCapabilitySnapshotAppliesCatalogGuardrailFields(t *testing.T) {
	profile := store.AgentProfileRecord{
		"id": "main-agent",
		"skills": []any{
			map[string]any{"id": "dangerous-skill", "enabled": true},
		},
		"mcps": []any{
			map[string]any{"id": "plugin-shell", "enabled": true},
		},
	}
	snapshot := BuildCapabilitySnapshot(CapabilitySnapshotInput{
		Profile: profile,
		SkillCatalog: []store.SkillCatalogEntry{{
			ID:             "dangerous-skill",
			Name:           "Dangerous Skill",
			Source:         "plugin:ops",
			DefaultEnabled: true,
			Risk:           "high",
			AllowedTools:   []string{"exec_command"},
			DeniedTools:    []string{"exec_command"},
		}},
		McpCatalog: []store.AgentMCPCatalogEntry{{
			ID:                           "plugin-shell",
			Name:                         "Plugin Shell",
			Source:                       "plugin:ops",
			DefaultEnabled:               true,
			Permission:                   "readwrite",
			RequiresExplicitUserApproval: true,
		}},
	})

	skill := capabilitySnapshotItem(snapshot, "skill", "dangerous-skill")
	if skill == nil || !skill.Enabled || skill.InvocationMode != "user_only" || skill.Risk != "high" {
		t.Fatalf("dangerous-skill item = %+v, want high-risk user_only enabled skill", skill)
	}
	if len(skill.AllowedTools) != 1 || len(skill.DeniedTools) != 1 || skill.AllowedTools[0] != "exec_command" || skill.DeniedTools[0] != "exec_command" {
		t.Fatalf("dangerous-skill tool hints = allowed:%v denied:%v", skill.AllowedTools, skill.DeniedTools)
	}
	mcp := capabilitySnapshotItem(snapshot, "mcp_server", "plugin-shell")
	if mcp == nil || mcp.Enabled || mcp.Policy != "pending_approval" || mcp.ApprovalStatus != "pending_approval" {
		t.Fatalf("plugin-shell item = %+v, want pending approval disabled MCP", mcp)
	}
}

func TestMCPApprovalCapabilityDeny(t *testing.T) {
	profile := store.AgentProfileRecord{
		"id": "main-agent",
		"mcps": []any{
			map[string]any{"id": "plugin-shell", "enabled": true},
			map[string]any{"id": "builtin-docs", "enabled": true, "source": "builtin"},
		},
	}
	snapshot := BuildCapabilitySnapshot(CapabilitySnapshotInput{
		Profile: profile,
		McpCatalog: []store.AgentMCPCatalogEntry{
			{ID: "plugin-shell", Name: "Plugin Shell", Source: "plugin:ops", DefaultEnabled: true, Permission: "readwrite", RequiresExplicitUserApproval: true},
			{ID: "builtin-docs", Name: "Builtin Docs", Source: "builtin", DefaultEnabled: true, Permission: "readonly", ApprovalStatus: "approved"},
		},
		Policy: AgentProfilePolicySettings{
			DisabledMCPServers: map[string]string{"builtin-docs": "disabled by admin deny"},
		},
	})

	pending := capabilitySnapshotItem(snapshot, "mcp_server", "plugin-shell")
	if pending == nil || pending.Enabled || pending.Policy != "pending_approval" || pending.SourceScope != "plugin" {
		t.Fatalf("plugin-shell item = %+v, want plugin pending approval", pending)
	}
	denied := capabilitySnapshotItem(snapshot, "mcp_server", "builtin-docs")
	if denied == nil || denied.Enabled || denied.Policy != "admin_deny" || denied.Reason != "disabled by admin deny" {
		t.Fatalf("builtin-docs item = %+v, want admin deny override", denied)
	}
}

func TestSkillInvocationAgentProfileSkill(t *testing.T) {
	profile := store.AgentProfileRecord{
		"id": "main-agent",
		"skills": []any{
			map[string]any{"id": "high-risk-skill", "enabled": true},
			map[string]any{"id": "readonly-skill", "enabled": true, "invocationMode": "model_auto"},
		},
	}
	snapshot := BuildCapabilitySnapshot(CapabilitySnapshotInput{
		Profile: profile,
		SkillCatalog: []store.SkillCatalogEntry{
			{ID: "high-risk-skill", Name: "High Risk Skill", Source: "plugin:ops", DefaultEnabled: true, Risk: "high", AllowedTools: []string{"exec_command"}, DeniedTools: []string{"exec_command"}},
			{ID: "readonly-skill", Name: "Readonly Skill", Source: "builtin", DefaultEnabled: true, Risk: "low"},
		},
	})

	highRisk := capabilitySnapshotItem(snapshot, "skill", "high-risk-skill")
	if highRisk == nil || highRisk.InvocationMode != "user_only" || len(highRisk.DeniedTools) != 1 {
		t.Fatalf("high-risk-skill item = %+v, want user_only with deniedTools preview", highRisk)
	}
	readonly := capabilitySnapshotItem(snapshot, "skill", "readonly-skill")
	if readonly == nil || readonly.InvocationMode != "model_auto" || !readonly.Enabled {
		t.Fatalf("readonly-skill item = %+v, want model_auto enabled skill", readonly)
	}
}

func capabilitySnapshotItem(snapshot CapabilitySnapshot, kind, id string) *CapabilitySnapshotItem {
	for i := range snapshot.Items {
		item := &snapshot.Items[i]
		if item.Kind == kind && item.ID == id {
			return item
		}
	}
	return nil
}
