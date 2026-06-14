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

func TestToolDiscoveryMetadataLoadingPolicyDefaults(t *testing.T) {
	mcp := ToolMetadata{
		Name:        "synthetic.mcp_metrics",
		Description: "read metrics from a synthetic MCP source",
		IsMCP:       true,
		MCPInfo: MCPInfo{
			ServerID: "synthetic-observability",
			ToolName: "metrics_query",
		},
	}
	effectiveMCP := mcp.EffectiveDiscovery()
	if effectiveMCP.LoadingPolicy != ToolLoadingPolicyMCP {
		t.Fatalf("MCP LoadingPolicy = %q, want %q", effectiveMCP.LoadingPolicy, ToolLoadingPolicyMCP)
	}
	if effectiveMCP.MCPServerID != "synthetic-observability" {
		t.Fatalf("MCPServerID = %q, want synthetic-observability", effectiveMCP.MCPServerID)
	}
	if !effectiveMCP.RequiresHealthyMCP {
		t.Fatal("MCP tools should require a healthy MCP server by default")
	}
	if !ToolRequiresSelect(mcp) {
		t.Fatal("MCP tools should require progressive selection by default")
	}

	alwaysLoadDeferred := ToolMetadata{
		Name:       "synthetic.always_load_deferred",
		Layer:      ToolLayerDeferred,
		AlwaysLoad: true,
		Discovery: ToolDiscoveryMetadata{
			RequiresSelect: true,
		},
	}
	if got := alwaysLoadDeferred.EffectiveDiscovery().LoadingPolicy; got != ToolLoadingPolicyCore {
		t.Fatalf("AlwaysLoad deferred LoadingPolicy = %q, want %q", got, ToolLoadingPolicyCore)
	}
	if ToolRequiresSelect(alwaysLoadDeferred) {
		t.Fatal("AlwaysLoad should override progressive select requirement")
	}

	internal := ToolMetadata{Name: "synthetic.internal", Layer: ToolLayerInternal}
	if got := internal.EffectiveDiscovery().LoadingPolicy; got != ToolLoadingPolicyInternal {
		t.Fatalf("internal LoadingPolicy = %q, want %q", got, ToolLoadingPolicyInternal)
	}
}

func TestToolDiscoveryMetadataNormalizesPolicyFields(t *testing.T) {
	meta := ToolMetadata{
		Name:     "synthetic.profile_tool",
		Layer:    ToolLayerProfile,
		Pack:     "Primary_Pack",
		Profiles: []string{" Host ", "host", "Manager"},
		Discovery: ToolDiscoveryMetadata{
			AgentProfiles:     []string{"Host", "Observer"},
			ToolPackIDs:       []string{"primary_pack", "Extra_Pack"},
			PermissionScope:   " Host:Read ",
			PromptBudgetClass: " Small ",
			SchemaBudgetClass: " Compact ",
		},
	}

	effective := meta.EffectiveDiscovery()
	if effective.LoadingPolicy != ToolLoadingPolicyProfile {
		t.Fatalf("LoadingPolicy = %q, want %q", effective.LoadingPolicy, ToolLoadingPolicyProfile)
	}
	assertStringListForDiscoveryTest(t, "AgentProfiles", effective.AgentProfiles, []string{"host", "manager", "observer"})
	assertStringListForDiscoveryTest(t, "ToolPackIDs", effective.ToolPackIDs, []string{"extra_pack", "primary_pack"})
	if effective.PermissionScope != "host:read" {
		t.Fatalf("PermissionScope = %q, want host:read", effective.PermissionScope)
	}
	if effective.PromptBudgetClass != "small" {
		t.Fatalf("PromptBudgetClass = %q, want small", effective.PromptBudgetClass)
	}
	if effective.SchemaBudgetClass != "compact" {
		t.Fatalf("SchemaBudgetClass = %q, want compact", effective.SchemaBudgetClass)
	}
}

func assertStringListForDiscoveryTest(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s len = %d (%v), want %d (%v)", label, len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q; full got=%v", label, i, got[i], want[i], got)
		}
	}
}
