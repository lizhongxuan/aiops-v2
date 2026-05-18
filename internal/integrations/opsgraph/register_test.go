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
	if tools := registry.AssembleTools("host", "inspect"); len(tools) != 0 {
		t.Fatalf("default AssembleTools() = %v, want opsgraph deferred by default", toolNamesForOpsGraphTest(tools))
	}
	tools := registry.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{EnabledPacks: []string{"opsgraph"}})
	if len(tools) != 4 {
		t.Fatalf("AssembleToolsWithOptions(opsgraph) len = %d, want 4", len(tools))
	}
	if chatTools := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"opsgraph"}}); len(chatTools) != 4 {
		t.Fatalf("chat opsgraph tools = %v, want 4 tools when pack is enabled", toolNamesForOpsGraphTest(chatTools))
	}
	for _, tool := range tools {
		meta := tool.Metadata()
		if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "opsgraph" || !meta.DeferByDefault {
			t.Fatalf("%s metadata = layer:%q pack:%q defer:%v, want deferred opsgraph", meta.Name, meta.Layer, meta.Pack, meta.DeferByDefault)
		}
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

func toolNamesForOpsGraphTest(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
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
