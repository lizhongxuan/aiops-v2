package appui

import (
	"fmt"
	"strings"

	"runner/workflow"
	"runner/workflow/visual"
)

const BuiltinHostAgentInstallWorkflowID = "builtin.host-agent-install/v1"

var builtinHostAgentInstallSteps = []string{
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

var forbiddenHostAgentInstallActions = map[string]struct{}{
	"cmd.run":   {},
	"shell.run": {},
}

var forbiddenHostAgentInstallActionPrefixes = []string{
	"llm.",
	"prompt.",
	"chat.",
	"completion.",
	"agent.",
}

func BuiltinHostAgentInstallGraph() visual.Graph {
	nodes := make([]visual.Node, 0, len(builtinHostAgentInstallSteps)+2)
	nodes = append(nodes, visual.Node{
		ID:       "start",
		Type:     visual.NodeTypeStart,
		Position: visual.Position{X: 0, Y: 0},
		Label:    "Start",
	})

	steps := make([]workflow.Step, 0, len(builtinHostAgentInstallSteps))
	for i, name := range builtinHostAgentInstallSteps {
		step := workflow.Step{
			ID:      name,
			Name:    name,
			Action:  "script.shell",
			Targets: []string{"server-local"},
			Args: map[string]any{
				"script":      hostAgentInstallScript(name),
				"export_vars": true,
			},
		}
		steps = append(steps, step)
		stepCopy := step
		nodes = append(nodes, visual.Node{
			ID:       name,
			Type:     visual.NodeTypeAction,
			Position: visual.Position{X: float64((i + 1) * 220), Y: 0},
			StepID:   name,
			StepName: name,
			Step:     &stepCopy,
			Label:    name,
		})
	}
	nodes = append(nodes, visual.Node{
		ID:       "end",
		Type:     visual.NodeTypeEnd,
		Position: visual.Position{X: float64((len(builtinHostAgentInstallSteps) + 1) * 220), Y: 0},
		Label:    "End",
	})

	edges := make([]visual.Edge, 0, len(nodes)-1)
	for i := 0; i < len(nodes)-1; i++ {
		edges = append(edges, visual.Edge{
			ID:     fmt.Sprintf("edge-%s-%s", nodes[i].ID, nodes[i+1].ID),
			Source: nodes[i].ID,
			Target: nodes[i+1].ID,
			Kind:   visual.EdgeKindNext,
		})
	}

	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "host-agent-install",
			Description: "Built-in controlled SSH bootstrap workflow for host-agent installation.",
			Plan:        workflow.Plan{Mode: "auto", Strategy: "graph"},
			Inventory: workflow.Inventory{Hosts: map[string]workflow.Host{
				"server-local": {Address: "local"},
			}},
			Steps: steps,
			Vars: map[string]any{
				"workflow_id": BuiltinHostAgentInstallWorkflowID,
			},
		},
		Layout: visual.Layout{Direction: "LR"},
		Nodes:  nodes,
		Edges:  edges,
		UI: map[string]any{
			"builtin_workflow_id": BuiltinHostAgentInstallWorkflowID,
		},
	}
}

func ValidateHostAgentInstallGraph(graph visual.Graph) error {
	if err := visual.ValidateGraph(graph); err != nil {
		return err
	}

	actionNodes := make([]visual.Node, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		step := node.Step
		if step != nil {
			if err := validateHostAgentInstallAction(node.ID, step.Action); err != nil {
				return err
			}
		}
		if node.Type == visual.NodeTypeAction {
			actionNodes = append(actionNodes, node)
		}
	}
	if len(actionNodes) != len(builtinHostAgentInstallSteps) {
		return fmt.Errorf("host-agent install workflow must contain exactly %d action nodes, got %d", len(builtinHostAgentInstallSteps), len(actionNodes))
	}
	for i, want := range builtinHostAgentInstallSteps {
		node := actionNodes[i]
		if node.Step == nil {
			return fmt.Errorf("host-agent install node %q is missing step", node.ID)
		}
		if node.Step.Name != want {
			return fmt.Errorf("host-agent install action node %d step = %q, want %q", i, node.Step.Name, want)
		}
		if node.Step.Action != "script.shell" {
			return fmt.Errorf("host-agent install step %q action = %q, want script.shell", want, node.Step.Action)
		}
	}
	return nil
}

func validateHostAgentInstallAction(nodeID, action string) error {
	action = strings.TrimSpace(action)
	if _, forbidden := forbiddenHostAgentInstallActions[action]; forbidden {
		return fmt.Errorf("host-agent install node %q uses forbidden action %q", nodeID, action)
	}
	for _, prefix := range forbiddenHostAgentInstallActionPrefixes {
		if strings.HasPrefix(action, prefix) {
			return fmt.Errorf("host-agent install node %q uses forbidden model action %q", nodeID, action)
		}
	}
	return nil
}

func hostAgentInstallScript(step string) string {
	body := map[string]string{
		"validate-inputs":        "required_vars=\"host_id ssh_host ssh_user ssh_credential_ref agent_version\"\nfor key in $required_vars; do\n  eval \"value=\\${$key:-}\"\n  if [ -z \"$value\" ]; then\n    printf 'missing required var: %s\\n' \"$key\" >&2\n    exit 64\n  fi\ndone",
		"tcp-preflight":          "port=\"${ssh_port:-22}\"\nif command -v nc >/dev/null 2>&1; then\n  nc -z \"$ssh_host\" \"$port\"\nelse\n  timeout 5 bash -c \"</dev/tcp/$ssh_host/$port\"\nfi",
		"ssh-preflight":          "port=\"${ssh_port:-22}\"\nssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -p \"$port\" \"$ssh_user@$ssh_host\" 'echo aiops-ssh-ok; id -u; command -v sudo >/dev/null'",
		"detect-platform":        "printf 'RUNNER_EXPORT_platform=pending\\n'\n# Platform probing is performed by the controlled SSH bootstrap implementation.",
		"resolve-artifact":       "printf 'RUNNER_EXPORT_artifact_ref=%s\\n' \"host-agent:${agent_version}\"",
		"upload-artifact":        "printf 'artifact upload prepared for host %s\\n' \"$host_id\"",
		"install-files":          "printf 'install file layout prepared for host %s\\n' \"$host_id\"",
		"install-service":        "printf 'service definition prepared for host %s\\n' \"$host_id\"",
		"start-service":          "printf 'service start requested for host %s\\n' \"$host_id\"",
		"verify-local-health":    "printf 'local health verification requested on port %s\\n' \"${agent_listen_port:-7072}\"",
		"verify-aiops-heartbeat": "printf 'heartbeat verification requested for host %s\\n' \"$host_id\"",
		"finalize-host":          "printf 'RUNNER_EXPORT_control_mode=managed\\n'\nprintf 'host finalization requested for host %s\\n' \"$host_id\"",
	}
	scriptBody, ok := body[step]
	if !ok {
		scriptBody = fmt.Sprintf("printf 'unknown host-agent install step: %s\\n' >&2\nexit 64", shellSingleQuote(step))
	}
	return fmt.Sprintf("set -euo pipefail\nprintf 'RUNNER_EXPORT_install_step=%%s\\n' '%s'\n%s\n", shellSingleQuote(step), scriptBody)
}

func shellSingleQuote(value string) string {
	return strings.ReplaceAll(value, "'", "'\"'\"'")
}
