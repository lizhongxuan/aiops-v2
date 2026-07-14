package appui

import "testing"

func TestCapabilityRecordMapsConnectorCatalogItemAsDataCapability(t *testing.T) {
	record := capabilityRecordFromMcpCatalogItem(McpCatalogItem{
		ID:             "snowflake",
		Name:           "Snowflake",
		Type:           "http",
		Source:         "plugin:data",
		SourceScope:    "plugin",
		DefaultEnabled: true,
		Permission:     "readonly",
		RuntimeStatus:  "connected",
		Risk:           "low",
	})

	if record.Kind != "capability" {
		t.Fatalf("Kind = %q, want capability", record.Kind)
	}
	if record.Category != "data" {
		t.Fatalf("Category = %q, want data", record.Category)
	}
	if record.Kind == "connector" {
		t.Fatalf("connector leaked as top-level record kind: %+v", record)
	}
	if record.Facets.Connection == nil {
		t.Fatalf("Connection facet is nil for connector-backed capability: %+v", record)
	}
	if record.Facets.Connection.Type != "http" || record.Facets.Connection.Permission != "readonly" {
		t.Fatalf("Connection facet = %+v, want type and permission", record.Facets.Connection)
	}
}

func TestCapabilityRecordMapsRunnerMCPAsAutomationCapability(t *testing.T) {
	record := capabilityRecordFromMcpCatalogItem(McpCatalogItem{
		ID:             "runner",
		Name:           "Runner Workflow Editor",
		Type:           "local",
		Source:         "builtin",
		SourceScope:    "mcp",
		DefaultEnabled: true,
		Permission:     "reviewed_mutation",
		RuntimeStatus:  "connected",
		Risk:           "high",
	})

	if record.Kind != "capability" || record.Category != "automation" {
		t.Fatalf("record = %+v, want automation capability", record)
	}
	if record.Facets.Plugin != nil {
		t.Fatalf("runner MCP capability should not be represented as plugin: %+v", record)
	}
	if record.Facets.Connection == nil || record.Facets.Connection.Permission != "reviewed_mutation" {
		t.Fatalf("connection facet = %+v, want reviewed mutation MCP connection", record.Facets.Connection)
	}
}
