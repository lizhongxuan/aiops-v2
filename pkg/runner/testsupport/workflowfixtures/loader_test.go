package workflowfixtures

import (
	"path/filepath"
	"testing"
)

func TestLoadCatalogAndFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "testdata", "workflow_canvas_mcp")
	catalog, err := LoadCatalog(filepath.Join(root, "test-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for id := range catalog.fixtures {
		fixture, err := catalog.Load(id)
		if err != nil {
			t.Fatalf("Load(%q): %v", id, err)
		}
		if fixture.FixtureID != id {
			t.Fatalf("Load(%q).FixtureID = %q", id, fixture.FixtureID)
		}
		if fixture.SchemaVersion != FixtureSchemaVersion {
			t.Fatalf("Load(%q).SchemaVersion = %q", id, fixture.SchemaVersion)
		}
	}
}

func TestLoadRejectsFixtureIdentityMismatch(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "testdata", "workflow_canvas_mcp")
	catalog, err := LoadCatalog(filepath.Join(root, "test-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry := catalog.fixtures["W0"]
	entry.ID = "wrong"
	catalog.fixtures["wrong"] = entry
	if _, err := catalog.Load("wrong"); err == nil {
		t.Fatal("Load accepted a fixture identity mismatch")
	}
}
