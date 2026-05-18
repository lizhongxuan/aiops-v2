package visual

import (
	"strings"
	"testing"

	"runner/workflow"
)

func TestLoopGraphRoundTripPreservesLoopSpec(t *testing.T) {
	graph := loopVisualGraph()

	raw, err := CompileGraphToYAML(graph)
	if err != nil {
		t.Fatalf("compile loop graph: %v", err)
	}
	compiled := string(raw)
	for _, want := range []string{"type: loop", "loop:", "mode: for_each", "max_iterations: 3", "item_var: batch"} {
		if !strings.Contains(compiled, want) {
			t.Fatalf("compiled YAML missing %q:\n%s", want, compiled)
		}
	}

	parsed, err := ParseYAMLToGraph(raw)
	if err != nil {
		t.Fatalf("parse compiled loop graph: %v", err)
	}
	loop := findNodeByID(t, parsed, "loop")
	if loop.Type != NodeTypeLoop {
		t.Fatalf("loop node type = %q, want loop", loop.Type)
	}
	if loop.Loop == nil {
		t.Fatalf("loop spec missing after round-trip: %+v", loop)
	}
	if loop.Loop.Mode != "for_each" || loop.Loop.MaxIterations != 3 || loop.Loop.ItemVar != "batch" {
		t.Fatalf("loop spec mismatch after round-trip: %+v", loop.Loop)
	}
	if len(loop.Loop.Items) != 2 {
		t.Fatalf("loop items len = %d, want 2: %+v", len(loop.Loop.Items), loop.Loop.Items)
	}
}

func TestValidateGraphRequiresLoopModeAndMaxIterations(t *testing.T) {
	graph := loopVisualGraph()
	graph.Nodes[1].Loop = &LoopSpec{}

	err := ValidateGraph(graph)
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("ValidateGraph error = %T %[1]v, want *ValidationError", err)
	}
	codes := map[string]bool{}
	for _, issue := range validationErr.Issues {
		codes[issue.Code] = true
	}
	for _, want := range []string{"loop_mode_required", "loop_max_iterations_required"} {
		if !codes[want] {
			t.Fatalf("missing loop validation issue %q in %+v", want, validationErr.Issues)
		}
	}
}

func TestValidateGraphRejectsLegacyStepLoopInsideGraphLoop(t *testing.T) {
	graph := loopVisualGraph()
	graph.Nodes[2].Step.Loop = []any{"legacy"}

	err := ValidateGraph(graph)
	if !hasValidationIssue(err, "loop_body_step_loop_unsupported", "loop-step") {
		t.Fatalf("expected legacy step.loop rejection for graph loop body, got %v", err)
	}
}

func TestValidateGraphAllowsLegacyStepLoopInsideNonLoopGroup(t *testing.T) {
	graph := loopVisualGraph()
	graph.Nodes[1].Type = NodeTypeGroup
	graph.Nodes[1].Loop = nil
	graph.Nodes[2].Step.Loop = []any{"legacy"}

	err := ValidateGraph(graph)
	if hasValidationIssue(err, "loop_body_step_loop_unsupported", "loop-step") {
		t.Fatalf("legacy step.loop should only be rejected inside graph loop bodies, got %v", err)
	}
}

func loopVisualGraph() Graph {
	return Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "loop-demo",
			Plan:    workflow.Plan{Mode: "auto", Strategy: "graph"},
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{
				ID:   "loop",
				Type: NodeTypeLoop,
				Loop: &LoopSpec{
					Mode:          "for_each",
					Items:         []any{"pg-1", "pg-2"},
					MaxIterations: 3,
					ItemVar:       "batch",
					IndexVar:      "batch_index",
				},
			},
			{
				ID:       "loop-step",
				Type:     NodeTypeAction,
				ParentID: "loop",
				StepName: "loop-step",
				Step: &workflow.Step{
					ID:      "loop-step",
					Name:    "loop-step",
					Action:  "script.shell",
					Targets: []string{"local"},
					Args:    map[string]any{"script": "echo item"},
				},
			},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "start-loop", Source: "start", Target: "loop", Kind: EdgeKindNext},
			{ID: "loop-step", Source: "loop", Target: "loop-step", Kind: EdgeKindNext},
			{ID: "loop-end", Source: "loop", Target: "end", Kind: EdgeKindSuccess},
		},
	}
}
