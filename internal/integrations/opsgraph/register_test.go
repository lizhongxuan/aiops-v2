package opsgraph

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	graph "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsAddsReadOnlyOpsGraphTools(t *testing.T) {
	store, err := graph.LoadSeedFile(filepath.Join("..", "..", "..", "data", "opsgraph", "erp.seed.yaml"))
	if err != nil {
		t.Fatalf("LoadSeedFile() error = %v", err)
	}
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, store); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("host", "inspect")
	if len(tools) != 4 {
		t.Fatalf("AssembleTools() len = %d, want 4", len(tools))
	}
	for _, tool := range tools {
		if !tool.IsReadOnly(nil) {
			t.Fatalf("%s should be read-only", tool.Metadata().Name)
		}
		if tool.IsDestructive(nil) {
			t.Fatalf("%s should not be destructive", tool.Metadata().Name)
		}
	}

	lookup := toolByName(t, tools, "opsgraph.lookup")
	result, err := lookup.Execute(context.Background(), json.RawMessage(`{"query":"订单提交"}`))
	if err != nil {
		t.Fatalf("lookup Execute() error = %v", err)
	}
	var body struct {
		Status  string         `json:"status"`
		Matches []graph.Entity `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode lookup result: %v", err)
	}
	if body.Status != "ok" || len(body.Matches) == 0 {
		t.Fatalf("lookup result = %#v, want ok matches", body)
	}
}

func toolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return nil
}
