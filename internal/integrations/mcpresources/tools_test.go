package mcpresources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func TestListAndReadMCPResourcesTools(t *testing.T) {
	registry := mcp.NewRegistry()
	seedMCPResource(t, registry, "ops", "ops://manuals/redis", "Redis manual")

	listTool := NewListTool(registry)
	listResult, err := listTool.Execute(context.Background(), json.RawMessage(`{"server":"ops"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listResult.Content, "ops://manuals/redis") {
		t.Fatalf("list result = %s", listResult.Content)
	}

	readTool := NewReadTool(registry)
	readResult, err := readTool.Execute(context.Background(), json.RawMessage(`{"server":"ops","uri":"ops://manuals/redis"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readResult.Content, "Redis manual") {
		t.Fatalf("read result = %s", readResult.Content)
	}
	if !strings.Contains(readResult.Content, `"digest"`) {
		t.Fatalf("read result = %s, want digest", readResult.Content)
	}
}

func TestRegisterBuiltinsAddsMCPResourceTools(t *testing.T) {
	base := tooling.NewRegistry()
	resources := mcp.NewRegistry()

	if err := RegisterBuiltins(base, resources); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	tools := base.AssembleTools("host", "inspect")
	for _, name := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !hasTool(tools, name) {
			t.Fatalf("missing %s in assembled tools", name)
		}
	}
}

func seedMCPResource(t *testing.T, registry *mcp.Registry, server, uri, text string) {
	t.Helper()
	if err := registry.OnServerResources(server, []mcp.Resource{{URI: uri, Name: "Redis manual", MimeType: "text/plain"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetResourceContent(server, uri, mcp.ResourceContent{URI: uri, MimeType: "text/plain", Text: text}); err != nil {
		t.Fatal(err)
	}
}

func hasTool(tools []tooling.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return true
		}
	}
	return false
}
