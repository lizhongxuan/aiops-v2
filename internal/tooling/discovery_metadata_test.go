package tooling

import (
	"strings"
	"testing"
)

func TestToolDiscoveryMetadataDefaults(t *testing.T) {
	internal := ToolMetadata{Name: "synthetic.internal", Layer: ToolLayerInternal}
	if !ToolHiddenFromDiscovery(internal) {
		t.Fatal("internal tools should be hidden from discovery by default")
	}
	if !ToolHiddenFromPrompt(internal) {
		t.Fatal("internal tools should be hidden from prompt by default")
	}

	read := ToolMetadata{
		Name:        "synthetic.read_resource",
		Description: "Read a bounded resource",
		Discovery: ToolDiscoveryMetadata{
			ResourceTypes:  []string{"resource"},
			OperationKinds: []string{"read"},
		},
	}
	effective := read.EffectiveDiscovery()
	if effective.CapabilityKind != "read" {
		t.Fatalf("CapabilityKind = %q, want read", effective.CapabilityKind)
	}
	if ToolRequiresSelect(read) {
		t.Fatal("core read tool without deferred layer should not require select by default")
	}
	searchText := ToolDiscoverySearchText(read)
	for _, want := range []string{"synthetic.read_resource", "bounded resource", "resource", "read"} {
		if !strings.Contains(searchText, want) {
			t.Fatalf("search text %q missing %q", searchText, want)
		}
	}

	deferred := ToolMetadata{
		Name:           "synthetic.deferred_read",
		Layer:          ToolLayerDeferred,
		DeferByDefault: true,
		Pack:           "synthetic_pack",
	}
	if !ToolRequiresSelect(deferred) {
		t.Fatal("deferred pack tool should require select by default")
	}
}
