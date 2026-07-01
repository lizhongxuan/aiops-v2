package opsgraph

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileRepositoryLoadsEmptyDocumentWhenFileMissing(t *testing.T) {
	repo := NewFileRepository(filepath.Join(t.TempDir(), "manual.graph.json"))
	doc, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if doc.SchemaVersion != ManualGraphSchemaVersion {
		t.Fatalf("schema = %q, want %q", doc.SchemaVersion, ManualGraphSchemaVersion)
	}
	if len(doc.Graphs) != 0 {
		t.Fatalf("graphs = %d, want empty", len(doc.Graphs))
	}
}

func TestFileRepositorySavesAndLoadsGraphDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "manual.graph.json")
	repo := NewFileRepository(path)
	doc := GraphDocument{SchemaVersion: ManualGraphSchemaVersion, Graphs: []GraphRecord{{ID: "graph.default", Name: "默认图谱", IsDefault: true}}}
	if err := repo.Save(context.Background(), doc); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(loaded.Graphs) != 1 || loaded.Graphs[0].ID != "graph.default" {
		t.Fatalf("loaded = %#v, want saved graph", loaded)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
}
