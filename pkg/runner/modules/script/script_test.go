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
