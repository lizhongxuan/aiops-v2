package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/plugins"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/store"
)

func TestCapabilityServiceListRecordsAggregatesCatalogsAndPlugins(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{{
			ID:             "ops-triage",
			Name:           "Ops Triage",
			Description:    "Triage incidents",
			DefaultEnabled: true,
			Risk:           "medium",
		}},
		mcpCatalog: []store.AgentMCPCatalogEntry{{
			ID:             "warehouse",
			Name:           "Warehouse",
			Type:           "http",
			DefaultEnabled: true,
			Permission:     "readonly",
			RuntimeStatus:  "connected",
		}},
	}
	service := NewCapabilityService(repo, repo, []plugins.Spec{{
		Name:   "ops-plugin",
		Skills: []skills.Definition{{Name: "plugin-diagnose", Description: "Plugin diagnosis"}},
		MCPServers: []plugins.MCPServerSpec{{
			Config: mcp.ServerConfig{ID: "plugin-docs", Name: "Plugin Docs", Transport: "http"},
		}},
	}})

	result, err := service.ListRecords(context.Background(), CapabilityListRequest{})
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}

	if got := capabilityRecordByID(result.Items, "ops-triage"); got == nil || got.Kind != "capability" || got.Category != "skill" || got.Facets.Skill == nil {
		t.Fatalf("ops-triage record = %+v, want skill capability", got)
	}
	if got := capabilityRecordByID(result.Items, "warehouse"); got == nil || got.Kind != "capability" || got.Category != "data" || got.Facets.Connection == nil {
		t.Fatalf("warehouse record = %+v, want data capability with connection facet", got)
	}
	if got := capabilityRecordByID(result.Items, "plugin:ops-plugin"); got == nil || got.Kind != "capability" || got.Category != "extension" || got.Facets.Plugin == nil {
		t.Fatalf("plugin record = %+v, want extension capability", got)
	}
	if got := capabilityRecordByID(result.Items, "plugin-docs"); got == nil || got.Category != "data" || got.Facets.Connection == nil {
		t.Fatalf("plugin-docs record = %+v, want plugin MCP as data capability", got)
	}
}

func TestCapabilityServiceListRecordsFiltersQuery(t *testing.T) {
	repo := &agentProfileRepoStub{
		skillCatalog: []store.SkillCatalogEntry{{ID: "ops-triage", Name: "Ops Triage"}},
		mcpCatalog:   []store.AgentMCPCatalogEntry{{ID: "warehouse", Name: "Warehouse", Type: "http"}},
	}
	service := NewCapabilityService(repo, repo, nil)

	result, err := service.ListRecords(context.Background(), CapabilityListRequest{Query: "warehouse"})
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}

	if len(result.Items) != 1 || result.Items[0].ID != "warehouse" {
		t.Fatalf("filtered items = %+v, want only warehouse", result.Items)
	}
}

func capabilityRecordByID(items []CapabilityRecord, id string) *CapabilityRecord {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}
