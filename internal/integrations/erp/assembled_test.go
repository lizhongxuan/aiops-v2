package erp_test

import (
	"testing"

	"aiops-v2/internal/integrations/changes"
	"aiops-v2/internal/integrations/erp"
	"aiops-v2/internal/integrations/k8s"
	"aiops-v2/internal/tooling"
)

func TestERPSREToolsRegisterIntoOneAssembledToolSet(t *testing.T) {
	registry := tooling.NewRegistry()
	if err := erp.RegisterBuiltins(registry); err != nil {
		t.Fatalf("erp.RegisterBuiltins() error = %v", err)
	}
	if err := k8s.RegisterBuiltins(registry, k8s.Options{ActionTokenSecret: []byte("secret")}); err != nil {
		t.Fatalf("k8s.RegisterBuiltins() error = %v", err)
	}
	if err := changes.RegisterBuiltins(registry); err != nil {
		t.Fatalf("changes.RegisterBuiltins() error = %v", err)
	}
	assembled := registry.AssembleTools("workspace", "execute")
	for _, name := range []string{
		"erp.business_metric", "erp.tenant_impact", "erp.job_status",
		"k8s.get_workload", "k8s.get_events", "k8s.get_logs", "k8s.rollout_status", "k8s.restart_workload", "k8s.scale_workload", "k8s.rollout_undo",
		"changes.recent_deployments", "changes.recent_config_changes",
	} {
		if !hasTool(assembled, name) {
			t.Fatalf("assembled tools missing %q", name)
		}
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
