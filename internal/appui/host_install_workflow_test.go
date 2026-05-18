package appui

import (
	"strings"
	"testing"

	"runner/workflow"
	"runner/workflow/visual"
)

func TestBuiltinHostAgentInstallWorkflowUsesOnlyScriptShell(t *testing.T) {
	graph := BuiltinHostAgentInstallGraph()

	if graph.Workflow.Name != "host-agent-install" {
		t.Fatalf("workflow name = %q, want host-agent-install", graph.Workflow.Name)
	}
	for _, node := range graph.Nodes {
		if node.Type != visual.NodeTypeAction {
			continue
		}
		if node.Step == nil {
			t.Fatalf("action node %q missing step", node.ID)
		}
		if node.Step.Action != "script.shell" {
			t.Fatalf("node %q action = %q, want script.shell", node.ID, node.Step.Action)
		}
		script, _ := node.Step.Args["script"].(string)
		if !strings.HasPrefix(script, "set -euo pipefail\n") {
			t.Fatalf("node %q script must start with set -euo pipefail: %q", node.ID, script)
		}
		if !strings.Contains(script, "RUNNER_EXPORT_install_step") {
			t.Fatalf("node %q script missing install step export: %q", node.ID, script)
		}
		if _, ok := node.Step.Args["script_ref"]; ok {
			t.Fatalf("node %q must use inline script, not script_ref", node.ID)
		}
	}
	if err := ValidateHostAgentInstallGraph(graph); err != nil {
		t.Fatalf("validate builtin graph: %v", err)
	}
}

func TestBuiltinHostAgentInstallWorkflowHasRequiredStepsInOrder(t *testing.T) {
	graph := BuiltinHostAgentInstallGraph()

	var got []string
	for _, node := range graph.Nodes {
		if node.Type == visual.NodeTypeAction && node.Step != nil {
			got = append(got, node.Step.Name)
		}
	}
	want := []string{
		"validate-inputs",
		"tcp-preflight",
		"ssh-preflight",
		"detect-platform",
		"resolve-artifact",
		"upload-artifact",
		"install-files",
		"install-service",
		"start-service",
		"verify-local-health",
		"verify-aiops-heartbeat",
		"finalize-host",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("action steps = %v, want %v", got, want)
	}
}

func TestBuiltinHostAgentInstallWorkflowRejectsForbiddenActions(t *testing.T) {
	for _, action := range []string{"cmd.run", "shell.run"} {
		graph := BuiltinHostAgentInstallGraph()
		graph.Nodes[1].Step.Action = action
		if err := ValidateHostAgentInstallGraph(graph); err == nil {
			t.Fatalf("expected %q to be rejected", action)
		}
	}
}

func TestBuiltinHostAgentInstallWorkflowRejectsModelActions(t *testing.T) {
	for _, action := range []string{"llm.generate", "prompt.render", "chat.complete", "completion.create", "agent.plan"} {
		graph := BuiltinHostAgentInstallGraph()
		graph.Nodes[1].Step.Action = action
		if err := ValidateHostAgentInstallGraph(graph); err == nil {
			t.Fatalf("expected %q to be rejected", action)
		}
	}
}

func TestBuiltinHostAgentInstallWorkflowRejectsMissingAndExtraRequiredSteps(t *testing.T) {
	graph := BuiltinHostAgentInstallGraph()
	graph.Nodes = append(graph.Nodes[:2], graph.Nodes[3:]...)
	if err := ValidateHostAgentInstallGraph(graph); err == nil {
		t.Fatal("expected missing required step to be rejected")
	}

	graph = BuiltinHostAgentInstallGraph()
	graph.Nodes = append(graph.Nodes, visual.Node{
		ID:   "unexpected",
		Type: visual.NodeTypeAction,
		Step: &workflow.Step{Name: "unexpected", Action: "script.shell", Args: map[string]any{"script": "set -euo pipefail\nprintf 'RUNNER_EXPORT_install_step=%s\\n' 'unexpected'\n"}},
	})
	if err := ValidateHostAgentInstallGraph(graph); err == nil {
		t.Fatal("expected extra action node to be rejected")
	}
}
