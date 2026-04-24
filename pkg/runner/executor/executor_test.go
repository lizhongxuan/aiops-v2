package executor

import (
	"context"
	"errors"
	"testing"

	"runner/workflow"
)

func TestMergeExportedVars(t *testing.T) {
	base := map[string]any{
		"env": map[string]any{
			"FOO": "old",
		},
		"OTHER": "1",
	}
	exported := map[string]any{
		"FOO": "new",
		"BAR": "2",
	}

	result := mergeExportedVars(base, exported)
	if result["FOO"] != "new" {
		t.Fatalf("expected top-level FOO to be new, got %v", result["FOO"])
	}
	env, ok := result["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env map, got %T", result["env"])
	}
	if env["FOO"] != "new" {
		t.Fatalf("expected env FOO to be new, got %v", env["FOO"])
	}
	if env["BAR"] != "2" {
		t.Fatalf("expected env BAR to be 2, got %v", env["BAR"])
	}
}

type fakeRunner struct {
	failOn string
	calls  []string
}

func (r *fakeRunner) Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (RunResult, error) {
	r.calls = append(r.calls, step.Name)
	if step.Name == r.failOn {
		return RunResult{}, errors.New("boom")
	}
	return RunResult{}, nil
}

func TestContinueOnError(t *testing.T) {
	runner := &fakeRunner{failOn: "step-1"}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", ContinueOnError: true},
			{Name: "step-2", Action: "cmd.run"},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(runner.calls) != 2 || runner.calls[1] != "step-2" {
		t.Fatalf("expected step-2 to run after failure, got %v", runner.calls)
	}
}

type exportRunner struct {
	output map[string]any
}

func (r *exportRunner) Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (RunResult, error) {
	return RunResult{Output: r.output}, nil
}

func TestExpectVarsFailure(t *testing.T) {
	runner := &exportRunner{output: map[string]any{}}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", ExpectVars: []string{"BACKUP_OK"}},
		},
	}

	if err := exec.Run(context.Background(), wf); err == nil {
		t.Fatalf("expected error when vars missing")
	}
}

func TestExpectVarsSuccess(t *testing.T) {
	runner := &exportRunner{output: map[string]any{"vars": map[string]any{"BACKUP_OK": "true"}}}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", ExpectVars: []string{"BACKUP_OK"}},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

type captureRunner struct {
	outputByStep map[string]map[string]any
	varsByStep   map[string]map[string]any
}

func (r *captureRunner) Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (RunResult, error) {
	if r.varsByStep == nil {
		r.varsByStep = map[string]map[string]any{}
	}
	r.varsByStep[step.Name] = vars
	if r.outputByStep != nil {
		if out, ok := r.outputByStep[step.Name]; ok {
			return RunResult{Output: out}, nil
		}
	}
	return RunResult{}, nil
}

func TestExportVarsPropagateToNextStep(t *testing.T) {
	runner := &captureRunner{
		outputByStep: map[string]map[string]any{
			"step-1": {"vars": map[string]any{"TOKEN": "abc123"}},
		},
	}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run"},
			{Name: "step-2", Action: "cmd.run"},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	vars, ok := runner.varsByStep["step-2"]
	if !ok {
		t.Fatalf("expected vars for step-2")
	}
	if vars["TOKEN"] != "abc123" {
		t.Fatalf("expected TOKEN to propagate, got %v", vars["TOKEN"])
	}
}

func TestMustVarsFailure(t *testing.T) {
	runner := &fakeRunner{}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", MustVars: []string{"REQUIRED_VAR"}},
		},
	}

	if err := exec.Run(context.Background(), wf); err == nil {
		t.Fatalf("expected error when must vars missing")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected runner not to be called, got %v", runner.calls)
	}
}

func TestMustVarsSuccess(t *testing.T) {
	runner := &fakeRunner{}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local", Vars: map[string]any{"REQUIRED_VAR": "ok"}},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", MustVars: []string{"REQUIRED_VAR"}},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "step-1" {
		t.Fatalf("expected runner call, got %v", runner.calls)
	}
}

func TestMustVarsFromExpectVars(t *testing.T) {
	runner := &captureRunner{
		outputByStep: map[string]map[string]any{
			"step-1": {"vars": map[string]any{"TOKEN": "abc123"}},
		},
	}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "local"},
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Action: "cmd.run", ExpectVars: []string{"TOKEN"}},
			{Name: "step-2", Action: "cmd.run", MustVars: []string{"TOKEN"}},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

type hostCaptureRunner struct {
	host workflow.HostSpec
}

func (r *hostCaptureRunner) Run(ctx context.Context, step workflow.Step, host workflow.HostSpec, vars map[string]any) (RunResult, error) {
	r.host = host
	return RunResult{}, nil
}

func TestTargetsLocalWithoutInventoryHost(t *testing.T) {
	runner := &hostCaptureRunner{}
	exec := &Executor{Runner: runner}
	wf := workflow.Workflow{
		Name: "demo",
		Inventory: workflow.Inventory{
			Vars: map[string]any{
				"GLOBAL_VAR": "ok",
			},
		},
		Steps: []workflow.Step{
			{Name: "step-1", Targets: []string{"local"}, Action: "cmd.run"},
		},
	}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if runner.host.Name != "local" {
		t.Fatalf("expected host name local, got %q", runner.host.Name)
	}
	if runner.host.Address != "127.0.0.1" {
		t.Fatalf("expected host address 127.0.0.1, got %q", runner.host.Address)
	}
	if runner.host.Vars["GLOBAL_VAR"] != "ok" {
		t.Fatalf("expected GLOBAL_VAR propagated, got %v", runner.host.Vars["GLOBAL_VAR"])
	}
}
