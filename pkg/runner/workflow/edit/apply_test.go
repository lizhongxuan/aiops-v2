package edit

import (
	"errors"
	"reflect"
	"testing"

	"runner/workflow"
	"runner/workflow/visual"
)

func TestApplyOperationsUsesStableClientRefsAndInverseRestoresGraph(t *testing.T) {
	base := editTestGraph()
	baseBefore := cloneTestGraph(t, base)
	actionType := visual.NodeTypeAction
	position := visual.Position{X: 320, Y: 80}
	source, target := "collect", "gate"
	source2, target2 := "gate", "end"
	result, err := ApplyOperations(base, []WorkflowOperation{
		NewOperation(&DeleteEdgeOperation{EdgeID: "collect-end"}),
		NewOperation(&AddNodeOperation{ClientRef: "gate", Node: EditableNode{ID: "gate-node", NodePatch: NodePatch{
			Type: &actionType, Position: &position, Label: stringPointer("Gate"),
			Step: &workflow.Step{Name: "gate", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "true"}},
		}}}),
		NewOperation(&AddEdgeOperation{ClientRef: "collect-gate", Edge: EditableEdge{EdgePatch: EdgePatch{Source: &source, Target: &target}}}),
		NewOperation(&AddEdgeOperation{ClientRef: "gate-end", Edge: EditableEdge{EdgePatch: EdgePatch{Source: &source2, Target: &target2}}}),
	})
	if err != nil {
		t.Fatalf("ApplyOperations() error = %v", err)
	}
	if !reflect.DeepEqual(base, baseBefore) {
		t.Fatal("ApplyOperations mutated the input graph")
	}
	if got := result.ClientRefs["gate"]; got != "gate-node" {
		t.Fatalf("client ref gate = %q", got)
	}
	if !reflect.DeepEqual(result.Diff.Delta.AddedNodeIDs, []string{"gate-node"}) {
		t.Fatalf("added nodes = %#v", result.Diff.Delta.AddedNodeIDs)
	}
	restored, err := ApplyInverse(result.Graph, result.Inverse)
	if err != nil {
		t.Fatalf("ApplyInverse() error = %v", err)
	}
	if !graphsEqual(t, restored.Graph, base) {
		t.Fatal("applying inverse did not restore the original graph")
	}
}

func TestApplyUpdateNodePreservesUntouchedFields(t *testing.T) {
	base := editTestGraph()
	base.Nodes[1].UI = map[string]any{"color": "blue"}
	base.Nodes[1].Ports = []visual.Port{{ID: "next", Type: "control"}}
	originalStep := *base.Nodes[1].Step
	result, err := ApplyOperations(base, []WorkflowOperation{NewOperation(&UpdateNodeOperation{
		NodeID: "collect", Set: NodePatch{Label: stringPointer("Collect metrics")},
	})})
	if err != nil {
		t.Fatalf("ApplyOperations() error = %v", err)
	}
	node := findEditTestNode(t, result.Graph, "collect")
	if node.Label != "Collect metrics" || !reflect.DeepEqual(*node.Step, originalStep) || !reflect.DeepEqual(node.UI, base.Nodes[1].UI) || !reflect.DeepEqual(node.Ports, base.Nodes[1].Ports) {
		t.Fatalf("updated node lost untouched fields: %#v", node)
	}
}

