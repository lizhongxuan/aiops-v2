package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"runner/scheduler"
)

func TestApplyYAMLWithRunStepInjection(t *testing.T) {
	yamlText := `
version: v0.1
name: yaml-inject
inventory:
  hosts:
    local:
      address: local
steps:
  - name: collect
    targets: [local]
    action: shell.run
    expect_vars: ["OUT"]
`

	run, err := ApplyYAMLWithRun(context.Background(), yamlText, map[string]any{
		"run_id": "run-yaml-inject-0001",
		"step_script": map[string]any{
			"collect": "echo \"BOPS_EXPORT:OUT=ok\"",
		},
		"step_args": map[string]any{
			"collect": map[string]any{
				"export_vars": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("apply yaml with run failed: %v", err)
	}
	if run.RunID != "run-yaml-inject-0001" {
		t.Fatalf("unexpected run id %q", run.RunID)
	}
	if run.Status != "success" {
		t.Fatalf("unexpected run status %q", run.Status)
	}
}

func TestApplyYAMLWithRunRuntimeRef(t *testing.T) {
	base := t.TempDir()
	runtimeDir := filepath.Join(base, "runtime")
	if err := os.Setenv("RUNNER_RUNTIME_PATH", runtimeDir); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("RUNNER_RUNTIME_PATH")
	})

	yamlPath := filepath.Join(runtimeDir, "file", "runner", "test.yaml")
	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	yamlText := `
version: v0.1
name: runtime-ref
inventory:
  hosts:
    local:
      address: local
steps:
  - name: run
    targets: [local]
    action: shell.run
    args:
      script: "echo ok"
`
	if err := os.WriteFile(yamlPath, []byte(yamlText), 0o644); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	run, err := ApplyYAMLWithRun(context.Background(), "runtime://file/runner/test.yaml", nil)
	if err != nil {
		t.Fatalf("apply runtime ref failed: %v", err)
	}
	if run.Status != "success" {
		t.Fatalf("unexpected run status %q", run.Status)
	}
}

func TestApplyYAMLWithRunRuntimeBaseDirParam(t *testing.T) {
	base := t.TempDir()
	runtimeDir := filepath.Join(base, "runtime")
	yamlPath := filepath.Join(runtimeDir, "file", "runner", "test-param.yaml")
	if err := os.MkdirAll(filepath.Dir(yamlPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	yamlText := `
version: v0.1
name: runtime-ref-param
inventory:
  hosts:
    local:
      address: local
steps:
  - name: run
    targets: [local]
    action: shell.run
    args:
      script: "echo ok"
`
	if err := os.WriteFile(yamlPath, []byte(yamlText), 0o644); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	run, err := ApplyYAMLWithRun(context.Background(), "runtime://file/runner/test-param.yaml", map[string]any{
		"runtime_base_dir": runtimeDir,
	})
	if err != nil {
		t.Fatalf("apply runtime ref with runtime_base_dir failed: %v", err)
	}
	if run.Status != "success" {
		t.Fatalf("unexpected run status %q", run.Status)
	}
}

func TestInjectWorkflowParamsHosts(t *testing.T) {
	wf := simpleWorkflow()
	err := injectWorkflowParams(&wf, map[string]any{
		"hosts": map[string]any{
			"local": map[string]any{
				"address": "http://127.0.0.1:9990",
				"vars": map[string]any{
					"k": "v",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("inject params failed: %v", err)
	}
	host, ok := wf.Inventory.Hosts["local"]
	if !ok {
		t.Fatalf("host local not found")
	}
	if host.Address != "http://127.0.0.1:9990" {
		t.Fatalf("unexpected host address %q", host.Address)
	}
	if host.Vars["k"] != "v" {
		t.Fatalf("unexpected host var %v", host.Vars["k"])
	}
}

func TestBuildDispatcherFromParamsAgent(t *testing.T) {
	dispatcher, err := buildDispatcherFromParams(map[string]any{
		"dispatch": map[string]any{
			"type":              "agent",
			"token":             "runner-token",
			"retry_max":         3,
			"retry_delay_sec":   1,
			"async_timeout_sec": 10,
			"poll_interval_sec": 2,
		},
	})
	if err != nil {
		t.Fatalf("build dispatcher failed: %v", err)
	}
	agentDispatcher, ok := dispatcher.(*scheduler.AgentDispatcher)
	if !ok {
		t.Fatalf("dispatcher is not agent type: %T", dispatcher)
	}
	if agentDispatcher.RetryMax != 3 {
		t.Fatalf("unexpected retry max %d", agentDispatcher.RetryMax)
	}
}
