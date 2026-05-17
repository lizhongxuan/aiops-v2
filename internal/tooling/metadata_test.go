package tooling

import (
	"testing"
)

func TestMockToolMetadataSurvivesAssembly(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&StaticTool{
		Meta: ToolMetadata{
			Name:        "k8s.get_events",
			Description: "mock events",
			Mock:        true,
			Domain:      "kubernetes",
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	tools := registry.AssembleTools("host", "inspect")
	if len(tools) != 1 {
		t.Fatalf("AssembleTools() len = %d, want 1", len(tools))
	}
	meta := tools[0].Metadata()
	if !meta.Mock {
		t.Fatal("mock metadata should survive assembly")
	}
	if meta.Domain != "kubernetes" {
		t.Fatalf("domain metadata = %q, want kubernetes", meta.Domain)
	}
}
