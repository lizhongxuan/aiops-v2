package script

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"runner/modules"
	"runner/workflow"
)

func TestScriptModuleInlineShell(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	mod := New("shell")
	req := modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script": "echo hello",
			},
		},
	}

	res, err := mod.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("apply script: %v", err)
	}

	stdout, _ := res.Output["stdout"].(string)
	if !strings.Contains(stdout, "hello") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestScriptModuleShellStderrAndExitCode(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	res, err := New("shell").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script": "echo bad >&2; exit 7",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected non-zero exit error")
	}
	if !strings.Contains(err.Error(), "script.shell failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	stderr, _ := res.Output["stderr"].(string)
	if !strings.Contains(stderr, "bad") {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
}

func TestScriptModuleArgs(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	mod := New("shell")
	req := modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script": "echo $1 $2",
				"args":   []any{"one", "two"},
			},
		},
	}

	res, err := mod.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("apply script with args: %v", err)
	}

	stdout, _ := res.Output["stdout"].(string)
	if !strings.Contains(stdout, "one two") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestScriptModuleEnvInjection(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	res, err := New("shell").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script": "printf '%s' \"$RUNNER_TEST_ENV\"",
				"env":    map[string]any{"RUNNER_TEST_ENV": "injected"},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply script with env: %v", err)
	}
	if got := res.Output["stdout"]; got != "injected" {
		t.Fatalf("stdout = %#v, want injected", got)
	}
}

func TestScriptModuleStepTimeout(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	res, err := New("shell").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action:  "script.shell",
			Timeout: "20ms",
			Args: map[string]any{
				"script": "sleep 1",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected timeout error, output=%#v", res.Output)
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func TestScriptModulePythonStdoutStderrArgsEnvAndExitCode(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	res, err := New("python").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action: "script.python",
			Args: map[string]any{
				"script": "import os, sys\nprint('{\"ok\": true}')\nprint(sys.argv[1], os.environ['PY_NODE_ENV'], file=sys.stderr)\nsys.exit(4)\n",
				"args":   []any{"arg-one"},
				"env":    map[string]any{"PY_NODE_ENV": "env-one"},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected python exit error")
	}
	if !strings.Contains(err.Error(), "script.python failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := res.Output["stdout"].(string); !strings.Contains(got, `{"ok": true}`) {
		t.Fatalf("stdout = %q", got)
	}
	if got := res.Output["stderr"].(string); !strings.Contains(got, "arg-one env-one") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestScriptModuleParsesNodeResultEnvelope(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	res, err := New("python").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action: "script.python",
			Args: map[string]any{
				"script": `import json
print("debug before")
print("AIOPS_NODE_RESULT_BEGIN")
print(json.dumps({"schema_version":"aiops.node_result/v1","node_id":"extract","node_type":"script.python","status":"success","outputs":{"items":[{"title":"A"}]},"metrics":{"count":1}}))
print("AIOPS_NODE_RESULT_END")
`,
			},
		},
	})
	if err != nil {
		t.Fatalf("apply script: %v", err)
	}
	nodeResult, ok := res.Output["node_result"].(map[string]any)
	if !ok {
		t.Fatalf("node_result = %#v, want parsed envelope map", res.Output["node_result"])
	}
	outputs, ok := nodeResult["outputs"].(map[string]any)
	if !ok || outputs["items"] == nil {
		t.Fatalf("node_result outputs = %#v", nodeResult["outputs"])
	}
}

func TestScriptModulePythonTimeout(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}

	_, err := New("python").Apply(context.Background(), modules.Request{
		Step: workflow.Step{
			Action:  "script.python",
			Timeout: "20ms",
			Args: map[string]any{
				"script": "import time\ntime.sleep(1)\n",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected python timeout error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
}

func TestScriptModuleRequiresScript(t *testing.T) {
	_, err := New("shell").Apply(context.Background(), modules.Request{
		Step: workflow.Step{Action: "script.shell", Args: map[string]any{}},
	})
	if err == nil || !strings.Contains(err.Error(), "args.script") {
		t.Fatalf("expected clear missing script error, got %v", err)
	}
}

func TestScriptModuleExposesScalarVarsAsEnv(t *testing.T) {
	if _, err := exec.LookPath("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}

	mod := New("shell")
	req := modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script": "printf '%s:%s' \"$host_id\" \"$ssh_port\"",
			},
		},
		Vars: map[string]any{
			"host_id":  "host-a",
			"ssh_port": 2222,
			"labels":   map[string]any{"env": "prod"},
		},
	}

	res, err := mod.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("apply script with vars: %v", err)
	}

	stdout, _ := res.Output["stdout"].(string)
	if stdout != "host-a:2222" {
		t.Fatalf("stdout = %q, want host-a:2222", stdout)
	}
}

func TestScriptModuleScriptRefUnsupported(t *testing.T) {
	mod := New("shell")
	req := modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script_ref": "py-script",
			},
		},
	}

	_, err := mod.Apply(context.Background(), req)
	if err == nil {
		t.Fatalf("expected script_ref unsupported error")
	}
}

func TestScriptModuleConflict(t *testing.T) {
	mod := New("shell")
	req := modules.Request{
		Step: workflow.Step{
			Action: "script.shell",
			Args: map[string]any{
				"script":     "echo hi",
				"script_ref": "demo",
			},
		},
	}

	_, err := mod.Apply(context.Background(), req)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
}