func TestApplyRejectsInvalidPortWithoutMutatingInput(t *testing.T) {
	base := editTestGraph()
	before := cloneTestGraph(t, base)
	result, err := ApplyOperations(base, []WorkflowOperation{NewOperation(&UpdateEdgeOperation{
		EdgeID: "start-collect", Set: EdgePatch{SourcePort: stringPointer("missing")},
	})})
	if err == nil {
		t.Fatal("ApplyOperations should reject an unknown port")
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code == "" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if !reflect.DeepEqual(base, before) {
		t.Fatal("failed apply mutated the input graph")
	}
}

func TestApplyRejectsNestedUIEscapeHatch(t *testing.T) {
	condition := &visual.ConditionSpec{If: "true", UI: map[string]any{"hidden": true}}
	_, err := ApplyOperations(editTestGraph(), []WorkflowOperation{NewOperation(&UpdateNodeOperation{
		NodeID: "collect", Set: NodePatch{Condition: condition},
	})})
	var domainError *DomainError
	if !errors.As(err, &domainError) || domainError.Code != ErrorInvalidPatch {
		t.Fatalf("nested UI error = %#v", err)
	}
}

func TestApplyRejectsUpdateWithoutRepresentableInverse(t *testing.T) {
	condition := &visual.ConditionSpec{If: "true"}
	_, err := ApplyOperations(editTestGraph(), []WorkflowOperation{NewOperation(&UpdateNodeOperation{
		NodeID: "collect", Set: NodePatch{Condition: condition},
	})})
	var domainError *DomainError
	if !errors.As(err, &domainError) || domainError.Code != ErrorInvalidPatch {
		t.Fatalf("non-reversible update error = %#v", err)
	}
}

func TestApplyUpdateEdgeProducesRestorableInverse(t *testing.T) {
	base := editTestGraph()
	result, err := ApplyOperations(base, []WorkflowOperation{NewOperation(&UpdateEdgeOperation{
		EdgeID: "collect-end", Set: EdgePatch{Condition: stringPointer("result.ok")},
	})})
	if err != nil {
		t.Fatalf("update edge error = %v", err)
	}
	if !reflect.DeepEqual(result.Diff.TouchedPaths, []string{"/edges/collect-end/condition"}) {
		t.Fatalf("edge paths = %#v", result.Diff.TouchedPaths)
	}
	restored, err := ApplyInverse(result.Graph, result.Inverse)
	if err != nil || !graphsEqual(t, restored.Graph, base) {
		t.Fatalf("edge inverse did not restore graph: %v", err)
	}
}

func TestApplyDeleteNodeRequiresCascadePolicy(t *testing.T) {
	base := editTestGraph()
	_, err := ApplyOperations(base, []WorkflowOperation{NewOperation(&DeleteNodeOperation{NodeID: "collect", CascadeEdges: false})})
	var domainError *DomainError
	if !errors.As(err, &domainError) || domainError.Code != ErrorInvalidPatch {
		t.Fatalf("delete error = %#v, want typed INVALID_PATCH", err)
	}
}

func TestApplyDeleteNodeCascadeProducesRestorableInverse(t *testing.T) {
	base := editTestGraph()
	keepStep := workflow.Step{Name: "keep", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "true"}}
	base.Nodes = []visual.Node{base.Nodes[0], {ID: "keep", Type: visual.NodeTypeAction, Position: visual.Position{X: 100, Y: 80}, Label: "Keep", Step: &keepStep}, base.Nodes[1], base.Nodes[2]}
	base.Edges = []visual.Edge{{ID: "start-keep", Source: "start", Target: "keep"}, {ID: "keep-collect", Source: "keep", Target: "collect"}, base.Edges[1]}
	source, target := "keep", "end"
	result, err := ApplyOperations(base, []WorkflowOperation{
		NewOperation(&DeleteNodeOperation{NodeID: "collect", CascadeEdges: true}),
		NewOperation(&AddEdgeOperation{ClientRef: "keep-end", Edge: EditableEdge{EdgePatch: EdgePatch{Source: &source, Target: &target}}}),
	})
	if err != nil {
		t.Fatalf("cascade delete error = %v", err)
	}
	if findNodeIndexForTest(result.Graph, "collect") >= 0 || !reflect.DeepEqual(result.Diff.Delta.DeletedNodeIDs, []string{"collect"}) {
		t.Fatalf("cascade delete result = %#v", result.Diff)
	}
	restored, err := ApplyInverse(result.Graph, result.Inverse)
	if err != nil || !graphsEqual(t, restored.Graph, base) {
		t.Fatalf("cascade inverse did not restore graph: %v", err)
	}
}

func TestApplyUnsupportedOperationReturnsPlannedDiagnostic(t *testing.T) {
	name := "renamed"
	result, err := ApplyOperations(editTestGraph(), []WorkflowOperation{NewOperation(&UpdateWorkflowOperation{Set: WorkflowPatch{Name: &name}})})
	if err == nil {
		t.Fatal("unsupported operation must not report success")
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "OPERATION_NOT_IMPLEMENTED" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func editTestGraph() visual.Graph {
	step := workflow.Step{Name: "collect", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "true"}}
	return visual.Graph{
		Version:  visual.GraphVersion,
		Workflow: workflow.Workflow{Version: "v0.1", Name: "test", Steps: []workflow.Step{step}},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart, Position: visual.Position{X: 0, Y: 80}, Label: "Start"},
			{ID: "collect", Type: visual.NodeTypeAction, Position: visual.Position{X: 160, Y: 80}, Label: "Collect", StepID: "collect", Step: &step},
			{ID: "end", Type: visual.NodeTypeEnd, Position: visual.Position{X: 320, Y: 80}, Label: "End"},
		},
		Edges: []visual.Edge{
			{ID: "start-collect", Source: "start", Target: "collect", Kind: visual.EdgeKindNext},
			{ID: "collect-end", Source: "collect", Target: "end", Kind: visual.EdgeKindNext},
		},
	}
}

func stringPointer(value string) *string { return &value }

func findEditTestNode(t *testing.T, graph visual.Graph, id string) visual.Node {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %s not found", id)
	return visual.Node{}
}

func findNodeIndexForTest(graph visual.Graph, id string) int {
	for index, node := range graph.Nodes {
		if node.ID == id {
			return index
		}
	}
	return -1
}
