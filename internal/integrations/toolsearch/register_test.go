package toolsearch

import (
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
