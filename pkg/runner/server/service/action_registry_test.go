package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"runner/workflow"
)

func TestActionRegistryLoadsRunnerCorePlugin(t *testing.T) {
	registry := NewActionRegistry()
	if err := registry.LoadPluginManifest(runnerCoreManifestPath(t)); err != nil {
		t.Fatalf("load runner-core manifest: %v", err)
	}

	catalog := NewActionCatalogFromRegistry(registry)
	for _, action := range []string{
		"script.shell",
		"script.python",
		"http.request",
		"builtin.tcp_ping",
		"builtin.http_check",
		"builtin.ssl_expiry_check",
		"builtin.dns_resolve",
		"wait.event",
	} {
		spec, ok := catalog.Get(context.Background(), action)
		if !ok {
			t.Fatalf("runner-core catalog missing %s", action)
		}
		if spec.Risk == "" {
			t.Fatalf("%s missing risk", action)
		}
		if spec.Category == "" {
			t.Fatalf("%s missing category", action)
		}
		if len(spec.ArgsSchema) == 0 {
			t.Fatalf("%s missing input schema", action)
		}
	}
}

func TestActionCatalogReportsMissingRunnerCoreAction(t *testing.T) {
	catalog := NewActionCatalogFromRegistry(NewActionRegistry())

	issues := catalog.ValidateStep(workflow.Step{
		Name:   "legacy",
		Action: "script.shell",
		Args:   map[string]any{"script": "echo ok"},
	})
	if len(issues) != 1 {
		t.Fatalf("expected one missing action issue, got %+v", issues)
	}
	if issues[0].Field != "action" {
		t.Fatalf("missing action field = %q, want action", issues[0].Field)
	}
	if !strings.Contains(issues[0].Message, "runner-core") || !strings.Contains(issues[0].Message, "script.shell") {
		t.Fatalf("missing action message should name runner-core and action, got %q", issues[0].Message)
	}
}

func TestDefaultActionCatalogValidatesHistoricalRunnerActions(t *testing.T) {
	catalog := NewActionCatalog()

	steps := []workflow.Step{
		{Name: "shell", Action: "script.shell", Args: map[string]any{"script": "echo ok"}},
		{Name: "python", Action: "script.python", Args: map[string]any{"script": "print('ok')"}},
		{Name: "http", Action: "http.request", Args: map[string]any{"url": "https://example.com/healthz"}},
		{Name: "tcp", Action: "builtin.tcp_ping", Args: map[string]any{"host": "example.com", "port": 443}},
		{Name: "http-check", Action: "builtin.http_check", Args: map[string]any{"url": "https://example.com/healthz"}},
		{Name: "ssl", Action: "builtin.ssl_expiry_check", Args: map[string]any{"host": "example.com", "port": 443}},
		{Name: "dns", Action: "builtin.dns_resolve", Args: map[string]any{"name": "example.com"}},
		{Name: "wait", Action: "wait.event", Args: map[string]any{"event": "approval.resolved"}},
	}
	for _, step := range steps {
		if issues := catalog.ValidateStep(step); len(issues) != 0 {
			t.Fatalf("%s should validate, got %+v", step.Action, issues)
		}
	}
}

func TestActionRegistryLoadsManifestOnlyAction(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, ".codex-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{
  "name": "custom-runner-actions",
  "aiops": {
    "runner_actions": [
      {
        "id": "custom.noop",
        "schema_version": "v1",
        "title": "Custom Noop",
        "category": "custom",
        "description": "A manifest-only custom action.",
        "risk": "low",
        "input_schema": {"type": "object", "properties": {"message": {"type": "string"}}},
        "outputs": [{"name": "ok", "type": "boolean"}],
        "defaults": {"message": "ok"},
        "ui": {"category": "custom", "icon": "circle"}
      }
    ]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	registry := NewActionRegistry()
	if err := registry.LoadPluginManifest(manifest); err != nil {
		t.Fatalf("load custom manifest: %v", err)
	}
	catalog := NewActionCatalogFromRegistry(registry)
	spec, ok := catalog.Get(context.Background(), "custom.noop")
	if !ok {
		t.Fatal("custom.noop should be available from manifest only")
	}
	if spec.Title != "Custom Noop" || spec.Category != "custom" || spec.Risk != "low" {
		t.Fatalf("custom.noop metadata mismatch: %+v", spec)
	}
}

func TestActionRegistryRejectsUnsupportedSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, ".codex-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`{
  "name": "future-runner-actions",
  "aiops": {
    "runner_actions": [
      {"id": "future.action", "schema_version": "v99", "title": "Future", "category": "custom", "input_schema": {"type": "object"}}
    ]
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := NewActionRegistry().LoadPluginManifest(manifest)
	if err == nil {
		t.Fatal("unsupported schema_version should be rejected")
	}
	if !strings.Contains(err.Error(), "schema_version") || !strings.Contains(err.Error(), "future.action") {
		t.Fatalf("schema version error should name field and action, got %v", err)
	}
}

func runnerCoreManifestPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..", "..", "..", "plugins", "builtin", "runner-core", ".codex-plugin", "plugin.json")
}
