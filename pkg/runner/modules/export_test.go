package modules

import (
	"testing"

	"runner/workflow"
)

func TestExportVarsEnabled(t *testing.T) {
	step := workflow.Step{Args: map[string]any{"export_vars": true}}
	if !ExportVarsEnabled(Request{Step: step}) {
		t.Fatalf("expected export_vars true to enable parsing")
	}

	step = workflow.Step{Args: map[string]any{"export_vars": "true"}}
	if !ExportVarsEnabled(Request{Step: step}) {
		t.Fatalf("expected string true to enable parsing")
	}

	step = workflow.Step{Args: map[string]any{"export_vars": "yes"}}
	if !ExportVarsEnabled(Request{Step: step}) {
		t.Fatalf("expected string yes to enable parsing")
	}

	step = workflow.Step{Args: map[string]any{"export_vars": "0"}}
	if ExportVarsEnabled(Request{Step: step}) {
		t.Fatalf("expected string 0 to disable parsing")
	}

	step = workflow.Step{Args: map[string]any{}}
	if ExportVarsEnabled(Request{Step: step}) {
		t.Fatalf("expected missing export_vars to disable parsing")
	}
}

func TestParseExportVars(t *testing.T) {
	output := `
hello
BOPS_EXPORT:FOO=bar
noise
BOPS_EXPORT:{BAZ=qux}
`
	parsed := ParseExportVars(output)
	if len(parsed) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(parsed))
	}
	if parsed["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got %v", parsed["FOO"])
	}
	if parsed["BAZ"] != "qux" {
		t.Fatalf("expected BAZ=qux, got %v", parsed["BAZ"])
	}

	empty := ParseExportVars("FOO=bar\n")
	if empty != nil {
		t.Fatalf("expected no vars without prefix")
	}
}
