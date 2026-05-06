package visual

import (
	"strings"
	"testing"

	"runner/workflow"
)

func TestGraphIORoundTripPreservesStructuredInputsAndOutputs(t *testing.T) {
	graph := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "io-demo",
			Plan:    workflow.Plan{Mode: "auto", Strategy: "sequential"},
		},
		Nodes: []Node{
			{
				ID:       "start",
				Type:     NodeTypeStart,
				Position: Position{X: 0, Y: 0},
				Outputs: []OutputParamSpec{{
					Key:           "backup_id",
					Type:          "string",
					ExtractSource: ExtractSource{Path: "$.backup_id"},
				}},
			},
			{
				ID:       "restore",
				Type:     NodeTypeAction,
				Position: Position{X: 240, Y: 0},
				StepName: "restore",
				Step: &workflow.Step{
					Name:   "restore",
					Action: "shell.run",
					Args:   map[string]any{"script": "echo restore"},
				},
				Inputs: []InputParamSpec{
					{
						Key:      "backup_id",
						Type:     "string",
						Label:    "Backup ID",
						Required: true,
						ValueSource: ValueSource{
							Type: "variable",
							Variable: &VariableRef{
								Scope: "workflow_input",
								Name:  "backup_id",
								Path:  "$.backup_id",
							},
						},
					},
					{
						Key:  "retry_limit",
						Type: "integer",
						ValueSource: ValueSource{
							Type:  "literal",
							Value: 3,
						},
					},
				},
				Outputs: []OutputParamSpec{
					{
						Key:         "restore_lsn",
						Type:        "string",
						Description: "Last restored WAL LSN.",
						ExtractSource: ExtractSource{
							Type: "stdout_jsonpath",
							Path: "$.restore_lsn",
						},
					},
				},
			},
			{ID: "end", Type: NodeTypeEnd, Position: Position{X: 480, Y: 0}},
		},
		Edges: []Edge{
			{ID: "edge-start-restore", Source: "start", Target: "restore", Kind: EdgeKindNext},
			{ID: "edge-restore-end", Source: "restore", Target: "end", Kind: EdgeKindNext},
		},
	}

	raw, err := CompileGraphToYAML(graph)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	compiled := string(raw)
	for _, want := range []string{"inputs:", "outputs:", "value_source:", "extract_source:", "workflow_input", "stdout_jsonpath"} {
		if !strings.Contains(compiled, want) {
			t.Fatalf("compiled YAML missing %q:\n%s", want, compiled)
		}
	}

	parsed, err := ParseYAMLToGraph(raw)
	if err != nil {
		t.Fatalf("parse compiled graph: %v", err)
	}
	restore := findNodeByID(t, parsed, "restore")
	if got := len(restore.Inputs); got != 2 {
		t.Fatalf("inputs len = %d, want 2: %+v", got, restore.Inputs)
	}
	if got := restore.Inputs[0].ValueSource.Variable.Scope; got != "workflow_input" {
		t.Fatalf("input variable scope = %q, want workflow_input", got)
	}
	if got := len(restore.Outputs); got != 1 {
		t.Fatalf("outputs len = %d, want 1: %+v", got, restore.Outputs)
	}
	if got := restore.Outputs[0].ExtractSource.Path; got != "$.restore_lsn" {
		t.Fatalf("output extract path = %q, want $.restore_lsn", got)
	}
}

func TestValidateGraphRejectsInvalidStructuredIO(t *testing.T) {
	graph := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "io-invalid",
			Plan:    workflow.Plan{Mode: "auto", Strategy: "sequential"},
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{
				ID:       "run",
				Type:     NodeTypeAction,
				StepName: "run",
				Step:     &workflow.Step{Name: "run", Action: "cmd.run", Args: map[string]any{"cmd": "echo ok"}},
				Inputs: []InputParamSpec{
					{Key: "target", Type: "string", ValueSource: ValueSource{Type: "variable"}},
					{Key: "target", Type: "unknown"},
				},
				Outputs: []OutputParamSpec{
					{Key: "", Type: "string", ExtractSource: ExtractSource{Type: "stdout_jsonpath"}},
				},
			},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "edge-start-run", Source: "start", Target: "run"},
			{ID: "edge-run-end", Source: "run", Target: "end"},
		},
	}

	err := ValidateGraph(graph)
	if err == nil {
		t.Fatal("ValidateGraph succeeded, want structured I/O errors")
	}
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	codes := map[string]bool{}
	for _, item := range validationErr.Issues {
		codes[item.Code] = true
	}
	for _, want := range []string{"input_key_duplicate", "input_type_invalid", "input_variable_required", "output_key_required", "output_extract_path_required"} {
		if !codes[want] {
			t.Fatalf("missing issue %q in %+v", want, validationErr.Issues)
		}
	}
}

func TestParseYAMLToGraphHydratesStepByStepID(t *testing.T) {
	raw := `version: v0.1
name: step-id-demo
x_runner_graph:
  version: v1
  nodes:
    - id: start
      type: start
    - id: restore-node
      type: action
      step_id: step-restore
    - id: end
      type: end
  edges:
    - id: edge-start-restore
      source: start
      target: restore-node
    - id: edge-restore-end
      source: restore-node
      target: end
steps:
  - id: step-restore
    name: restore
    action: cmd.run
    args:
      cmd: echo restore
`

	graph, err := ParseYAMLToGraph([]byte(raw))
	if err != nil {
		t.Fatalf("parse graph with step_id-only node: %v", err)
	}
	node := findNodeByID(t, graph, "restore-node")
	if node.Step == nil || node.Step.Name != "restore" || node.StepID != "step-restore" || node.StepName != "restore" {
		t.Fatalf("step_id-only node was not hydrated: %+v", node)
	}
}

func TestParseYAMLToGraphExplicitEmptyIOOverridesDataFallback(t *testing.T) {
	raw := `version: v0.1
name: io-clear-demo
x_runner_graph:
  version: v1
  nodes:
    - id: start
      type: start
    - id: run
      type: action
      step: run
      inputs: []
      outputs: []
      data:
        inputs:
          - key: stale_input
            type: string
        outputs:
          - key: stale_output
            type: string
    - id: end
      type: end
  edges:
    - id: edge-start-run
      source: start
      target: run
    - id: edge-run-end
      source: run
      target: end
steps:
  - name: run
    action: cmd.run
    args:
      cmd: echo ok
`

	graph, err := ParseYAMLToGraph([]byte(raw))
	if err != nil {
		t.Fatalf("parse graph with explicit empty IO: %v", err)
	}
	node := findNodeByID(t, graph, "run")
	if len(node.Inputs) != 0 || len(node.Outputs) != 0 {
		t.Fatalf("explicit empty IO should override data fallback, got inputs=%+v outputs=%+v", node.Inputs, node.Outputs)
	}
}

func findNodeByID(t *testing.T, graph Graph, id string) Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %q not found", id)
	return Node{}
}
