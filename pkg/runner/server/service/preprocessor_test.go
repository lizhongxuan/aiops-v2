package service

import (
	"context"
	"testing"

	"runner/scriptstore"
	"runner/workflow"
)

func TestPreprocessorResolvesScriptRef(t *testing.T) {
	store := scriptstore.NewFileStore(t.TempDir())
	svc := NewScriptService(store)
	if err := svc.Create(context.Background(), &ScriptRecord{
		Name:     "deploy_script",
		Language: "shell",
		Content:  "echo deploy",
	}); err != nil {
		t.Fatalf("create script: %v", err)
	}
	pre := NewPreprocessor(svc, nil, nil)
	wf := &workflow.Workflow{
		Name:    "wf",
		Version: "1",
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{
				"local": {Address: "127.0.0.1"},
			},
		},
		Steps: []workflow.Step{
			{
				Name:    "step1",
				Action:  "script.shell",
				Targets: []string{"local"},
				Args: map[string]any{
					"script_ref": "deploy_script",
				},
			},
		},
	}
	if err := pre.Process(context.Background(), wf); err != nil {
		t.Fatalf("process: %v", err)
	}
	step := wf.Steps[0]
	if _, ok := step.Args["script_ref"]; ok {
		t.Fatalf("script_ref should be removed")
	}
	if got := step.Args["script"]; got != "echo deploy" {
		t.Fatalf("unexpected script: %v", got)
	}
}

func TestPreprocessorRejectsScriptConflict(t *testing.T) {
	pre := NewPreprocessor(nil, nil, nil)
	wf := &workflow.Workflow{
		Name:    "wf",
		Version: "1",
		Steps: []workflow.Step{
			{
				Name:   "step1",
				Action: "script.shell",
				Args: map[string]any{
					"script":     "echo 1",
					"script_ref": "a",
				},
			},
		},
	}
	if err := pre.Process(context.Background(), wf); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestPreprocessorRejectsLanguageMismatch(t *testing.T) {
	store := scriptstore.NewFileStore(t.TempDir())
	svc := NewScriptService(store)
	if err := svc.Create(context.Background(), &ScriptRecord{
		Name:     "python_script",
		Language: "python",
		Content:  "print('ok')",
	}); err != nil {
		t.Fatalf("create script: %v", err)
	}
	pre := NewPreprocessor(svc, nil, nil)
	wf := &workflow.Workflow{
		Name:    "wf",
		Version: "1",
		Steps: []workflow.Step{
			{
				Name:   "step1",
				Action: "script.shell",
				Args: map[string]any{
					"script_ref": "python_script",
				},
			},
		},
	}
	if err := pre.Process(context.Background(), wf); err == nil {
		t.Fatalf("expected language mismatch error")
	}
}
