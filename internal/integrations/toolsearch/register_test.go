package toolsearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsRegistersToolSearch(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	tool, ok := registry.Get("tool_search")
	if !ok {
		t.Fatal("tool_search should be registered")
	}
	if !tool.IsReadOnly(nil) {
		t.Fatal("tool_search should be read-only")
	}
}

func TestRegisterBuiltinsUsesInjectedCatalogProvider(t *testing.T) {
	registry := tooling.NewRegistry()
	providerRegistry := tooling.NewRegistry()
	if err := providerRegistry.Register(fakeDeferredTool("provider.only_tool", "Provider only tool")); err != nil {
		t.Fatalf("provider register error = %v", err)
	}
	if err := RegisterBuiltins(registry, providerRegistry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	tool, ok := registry.Get("tool_search")
	if !ok {
		t.Fatal("tool_search should be registered")
	}
	input, err := json.Marshal(map[string]any{"query": "provider only", "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("tool_search Execute() error = %v", err)
	}
	if !strings.Contains(result.Content, "provider.only_tool") {
		t.Fatalf("tool_search result = %s, want provider.only_tool", result.Content)
	}
}
