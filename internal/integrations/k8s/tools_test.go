package k8s

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestK8sReadOnlyToolsDoNotNeedToken(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, Options{ActionTokenSecret: []byte("k8s-secret")}); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("workspace", "execute")
	for _, name := range []string{"k8s.get_workload", "k8s.get_events", "k8s.get_logs", "k8s.rollout_status"} {
		tool := k8sToolByName(t, tools, name)
		input := json.RawMessage(`{"namespace":"prod","name":"order-api","kind":"deployment"}`)
		if !tool.IsReadOnly(input) {
			t.Fatalf("%s IsReadOnly() = false", name)
		}
		if decision := tool.CheckPermissions(context.Background(), input); decision.Action != tooling.PermissionActionAllow {
			t.Fatalf("%s CheckPermissions() = %#v, want allow", name, decision)
		}
	}
}

func TestK8sMutatingToolWithoutTokenNeedsEvidenceInProd(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, Options{ActionTokenSecret: []byte("k8s-secret")}); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tool := k8sToolByName(t, registry.AssembleTools("workspace", "execute"), "k8s.restart_workload")
	input := json.RawMessage(`{"environment":"prod","namespace":"prod","name":"order-api","kind":"deployment"}`)

	if tool.IsReadOnly(input) {
		t.Fatal("restart_workload must not be read-only")
	}
	decision := tool.CheckPermissions(context.Background(), input)
	if decision.Action != tooling.PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
}

func k8sToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not registered", name)
	return nil
}
