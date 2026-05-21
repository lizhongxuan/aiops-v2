package plugins

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAIOpsManifestLoadsDomainExtensions(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "aiops-plugin")
	writeTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "aiops-plugin",
  "aiops": {
    "opsmanual_capability_packs": [
      {"id":"kubernetes","surfaces":["kubectl"],"resource_types":["pod"],"mcp_server":"kubernetes-observer","skill":"kubernetes-ops"}
    ],
    "runner_actions": [
      {"id":"script.shell","module":"script","handler":"shell","input_schema":"schemas/script.shell.input.json","risk":"medium","approval":"required_for_write","ui":{"category":"Script","icon":"terminal"}}
    ],
    "agent_ui_renderers": [
      {"id":"coroot.chart.v1","artifact_types":["coroot_chart","observability.chart"],"schema_version":"coroot.chart.v1","component":"CorootChartArtifact","fallback":"json_summary","display":{"title_field":"title","icon":"activity","hide_footer":true}}
    ],
    "agent_profiles": [
      {"id":"ops-triage","display_name":"Ops Triage","recommended_skills":["ops-triage"],"recommended_mcp_servers":["coroot"],"mode":"suggested"}
    ],
    "settings_schemas": [
      {"id":"coroot-settings","path":"settings/coroot.schema.json"}
    ],
    "permission_defaults": [
      {"id":"coroot-readonly","target":"mcp:coroot","mode":"allow_readonly"}
    ]
  }
}`)

	loader := NewManifestLoader(pluginDir)
	specs, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("Load() len = %d, want 1", len(specs))
	}
	aiops := specs[0].Manifest.AIOps
	if len(aiops.OpsManualCapabilityPacks) != 1 || aiops.OpsManualCapabilityPacks[0].ID != "kubernetes" {
		t.Fatalf("unexpected opsmanual capability packs: %#v", aiops.OpsManualCapabilityPacks)
	}
	if got := aiops.RunnerActions[0]; got.ID != "script.shell" || got.UI.Category != "Script" {
		t.Fatalf("unexpected runner action: %#v", got)
	}
	if got := aiops.AgentUIRenderers[0]; got.ID != "coroot.chart.v1" || got.SchemaVersion != "coroot.chart.v1" || got.Fallback != "json_summary" || !got.Display.HideFooter {
		t.Fatalf("unexpected renderer: %#v", got)
	}
	if got := aiops.AgentProfiles[0]; got.ID != "ops-triage" || got.RecommendedMCPServers[0] != "coroot" {
		t.Fatalf("unexpected agent profile: %#v", got)
	}
	if got := aiops.SettingsSchemas[0]; got.ID != "coroot-settings" || got.Path != "settings/coroot.schema.json" {
		t.Fatalf("unexpected settings schema: %#v", got)
	}
	if got := aiops.PermissionDefaults[0]; got.ID != "coroot-readonly" || got.Target != "mcp:coroot" {
		t.Fatalf("unexpected permission default: %#v", got)
	}
}

func TestAIOpsManifestRequiresIDsWithFieldPath(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "broken-aiops-plugin")
	writeTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "broken-aiops-plugin",
  "aiops": {
    "runner_actions": [
      {"module":"script","handler":"shell"}
    ]
  }
}`)

	loader := NewManifestLoader(pluginDir)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected Load() to fail")
	}
	if !strings.Contains(err.Error(), "aiops.runner_actions[0].id") {
		t.Fatalf("expected field path in error, got %v", err)
	}
}

func TestAIOpsManifestStrictModeRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "strict-aiops-plugin")
	writeTestFile(t, filepath.Join(pluginDir, ".codex-plugin", "plugin.json"), `{
  "name": "strict-aiops-plugin",
  "strictPluginOnlyCustomization": true,
  "aiops": {
    "runner_actions": [
      {"id":"script.shell"}
    ],
    "unknown_surface": []
  }
}`)

	loader := NewManifestLoader(pluginDir)
	_, err := loader.Load()
	if err == nil {
		t.Fatal("expected Load() to fail")
	}
	if !strings.Contains(err.Error(), "aiops.unknown_surface") {
		t.Fatalf("expected unknown field path in error, got %v", err)
	}
}
