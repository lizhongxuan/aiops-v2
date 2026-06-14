package gateway

import (
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
)

func TestMigrateServerConfigPromotesHTTPCommandToStreamableHTTPEndpoint(t *testing.T) {
	old := mcp.ServerConfig{
		ID:        "docs",
		Name:      "Docs MCP",
		Transport: "http",
		Command:   []string{" https://mcp.example.com/mcp ", "--legacy-arg"},
		Disabled:  true,
		Source:    "user_settings",
		TenantScope: mcp.TenantScope{
			TenantIDs: []string{"tenant-a"},
		},
		UserScope: mcp.UserScope{
			UserIDs: []string{"user-a"},
		},
		Profiles: []string{"prod"},
	}

	migrated, report := MigrateServerConfig(old)

	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", report.Warnings)
	}
	if migrated.ID != old.ID || migrated.Name != old.Name || migrated.Disabled != old.Disabled || migrated.Source != old.Source {
		t.Fatalf("preserved fields = %#v, want id/name/disabled/source from old config", migrated)
	}
	if got := strings.Join(migrated.TenantScope.TenantIDs, ","); got != "tenant-a" {
		t.Fatalf("tenant scope = %q, want tenant-a", got)
	}
	if got := strings.Join(migrated.UserScope.UserIDs, ","); got != "user-a" {
		t.Fatalf("user scope = %q, want user-a", got)
	}
	if got := strings.Join(migrated.Profiles, ","); got != "prod" {
		t.Fatalf("profiles = %q, want prod", got)
	}
	if migrated.Endpoint == nil {
		t.Fatalf("Endpoint is nil, want streamable HTTP endpoint")
	}
	if migrated.Endpoint.Type != EndpointTypeStreamableHTTP {
		t.Fatalf("endpoint type = %q, want %q", migrated.Endpoint.Type, EndpointTypeStreamableHTTP)
	}
	if migrated.Endpoint.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("endpoint url = %q, want trimmed URL", migrated.Endpoint.URL)
	}
	if migrated.Stdio != nil {
		t.Fatalf("Stdio = %#v, want nil for URL command migration", migrated.Stdio)
	}
}

func TestMigrateServerConfigPreservesStdioCommandAndArgs(t *testing.T) {
	old := mcp.ServerConfig{
		ID:        "local-tools",
		Name:      "Local Tools",
		Transport: "stdio",
		Command:   []string{"node", "server.js", "--verbose"},
		Source:    "plugin",
	}

	migrated, report := MigrateServerConfig(old)

	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", report.Warnings)
	}
	if migrated.Endpoint != nil {
		t.Fatalf("Endpoint = %#v, want nil for stdio migration", migrated.Endpoint)
	}
	if migrated.Stdio == nil {
		t.Fatalf("Stdio is nil, want command/args")
	}
	if migrated.Stdio.Command != "node" {
		t.Fatalf("stdio command = %q, want node", migrated.Stdio.Command)
	}
	if got, want := strings.Join(migrated.Stdio.Args, "|"), "server.js|--verbose"; got != want {
		t.Fatalf("stdio args = %q, want %q", got, want)
	}

	old.Command[1] = "mutated.js"
	if migrated.Stdio.Args[0] != "server.js" {
		t.Fatalf("stdio args were not cloned: %#v", migrated.Stdio.Args)
	}
}

func TestMigrateServerConfigReportsMissingEndpointOrCommandWithoutPanic(t *testing.T) {
	old := mcp.ServerConfig{
		ID:        "empty",
		Name:      "Empty",
		Transport: "stdio",
		Command:   []string{" "},
		Disabled:  true,
		Source:    "user_settings",
	}

	migrated, report := MigrateServerConfig(old)

	if migrated.ID != old.ID || migrated.Name != old.Name || migrated.Disabled != old.Disabled || migrated.Source != old.Source {
		t.Fatalf("preserved fields = %#v, want id/name/disabled/source from old config", migrated)
	}
	if migrated.Endpoint != nil || migrated.Stdio != nil {
		t.Fatalf("migrated endpoint = %#v, stdio = %#v, want neither for empty config", migrated.Endpoint, migrated.Stdio)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning", report.Warnings)
	}
	if report.Warnings[0].ServerID != "empty" {
		t.Fatalf("warning server id = %q, want empty", report.Warnings[0].ServerID)
	}
	if !strings.Contains(report.Warnings[0].Message, "missing endpoint or command") {
		t.Fatalf("warning message = %q, want missing endpoint or command", report.Warnings[0].Message)
	}
	if report.HasError() {
		t.Fatalf("HasError returned true for warnings-only report: %#v", report)
	}
	if !report.HasWarning() {
		t.Fatalf("HasWarning returned false for warnings-only report: %#v", report)
	}
}
