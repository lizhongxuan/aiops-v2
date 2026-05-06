package changes

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestChangesToolsReturnFixedSchemas(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("workspace", "execute")
	for _, name := range []string{"changes.recent_deployments", "changes.recent_config_changes"} {
		tool := changeToolByName(t, tools, name)
		if decision := tool.CheckPermissions(context.Background(), nil); decision.Action != tooling.PermissionActionAllow {
			t.Fatalf("%s CheckPermissions() = %#v, want allow", name, decision)
		}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"service":"order-api","environment":"prod"}`))
		if err != nil {
			t.Fatalf("%s Execute() error = %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
			t.Fatalf("%s returned non-json: %v", name, err)
		}
		if payload["schemaVersion"] != schemaVersion || payload["tool"] != name {
			t.Fatalf("%s payload = %#v", name, payload)
		}
	}
}

func changeToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil
}
