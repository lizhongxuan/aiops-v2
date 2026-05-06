package erp

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestERPToolsReturnFixedSchemas(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("workspace", "execute")
	for _, name := range []string{"erp.business_metric", "erp.tenant_impact", "erp.job_status"} {
		tool := toolByName(t, tools, name)
		decision := tool.CheckPermissions(context.Background(), json.RawMessage(`{}`))
		if decision.Action != tooling.PermissionActionAllow {
			t.Fatalf("%s CheckPermissions() = %#v, want allow", name, decision)
		}
		result, err := tool.Execute(context.Background(), json.RawMessage(`{"capability":"订单提交","service":"order-api"}`))
		if err != nil {
			t.Fatalf("%s Execute() error = %v", name, err)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
			t.Fatalf("%s returned non-json: %v", name, err)
		}
		if payload["schemaVersion"] != schemaVersion {
			t.Fatalf("%s schemaVersion = %v", name, payload["schemaVersion"])
		}
		if payload["tool"] != name || payload["status"] != "ok" {
			t.Fatalf("%s payload = %#v", name, payload)
		}
	}
}

func toolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil
}
