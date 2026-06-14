package tooling

import (
	"reflect"
	"strings"
	"testing"
)

func TestInitialVisibleOptionsFromTurnMetadata(t *testing.T) {
	opts := AssembleOptionsForTurnMetadata(map[string]string{
		"profile":                  "host_agent",
		"enableTool":               "synthetic.read",
		"enableToolPack":           "context_artifact",
		"runtimeCapability":        "powershell,repl",
		"contextArtifactAvailable": "true",
		"mcpHealth.synthetic_obs":  "healthy",
	})

	if opts.Profile != "host_agent" {
		t.Fatalf("Profile = %q, want host_agent", opts.Profile)
	}
	if !opts.ContextArtifactAvailable {
		t.Fatal("ContextArtifactAvailable = false, want true")
	}
	if !reflect.DeepEqual(opts.RuntimeCapabilities, []string{"powershell", "repl"}) {
		t.Fatalf("RuntimeCapabilities = %#v, want powershell/repl", opts.RuntimeCapabilities)
	}
	if opts.MCPHealthSnapshot["synthetic_obs"] != "healthy" {
		t.Fatalf("MCPHealthSnapshot = %#v, want synthetic_obs healthy", opts.MCPHealthSnapshot)
	}
}

func TestTurnMetadataOverridesExecCommandDescriptionForSelectedHostOS(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&StaticTool{Meta: ToolMetadata{
		Name:        "exec_command",
		Description: "Execute command. Host OS: darwin for server-local. For host resource inspection on macOS, prefer top -l 1 -s 0.",
		Layer:       ToolLayerCore,
		AlwaysLoad:  true,
	}}); err != nil {
		t.Fatalf("Register(exec_command) failed: %v", err)
	}

	tools := registry.AssembleToolsWithOptions("host", "chat", AssembleOptionsForTurnMetadata(map[string]string{
		"aiops.host.metadataAvailable": "true",
		"aiops.host.id":                "remote-linux-01",
		"aiops.host.os":                "linux",
		"aiops.host.arch":              "amd64",
		"aiops.host.transport":         "agent_http",
	}))
	if len(tools) != 1 {
		t.Fatalf("assembled tools len = %d, want 1", len(tools))
	}
	description := tools[0].Metadata().Description
	for _, want := range []string{"host=remote-linux-01", "os=linux", "arch=amd64", "nproc", "free -h"} {
		if !strings.Contains(description, want) {
			t.Fatalf("exec_command description missing %q:\n%s", want, description)
		}
	}
	for _, forbidden := range []string{"Host OS: darwin for server-local", "top -l 1 -s 0"} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("exec_command description retained stale macOS guidance %q:\n%s", forbidden, description)
		}
	}
}

func TestInitialVisibleToolsForDefaultAIChatStaySlim(t *testing.T) {
	registry := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "tool_search", Description: "discover tools", Layer: ToolLayerCore, AlwaysLoad: true},
		{Name: "exec_command", Description: "execute command", Layer: ToolLayerCore, AlwaysLoad: true},
		{Name: "web_search", Description: "search web", Layer: ToolLayerCore, Pack: "public_web", AlwaysLoad: true},
		{Name: "browse_url", Description: "browse known URL", Layer: ToolLayerDeferred, Pack: "public_web", DeferByDefault: true},
		{Name: "list_mcp_resources", Description: "list MCP resources", Layer: ToolLayerCore, Pack: "mcp_resources", AlwaysLoad: true},
		{Name: "read_mcp_resource", Description: "read MCP resource", Layer: ToolLayerCore, Pack: "mcp_resources", AlwaysLoad: true},
		{Name: "skill_search", Description: "search skills", Layer: ToolLayerCore, AlwaysLoad: true},
		{Name: "skill_read", Description: "read skills", Layer: ToolLayerCore, AlwaysLoad: true},
		{Name: "grep", Description: "search files", Layer: ToolLayerCore, Pack: "filesystem_search", AlwaysLoad: true},
		{Name: "get_current_model_config", Description: "runtime config", Layer: ToolLayerDeferred, Pack: "runtime_config", DeferByDefault: true},
		{Name: "read_context_artifact", Description: "read artifact", Layer: ToolLayerConditional, Pack: "context_artifact"},
		{Name: "coroot.service_metrics", Description: "synthetic observability metrics", Layer: ToolLayerMCP, Pack: "observability", IsMCP: true, MCPInfo: MCPInfo{ServerID: "synthetic_obs", ToolName: "service_metrics"}},
		{Name: "agent", Description: "delegate work", Layer: ToolLayerProfile, Profiles: []string{"manager"}},
	} {
		if err := registry.Register(&StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) failed: %v", meta.Name, err)
		}
	}

	defaultNames := toolNamesForTest(registry.AssembleToolsWithOptions("host", "chat", AssembleOptions{}))
	for _, want := range []string{
		"exec_command",
		"grep",
		"list_mcp_resources",
		"read_mcp_resource",
		"skill_read",
		"skill_search",
		"tool_search",
		"web_search",
	} {
		if !containsToolNameForRegistryTest(defaultNames, want) {
			t.Fatalf("default visible tools = %#v, missing %q", defaultNames, want)
		}
	}
	for _, forbidden := range []string{"browse_url", "get_current_model_config", "read_context_artifact", "coroot.service_metrics"} {
		if containsToolNameForRegistryTest(defaultNames, forbidden) {
			t.Fatalf("default visible tools = %#v, should not include %q", defaultNames, forbidden)
		}
	}
	if len(defaultNames) > 12 {
		t.Fatalf("default visible tool count = %d, want <= 12", len(defaultNames))
	}

	managerNames := toolNamesForTest(registry.AssembleToolsWithOptions("host", "chat", AssembleOptions{Profile: "manager"}))
	if !containsToolNameForRegistryTest(managerNames, "agent") {
		t.Fatalf("manager visible tools = %#v, want agent", managerNames)
	}
	if len(managerNames) > 13 {
		t.Fatalf("manager visible tool count = %d, want <= 13", len(managerNames))
	}
}
