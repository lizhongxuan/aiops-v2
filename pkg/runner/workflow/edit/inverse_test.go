package edit

import (
	"errors"
	"reflect"
	"testing"

	"runner/workflow/visual"
)

func TestInverseRejectsInterleavedSamePathModification(t *testing.T) {
	result, err := ApplyOperations(editTestGraph(), []WorkflowOperation{NewOperation(&UpdateNodeOperation{
		NodeID: "collect", Set: NodePatch{Label: stringPointer("AI label")},
	})})
	if err != nil {
		t.Fatal(err)
	}
	current := cloneTestGraph(t, result.Graph)
	current.Nodes[1].Label = "User label"
	_, err = ApplyInverse(current, result.Inverse)
	var domainError *DomainError
	if !errors.As(err, &domainError) || domainError.Code != ErrorStaleRevision || !reflect.DeepEqual(domainError.ConflictingPaths, []string{"/nodes/collect/label"}) {
		t.Fatalf("inverse conflict = %#v", err)
	}
}

func TestInversePreservesInterleavedUntouchedPath(t *testing.T) {
	base := editTestGraph()
	result, err := ApplyOperations(base, []WorkflowOperation{NewOperation(&UpdateNodeOperation{
		NodeID: "collect", Set: NodePatch{Label: stringPointer("AI label")},
	})})
	if err != nil {
		t.Fatal(err)
	}
	current := cloneTestGraph(t, result.Graph)
	current.Nodes[1].Position = visual.Position{X: 999, Y: 222}
	restored, err := ApplyInverse(current, result.Inverse)
	if err != nil {
		t.Fatalf("ApplyInverse() error = %v", err)
	}
	node := findEditTestNode(t, restored.Graph, "collect")
	if node.Label != "Collect" || node.Position != (visual.Position{X: 999, Y: 222}) {
		t.Fatalf("inverse did not preserve unrelated position: %#v", node)
	}
}
