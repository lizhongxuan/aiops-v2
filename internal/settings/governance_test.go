package settings

import "testing"

func TestGovernanceAggregatesRestrictionsAndScopes(t *testing.T) {
	g := NewGovernance()

	if err := g.Register("plugin-a", GovernanceContribution{
		RestrictToPluginOnly: AllCustomizationSurfaces(),
		AllowedMCPServers:    []string{"plugin-mcp", "plugin-mcp"},
		AdditionalDirectories: []string{
			"/tmp/plugin-a",
			"/tmp/plugin-a",
		},
	}); err != nil {
		t.Fatalf("Register(plugin-a) error = %v", err)
	}
	if err := g.Register("plugin-b", GovernanceContribution{
		RestrictToPluginOnly: []CustomizationSurface{SurfaceHooks},
		AllowedMCPServers:    []string{"shared-mcp"},
		AdditionalDirectories: []string{
			"/tmp/plugin-b",
		},
	}); err != nil {
		t.Fatalf("Register(plugin-b) error = %v", err)
	}

	snapshot := g.Snapshot()
	for _, surface := range AllCustomizationSurfaces() {
		if !snapshot.IsRestrictedToPluginOnly(surface) {
			t.Fatalf("expected %q to be restricted to plugin-only", surface)
		}
	}

	if snapshot.AllowsSource(SurfaceSkills, "userSettings") {
		t.Fatal("expected userSettings skills to be blocked under strict plugin-only policy")
	}
	if !snapshot.AllowsSource(SurfaceSkills, "plugin") {
		t.Fatal("expected plugin skills to remain allowed")
	}
	if !snapshot.AllowsSource(SurfaceAgents, "policySettings") {
		t.Fatal("expected policySettings agents to remain allowed")
	}
	if snapshot.AllowsMCPServer("userSettings", "blocked-mcp") {
		t.Fatal("expected unlisted custom MCP server to be blocked")
	}
	if snapshot.AllowsMCPServer("userSettings", "plugin-mcp") {
		t.Fatal("expected strict plugin-only MCP lock to override custom allowlist entries")
	}
	if !snapshot.AllowsMCPServer("plugin", "blocked-mcp") {
		t.Fatal("expected plugin MCP server to bypass custom allowlist checks")
	}

	allowed := snapshot.AllowedMCPServers()
	if len(allowed) != 2 || allowed[0] != "plugin-mcp" || allowed[1] != "shared-mcp" {
		t.Fatalf("AllowedMCPServers() = %#v", allowed)
	}
	directories := snapshot.AdditionalDirectories()
	if len(directories) != 2 || directories[0] != "/tmp/plugin-a" || directories[1] != "/tmp/plugin-b" {
		t.Fatalf("AdditionalDirectories() = %#v", directories)
	}
}

func TestGovernanceUnregisterRemovesContribution(t *testing.T) {
	g := NewGovernance()

	if err := g.Register("plugin-a", GovernanceContribution{
		RestrictToPluginOnly: []CustomizationSurface{SurfaceSkills},
		AllowedMCPServers:    []string{"plugin-mcp"},
		AdditionalDirectories: []string{
			"/tmp/plugin-a",
		},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	g.Unregister("plugin-a")
	snapshot := g.Snapshot()
	if snapshot.IsRestrictedToPluginOnly(SurfaceSkills) {
		t.Fatal("expected restriction to be removed after unregister")
	}
	if !snapshot.AllowsMCPServer("userSettings", "other-mcp") {
		t.Fatal("expected custom MCP server to be allowed after unregister")
	}
	if len(snapshot.AllowedMCPServers()) != 0 {
		t.Fatalf("AllowedMCPServers() = %#v, want empty", snapshot.AllowedMCPServers())
	}
	if len(snapshot.AdditionalDirectories()) != 0 {
		t.Fatalf("AdditionalDirectories() = %#v, want empty", snapshot.AdditionalDirectories())
	}
}
