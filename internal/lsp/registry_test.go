package lsp

import "testing"

func TestRegistryRegisterAndUnregister(t *testing.T) {
	r := NewRegistry()

	cfg := ServerConfig{
		ID:        "gopls",
		Name:      "gopls",
		Command:   []string{"gopls"},
		Languages: []string{"go"},
	}
	if err := r.RegisterServer(cfg); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}

	got, ok := r.GetServer("gopls")
	if !ok {
		t.Fatal("expected LSP server to be registered")
	}
	if got.Name != "gopls" {
		t.Fatalf("GetServer().Name = %q, want %q", got.Name, "gopls")
	}

	r.UnregisterServer("gopls")
	if _, ok := r.GetServer("gopls"); ok {
		t.Fatal("expected LSP server to be removed")
	}
}
