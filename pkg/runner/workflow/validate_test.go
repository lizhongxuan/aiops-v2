package workflow

import "testing"

func TestWorkflowValidate_MissingBasics(t *testing.T) {
	wf := Workflow{}
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	assertIssue(t, verr.Issues, "version is required")
	assertIssue(t, verr.Issues, "name is required")
	assertIssue(t, verr.Issues, "steps must not be empty")
}

func TestWorkflowValidate_PlanAndDuplicates(t *testing.T) {
	wf := Workflow{
		Version: "v0.1",
		Name:    "demo",
		Plan: Plan{
			Mode:     "fast",
			Strategy: "parallel",
		},
		Handlers: []Handler{
			{Name: "restart", Action: "cmd.run"},
			{Name: "restart", Action: "cmd.run"},
		},
		Steps: []Step{
			{
				Name:   "deploy",
				Action: "cmd.run",
				Notify: []string{"missing"},
			},
			{
				Name:   "deploy",
				Action: "cmd.run",
			},
		},
	}
	err := wf.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	assertIssue(t, verr.Issues, `plan.mode must be manual-approve or auto, got "fast"`)
	assertIssue(t, verr.Issues, `plan.strategy must be sequential, got "parallel"`)
	assertIssue(t, verr.Issues, `handler name "restart" is duplicated`)
	assertIssue(t, verr.Issues, `step name "deploy" is duplicated`)
	assertIssue(t, verr.Issues, `steps[0] notify handler "missing" not found`)
}

func assertIssue(t *testing.T, issues []string, expected string) {
	t.Helper()
	for _, issue := range issues {
		if issue == expected {
			return
		}
	}
	t.Fatalf("expected issue %q, got %v", expected, issues)
}
