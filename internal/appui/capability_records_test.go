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
